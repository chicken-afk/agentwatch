package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chicken-afk/agentwatch/internal/config"
	"github.com/chicken-afk/agentwatch/internal/pathmatch"
)

const (
	metadataTTL     = 15 * time.Second // how often settings & agent definitions are re-read
	recentScanEvery = 3 * time.Second
	agentIdleAfter  = 20 * time.Second // without hooks: a subagent counts as done once its transcript has been quiet this long
)

type Store struct {
	Sessions map[string]*Session
	events   *tailer
	legacy   *tailer // events.jsonl of pre-rebrand (ccwatch) hooks
	opts     Options
	lastScan time.Time

	lastProviderScan time.Time
	sigs             map[string]fileSig // rebuilt-provider session key → last seen file signature
	codexMeta        map[string]codexMetaInfo
}

func New(o Options) *Store {
	return &Store{
		Sessions:  map[string]*Session{},
		events:    &tailer{path: config.EventsPath()},
		legacy:    &tailer{path: config.LegacyEventsPath()},
		opts:      o,
		sigs:      map[string]fileSig{},
		codexMeta: map[string]codexMetaInfo{},
	}
}

func (st *Store) Options() Options { return st.opts }

// session: the Claude Code session id (created if needed).
func (st *Store) session(id string) *Session { return st.sessionFor(AgentClaude, id) }

// sessionFor: session id of the given agent (created if needed).
func (st *Store) sessionFor(agent, id string) *Session {
	key := sessionKey(agent, id)
	s := st.Sessions[key]
	if s == nil {
		s = newSession(id)
		if agent != "" {
			s.Agent = agent
		}
		st.Sessions[key] = s
	}
	return s
}

// Visible: sessions passing the folder filter (Options.Dir), most recently active first.
func (st *Store) Visible() []*Session {
	var out []*Session
	for _, s := range st.Sessions {
		if st.opts.Dir != "" && !pathmatch.InDir(s.Cwd, st.opts.Dir) {
			continue
		}
		if !st.opts.wantsAgent(s.Agent) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	return out
}

// Refresh reads new events & transcripts and updates every session.
func (st *Store) Refresh() {
	st.events.read(st.ingestEvent)
	if st.legacy.path != st.events.path {
		st.legacy.read(st.ingestEvent)
	}

	now := time.Now()
	if st.opts.Recent > 0 && now.Sub(st.lastScan) > recentScanEvery {
		st.lastScan = now
		st.scanRecentTranscripts(st.opts.Recent)
	}

	st.scanProviders(now)

	for _, s := range st.Sessions {
		if !s.IsClaude() {
			continue // other agents are rebuilt by their provider in scanProviders
		}
		st.refreshMetadata(s, now)
		if s.Transcript != "" {
			st.tailTranscripts(s, now)
		}
	}
}

// refreshMetadata reloads subagent definitions and session advisor labels.
func (st *Store) refreshMetadata(s *Session, now time.Time) {
	if s.Cwd != "" && (s.defsCwd != s.Cwd || now.Sub(s.defsAt) > metadataTTL) {
		s.defsAt, s.defsCwd = now, s.Cwd
		s.AgentDefs = readAgentDefs(s.Cwd)
	}
	if s.AdvisorCfg == "" || now.Sub(s.settingsAt) > metadataTTL {
		s.settingsAt = now
		if adv := st.opts.advisorFor(s.ID); adv != "" {
			s.AdvisorCfg = adv
		} else {
			s.AdvisorCfg = config.AdvisorModel(s.Cwd)
		}
	}
}

// tailTranscripts reads the session's lead and subagent transcripts.
func (st *Store) tailTranscripts(s *Session, now time.Time) {
	s.tail(s.Transcript, "")
	// subagent transcript: <session>/subagents/agent-*.jsonl
	dir := strings.TrimSuffix(s.Transcript, ".jsonl")
	matches, _ := filepath.Glob(filepath.Join(dir, "subagents", "agent-*.jsonl"))
	for _, m := range matches {
		id := agentKey(strings.TrimSuffix(filepath.Base(m), ".jsonl"))
		a := s.agent(id)
		if a.Transcript == "" {
			a.Transcript = m
		}
		s.tail(m, id)
		if !a.hookStarted && a.Status != "done" {
			// without hooks: treat it as done if the file hasn't changed
			if fi, err := os.Stat(m); err == nil && now.Sub(fi.ModTime()) > agentIdleAfter {
				a.Status = "done"
				if a.End.IsZero() {
					a.End = a.LastSeen
				}
			}
		}
	}
	for _, a := range s.Agents {
		if a.Transcript != "" {
			s.tail(a.Transcript, a.ID)
		}
	}
}

// scanRecentTranscripts registers transcripts active within d (--recent mode, without hooks).
func (st *Store) scanRecentTranscripts(d time.Duration) {
	files, _ := filepath.Glob(filepath.Join(config.ClaudeDir(), "projects", "*", "*.jsonl"))
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil || time.Since(fi.ModTime()) > d {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		s := st.session(id)
		if s.Transcript == "" {
			s.Transcript = f
		}
		if s.Last.Before(fi.ModTime()) {
			s.Last = fi.ModTime()
		}
	}
}

func (s *Session) tail(path, agentID string) {
	t := s.tailers[path]
	if t == nil {
		t = &tailer{path: path}
		s.tailers[path] = t
	}
	t.read(func(line []byte) { s.ingestTranscript(agentID, line) })
}
