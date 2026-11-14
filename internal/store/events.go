package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"ccwatch/internal/format"
)

// hookEvent: satu baris events.jsonl (ditulis `ccwatch hook`) beserta konteks
// pelaku (lead atau subagent) yang menjalankannya.
type hookEvent struct {
	raw       map[string]any
	name      string
	t         time.Time
	agentID   string
	toolInput map[string]any
	who, mdl  string // pelaku & modelnya, untuk entri log
}

func (e hookEvent) str(k string) string { return str(e.raw, k) }

func str(m map[string]any, k string) string { v, _ := m[k].(string); return v }

func (st *Store) ingestEvent(line []byte) {
	var raw map[string]any
	if json.Unmarshal(line, &raw) != nil {
		return
	}
	sid := str(raw, "session_id")
	if sid == "" {
		return
	}
	t, _ := time.Parse(time.RFC3339Nano, str(raw, "ccwatch_ts"))
	ti, _ := raw["tool_input"].(map[string]any)
	st.session(sid).applyHook(hookEvent{
		raw: raw, name: str(raw, "hook_event_name"), t: t,
		agentID: agentKey(str(raw, "agent_id")), toolInput: ti,
	})
}

func (s *Session) applyHook(e hookEvent) {
	s.HasHooks = true
	s.touch(e.t)
	if v := e.str("cwd"); v != "" && s.Cwd == "" {
		s.Cwd = v
	}
	if v := e.str("transcript_path"); v != "" {
		s.Transcript = v
	}
	if v := e.str("permission_mode"); v != "" {
		s.PermMode = v
	}

	e.who, e.mdl = "lead", s.Lead
	if e.agentID != "" && e.name != "SubagentStart" && e.name != "SubagentStop" {
		if a, ok := s.Agents[e.agentID]; ok {
			e.who, e.mdl = a.Label(), a.Model()
		}
	}

	switch e.name {
	case "SessionStart":
		s.onSessionStart(e)
	case "SessionEnd":
		s.Status, s.EndReason = "ended", e.str("reason")
		s.log(e.t, "sys", "", "session end ("+s.EndReason+")", false)
	case "StatusLine":
		s.onStatusLine(e)
	case "UserPromptSubmit":
		s.onUserPrompt(e)
	case "PreToolUse":
		s.onPreToolUse(e)
	case "PostToolUse":
		if e.agentID == "" {
			s.CurrentTool = ""
		}
	case "PostToolUseFailure":
		s.onToolFailure(e)
	case "SubagentStart":
		s.onSubagentStart(e)
	case "SubagentStop":
		s.onSubagentStop(e)
	case "Stop":
		s.Turns++
		s.Status = "idle"
		s.CurrentTool = ""
		s.log(e.t, "lead", s.Lead, "giliran selesai (turn #"+strconv.Itoa(s.Turns)+")", false)
	case "PreCompact":
		s.Compacts++
		s.log(e.t, "sys", "", "compact konteks ("+e.str("trigger")+")", false)
	case "Notification":
		s.log(e.t, "sys", "", "notif: "+format.OneLine(e.str("message")), false)
	}
}

func (s *Session) onSessionStart(e hookEvent) {
	if s.Status == "ended" {
		s.Status = "idle"
	}
	if m := e.str("model"); m != "" {
		s.Lead = m
	}
	s.log(e.t, "sys", "", "session start ("+e.str("source")+")", false)
}

func (s *Session) onStatusLine(e hookEvent) {
	m := e.str("model")
	if m == "" || m == s.Lead {
		return
	}
	old := s.Lead
	s.Lead = m
	if old != "" {
		s.log(e.t, "sys", m, "model lead diganti: "+format.PrettyModel(old)+" → "+format.PrettyModel(m), false)
	}
}

func (s *Session) onUserPrompt(e hookEvent) {
	prompt := e.str("prompt")
	if kind := systemPrompt(prompt); kind != "" {
		// notifikasi otomatis (background task selesai, hand-back subagent):
		// lanjutan task yang sama, bukan prompt baru dari user
		s.Status = "working"
		s.log(e.t, "sys", "", "notifikasi "+kind+" (lanjutan prompt #"+strconv.Itoa(s.Prompts)+")", false)
		return
	}
	s.Prompts++
	s.Status = "working"
	s.LastPrompt = format.OneLine(prompt)
	s.PromptTimes = append(s.PromptTimes, e.t)
	if name, args, ok := slashSkill(prompt); ok {
		s.addSkill(SkillUse{T: e.t, Name: name, Args: args, Source: "slash", Who: "user", Model: s.Lead})
	}
	s.log(e.t, "user", "", "prompt #"+strconv.Itoa(s.Prompts)+": "+s.LastPrompt, false)
}

func (s *Session) onPreToolUse(e hookEvent) {
	tool := e.str("tool_name")
	sum := toolSummary(tool, e.toolInput)
	if e.agentID == "" {
		s.CurrentTool = sum
		s.lastTool = tool
		s.Status = "working"
	} else if a, ok := s.Agents[e.agentID]; ok {
		a.LastTool = tool
	}
	if tool == "Skill" {
		s.addSkill(SkillUse{T: e.t, Name: str(e.toolInput, "skill"), Source: "tool", AgentID: e.agentID,
			Who: e.who, Model: e.mdl, ID: e.str("tool_use_id")})
	}
	if tool != "Agent" && tool != "Task" {
		s.log(e.t, e.who, e.mdl, sum, false)
		return
	}

	typ := str(e.toolInput, "subagent_type")
	if typ == "" {
		typ = "general-purpose"
	}
	desc, reqModel := str(e.toolInput, "description"), str(e.toolInput, "model")
	s.pendingDesc[typ] = append(s.pendingDesc[typ], pendingAgent{desc, reqModel})
	txt := "dispatch → " + typ
	if desc != "" {
		txt += ": " + desc
	}
	if reqModel != "" {
		txt += " [" + reqModel + "]"
	}
	s.log(e.t, e.who, e.mdl, txt, false)
}

func (s *Session) onToolFailure(e hookEvent) {
	if e.agentID == "" {
		s.CurrentTool = ""
	}
	s.Failures++
	msg := e.str("tool_name") + " gagal"
	if er := e.str("error"); er != "" {
		msg += ": " + format.OneLine(er)
	}
	s.log(e.t, e.who, e.mdl, msg, true)
}

func (s *Session) onSubagentStart(e hookEvent) {
	typ := e.str("agent_type")
	a := s.agent(e.agentID)
	a.hookStarted = true
	a.Type = typ
	a.Status = "running"
	if a.Start.IsZero() {
		a.Start = e.t
	}
	if q := s.pendingDesc[typ]; len(q) > 0 {
		a.Desc, a.ReqModel = q[0].desc, q[0].model
		s.pendingDesc[typ] = q[1:]
	}
	s.log(e.t, typ, a.Model(), "subagent mulai"+descSuffix(a.Desc), false)
}

func (s *Session) onSubagentStop(e hookEvent) {
	typ := e.str("agent_type")
	p := e.str("agent_transcript_path")
	if _, known := s.Agents[e.agentID]; !known && typ == "" {
		// summarizer internal Claude Code: SubagentStop tanpa tipe & tanpa transcript
		if _, err := os.Stat(p); p == "" || err != nil {
			return
		}
	}
	a := s.agent(e.agentID)
	if typ != "" {
		a.Type = typ
	}
	a.Status = "done"
	a.End = e.t
	if p != "" {
		a.Transcript = p
	}
	s.log(e.t, a.Label(), a.Model(), "subagent selesai · "+format.Dur(a.End.Sub(a.Start))+" · "+strconv.Itoa(a.Tools)+" tool", false)
}

func descSuffix(d string) string {
	if d == "" {
		return ""
	}
	return ": " + d
}

// toolSummary: ringkasan satu baris "Tool: argumen utama".
func toolSummary(tool string, ti map[string]any) string {
	arg := ""
	for _, k := range []string{"command", "file_path", "pattern", "path", "url", "query", "description", "skill", "notebook_path"} {
		if v := str(ti, k); v != "" {
			arg = v
			if k == "file_path" || k == "notebook_path" {
				arg = filepath.Base(v)
			}
			break
		}
	}
	if arg == "" {
		return tool
	}
	return tool + ": " + format.OneLine(arg)
}
