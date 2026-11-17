// Package store builds Claude Code session state from two sources:
// events.jsonl (written by the ccwatch hooks) and Claude Code's own JSONL transcripts.
package store

import (
	"strings"
	"time"
)

type Usage struct{ In, Out, CacheR, CacheW int64 }

func (u *Usage) add(o Usage) {
	u.In += o.In
	u.Out += o.Out
	u.CacheR += o.CacheR
	u.CacheW += o.CacheW
}

// InputTotal: all input tokens (new + cache read + cache write).
func (u Usage) InputTotal() int64 { return u.In + u.CacheR + u.CacheW }

type ModelStat struct {
	Model       string
	Roles       map[string]int // role -> message count
	Msgs        int
	Usage       Usage
	First, Last time.Time
}

type Agent struct {
	ID, Type, Desc string
	ReqModel       string // model requested in the Agent tool (if any)
	Models         []string
	Status         string // running | done
	Start, End     time.Time
	LastSeen       time.Time
	Tools          int
	LastTool       string
	Msgs           int
	Usage          Usage
	Transcript     string
	hookStarted    bool
	codexTotal     int64
}

func (a *Agent) Label() string {
	if a.Type != "" {
		return a.Type
	}
	return "subagent"
}

func (a *Agent) Model() string {
	if len(a.Models) > 0 {
		return a.Models[len(a.Models)-1]
	}
	return a.ReqModel
}

// point = one assistant message (or one advisor call) at a point in time.
type point struct {
	T       time.Time
	Role    string // lead | advisor | <subagent type>
	AgentID string
	Model   string
	Tools   int
}

type FlowStep struct {
	Start, End time.Time
	Role       string
	AgentID    string
	Model      string
	Count      int
	Tools      int
}

type LogEntry struct {
	T    time.Time
	Who  string // lead | advisor | subagent type | sys
	Mdl  string // related model (for color)
	Text string
	Bad  bool
}

type AdvisorCall struct {
	T       time.Time
	Model   string
	After   string // last tool / context before the advisor was called
	Usage   Usage
	FromSub string
}

type Session struct {
	ID, Cwd, Transcript string
	Agent               string // claude | codex | gemini | opencode | <name from `ccwatch hook --agent`>
	Start, Last         time.Time
	Status              string // idle | working | ended
	EndReason           string
	Lead                string
	PermMode            string
	AdvisorCfg          string
	Advisor             []AdvisorCall
	AgentDefs           []AgentDef // configured subagents (.claude/agents)
	Skills              []SkillUse
	PromptTimes         []time.Time     // time of each UserPromptSubmit
	InvokedSkills       map[string]bool // from the invoked_skills attachment in the transcript (survives compact)
	Prompts, Turns      int
	LastPrompt          string
	CurrentTool         string
	MainTools           int
	MainMsgs            int
	MainUsage           Usage
	Compacts            int
	Failures            int
	Agents              map[string]*Agent
	AgentOrder          []string
	Stats               map[string]*ModelStat
	Points              []point
	Log                 []LogEntry
	HasHooks            bool

	tailers     map[string]*tailer
	seenMsg     map[string]bool
	seenBlock   map[string]bool
	pendingDesc map[string][]pendingAgent
	lastPoint   map[string]int
	lastTool    string
	settingsAt  time.Time
	defsAt      time.Time
	defsCwd     string
	codexTotal  int64 // last Codex total_token_usage.total_tokens (dedupes repeated token_count)
}

type pendingAgent struct{ desc, model string }

func newSession(id string) *Session {
	return &Session{
		ID: id, Status: "idle", Agent: AgentClaude,
		Agents: map[string]*Agent{}, Stats: map[string]*ModelStat{},
		tailers: map[string]*tailer{}, seenMsg: map[string]bool{}, seenBlock: map[string]bool{},
		pendingDesc: map[string][]pendingAgent{}, lastPoint: map[string]int{},
	}
}

// AgentClaude is the default agent: events without an agent name come from Claude Code.
const AgentClaude = "claude"

// IsClaude: whether the session comes from Claude Code (hooks + ~/.claude transcripts).
// Claude-only parts of the dashboard (advisor, .claude/agents) are hidden for other agents.
func (s *Session) IsClaude() bool { return s.Agent == "" || s.Agent == AgentClaude }

// sessionKey: the Store.Sessions key. Claude ids stay as-is (existing events.jsonl keeps working);
// other agents are prefixed so ids from different tools never collide.
func sessionKey(agent, id string) string {
	if agent == "" || agent == AgentClaude {
		return id
	}
	return agent + ":" + id
}

// agent returns the Agent for id (the "agent-" prefix is ignored), creating it if needed.
func (s *Session) agent(id string) *Agent {
	id = agentKey(id)
	a := s.Agents[id]
	if a == nil {
		a = &Agent{ID: id, Status: "running"}
		s.Agents[id] = a
		s.AgentOrder = append(s.AgentOrder, id)
	}
	return a
}

func agentKey(id string) string { return strings.TrimPrefix(id, "agent-") }

func (s *Session) log(t time.Time, who, mdl, text string, bad bool) {
	s.Log = append(s.Log, LogEntry{T: t, Who: who, Mdl: mdl, Text: text, Bad: bad})
	// hook events & transcripts come from different files → keep time order
	for i := len(s.Log) - 1; i > 0 && s.Log[i-1].T.After(s.Log[i].T); i-- {
		s.Log[i-1], s.Log[i] = s.Log[i], s.Log[i-1]
	}
	if len(s.Log) > 2000 {
		s.Log = s.Log[len(s.Log)-1500:]
	}
}

func (s *Session) touch(t time.Time) {
	if t.IsZero() {
		return
	}
	if s.Start.IsZero() || t.Before(s.Start) {
		s.Start = t
	}
	if t.After(s.Last) {
		s.Last = t
	}
}

// AdvisorLabel: the configured advisor model, or "advisor" if unknown.
func (s *Session) AdvisorLabel() string {
	if s.AdvisorCfg != "" {
		return s.AdvisorCfg
	}
	return "advisor"
}

// recordUsage adds to the model usage stats.
func (s *Session) recordUsage(model, role string, t time.Time, u Usage) {
	ms := s.Stats[model]
	if ms == nil {
		ms = &ModelStat{Model: model, Roles: map[string]int{}, First: t}
		s.Stats[model] = ms
	}
	ms.Msgs++
	ms.Roles[role]++
	ms.Usage.add(u)
	if !t.IsZero() && (ms.First.IsZero() || t.Before(ms.First)) {
		ms.First = t
	}
	if t.After(ms.Last) {
		ms.Last = t
	}
}
