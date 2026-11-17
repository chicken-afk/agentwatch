package store

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/chicken-afk/agentwatch/internal/format"
)

// transcriptLine: one line of a Claude Code JSONL transcript (only the fields we use).
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

// ingestTranscript processes one transcript line from the lead (agentID "") or a subagent.
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

// noteLeadMeta: cwd (sessions without hooks, --recent) and skills preserved after a compact.
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

// msgCtx: context for the assistant message being processed.
type msgCtx struct {
	agentID string
	agent   *Agent // nil = lead
	role    string // lead | subagent type
	model   string
	t       time.Time
	first   bool // false if this message.id has already been counted
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

	// Claude Code writes one line per content block with the same message.id →
	// count the message & usage only once per id.
	key := agentID + "|" + m.Message.ID
	c.first = m.Message.ID == "" || !s.seenMsg[key]
	if m.Message.ID != "" {
		s.seenMsg[key] = true
	}

	var blocks []map[string]any
	_ = json.Unmarshal(m.Message.Content, &blocks)

	// advisor points & log are added last, after the advisor model
	// has been corrected by usage.iterations below
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
		s.log(t, "sys", model, "lead model: "+format.PrettyModel(s.Lead)+" → "+format.PrettyModel(model), false)
	}
	s.Lead = model
}

// ingestBlock processes one content block: tool calls, skills, and the advisor.
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

// applyAdvisorIterations: usage.iterations (advisor tool) → the advisor's actual model & tokens.
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

// flushAdvisorCalls records flow points and log entries for new advisor calls (index >= from).
func (s *Session) flushAdvisorCalls(from int, role string) {
	for _, c := range s.Advisor[from:] {
		s.Points = append(s.Points, point{T: c.T.Add(time.Millisecond), Role: "advisor", Model: c.Model})
		s.log(c.T, "advisor", c.Model, "advisor called by "+role+afterSuffix(c.After), false)
	}
}

func afterSuffix(after string) string {
	if after == "" {
		return ""
	}
	return " (after " + after + ")"
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
