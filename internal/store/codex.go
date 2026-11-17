package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ccwatch/internal/config"
	"ccwatch/internal/format"
)

// Codex CLI writes one rollout per session:
//
//	$CODEX_HOME/sessions/YYYY/MM/DD/rollout-<datetime>-<uuid>.jsonl
//
// Each line is {"timestamp", "type", "payload"} where type is one of
// session_meta (id, cwd), turn_context (cwd, model), response_item (message,
// function_call, custom_tool_call, local_shell_call, …), event_msg (user_message,
// agent_message, task_started, task_complete, turn_aborted, token_count) or compacted.

// discoverCodex registers rollouts modified since `since`. A rollout whose session_meta has
// parent_thread_id is a subagent thread: it is shown as a subagent of its parent session.
func (st *Store) discoverCodex(since time.Time) {
	files, _ := filepath.Glob(filepath.Join(config.CodexDir(), "sessions", "*", "*", "*", "rollout-*.jsonl"))
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil || fi.ModTime().Before(since) {
			continue
		}
		meta, ok := st.codexMeta[f]
		if !ok {
			meta = readCodexMeta(f)
			st.codexMeta[f] = meta
		}
		if meta.parent != "" {
			parent := st.sessionFor(AgentCodex, meta.parent)
			a := parent.agent(meta.id)
			if a.Transcript == "" {
				a.Transcript, a.Type = f, meta.role
			}
			continue
		}
		s := st.sessionFor(AgentCodex, codexID(f))
		if s.Transcript == "" {
			s.Transcript = f
		}
		if s.Last.Before(fi.ModTime()) {
			s.Last = fi.ModTime()
		}
	}
}

type codexMetaInfo struct{ id, parent, role string }

// readCodexMeta reads the session_meta line (the first line of a rollout).
func readCodexMeta(path string) codexMetaInfo {
	info := codexMetaInfo{id: codexID(path)}
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	line, _ := r.ReadBytes('\n')
	var l codexLine
	if json.Unmarshal(line, &l) != nil || l.Type != "session_meta" {
		return info
	}
	info.parent = str(l.Payload, "parent_thread_id")
	for _, k := range []string{"agent_role", "agent_type", "agent_nickname"} {
		if v := str(l.Payload, k); v != "" {
			info.role = v
			break
		}
	}
	if info.role == "" {
		info.role = "subagent"
	}
	return info
}

// ingestCodexAgent reads a subagent thread's rollout into Agent a of session s.
func (s *Session) ingestCodexAgent(a *Agent, line []byte) {
	var l codexLine
	if json.Unmarshal(line, &l) != nil || l.Payload == nil {
		return
	}
	t, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	if !t.IsZero() {
		if a.Start.IsZero() {
			a.Start = t
			s.log(t, a.Label(), a.Model(), "subagent started", false)
		}
		a.LastSeen = t
	}
	p := l.Payload
	switch l.Type {
	case "turn_context":
		if m := str(p, "model"); m != "" && a.Model() != m {
			a.Models = append(a.Models, m)
		}
	case "event_msg":
		switch str(p, "type") {
		case "user_message":
			if a.Desc == "" && !codexInjected(str(p, "message")) {
				a.Desc = format.OneLine(str(p, "message"))
			}
		case "task_started", "turn_started":
			a.Status = "running"
		case "task_complete", "turn_complete", "turn_aborted":
			a.Status, a.End = "done", t
			s.log(t, a.Label(), a.Model(), "subagent done · "+format.Dur(a.End.Sub(a.Start))+" · "+strconv.Itoa(a.Tools)+" tools", false)
		case "token_count":
			info, _ := p["info"].(map[string]any)
			last, _ := info["last_token_usage"].(map[string]any)
			total, _ := info["total_token_usage"].(map[string]any)
			if last == nil || num(total, "total_tokens") == a.codexTotal {
				return
			}
			a.codexTotal = num(total, "total_tokens")
			u := codexUsage(last)
			a.Msgs++
			a.Usage.add(u)
			s.recordUsage(a.Model(), a.Label(), t, u)
			s.Points = append(s.Points, point{T: t, Role: a.Label(), AgentID: a.ID, Model: a.Model()})
		}
	case "response_item":
		switch typ := str(p, "type"); typ {
		case "function_call", "custom_tool_call", "local_shell_call", "web_search_call":
			a.Tools++
			a.LastTool = str(p, "name")
		}
	}
}

// codexID: the session uuid at the end of the rollout file name (the whole name if it has none).
func codexID(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if len(base) >= 36 {
		if id := base[len(base)-36:]; strings.Count(id, "-") == 4 {
			return id
		}
	}
	return strings.TrimPrefix(base, "rollout-")
}

type codexLine struct {
	Timestamp string         `json:"timestamp"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
}

func (s *Session) ingestCodex(line []byte) {
	var l codexLine
	if json.Unmarshal(line, &l) != nil || l.Payload == nil {
		return
	}
	t, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	s.touch(t)
	p := l.Payload
	switch l.Type {
	case "session_meta":
		if v := str(p, "cwd"); v != "" {
			s.Cwd = v
		}
		s.log(t, "sys", "", "session start (codex "+str(p, "cli_version")+")", false)
	case "turn_context":
		if v := str(p, "cwd"); v != "" {
			s.Cwd = v
		}
		s.setLead(t, str(p, "model"))
		if v := str(p, "approval_policy"); v != "" {
			s.PermMode = v
		}
	case "compacted":
		s.Compacts++
		s.log(t, "sys", "", "context compact", false)
	case "event_msg":
		s.codexEvent(t, p)
	case "response_item":
		s.codexItem(t, p)
	}
}

func (s *Session) codexEvent(t time.Time, p map[string]any) {
	switch str(p, "type") {
	case "user_message":
		if msg := str(p, "message"); strings.TrimSpace(msg) != "" && !codexInjected(msg) {
			s.beginPrompt(t, msg)
		}
	case "task_started", "turn_started":
		s.Status = "working"
	case "task_complete", "turn_complete":
		s.endTurn(t)
	case "turn_aborted":
		s.Status, s.CurrentTool = "idle", ""
		s.log(t, "sys", "", "turn aborted ("+str(p, "reason")+")", false)
	case "token_count":
		info, _ := p["info"].(map[string]any)
		total, _ := info["total_token_usage"].(map[string]any)
		last, _ := info["last_token_usage"].(map[string]any)
		if last == nil {
			return
		}
		// Codex repeats token_count without a new response; only count it when the total grows.
		if tt := num(total, "total_tokens"); tt > 0 {
			if tt == s.codexTotal {
				return
			}
			s.codexTotal = tt
		}
		s.leadReply(t, "", codexUsage(last))
	}
}

// codexUsage: Codex's input_tokens includes the cached part, so it is split out.
func codexUsage(u map[string]any) Usage {
	in, cached := num(u, "input_tokens"), num(u, "cached_input_tokens")
	return Usage{In: max(0, in-cached), CacheR: cached, CacheW: num(u, "cache_write_input_tokens"), Out: num(u, "output_tokens")}
}

// codexInjected: context Codex adds as a "user" message (AGENTS.md, environment), not typed by the user.
func codexInjected(msg string) bool {
	m := strings.TrimSpace(msg)
	for _, tag := range []string{"<environment_context>", "<user_instructions>", "<turn_aborted>", "# AGENTS.md instructions", "<user_shell_command>"} {
		if strings.HasPrefix(m, tag) {
			return true
		}
	}
	return false
}

func (s *Session) codexItem(t time.Time, p map[string]any) {
	id := str(p, "call_id")
	switch str(p, "type") {
	case "function_call":
		name := str(p, "name")
		var args map[string]any
		_ = json.Unmarshal([]byte(str(p, "arguments")), &args)
		cmd := codexCommand(args)
		sum := name
		if cmd != "" {
			sum = "shell: " + format.OneLine(cmd)
		} else if a := toolSummary(name, args); a != name {
			sum = a
		}
		skill := skillFromText(cmd)
		if name == "skill" || name == "use_skill" {
			skill = str(args, "name")
		}
		s.leadTool(t, id, name, sum, skill)
	case "local_shell_call":
		action, _ := p["action"].(map[string]any)
		cmd := codexCommand(action)
		s.leadTool(t, id, "shell", "shell: "+format.OneLine(cmd), skillFromText(cmd))
	case "custom_tool_call":
		name := str(p, "name")
		sum := name
		if f := patchFile(str(p, "input")); f != "" {
			sum += ": " + filepath.Base(f)
		}
		s.leadTool(t, id, name, sum, "")
	case "web_search_call":
		action, _ := p["action"].(map[string]any)
		s.leadTool(t, id, "web_search", toolSummary("web_search", action), "")
	case "function_call_output", "custom_tool_call_output":
		s.CurrentTool = ""
		if code, ok := codexExitCode(p["output"]); ok && code != 0 {
			s.toolFailed(t, "lead", s.Lead, "tool failed (exit "+strconv.FormatInt(code, 10)+")")
		}
	}
}

// codexCommand: the shell command of a shell/exec_command call ("bash -lc <cmd>" → cmd).
func codexCommand(args map[string]any) string {
	if c := str(args, "cmd"); c != "" {
		return c
	}
	switch c := args["command"].(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, x := range c {
			if v, ok := x.(string); ok {
				parts = append(parts, v)
			}
		}
		if n := len(parts); n >= 3 && (parts[n-2] == "-lc" || parts[n-2] == "-c") {
			return parts[n-1]
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// patchFile: the first file named in an apply_patch body.
func patchFile(patch string) string {
	for _, l := range strings.Split(patch, "\n") {
		for _, pre := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: "} {
			if strings.HasPrefix(l, pre) {
				return strings.TrimSpace(strings.TrimPrefix(l, pre))
			}
		}
	}
	return ""
}

// codexExitCode reads metadata.exit_code from a tool output (a JSON string or object).
func codexExitCode(out any) (int64, bool) {
	var m map[string]any
	switch v := out.(type) {
	case string:
		if json.Unmarshal([]byte(v), &m) != nil {
			return 0, false
		}
	case map[string]any:
		m = v
	}
	md, _ := m["metadata"].(map[string]any)
	if _, ok := md["exit_code"]; !ok {
		return 0, false
	}
	return num(md, "exit_code"), true
}
