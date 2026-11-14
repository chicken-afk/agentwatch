package store

import (
	"encoding/json"
	"strings"
	"time"

	"ccwatch/internal/format"
)

// transcriptLine: satu baris transcript JSONL Claude Code (hanya field yang dipakai).
type transcriptLine struct {
	Type       string `json:"type"`
	Timestamp  string `json:"timestamp"`
	Cwd        string `json:"cwd"`
	Attachment *struct {
		Type   string `json:"type"`
		Skills []struct {
			Name string `json:"name"`
		} `json:"skills"`
	} `json:"attachment"`
	Message *struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   json.RawMessage `json:"usage"`
	} `json:"message"`
}

// ingestTranscript memproses satu baris transcript lead (agentID "") atau subagent.
func (s *Session) ingestTranscript(agentID string, line []byte) {
	var m transcriptLine
	if json.Unmarshal(line, &m) != nil {
		return
	}
	if agentID == "" {
		s.noteLeadMeta(m)
	}
	if m.Message == nil {
		return
	}
	t, _ := time.Parse(time.RFC3339Nano, m.Timestamp)
	switch m.Type {
	case "user":
		if name, args, ok := transcriptSlash(m.Message.Content); ok {
			s.addSkill(SkillUse{T: t, Name: name, Args: args, Source: "slash", AgentID: agentID, Who: "user", Model: s.Lead})
		}
	case "assistant":
		s.ingestAssistant(agentID, m, t)
	}
}

// noteLeadMeta: cwd (sesi tanpa hooks, --recent) dan skill yang dipertahankan setelah compact.
func (s *Session) noteLeadMeta(m transcriptLine) {
	if s.Cwd == "" && m.Cwd != "" {
		s.Cwd = m.Cwd
	}
	if m.Attachment == nil || m.Attachment.Type != "invoked_skills" {
		return
	}
	if s.InvokedSkills == nil {
		s.InvokedSkills = map[string]bool{}
	}
	for _, sk := range m.Attachment.Skills {
		if sk.Name != "" {
			s.InvokedSkills[sk.Name] = true
		}
	}
}

// msgCtx: konteks satu pesan assistant yang sedang diproses.
type msgCtx struct {
	agentID string
	agent   *Agent // nil = lead
	role    string // lead | tipe subagent
	model   string
	t       time.Time
	first   bool // false kalau message.id ini sudah pernah dihitung
}

func (s *Session) ingestAssistant(agentID string, m transcriptLine, t time.Time) {
	model := m.Message.Model
	if model == "" || strings.HasPrefix(model, "<") { // <synthetic>
		return
	}
	s.touch(t)

	c := msgCtx{agentID: agentID, role: "lead", model: model, t: t}
	if agentID != "" {
		c.agent = s.agent(agentID)
		c.role = c.agent.Label()
		if t.After(c.agent.LastSeen) {
			c.agent.LastSeen = t
		}
		if c.agent.Start.IsZero() || t.Before(c.agent.Start) {
			c.agent.Start = t
		}
	}

	// Claude Code menulis satu baris per content block dengan message.id sama →
	// hitung pesan & usage sekali saja per id.
	key := agentID + "|" + m.Message.ID
	c.first = m.Message.ID == "" || !s.seenMsg[key]
	if m.Message.ID != "" {
		s.seenMsg[key] = true
	}

	var blocks []map[string]any
	_ = json.Unmarshal(m.Message.Content, &blocks)

	// titik & log advisor ditambahkan paling akhir, setelah model advisor
	// dikoreksi oleh usage.iterations di bawah
	advStart := len(s.Advisor)
	defer s.flushAdvisorCalls(advStart, c.role)

	for _, b := range blocks {
		s.ingestBlock(c, b)
	}
	if !c.first {
		return
	}

	u, advIters := parseUsage(m.Message.Usage)
	s.applyAdvisorIterations(advIters, t)
	s.recordUsage(model, c.role, t, u)

	s.Points = append(s.Points, point{T: t, Role: c.role, AgentID: agentID, Model: model, Tools: countToolUse(blocks)})
	s.lastPoint[agentID] = len(s.Points) - 1

	if c.agent != nil {
		c.agent.Msgs++
		c.agent.Usage.add(u)
		if len(c.agent.Models) == 0 || c.agent.Models[len(c.agent.Models)-1] != model {
			c.agent.Models = append(c.agent.Models, model)
		}
		return
	}
	s.MainMsgs++
	s.MainUsage.add(u)
	if s.Lead != "" && s.Lead != model && s.MainMsgs > 1 {
		s.log(t, "sys", model, "model lead: "+format.PrettyModel(s.Lead)+" → "+format.PrettyModel(model), false)
	}
	s.Lead = model
}

// ingestBlock memproses satu content block: pemanggilan tool, skill, dan advisor.
func (s *Session) ingestBlock(c msgCtx, b map[string]any) {
	bt, _ := b["type"].(string)
	name, _ := b["name"].(string)
	bid, _ := b["id"].(string)
	if bid != "" {
		if s.seenBlock[c.agentID+bid] {
			return
		}
		s.seenBlock[c.agentID+bid] = true
	}
	switch {
	case bt == "tool_use":
		if !c.first {
			if i, ok := s.lastPoint[c.agentID]; ok {
				s.Points[i].Tools++
			}
		}
		if c.agent != nil {
			c.agent.Tools++
			c.agent.LastTool = name
		} else {
			s.MainTools++
			s.lastTool = name
		}
		if name == "Skill" {
			in, _ := b["input"].(map[string]any)
			s.addSkill(SkillUse{T: c.t, Name: str(in, "skill"), Args: str(in, "args"), Source: "tool",
				AgentID: c.agentID, Who: c.role, Model: c.model, ID: bid})
		}
	case bt == "server_tool_use" && name == "advisor":
		after := s.lastTool
		call := AdvisorCall{T: c.t, Model: s.AdvisorLabel()}
		if c.agent != nil {
			after = c.agent.LastTool
			call.FromSub = c.agent.Label()
		}
		call.After = after
		s.Advisor = append(s.Advisor, call)
	}
}

// applyAdvisorIterations: usage.iterations (advisor tool) → model & token advisor yang sebenarnya.
func (s *Session) applyAdvisorIterations(iters []advIter, t time.Time) {
	for _, it := range iters {
		if n := len(s.Advisor); n > 0 {
			call := &s.Advisor[n-1]
			if it.model != "" {
				call.Model = it.model
			}
			call.Usage.add(it.u)
		}
		am := it.model
		if am == "" {
			am = s.AdvisorLabel()
		}
		s.recordUsage(am, "advisor", t, it.u)
	}
}

// flushAdvisorCalls mencatat titik alur dan log untuk panggilan advisor baru (index >= from).
func (s *Session) flushAdvisorCalls(from int, role string) {
	for _, c := range s.Advisor[from:] {
		s.Points = append(s.Points, point{T: c.T.Add(time.Millisecond), Role: "advisor", Model: c.Model})
		s.log(c.T, "advisor", c.Model, "advisor dipanggil oleh "+role+afterSuffix(c.After), false)
	}
}

func afterSuffix(after string) string {
	if after == "" {
		return ""
	}
	return " (setelah " + after + ")"
}

func countToolUse(blocks []map[string]any) int {
	n := 0
	for _, b := range blocks {
		if bt, _ := b["type"].(string); bt == "tool_use" {
			n++
		}
	}
	return n
}

type advIter struct {
	model string
	u     Usage
}

func parseUsage(raw json.RawMessage) (Usage, []advIter) {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return Usage{}, nil
	}
	u := usageFrom(m)
	var adv []advIter
	if its, ok := m["iterations"].([]any); ok {
		for _, it := range its {
			im, _ := it.(map[string]any)
			typ, _ := im["type"].(string)
			if !strings.Contains(typ, "advisor") {
				continue
			}
			mdl, _ := im["model"].(string)
			adv = append(adv, advIter{model: mdl, u: usageFrom(im)})
		}
	}
	return u, adv
}

func usageFrom(m map[string]any) Usage {
	num := func(k string) int64 { f, _ := m[k].(float64); return int64(f) }
	return Usage{In: num("input_tokens"), Out: num("output_tokens"),
		CacheR: num("cache_read_input_tokens"), CacheW: num("cache_creation_input_tokens")}
}
