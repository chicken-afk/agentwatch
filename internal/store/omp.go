package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ccwatch/internal/config"
	"ccwatch/internal/format"
)

// oh-my-pi (omp) writes one append-only JSONL tree per session:
//
//	~/.omp/agent/sessions/<encoded-cwd>/<time>_<session-id>.jsonl
//	~/.omp/agent/sessions/<encoded-cwd>/<time>_<session-id>/<Name>.jsonl   ← subagent runs
//
// Lines: a "title" record, a "session" header {id, cwd, parentSession}, then entries
// {type, id, parentId, timestamp(ISO)}: message (role user|developer|assistant|toolResult),
// model_change, mode_change, compaction, session_init (subagent: agent, task, resolvedModel),
// model_usage (side calls: title, job-widget…), custom (session_exit, tool_execution_start)
// and custom_message (skill-prompt: {details: {name, prompt}}).

const AgentOmp = "omp"

// ompRoots: the session folders of the default profile and every named profile.
func ompRoots() []string {
	home := config.OmpDir()
	roots := []string{filepath.Join(home, "agent", "sessions")}
	profiles, _ := filepath.Glob(filepath.Join(home, "profiles", "*", "agent", "sessions"))
	return append(roots, profiles...)
}

// discoverOmp registers omp sessions (and their subagent runs) modified since `since`.
func (st *Store) discoverOmp(since time.Time) {
	for _, root := range ompRoots() {
		leads, _ := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
		for _, f := range leads {
			fi, err := os.Stat(f)
			if err != nil || fi.ModTime().Before(since) {
				continue
			}
			s := st.sessionFor(AgentOmp, ompID(f))
			if s.Transcript == "" {
				s.Transcript = f
			}
			if s.Last.Before(fi.ModTime()) {
				s.Last = fi.ModTime()
			}
		}
		subs, _ := filepath.Glob(filepath.Join(root, "*", "*", "*.jsonl"))
		for _, f := range subs {
			fi, err := os.Stat(f)
			if err != nil || fi.ModTime().Before(since) {
				continue
			}
			parent := filepath.Dir(f) + ".jsonl"
			s := st.sessionFor(AgentOmp, ompID(parent))
			if s.Transcript == "" {
				s.Transcript = parent
			}
			a := s.agent(strings.TrimSuffix(filepath.Base(f), ".jsonl"))
			if a.Transcript == "" {
				a.Transcript = f
			}
		}
	}
}

// ompID: the session id after the last "_" of "<time>_<id>.jsonl".
func ompID(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if i := strings.LastIndex(base, "_"); i >= 0 {
		return base[i+1:]
	}
	return base
}

type ompLine struct {
	Type          string          `json:"type"`
	Timestamp     string          `json:"timestamp"`
	Cwd           string          `json:"cwd"`
	Model         string          `json:"model"`
	Role          string          `json:"role"`
	Mode          string          `json:"mode"`
	Purpose       string          `json:"purpose"`
	Agent         any             `json:"agent"`
	Task          any             `json:"task"`
	ResolvedModel string          `json:"resolvedModel"`
	CustomType    string          `json:"customType"`
	Data          map[string]any  `json:"data"`
	Details       map[string]any  `json:"details"`
	Usage         map[string]any  `json:"usage"`
	Message       *ompMessage     `json:"message"`
	Raw           json.RawMessage `json:"-"`
}

type ompMessage struct {
	Role        string           `json:"role"`
	Content     json.RawMessage  `json:"content"`
	Model       string           `json:"model"`
	Usage       map[string]any   `json:"usage"`
	StopReason  string           `json:"stopReason"`
	Attribution any              `json:"attribution"`
	Synthetic   bool             `json:"synthetic"`
	ToolName    string           `json:"toolName"`
	IsError     bool             `json:"isError"`
	blocks      []map[string]any `json:"-"`
}

func parseOmp(line []byte) (ompLine, time.Time, bool) {
	var l ompLine
	if json.Unmarshal(line, &l) != nil {
		return l, time.Time{}, false
	}
	t, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	if l.Message != nil {
		_ = json.Unmarshal(l.Message.Content, &l.Message.blocks)
	}
	return l, t, true
}

// ompModel: "provider/model" → "model".
func ompModel(m string) string {
	if i := strings.LastIndex(m, "/"); i >= 0 {
		return m[i+1:]
	}
	return m
}

func ompUsage(u map[string]any) Usage {
	return Usage{In: num(u, "input"), Out: num(u, "output"), CacheR: num(u, "cacheRead"), CacheW: num(u, "cacheWrite")}
}

// ompText: the text of a message (a string or a list of {type:"text", text} blocks).
func ompText(m *ompMessage) string {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	var b strings.Builder
	for _, bl := range m.blocks {
		if str(bl, "type") == "text" {
			b.WriteString(str(bl, "text"))
		}
	}
	return b.String()
}

// ompToolSkill: a skill the agent loads itself with the read tool (a SKILL.md or a skill:// url).
// Shell commands that merely look inside a skill folder don't count.
func ompToolSkill(tool string, args map[string]any) string {
	if tool != "read" {
		return ""
	}
	for _, k := range []string{"path", "file_path", "url"} {
		v := str(args, k)
		if name := skillFromText(v); name != "" {
			return name
		}
		if rest, ok := strings.CutPrefix(v, "skill://"); ok {
			name, _, _ := strings.Cut(rest, "/")
			return name
		}
	}
	return ""
}

func (s *Session) ingestOmp(line []byte) {
	l, t, ok := parseOmp(line)
	if !ok {
		return
	}
	s.touch(t)
	switch l.Type {
	case "session":
		if l.Cwd != "" {
			s.Cwd = l.Cwd
		}
		s.log(t, "sys", "", "session start (omp)", false)
	case "model_change":
		if l.Role == "" || l.Role == "default" {
			s.setLead(t, ompModel(l.Model))
		}
	case "mode_change":
		if l.Mode != "" {
			s.PermMode = l.Mode
		}
	case "compaction":
		s.Compacts++
		s.log(t, "sys", "", "context compact", false)
	case "model_usage":
		// side calls (title, job captions, completion probes): model stats only, not the lead's usage
		if l.Purpose != "" {
			s.recordUsage(ompModel(l.Model), l.Purpose, t, ompUsage(l.Usage))
		}
	case "custom":
		if l.CustomType == "session_exit" {
			s.Status, s.CurrentTool = "ended", ""
			s.log(t, "sys", "", "session exit ("+str(l.Data, "reason")+")", false)
		}
	case "custom_message":
		if l.CustomType == "skill-prompt" {
			// "/skill:<name> …" typed by the user is stored here instead of as a user message:
			// it is both the prompt and the skill use (beginPrompt records the slash skill).
			prompt := str(l.Details, "prompt")
			if _, _, ok := slashSkill(prompt); !ok {
				prompt = "/skill:" + str(l.Details, "name") + " " + prompt
			}
			s.beginPrompt(t, strings.TrimSpace(prompt))
		}
	case "message":
		if l.Message != nil {
			s.ompMessage(t, l.Message)
		}
	}
}

func (s *Session) ompMessage(t time.Time, m *ompMessage) {
	switch m.Role {
	case "user":
		attr, _ := m.Attribution.(string)
		if m.Synthetic || (attr != "" && attr != "user") {
			return // added by omp itself (reminders, async results), not typed by the user
		}
		if txt := strings.TrimSpace(ompText(m)); txt != "" {
			s.beginPrompt(t, txt)
		}
	case "assistant":
		s.setLead(t, m.Model)
		if m.Usage != nil {
			s.leadReply(t, m.Model, ompUsage(m.Usage))
		}
		for _, b := range m.blocks {
			if str(b, "type") != "toolCall" {
				continue
			}
			name := str(b, "name")
			args, _ := b["arguments"].(map[string]any)
			sum := toolSummary(name, args)
			if name == "task" {
				sum = "dispatch → task"
				if i := str(args, "i"); i != "" {
					sum += ": " + format.OneLine(i)
				}
			}
			s.leadTool(t, str(b, "id"), name, sum, ompToolSkill(name, args))
		}
		switch m.StopReason {
		case "stop", "length":
			s.endTurn(t)
		case "error", "aborted":
			s.Status, s.CurrentTool = "idle", ""
			s.log(t, "lead", s.Lead, "turn "+m.StopReason, m.StopReason == "error")
		}
	case "toolResult":
		s.CurrentTool = ""
		if m.IsError {
			s.toolFailed(t, "lead", s.Lead, m.ToolName+" failed")
		}
	}
}

// ingestOmpAgent reads a subagent run (<session>/<Name>.jsonl) into Agent a of session s.
func (s *Session) ingestOmpAgent(a *Agent, line []byte) {
	l, t, ok := parseOmp(line)
	if !ok {
		return
	}
	if !t.IsZero() {
		if a.Start.IsZero() {
			a.Start = t
		}
		a.LastSeen = t
	}
	switch l.Type {
	case "session_init":
		if name, ok := l.Agent.(string); ok && name != "" {
			a.Type = name
		}
		if a.Type == "" {
			a.Type = "task"
		}
		if task, ok := l.Task.(string); ok && a.Desc == "" {
			a.Desc = format.Clip(format.OneLine(task), 120)
		}
		if a.Desc == "" {
			a.Desc = a.ID
		}
		if m := ompModel(l.ResolvedModel); m != "" {
			a.ReqModel = m
		}
		a.Status = "running"
		s.log(t, a.Label(), a.Model(), "subagent started: "+a.ID, false)
	case "custom":
		if l.CustomType == "session_exit" && a.Status != "done" {
			a.Status, a.End = "done", t
			s.log(t, a.Label(), a.Model(), "subagent done · "+format.Dur(a.End.Sub(a.Start))+" · "+strconv.Itoa(a.Tools)+" tools", false)
		}
	case "message":
		m := l.Message
		if m == nil || m.Role != "assistant" {
			return
		}
		if md := m.Model; md != "" && a.Model() != md {
			a.Models = append(a.Models, md)
		}
		if a.Status != "done" {
			a.Status = "running"
		}
		for _, b := range m.blocks {
			if str(b, "type") == "toolCall" {
				a.Tools++
				a.LastTool = str(b, "name")
				args, _ := b["arguments"].(map[string]any)
				if sk := ompToolSkill(a.LastTool, args); sk != "" {
					s.addSkill(SkillUse{T: t, Name: sk, Source: "tool", AgentID: a.ID, Who: a.Label(), Model: a.Model(), ID: str(b, "id")})
				}
			}
		}
		if m.Usage != nil {
			u := ompUsage(m.Usage)
			a.Msgs++
			a.Usage.add(u)
			s.recordUsage(a.Model(), a.Label(), t, u)
			s.Points = append(s.Points, point{T: t, Role: a.Label(), AgentID: a.ID, Model: a.Model()})
		}
	}
}
