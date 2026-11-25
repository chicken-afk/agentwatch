package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo), registered as "sqlite"

	"github.com/chicken-afk/agentwatch/internal/config"
	"github.com/chicken-afk/agentwatch/internal/format"
)

// Antigravity CLI (agy) keeps each conversation in a SQLite database:
//
//	~/.gemini/antigravity-cli/conversations/<conversation-id>.db
//	~/.gemini/antigravity-cli/history.jsonl   {display, timestamp(ms), workspace, conversationId?, type?}
//
// The database rows hold protobuf blobs without a published schema; these field numbers were
// read off real conversations:
//
//	steps(idx, step_type, status, metadata, error_details)
//	  step_type 14 = user prompt, 15 = model response, 132 = tool call
//	  status 3 = done; error_details set = failed or cancelled
//	  metadata: 1 = created {1: seconds, 2: nanos}, 11 = model enum,
//	            9 = usage {2: input, 3: output, 5: cache read},
//	            4 = tool call {1: id, 2: name, 3: JSON arguments}
//	gen_metadata(idx, data): data.1 = {3: model enum, 19: model name}
//
// Prompt text and the project folder come from history.jsonl, matched to the prompt steps by
// conversation id or, for the newest prompts that don't carry one yet, by timestamp.

const AgentAntigravity = "antigravity"

const (
	agyStepPrompt = 14
	agyStepTool   = 132
	agyDone       = 3
)

type agyStep struct {
	typ    int64
	status int64
	failed bool
	t      time.Time
	model  int64 // model enum
	usage  Usage
	hasUse bool
	tool   string
	callID string
	args   map[string]any
}

type agyConv struct {
	id     string
	steps  []agyStep
	models map[int64]string // model enum → name
	maxIdx int64            // highest step idx read so far (-1 = none)
}

// agyRecheck: how many of the last steps are re-read on each update, since a running step's
// status (and a reply's usage) are filled in after the step is first written.
const agyRecheck = 8

// agyHistory: one history.jsonl line.
type agyHistory struct {
	Display        string `json:"display"`
	Timestamp      int64  `json:"timestamp"`
	Workspace      string `json:"workspace"`
	ConversationID string `json:"conversationId"`
	Type           string `json:"type"`
}

// openAgy opens a conversation database read-only (it may be in WAL mode while agy runs).
func openAgy(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(2000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// update reads the steps written since the last read (plus the last few, whose status may
// have changed) and refreshes the model names. Steps are append-only, keyed by idx.
func (c *agyConv) update(path string) error {
	db, err := openAgy(path)
	if err != nil {
		return err
	}
	defer db.Close()

	if rows, err := db.Query(`SELECT data FROM gen_metadata`); err == nil {
		for rows.Next() {
			var data []byte
			if rows.Scan(&data) == nil {
				g := pbDecode(data).msg(1)
				if name := g.str(19); name != "" {
					c.models[g.int(3)] = name
				}
			}
		}
		rows.Close()
	}

	from := max(0, c.maxIdx-agyRecheck+1)
	rows, err := db.Query(`SELECT idx, step_type, status, metadata, error_details FROM steps WHERE idx >= ? ORDER BY idx`, from)
	if err != nil {
		return err
	}
	defer rows.Close()
	var fresh []agyStep
	var freshIdx []int64
	for rows.Next() {
		var s agyStep
		var idx int64
		var meta, errDetails []byte
		if rows.Scan(&idx, &s.typ, &s.status, &meta, &errDetails) != nil {
			continue
		}
		m := pbDecode(meta)
		if ts := m.msg(1); ts.int(1) > 0 {
			s.t = time.Unix(ts.int(1), ts.int(2))
		}
		s.model = m.int(11)
		s.failed = len(errDetails) > 0
		if u := m.msg(9); u != nil {
			s.hasUse = true
			s.usage = Usage{In: u.int(2), Out: u.int(3), CacheR: u.int(5)}
		}
		if tc := m.msg(4); tc != nil {
			s.callID, s.tool = tc.str(1), tc.str(2)
			_ = json.Unmarshal([]byte(tc.str(3)), &s.args)
		}
		fresh, freshIdx = append(fresh, s), append(freshIdx, idx)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// replace the re-read tail, append the rest
	keep := len(c.steps)
	if len(freshIdx) > 0 && c.maxIdx >= freshIdx[0] {
		keep = max(0, len(c.steps)-int(c.maxIdx-freshIdx[0]+1))
	}
	c.steps = append(c.steps[:keep], fresh...)
	if n := len(freshIdx); n > 0 {
		c.maxIdx = freshIdx[n-1]
	}
	return nil
}

// readAgyHistory reads history.jsonl (missing file = no prompt text).
func readAgyHistory(path string) []agyHistory {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []agyHistory
	for _, line := range strings.Split(string(data), "\n") {
		var h agyHistory
		if strings.TrimSpace(line) != "" && json.Unmarshal([]byte(line), &h) == nil {
			out = append(out, h)
		}
	}
	return out
}

// agyMatch: the history entry of a prompt step: same conversation id (or none yet), with the
// closest timestamp within 5 seconds. Slash commands don't create prompt steps.
func agyMatch(hist []agyHistory, convID string, t time.Time) *agyHistory {
	var best *agyHistory
	bestD := int64(5000)
	ms := t.UnixMilli()
	for i := range hist {
		h := &hist[i]
		if h.Type != "" || (h.ConversationID != "" && h.ConversationID != convID) {
			continue
		}
		d := h.Timestamp - ms
		if d < 0 {
			d = -d
		}
		if d <= bestD {
			best, bestD = h, d
		}
	}
	return best
}

// agyLastActive: the time of a conversation's last step (one row), zero if unknown.
// agy touches every database when it starts, so file times don't say which ones are active.
func agyLastActive(path string) time.Time {
	db, err := openAgy(path)
	if err != nil {
		return time.Time{}
	}
	defer db.Close()
	var meta []byte
	if db.QueryRow(`SELECT metadata FROM steps ORDER BY idx DESC LIMIT 1`).Scan(&meta) != nil {
		return time.Time{}
	}
	if ts := pbDecode(meta).msg(1); ts.int(1) > 0 {
		return time.Unix(ts.int(1), ts.int(2))
	}
	return time.Time{}
}

// agyFiles: a conversation's database plus its WAL (recent writes live there while agy runs).
func agyFiles(db string) []string { return []string{db, db + "-wal"} }

func (st *Store) scanAntigravity(since time.Time) {
	root := config.AntigravityDir()
	files, _ := filepath.Glob(filepath.Join(root, "conversations", "*.db"))
	if len(files) == 0 {
		return
	}
	histPath := filepath.Join(root, "history.jsonl")
	var hist []agyHistory
	histLoaded := false
	for _, f := range files {
		var sig fileSig
		var latest time.Time
		for _, p := range append(agyFiles(f), histPath) {
			fi, err := os.Stat(p)
			if err != nil {
				continue
			}
			sig.add(fi)
			if p != histPath && fi.ModTime().After(latest) {
				latest = fi.ModTime()
			}
		}
		if latest.Before(since) {
			continue
		}
		key := "antigravity:" + f
		if st.sigs[key] == sig {
			continue
		}
		conv := st.agy[f]
		if conv == nil && agyLastActive(f).Before(since) {
			st.sigs[key] = sig // old conversation: look again only when its files change
			continue
		}
		if conv == nil {
			conv = &agyConv{id: strings.TrimSuffix(filepath.Base(f), ".db"), models: map[int64]string{}, maxIdx: -1}
		}
		if conv.update(f) != nil {
			continue // locked mid-write: retry on the next scan
		}
		st.agy[f] = conv
		if len(conv.steps) == 0 {
			continue // an empty conversation (opened, nothing asked yet)
		}
		if !histLoaded {
			hist, histLoaded = readAgyHistory(histPath), true
		}
		st.sigs[key] = sig
		fresh := buildAntigravity(conv, hist, latest)
		fresh.Transcript = f
		*st.sessionFor(AgentAntigravity, conv.id) = *fresh
	}
}

func buildAntigravity(c *agyConv, hist []agyHistory, mod time.Time) *Session {
	s := newSession(c.id)
	s.Agent = AgentAntigravity
	s.log(firstStepTime(c), "sys", "", "session start (antigravity)", false)
	answered := false
	for _, st := range c.steps {
		t := st.t
		s.touch(t)
		switch {
		case st.typ == agyStepPrompt:
			if answered {
				s.Turns++ // the previous prompt got its answer
			}
			answered = false
			text := "(prompt)"
			if h := agyMatch(hist, c.id, t); h != nil {
				if s.Cwd == "" {
					s.Cwd = h.Workspace
				}
				if strings.TrimSpace(h.Display) != "" {
					text = h.Display
				}
			}
			s.beginPrompt(t, text)
		case st.hasUse:
			model := c.models[st.model]
			if model == "" {
				model = "unknown model"
			}
			s.setLead(t, model)
			s.leadReply(t, model, st.usage)
			answered = true
		}
		if st.tool != "" {
			skill := ""
			if st.tool == "view_file" || st.tool == "read_file" {
				skill = skillFromText(str(st.args, "AbsolutePath") + " " + str(st.args, "path") + " " + str(st.args, "file_path"))
			}
			s.leadTool(t, st.callID, st.tool, agyToolSummary(st.tool, st.args), skill)
			if st.failed || st.status > agyDone {
				s.toolFailed(t, "lead", s.Lead, st.tool+" failed or cancelled")
			} else if st.status == agyDone {
				s.CurrentTool = ""
			}
		}
	}

	// status: working while the last step is a prompt or an unfinished step, unless the
	// database has been quiet for a while
	s.Status = "idle"
	if n := len(c.steps); n > 0 && time.Since(mod) < staleWorking {
		if last := c.steps[n-1]; last.typ == agyStepPrompt || last.status < agyDone || last.typ == agyStepTool {
			s.Status = "working"
		}
	}
	if s.Status == "idle" {
		s.CurrentTool = ""
		if answered {
			s.Turns++
		}
	}
	return s
}

func firstStepTime(c *agyConv) time.Time {
	for _, st := range c.steps {
		if !st.t.IsZero() {
			return st.t
		}
	}
	return time.Time{}
}

// agyToolSummary: "tool: main argument", using the argument names agy's tools use.
func agyToolSummary(tool string, args map[string]any) string {
	for _, k := range []string{"CommandLine", "command", "AbsolutePath", "TargetFile", "path", "file_path", "Query", "query", "SearchPath", "Pattern", "pattern", "Url", "url"} {
		v := str(args, k)
		if v == "" {
			continue
		}
		if strings.Contains(k, "Path") || strings.Contains(k, "File") || k == "path" || k == "file_path" {
			v = filepath.Base(v)
		}
		return tool + ": " + format.OneLine(v)
	}
	return toolSummary(tool, args)
}
