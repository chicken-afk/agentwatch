package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chicken-afk/agentwatch/internal/config"
)

// History: token usage of every session of every agent over the last month, read from the
// same files the dashboard uses (Claude transcripts incl. subagents and advisor calls, Codex
// rollouts, Gemini chats, OpenCode messages, Antigravity conversations, omp sessions incl.
// workers and side calls, and
// generic hook "Message" events). Append-only files are read incrementally, so a refresh
// only parses what's new. Safe for use from a background goroutine.
type History struct {
	mu    sync.Mutex
	files map[string]*histFile
}

// UsagePeriods: the rolling windows of the usage report.
var UsagePeriods = []struct {
	Label string
	D     time.Duration
}{
	{"last hour", time.Hour},
	{"last 24h", 24 * time.Hour},
	{"last 7 days", 7 * 24 * time.Hour},
	{"last 30 days", 30 * 24 * time.Hour},
}

// historyKeep: files and records older than this are ignored.
const historyKeep = 31 * 24 * time.Hour

// usageRec: the tokens of one model response at time t.
type usageRec struct {
	t     time.Time
	agent string
	model string
	u     Usage
}

type histFile struct {
	tail  *tailer // append-only formats
	sig   fileSig // rewritten formats: reparse when this changes
	recs  []usageRec
	seen  map[string]bool // Claude: message ids already counted
	model string          // Codex: current model
	total int64           // Codex: last total_tokens (dedupes repeated token_count)
	agy   *agyConv        // Antigravity: parsed steps, updated incrementally
}

func NewHistory() *History { return &History{files: map[string]*histFile{}} }

// Scan reads new usage from every agent's files modified within the last month.
func (h *History) Scan(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	since := now.Add(-historyKeep)
	live := map[string]bool{}

	tailAll := func(paths []string, parse func(f *histFile, line []byte)) {
		for _, p := range paths {
			fi, err := os.Stat(p)
			if err != nil || fi.ModTime().Before(since) {
				continue
			}
			live[p] = true
			f := h.file(p)
			if f.tail == nil {
				f.tail = &tailer{path: p}
			}
			f.tail.read(func(line []byte) { parse(f, line) })
		}
	}
	reparse := func(paths []string, parse func(f *histFile, data []byte)) {
		for _, p := range paths {
			fi, err := os.Stat(p)
			if err != nil || fi.ModTime().Before(since) {
				continue
			}
			live[p] = true
			f := h.file(p)
			var sig fileSig
			sig.add(fi)
			if f.sig == sig {
				continue
			}
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			f.sig, f.recs = sig, nil
			parse(f, data)
		}
	}

	claude := config.ClaudeDir()
	tailAll(globAll(filepath.Join(claude, "projects", "*", "*.jsonl"),
		filepath.Join(claude, "projects", "*", "*", "subagents", "agent-*.jsonl")), parseClaudeUsage)
	tailAll(globAll(filepath.Join(config.CodexDir(), "sessions", "*", "*", "*", "rollout-*.jsonl")), parseCodexUsage)
	var omp []string
	for _, root := range ompRoots() {
		omp = append(omp, globAll(filepath.Join(root, "*", "*.jsonl"), filepath.Join(root, "*", "*", "*.jsonl"))...)
	}
	tailAll(omp, parseOmpUsage)
	tailAll(globAll(config.EventsPath(), config.LegacyEventsPath()), parseHookUsage)
	reparse(globAll(filepath.Join(config.GeminiDir(), "tmp", "*", "chats", "session-*.json")), parseGeminiUsage)
	reparse(globAll(filepath.Join(config.OpenCodeDir(), "storage", "message", "*", "*.json")), parseOpenCodeUsage)

	// Antigravity: SQLite databases whose recent writes live in the -wal file, so the
	// signature covers both; a changed conversation is re-read whole
	for _, p := range globAll(filepath.Join(config.AntigravityDir(), "conversations", "*.db")) {
		var sig fileSig
		var latest time.Time
		for _, q := range agyFiles(p) {
			if fi, err := os.Stat(q); err == nil {
				sig.add(fi)
				if fi.ModTime().After(latest) {
					latest = fi.ModTime()
				}
			}
		}
		if latest.Before(since) {
			continue
		}
		live[p] = true
		f := h.file(p)
		if f.sig == sig {
			continue
		}
		if f.agy == nil {
			f.agy = &agyConv{models: map[int64]string{}, maxIdx: -1}
		}
		if f.agy.update(p) != nil {
			continue // locked mid-write: retry on the next scan
		}
		conv := f.agy
		f.sig, f.recs = sig, nil
		for _, st := range conv.steps {
			if st.hasUse {
				model := conv.models[st.model]
				if model == "" {
					model = "unknown model"
				}
				f.add(st.t, AgentAntigravity, model, st.usage)
			}
		}
	}

	for p := range h.files {
		if !live[p] {
			delete(h.files, p) // older than a month, or deleted
		}
	}
}

func (h *History) file(p string) *histFile {
	f := h.files[p]
	if f == nil {
		f = &histFile{}
		h.files[p] = f
	}
	return f
}

func globAll(patterns ...string) []string {
	var out []string
	for _, p := range patterns {
		m, _ := filepath.Glob(p)
		out = append(out, m...)
	}
	return out
}

func (f *histFile) add(t time.Time, agent, model string, u Usage) {
	if t.IsZero() || model == "" || u.InputTotal()+u.Out == 0 {
		return
	}
	f.recs = append(f.recs, usageRec{t: t, agent: agent, model: model, u: u})
}

// --- per-agent parsers -------------------------------------------------------------

var (
	keyAssistant = []byte(`"assistant"`)
	keyUsage     = []byte(`"usage"`)
	keyMessage   = []byte(`"Message"`)
)

func parseClaudeUsage(f *histFile, line []byte) {
	// cheap pre-filter: most transcript lines (user turns, tool results, attachments) carry no usage
	if !bytes.Contains(line, keyAssistant) || !bytes.Contains(line, keyUsage) {
		return
	}
	var m transcriptLine
	if json.Unmarshal(line, &m) != nil || m.Type != "assistant" || m.Message == nil {
		return
	}
	model := m.Message.Model
	if model == "" || strings.HasPrefix(model, "<") {
		return
	}
	// one line per content block shares a message id: count it once
	if id := m.Message.ID; id != "" {
		if f.seen == nil {
			f.seen = map[string]bool{}
		}
		if f.seen[id] {
			return
		}
		f.seen[id] = true
	}
	t, _ := time.Parse(time.RFC3339Nano, m.Timestamp)
	u, adv := parseUsage(m.Message.Usage)
	f.add(t, AgentClaude, model, u)
	for _, a := range adv {
		mdl := a.model
		if mdl == "" {
			mdl = "advisor"
		}
		f.add(t, AgentClaude, mdl, a.u)
	}
}

func parseCodexUsage(f *histFile, line []byte) {
	var l codexLine
	if json.Unmarshal(line, &l) != nil || l.Payload == nil {
		return
	}
	switch {
	case l.Type == "turn_context":
		if m := str(l.Payload, "model"); m != "" {
			f.model = m
		}
	case l.Type == "event_msg" && str(l.Payload, "type") == "token_count":
		info, _ := l.Payload["info"].(map[string]any)
		last, _ := info["last_token_usage"].(map[string]any)
		total, _ := info["total_token_usage"].(map[string]any)
		if last == nil {
			return
		}
		if tt := num(total, "total_tokens"); tt > 0 {
			if tt == f.total {
				return
			}
			f.total = tt
		}
		t, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
		model := f.model
		if model == "" {
			model = "codex"
		}
		f.add(t, AgentCodex, model, codexUsage(last))
	}
}

func parseOmpUsage(f *histFile, line []byte) {
	if !bytes.Contains(line, keyUsage) {
		return
	}
	l, t, ok := parseOmp(line)
	if !ok {
		return
	}
	switch {
	case l.Type == "message" && l.Message != nil && l.Message.Role == "assistant" && l.Message.Usage != nil:
		f.add(t, AgentOmp, ompModel(l.Message.Model), ompUsage(l.Message.Usage))
	case l.Type == "model_usage": // omp's own side calls (titles, captions, probes) cost tokens too
		f.add(t, AgentOmp, ompModel(l.Model), ompUsage(l.Usage))
	}
}

func parseHookUsage(f *histFile, line []byte) {
	if !bytes.Contains(line, keyMessage) {
		return
	}
	var raw map[string]any
	if json.Unmarshal(line, &raw) != nil || str(raw, "hook_event_name") != "Message" {
		return
	}
	agent := first(str(raw, "agentwatch_agent"), str(raw, "ccwatch_agent"))
	if agent == "" || agent == AgentClaude {
		return // Claude's usage comes from its transcripts
	}
	t, _ := time.Parse(time.RFC3339Nano, first(str(raw, "agentwatch_ts"), str(raw, "ccwatch_ts")))
	model := str(raw, "model")
	if model == "" {
		model = agent
	}
	f.add(t, agent, model, Usage{In: num(raw, "input_tokens"), Out: num(raw, "output_tokens"), CacheR: num(raw, "cache_read_tokens")})
}

func parseGeminiUsage(f *histFile, data []byte) {
	var c geminiChat
	if json.Unmarshal(data, &c) != nil {
		return
	}
	for _, m := range c.Messages {
		if m.Type != "gemini" || m.Tokens == nil {
			continue
		}
		t, _ := time.Parse(time.RFC3339Nano, m.Timestamp)
		in, cached := num(m.Tokens, "input"), num(m.Tokens, "cached")
		f.add(t, AgentGemini, m.Model, Usage{In: max(0, in-cached), CacheR: cached, Out: num(m.Tokens, "output") + num(m.Tokens, "thoughts")})
	}
}

func parseOpenCodeUsage(f *histFile, data []byte) {
	var m ocMessage
	if json.Unmarshal(data, &m) != nil || m.Role != "assistant" || m.Time.Completed == 0 {
		return
	}
	f.add(msTime(m.Time.Completed), AgentOpenCode, m.model(), ocUsage(&m))
}

// --- report ------------------------------------------------------------------------

// UsageTotals: tokens and message count for one period.
type UsageTotals struct {
	Usage
	Msgs int
}

// Tokens: every token: input, output, cache read and cache write.
func (t UsageTotals) Tokens() int64 { return t.InputTotal() + t.Out }

func (t *UsageTotals) addRec(u Usage) {
	t.Usage.add(u)
	t.Msgs++
}

func (t *UsageTotals) addTotals(o UsageTotals) {
	t.Usage.add(o.Usage)
	t.Msgs += o.Msgs
}

// ModelUsage: one model's totals per period (indexes match UsagePeriods).
type ModelUsage struct {
	Model   string
	Periods []UsageTotals
}

// AgentUsage: one agent's totals per period and its models, busiest first.
type AgentUsage struct {
	Agent   string
	Periods []UsageTotals
	Models  []ModelUsage
}

// UsageReport: usage of all sessions, per agent and per model, for every period.
type UsageReport struct {
	At     time.Time
	Agents []AgentUsage
	Total  []UsageTotals
}

// Report aggregates the scanned usage at time now, optionally limited to some agents.
func (h *History) Report(now time.Time, only []string) UsageReport {
	h.mu.Lock()
	defer h.mu.Unlock()
	filter := Options{Agents: only}
	n := len(UsagePeriods)
	type key struct{ agent, model string }
	byModel := map[key][]UsageTotals{}
	rep := UsageReport{At: now, Total: make([]UsageTotals, n)}
	for _, f := range h.files {
		for _, r := range f.recs {
			if !filter.wantsAgent(r.agent) {
				continue
			}
			age := max(now.Sub(r.t), 0)
			k := key{r.agent, r.model}
			p := byModel[k]
			if p == nil {
				p = make([]UsageTotals, n)
				byModel[k] = p
			}
			for i, per := range UsagePeriods {
				if age <= per.D {
					p[i].addRec(r.u)
					rep.Total[i].addRec(r.u)
				}
			}
		}
	}
	agents := map[string]*AgentUsage{}
	for k, p := range byModel {
		if p[n-1].Msgs == 0 {
			continue
		}
		a := agents[k.agent]
		if a == nil {
			a = &AgentUsage{Agent: k.agent, Periods: make([]UsageTotals, n)}
			agents[k.agent] = a
		}
		a.Models = append(a.Models, ModelUsage{Model: k.model, Periods: p})
		for i := range p {
			a.Periods[i].addTotals(p[i])
		}
	}
	for _, a := range agents {
		sort.Slice(a.Models, func(i, j int) bool {
			if ti, tj := a.Models[i].Periods[n-1].Tokens(), a.Models[j].Periods[n-1].Tokens(); ti != tj {
				return ti > tj
			}
			return a.Models[i].Model < a.Models[j].Model
		})
		rep.Agents = append(rep.Agents, *a)
	}
	sort.Slice(rep.Agents, func(i, j int) bool {
		if ti, tj := rep.Agents[i].Periods[n-1].Tokens(), rep.Agents[j].Periods[n-1].Tokens(); ti != tj {
			return ti > tj
		}
		return rep.Agents[i].Agent < rep.Agents[j].Agent
	})
	return rep
}

// SessionUsage: the session's totals per model (the report's "this session" column).
func (s *Session) SessionUsage() map[string]UsageTotals {
	out := map[string]UsageTotals{}
	for m, st := range s.Stats {
		out[m] = UsageTotals{Usage: st.Usage, Msgs: st.Msgs}
	}
	return out
}
