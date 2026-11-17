package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"ccwatch/internal/format"
	"ccwatch/internal/store"

	"github.com/charmbracelet/lipgloss"
)

const (
	agentCardWidth = 30
	agentCardRows  = 2 // max subagent card rows on the dashboard
)

// agentEnd: end time used for a subagent's duration (now if it's still running).
func agentEnd(a *store.Agent) time.Time {
	if a.Status != "done" {
		return time.Now()
	}
	if a.End.IsZero() {
		return a.LastSeen
	}
	return a.End
}

func agentElapsed(a *store.Agent) string {
	if a.Start.IsZero() {
		return ""
	}
	return format.Dur(agentEnd(a).Sub(a.Start))
}

func (m *model) agentCard(a *store.Agent, w int) string {
	mdl := a.Model()
	status := sGreen.Render(m.spin() + " running ")
	border := pulse(modelColor(mdl))
	if a.Status == "done" {
		status = sDim.Render("✓ done ")
		border = cBorder
	}

	modelLabel := mc(mdl, format.PrettyModel(mdl))
	if len(a.Models) > 1 {
		var ps []string
		for _, x := range a.Models {
			ps = append(ps, mc(x, format.PrettyModel(x)))
		}
		modelLabel = strings.Join(ps, sDim.Render("→"))
	}
	lines := []string{
		modelLabel,
		status + sDim.Render(agentElapsed(a)),
		sDim.Render("tools ") + sText.Render(strconv.Itoa(a.Tools)) + sDim.Render(" · ") + sText.Render(a.LastTool),
		sDim.Render("tok ") + sText.Render(format.Human(a.Usage.InputTotal())) + sDim.Render("↑ ") + sText.Render(format.Human(a.Usage.Out)) + sDim.Render("↓"),
	}
	if a.Desc != "" {
		lines = append(lines, sDim.Render(a.Desc))
	}
	return box(mcb(mdl, a.Title()), border, w, lines, 5)
}

func (m *model) agentGrid(s *store.Session, w int) string {
	ag := s.AgentsOrdered()
	idle := s.IdleDefs()
	if len(ag) == 0 {
		lines := []string{sDim.Render("no subagents called in this session yet")}
		if len(idle) > 0 {
			lines = append(lines, "", sDim.Render("configured (.claude/agents):"))
			for _, d := range idle {
				lines = append(lines, idleDefLine(d, w-4))
			}
		}
		return box(sBold.Render("SUBAGENTS"), cBorder, w, lines, 0)
	}

	cols := max(1, (w+1)/(agentCardWidth+1))
	cardW := (w - (cols - 1)) / cols
	shown := min(len(ag), cols*agentCardRows)
	var rows, row []string
	for _, a := range ag[:shown] {
		row = append(row, m.agentCard(a, cardW))
		if len(row) == cols {
			rows = append(rows, joinGap(row))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, joinGap(row))
	}
	title := sDim.Render(fmt.Sprintf(" subagent %d running · %d total", s.Running(), len(ag)))
	if len(ag) > shown {
		title += sDim.Render(fmt.Sprintf(" · +%d more (tab 4)", len(ag)-shown))
	}
	out := title + "\n" + strings.Join(rows, "\n")
	if len(idle) > 0 {
		var ps []string
		for _, d := range idle {
			ps = append(ps, mc(d.ModelLabel(), "◇ "+d.Name)+sDim.Render(" ("+format.PrettyModel(d.ModelLabel())+")"))
		}
		out += "\n" + fit(sDim.Render(" ready, not called yet: ")+strings.Join(ps, sDim.Render(" · ")), w)
	}
	return out
}

// idleDefLine: one line for a configured subagent that hasn't been called yet.
func idleDefLine(d store.AgentDef, w int) string {
	mdl := d.ModelLabel()
	head := mc(mdl, "◇ ") + mcb(mdl, d.Name) + sDim.Render(" · ") + mc(mdl, format.PrettyModel(mdl)) +
		sDim.Render(" · "+d.Source+" · ready")
	if d.Desc == "" {
		return head
	}
	return head + sDim.Render(" — "+format.Clip(d.Desc, max(10, w-lipgloss.Width(head)-3)))
}

// agentsView: tab 4, a table of all subagents plus configured ones not called yet.
func (m *model) agentsView(s *store.Session, w, h int) string {
	hdr := sDim.Render(fmt.Sprintf("%-3s %-18s %-22s %-9s %8s %6s %6s %8s %7s  %s",
		"#", "NAME / TYPE", "MODEL", "STATUS", "DURATION", "TOOL", "MSGS", "IN", "OUT", "TASK"))
	var lines []string
	ag := s.AgentsOrdered()
	sort.SliceStable(ag, func(i, j int) bool { return ag[i].Start.Before(ag[j].Start) })
	for i, a := range ag {
		lines = append(lines, agentRow(i+1, a))
	}
	idle := s.IdleDefs()
	for i, d := range idle {
		lines = append(lines, idleDefRow(len(ag)+i+1, d))
	}
	if len(lines) == 0 {
		lines = []string{sDim.Render("no subagents yet")}
		if s.IsClaude() {
			lines = []string{sDim.Render("no subagents yet (and none in .claude/agents)")}
		}
	}
	view, sc := scrollWindow(lines, h-3, m.scroll)
	m.scroll = sc
	title := sBold.Render("ALL SUBAGENTS") + sDim.Render(fmt.Sprintf(" · %d running · %d total · %d ready", s.Running(), len(ag), len(idle)))
	return box(title, cBorder, w, append([]string{hdr}, view...), h-2)
}

func agentRow(n int, a *store.Agent) string {
	var models []string
	for _, x := range a.Models {
		models = append(models, format.PrettyModel(x))
	}
	if len(models) == 0 && a.ReqModel != "" {
		models = []string{a.ReqModel + "?"}
	}
	status := sGreen.Render(fmt.Sprintf("%-9s", "running"))
	if a.Status == "done" {
		status = sDim.Render(fmt.Sprintf("%-9s", "done"))
	}
	return sDim.Render(fmt.Sprintf("%-3d ", n)) +
		mcb(a.Model(), fmt.Sprintf("%-18s", format.Clip(a.Title(), 18))) + " " +
		mc(a.Model(), fmt.Sprintf("%-22s", format.Clip(strings.Join(models, "→"), 22))) + " " + status +
		sText.Render(fmt.Sprintf(" %8s %6d %6d %8s %7s  ", agentElapsed(a), a.Tools, a.Msgs,
			format.Human(a.Usage.InputTotal()), format.Human(a.Usage.Out))) + sDim.Render(a.Desc)
}

func idleDefRow(n int, d store.AgentDef) string {
	mdl := d.ModelLabel()
	return sDim.Render(fmt.Sprintf("%-3d ", n)) +
		mc(mdl, fmt.Sprintf("%-18s", format.Clip(d.Name, 18))) + " " +
		mc(mdl, fmt.Sprintf("%-22s", format.Clip(format.PrettyModel(mdl), 22))) + " " +
		sDim.Render(fmt.Sprintf("%-9s", "ready")) +
		sDim.Render(fmt.Sprintf(" %8s %6s %6s %8s %7s  ", "-", "-", "-", "-", "-")) +
		sDim.Render("["+d.Source+"] "+d.Desc)
}
