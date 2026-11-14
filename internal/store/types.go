// Package store membangun state sesi Claude Code dari dua sumber:
// events.jsonl (ditulis hooks ccwatch) dan transcript JSONL milik Claude Code.
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

// InputTotal: semua token masukan (baru + cache read + cache write).
func (u Usage) InputTotal() int64 { return u.In + u.CacheR + u.CacheW }

type ModelStat struct {
	Model       string
	Roles       map[string]int // role -> jumlah pesan
	Msgs        int
	Usage       Usage
	First, Last time.Time
}

type Agent struct {
	ID, Type, Desc string
	ReqModel       string // model yang diminta di Agent tool (kalau ada)
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

// point = satu pesan assistant (atau satu panggilan advisor) pada satu waktu.
type point struct {
	T       time.Time
	Role    string // lead | advisor | <tipe subagent>
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
	Who  string // lead | advisor | tipe subagent | sys
	Mdl  string // model terkait (untuk warna)
	Text string
	Bad  bool
}

type AdvisorCall struct {
	T       time.Time
	Model   string
	After   string // tool / konteks terakhir sebelum advisor dipanggil
	Usage   Usage
	FromSub string
}

type Session struct {
	ID, Cwd, Transcript string
	Start, Last         time.Time
	Status              string // idle | working | ended
	EndReason           string
	Lead                string
	PermMode            string
	AdvisorCfg          string
	Advisor             []AdvisorCall
	AgentDefs           []AgentDef // subagent terkonfigurasi (.claude/agents)
	Skills              []SkillUse
	PromptTimes         []time.Time     // waktu tiap UserPromptSubmit
	InvokedSkills       map[string]bool // dari attachment invoked_skills di transcript (bertahan setelah compact)
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
}

type pendingAgent struct{ desc, model string }

func newSession(id string) *Session {
	return &Session{
		ID: id, Status: "idle",
		Agents: map[string]*Agent{}, Stats: map[string]*ModelStat{},
		tailers: map[string]*tailer{}, seenMsg: map[string]bool{}, seenBlock: map[string]bool{},
		pendingDesc: map[string][]pendingAgent{}, lastPoint: map[string]int{},
	}
}

// agent mengembalikan Agent untuk id (prefix "agent-" diabaikan), membuatnya kalau belum ada.
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
	// event hooks & transcript dibaca dari file berbeda → jaga urutan waktu
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

// AdvisorLabel: model advisor yang dikonfigurasi, atau "advisor" kalau tidak diketahui.
func (s *Session) AdvisorLabel() string {
	if s.AdvisorCfg != "" {
		return s.AdvisorCfg
	}
	return "advisor"
}

// recordUsage menambah statistik pemakaian model.
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
