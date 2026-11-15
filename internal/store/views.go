package store

import (
	"sort"
)

// Flow merges consecutive points with the same role+model into one step.
func (s *Session) Flow() []FlowStep {
	pts := make([]point, len(s.Points))
	copy(pts, s.Points)
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].T.Before(pts[j].T) })
	var out []FlowStep
	for _, p := range pts {
		if n := len(out); n > 0 && out[n-1].Role == p.Role && out[n-1].Model == p.Model && out[n-1].AgentID == p.AgentID {
			out[n-1].Count++
			out[n-1].End = p.T
			out[n-1].Tools += p.Tools
			continue
		}
		out = append(out, FlowStep{Start: p.T, End: p.T, Role: p.Role, AgentID: p.AgentID, Model: p.Model, Count: 1, Tools: p.Tools})
	}
	return out
}

// StatsOrdered: per-model stats, ordered by first use.
func (s *Session) StatsOrdered() []*ModelStat {
	var out []*ModelStat
	for _, m := range s.Stats {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].First.Before(out[j].First) })
	return out
}

// AgentsOrdered: running subagents first, then the most recently started.
func (s *Session) AgentsOrdered() []*Agent {
	var out []*Agent
	for _, id := range s.AgentOrder {
		out = append(out, s.Agents[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := out[i].Status == "running", out[j].Status == "running"
		if ri != rj {
			return ri
		}
		return out[i].Start.After(out[j].Start)
	})
	return out
}

// Running: the number of subagents currently running.
func (s *Session) Running() int {
	n := 0
	for _, a := range s.Agents {
		if a.Status == "running" {
			n++
		}
	}
	return n
}

// AdvisorUsage: total tokens across all advisor calls in this session.
func (s *Session) AdvisorUsage() Usage {
	var total Usage
	for _, c := range s.Advisor {
		total.add(c.Usage)
	}
	return total
}
