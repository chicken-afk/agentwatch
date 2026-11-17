package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chicken-afk/agentwatch/internal/config"
	"github.com/chicken-afk/agentwatch/internal/format"
)

// Gemini CLI records each chat as one JSON file that is rewritten as the chat grows:
//
//	~/.gemini/tmp/<project>/chats/session-<time>-<id>.json
//
// {"sessionId", "projectHash", "startTime", "lastUpdated", "messages": [{"id", "timestamp",
// "type": user|gemini|info|error|warning, "content", "model", "tokens": {input, output,
// cached, thoughts, tool, total}, "toolCalls": [{"id", "name", "args", "status", "timestamp"}]}]}
//
// <project> is sha256(project root) (older versions) or a short name with a .project_root
// file holding the path (newer versions).

type geminiChat struct {
	SessionID   string          `json:"sessionId"`
	ProjectHash string          `json:"projectHash"`
	StartTime   string          `json:"startTime"`
	LastUpdated string          `json:"lastUpdated"`
	Messages    []geminiMessage `json:"messages"`
}

type geminiMessage struct {
	Timestamp string           `json:"timestamp"`
	Type      string           `json:"type"`
	Content   json.RawMessage  `json:"content"`
	Model     string           `json:"model"`
	Tokens    map[string]any   `json:"tokens"`
	ToolCalls []map[string]any `json:"toolCalls"`
}

func (st *Store) scanGemini(since time.Time) {
	files, _ := filepath.Glob(filepath.Join(config.GeminiDir(), "tmp", "*", "chats", "session-*.json"))
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil || fi.ModTime().Before(since) {
			continue
		}
		key := "gemini-file:" + f
		var sig fileSig
		sig.add(fi)
		if st.sigs[key] == sig {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var c geminiChat
		if json.Unmarshal(b, &c) != nil {
			continue // mid-write; retry on the next scan
		}
		st.sigs[key] = sig
		id := c.SessionID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(f), ".json")
		}
		fresh := buildGemini(id, c, st.geminiCwd(filepath.Dir(filepath.Dir(f)), c.ProjectHash), fi.ModTime())
		fresh.Transcript = f
		*st.sessionFor(AgentGemini, id) = *fresh
	}
}

// geminiCwd: the project root for a ~/.gemini/tmp/<project> folder, "" if unknown.
func (st *Store) geminiCwd(projDir, hash string) string {
	if b, err := os.ReadFile(filepath.Join(projDir, ".project_root")); err == nil {
		return strings.TrimSpace(string(b))
	}
	if hash == "" {
		hash = filepath.Base(projDir)
	}
	// older layout: the folder is sha256(cwd); it can only be matched against a known folder
	if d := st.opts.Dir; d != "" && sha256Hex(d) == hash {
		return d
	}
	return ""
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func buildGemini(id string, c geminiChat, cwd string, mod time.Time) *Session {
	s := newSession(id)
	s.Agent, s.Cwd = AgentGemini, cwd
	if t, err := time.Parse(time.RFC3339Nano, c.StartTime); err == nil {
		s.touch(t)
		s.log(t, "sys", "", "session start (gemini)", false)
	}
	for _, m := range c.Messages {
		t, _ := time.Parse(time.RFC3339Nano, m.Timestamp)
		s.touch(t)
		switch m.Type {
		case "user":
			if txt := geminiText(m.Content); strings.TrimSpace(txt) != "" {
				s.beginPrompt(t, txt)
			}
		case "gemini":
			s.setLead(t, m.Model)
			if m.Tokens != nil {
				in, cached := num(m.Tokens, "input"), num(m.Tokens, "cached")
				s.leadReply(t, m.Model, Usage{In: max(0, in-cached), CacheR: cached,
					Out: num(m.Tokens, "output") + num(m.Tokens, "thoughts")})
			}
			for _, tc := range m.ToolCalls {
				s.geminiTool(t, tc)
			}
		case "error":
			s.toolFailed(t, "sys", "", format.OneLine(geminiText(m.Content)))
		case "info", "warning":
			s.log(t, "sys", "", format.OneLine(geminiText(m.Content)), false)
		}
	}
	s.geminiStatus(c, mod)
	return s
}

func (s *Session) geminiTool(t time.Time, tc map[string]any) {
	name := str(tc, "name")
	args, _ := tc["args"].(map[string]any)
	if ts, err := time.Parse(time.RFC3339Nano, str(tc, "timestamp")); err == nil {
		t = ts
	}
	skill := ""
	switch name {
	case "activate_skill", "skill":
		skill = str(args, "name")
	case "read_file", "run_shell_command":
		skill = skillFromText(str(args, "absolute_path") + " " + str(args, "file_path") + " " + str(args, "command"))
	}
	s.leadTool(t, str(tc, "id"), name, toolSummary(name, args), skill)
	switch str(tc, "status") {
	case "error", "cancelled":
		s.toolFailed(t, "lead", s.Lead, name+" "+str(tc, "status"))
	case "success":
		s.CurrentTool = ""
	}
}

// geminiStatus: working while the last message is the user's or a tool is still running,
// unless the file has been quiet for a while.
func (s *Session) geminiStatus(c geminiChat, mod time.Time) {
	s.Status = "idle"
	if n := len(c.Messages); n > 0 && time.Since(mod) < staleWorking {
		last := c.Messages[n-1]
		running := false
		for _, tc := range last.ToolCalls {
			if st := str(tc, "status"); st == "executing" || st == "scheduled" || st == "validating" || st == "awaiting_approval" {
				running = true
			}
		}
		if last.Type == "user" || running {
			s.Status = "working"
		}
	}
	if s.Status == "idle" {
		s.CurrentTool = ""
	}
}

// geminiText: message content as plain text (a string or a list of {text} parts).
func geminiText(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	var parts []map[string]any
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if t, ok := p["text"].(string); ok {
				b.WriteString(t)
			}
		}
		return b.String()
	}
	return ""
}
