package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ccwatch/internal/config"
	"ccwatch/internal/format"
)

// OpenCode stores everything as small JSON files under <data>/storage:
//
//	session/<projectID>/<sessionID>.json  {id, directory, parentID?, title, time:{created, updated}}
//	message/<sessionID>/<messageID>.json  {id, role, time:{created, completed?}, modelID, providerID,
//	                                       agent|mode, tokens:{input, output, reasoning, cache:{read, write}}}
//	part/<messageID>/<partID>.json        {type: text|tool|…, text, tool, callID, state:{status, input, error}}
//
// Times are Unix milliseconds. A session with parentID is a subagent run (the task tool),
// so it is shown as a subagent of its parent.

type ocSession struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
	ParentID  string `json:"parentID"`
	Title     string `json:"title"`
	Time      struct {
		Created float64 `json:"created"`
		Updated float64 `json:"updated"`
	} `json:"time"`
	path string
	mod  time.Time
}

type ocMessage struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	ModelID    string `json:"modelID"`
	ProviderID string `json:"providerID"`
	Agent      string `json:"agent"`
	Mode       string `json:"mode"`
	Model      struct {
		ModelID string `json:"modelID"`
	} `json:"model"`
	Time struct {
		Created   float64 `json:"created"`
		Completed float64 `json:"completed"`
	} `json:"time"`
	Tokens struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	Error any `json:"error"`
	parts []ocPart
}

type ocPart struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	Synthetic bool           `json:"synthetic"`
	Tool      string         `json:"tool"`
	CallID    string         `json:"callID"`
	State     map[string]any `json:"state"`
}

func (m *ocMessage) model() string {
	if m.ModelID != "" {
		return m.ModelID
	}
	return m.Model.ModelID
}

func (m *ocMessage) agentName() string {
	if m.Agent != "" {
		return m.Agent
	}
	return m.Mode
}

func (st *Store) scanOpenCode(since time.Time) {
	root := filepath.Join(config.OpenCodeDir(), "storage")
	files, _ := filepath.Glob(filepath.Join(root, "session", "*", "*.json"))
	var all []*ocSession
	children := map[string][]*ocSession{}
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil || fi.ModTime().Before(since) {
			continue // an active session (or subagent run) rewrites its file on every update
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s ocSession
		if json.Unmarshal(b, &s) != nil || s.ID == "" {
			continue
		}
		s.path, s.mod = f, fi.ModTime()
		if u := msTime(s.Time.Updated); u.After(s.mod) {
			s.mod = u
		}
		if s.ParentID != "" {
			children[s.ParentID] = append(children[s.ParentID], &s)
			continue
		}
		all = append(all, &s)
	}
	for _, s := range all {
		latest := s.mod
		for _, c := range children[s.ID] {
			if c.mod.After(latest) {
				latest = c.mod
			}
		}
		if latest.Before(since) {
			continue
		}
		// rebuild only when one of the session's message/part files changed (stat only, no reads)
		sig := fileSig{mod: latest}
		ocSignature(root, s.ID, &sig)
		for _, c := range children[s.ID] {
			ocSignature(root, c.ID, &sig)
		}
		key := "opencode:" + s.ID
		if st.sigs[key] == sig {
			continue
		}
		st.sigs[key] = sig
		msgs := readOCMessages(root, s.ID)
		kids := map[*ocSession][]*ocMessage{}
		for _, c := range children[s.ID] {
			kids[c] = readOCMessages(root, c.ID)
		}
		fresh := buildOpenCode(s, msgs, kids)
		fresh.Transcript = s.path
		*st.sessionFor(AgentOpenCode, s.ID) = *fresh
	}
}

// ocSignature adds the message and part files of a session to sig.
func ocSignature(root, sessionID string, sig *fileSig) {
	files, _ := filepath.Glob(filepath.Join(root, "message", sessionID, "*.json"))
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			sig.add(fi)
		}
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		parts, _ := filepath.Glob(filepath.Join(root, "part", id, "*.json"))
		for _, pf := range parts {
			if fi, err := os.Stat(pf); err == nil {
				sig.add(fi)
			}
		}
	}
}

// readOCMessages reads a session's messages (sorted by creation) with their parts.
func readOCMessages(root, sessionID string) []*ocMessage {
	files, _ := filepath.Glob(filepath.Join(root, "message", sessionID, "*.json"))
	var out []*ocMessage
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m ocMessage
		if json.Unmarshal(b, &m) != nil || m.ID == "" {
			continue
		}
		parts, _ := filepath.Glob(filepath.Join(root, "part", m.ID, "*.json"))
		for _, pf := range parts {
			pb, err := os.ReadFile(pf)
			if err != nil {
				continue
			}
			var p ocPart
			if json.Unmarshal(pb, &p) == nil {
				m.parts = append(m.parts, p)
			}
		}
		sort.Slice(m.parts, func(i, j int) bool { return m.parts[i].ID < m.parts[j].ID }) // ids are time-ordered
		out = append(out, &m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Created < out[j].Time.Created })
	return out
}

func buildOpenCode(info *ocSession, msgs []*ocMessage, kids map[*ocSession][]*ocMessage) *Session {
	s := newSession(info.ID)
	s.Agent, s.Cwd = AgentOpenCode, info.Directory
	if t := msTime(info.Time.Created); !t.IsZero() {
		s.touch(t)
		s.log(t, "sys", "", "session start (opencode) "+format.OneLine(info.Title), false)
	}
	for _, m := range msgs {
		t := msTime(m.Time.Created)
		s.touch(t)
		if m.Role == "user" {
			if txt := ocUserText(m); txt != "" {
				s.beginPrompt(t, txt)
			}
			continue
		}
		s.setLead(t, m.model())
		if a := m.agentName(); a != "" {
			s.PermMode = a // OpenCode's primary agent (build/plan) plays the role of a mode
		}
		for _, p := range m.parts {
			if p.Type == "tool" {
				s.ocTool(t, p)
			}
		}
		if m.Time.Completed > 0 {
			done := msTime(m.Time.Completed)
			s.touch(done)
			s.leadReply(done, m.model(), ocUsage(m))
			s.CurrentTool = ""
		}
		if m.Error != nil {
			s.toolFailed(t, "lead", s.Lead, "error: "+format.OneLine(ocErrorText(m.Error)))
		}
	}
	for c, cm := range kids {
		s.ocSubagent(c, cm)
	}
	s.Status = "idle"
	if n := len(msgs); n > 0 && time.Since(info.mod) < staleWorking {
		last := msgs[n-1]
		if last.Role == "user" || last.Time.Completed == 0 {
			s.Status = "working"
		}
	}
	if s.Running() > 0 && time.Since(info.mod) < staleWorking {
		s.Status = "working"
	}
	if s.Status == "idle" {
		s.CurrentTool = ""
	}
	return s
}

func (s *Session) ocTool(t time.Time, p ocPart) {
	input, _ := p.State["input"].(map[string]any)
	if tm, ok := p.State["time"].(map[string]any); ok {
		if st := msTime(fnum(tm, "start")); !st.IsZero() {
			t = st
		}
	}
	skill := ""
	switch p.Tool {
	case "skill":
		skill = str(input, "name")
	case "read", "bash":
		skill = skillFromText(str(input, "filePath") + " " + str(input, "command"))
	case "task":
		return // shown through the child session as a subagent
	}
	s.leadTool(t, p.CallID, p.Tool, toolSummary(p.Tool, ocInput(input)), skill)
	switch str(p.State, "status") {
	case "error":
		s.toolFailed(t, "lead", s.Lead, p.Tool+" failed: "+format.OneLine(str(p.State, "error")))
	case "completed":
		s.CurrentTool = ""
	}
}

// ocInput maps OpenCode's camelCase tool inputs onto the keys toolSummary knows.
func ocInput(in map[string]any) map[string]any {
	if fp := str(in, "filePath"); fp != "" {
		in = map[string]any{"file_path": fp}
	}
	return in
}

// ocSubagent adds a child session (task tool run) as a subagent of s.
func (s *Session) ocSubagent(c *ocSession, msgs []*ocMessage) {
	a := s.agent(c.ID)
	a.Desc = c.Title
	a.Start = msTime(c.Time.Created)
	a.Status = "done"
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		if a.Type == "" {
			a.Type = m.agentName()
		}
		if md := m.model(); md != "" && (len(a.Models) == 0 || a.Models[len(a.Models)-1] != md) {
			a.Models = append(a.Models, md)
		}
		for _, p := range m.parts {
			if p.Type == "tool" {
				a.Tools++
				a.LastTool = p.Tool
			}
		}
		t := msTime(m.Time.Completed)
		if t.IsZero() {
			a.Status = "running"
			continue
		}
		u := ocUsage(m)
		a.Msgs++
		a.Usage.add(u)
		a.LastSeen, a.End = t, t
		s.recordUsage(m.model(), a.Label(), t, u)
		s.Points = append(s.Points, point{T: t, Role: a.Label(), AgentID: a.ID, Model: m.model()})
	}
	if a.Status == "running" && time.Since(c.mod) > staleWorking {
		a.Status = "done"
	}
	if a.Type == "" {
		a.Type = "subagent"
	}
	s.log(a.Start, a.Label(), a.Model(), "subagent started"+descSuffix(a.Desc), false)
	if a.Status == "done" && !a.End.IsZero() {
		s.log(a.End, a.Label(), a.Model(), "subagent done · "+format.Dur(a.End.Sub(a.Start)), false)
	}
}

func ocUsage(m *ocMessage) Usage {
	return Usage{In: m.Tokens.Input, Out: m.Tokens.Output + m.Tokens.Reasoning,
		CacheR: m.Tokens.Cache.Read, CacheW: m.Tokens.Cache.Write}
}

// ocUserText: the text the user typed (synthetic parts are added by OpenCode itself).
func ocUserText(m *ocMessage) string {
	var b strings.Builder
	for _, p := range m.parts {
		if p.Type == "text" && !p.Synthetic {
			b.WriteString(p.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func ocErrorText(e any) string {
	if m, ok := e.(map[string]any); ok {
		if d, ok := m["data"].(map[string]any); ok {
			if msg := str(d, "message"); msg != "" {
				return msg
			}
		}
		return str(m, "name")
	}
	return "error"
}

// fnum reads a JSON number as float64.
func fnum(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }
