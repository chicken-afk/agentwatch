package ui

import (
	"fmt"
	"strings"

	"github.com/chicken-afk/agentwatch/internal/format"
	"github.com/chicken-afk/agentwatch/internal/store"

	"github.com/charmbracelet/lipgloss"
)

// Usage report: tokens and messages of all sessions, per agent and model, for each rolling
// period, plus the current session. Shared by the dashboard's usage tab and `agentwatch usage`.

const (
	usageNameW = 26
	usageCellW = 15
)

// UsageSession: the current session's usage (shown as the first column), or nil.
type UsageSession struct {
	Agent  string
	Models map[string]store.UsageTotals
}

func (s *UsageSession) total() store.UsageTotals {
	var t store.UsageTotals
	for _, m := range s.Models {
		t.Usage.In += m.In
		t.Usage.Out += m.Out
		t.Usage.CacheR += m.CacheR
		t.Usage.CacheW += m.CacheW
		t.Msgs += m.Msgs
	}
	return t
}

// usageCell: "2.96B · 10.5k" (tokens · messages), right-aligned in a column.
func usageCell(t store.UsageTotals, ok bool) string {
	if !ok || t.Msgs == 0 {
		return sDim.Render(fmt.Sprintf("%*s", usageCellW, "–"))
	}
	tok := format.Human(t.Tokens())
	msgs := format.Human(int64(t.Msgs))
	pad := usageCellW - len(tok) - 3 - len(msgs)
	return strings.Repeat(" ", max(0, pad)) + sText.Render(tok) + sDim.Render(" · "+msgs)
}

// UsageMatrix: rows of agents and their models × columns of periods (tokens · messages).
func UsageMatrix(rep store.UsageReport, sess *UsageSession) []string {
	head := fmt.Sprintf("%-*s", usageNameW, "AGENT / MODEL")
	if sess != nil {
		head += fmt.Sprintf("%*s", usageCellW, "THIS SESSION")
	}
	for _, p := range store.UsagePeriods {
		head += fmt.Sprintf("%*s", usageCellW, strings.ToUpper(p.Label))
	}
	lines := []string{sDim.Render(head)}
	if len(rep.Agents) == 0 {
		return append(lines, sDim.Render("no usage in the last 30 days"))
	}
	for _, a := range rep.Agents {
		row := lipgloss.NewStyle().Width(usageNameW).Render(agentBadge(a.Agent))
		if sess != nil {
			row += usageCell(sess.total(), sess.Agent == a.Agent)
		}
		for _, p := range a.Periods {
			row += usageCell(p, true)
		}
		lines = append(lines, row)
		for _, m := range a.Models {
			name := "  " + mc(m.Model, "● "+format.Clip(format.PrettyModel(m.Model), usageNameW-5))
			row := lipgloss.NewStyle().Width(usageNameW).Render(name)
			if sess != nil {
				st, ok := sess.Models[m.Model]
				row += usageCell(st, ok && sess.Agent == a.Agent)
			}
			for _, p := range m.Periods {
				row += usageCell(p, true)
			}
			lines = append(lines, row)
		}
	}
	total := lipgloss.NewStyle().Width(usageNameW).Render(sBold.Render("all agents"))
	if sess != nil {
		total += usageCell(sess.total(), true)
	}
	for _, p := range rep.Total {
		total += usageCell(p, true)
	}
	return append(lines, "", total)
}

// UsageDetail: for one period, each agent/model split into input, output, cache and messages,
// with its share of the period's tokens.
func UsageDetail(rep store.UsageReport, period int) []string {
	cols := fmt.Sprintf("%-*s%10s%10s%13s%13s%9s  %s", usageNameW, "AGENT / MODEL", "INPUT", "OUTPUT", "CACHE READ", "CACHE WRITE", "MSGS", "SHARE OF TOKENS")
	lines := []string{sDim.Render(cols)}
	all := rep.Total[period].Tokens()
	line := func(name string, t store.UsageTotals, c lipgloss.Color) string {
		frac := 0.0
		if all > 0 {
			frac = float64(t.Tokens()) / float64(all)
		}
		return lipgloss.NewStyle().Width(usageNameW).Render(name) +
			sText.Render(fmt.Sprintf("%10s%10s%13s%13s%9s", format.Human(t.In), format.Human(t.Out),
				format.Human(t.CacheR), format.Human(t.CacheW), format.Human(int64(t.Msgs)))) +
			"  " + bar(frac, 12, c) + sDim.Render(fmt.Sprintf(" %3.0f%%", frac*100))
	}
	any := false
	for _, a := range rep.Agents {
		if a.Periods[period].Msgs == 0 {
			continue
		}
		any = true
		lines = append(lines, line(agentBadge(a.Agent), a.Periods[period], agentColor(a.Agent)))
		for _, m := range a.Models {
			if m.Periods[period].Msgs == 0 {
				continue
			}
			lines = append(lines, line("  "+mc(m.Model, "● "+format.Clip(format.PrettyModel(m.Model), usageNameW-5)), m.Periods[period], modelColor(m.Model)))
		}
	}
	if !any {
		lines = append(lines, sDim.Render("no usage in this period"))
	}
	return lines
}

// UsageText: the full report as plain lines (for `agentwatch usage`).
func UsageText(rep store.UsageReport, period int) string {
	var b strings.Builder
	b.WriteString(sBold.Render("agentwatch usage") + sDim.Render(" · all sessions, all folders · tokens · messages") + "\n\n")
	for _, l := range UsageMatrix(rep, nil) {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n" + sBold.Render(strings.ToUpper(store.UsagePeriods[period].Label)) + sDim.Render(" · input, output and cache per model") + "\n")
	for _, l := range UsageDetail(rep, period) {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n" + sDim.Render("Tokens include cache reads and writes. Cache reads usually dominate and are billed much lower than new input."))
	return b.String()
}

// usageView: tab 5, usage of all sessions plus the current one.
func (m *model) usageView(s *store.Session, w, h int) string {
	title := sBold.Render("USAGE") + sDim.Render(" · all sessions, all folders · tokens · messages")
	if m.usage == nil {
		return box(title, cBorder, w, []string{sDim.Render(m.spin() + " reading the last 30 days of sessions…")}, h-2)
	}
	title += sDim.Render(" · updated " + format.Clock(m.usageAt))
	if m.usageScanning {
		title += sDim.Render(" " + m.spin())
	}
	sess := &UsageSession{Agent: s.Agent, Models: s.SessionUsage()}
	if sess.Agent == "" {
		sess.Agent = store.AgentClaude
	}
	lines := UsageMatrix(*m.usage, sess)
	p := m.usagePeriod
	lines = append(lines, "", "",
		sBold.Render(strings.ToUpper(store.UsagePeriods[p].Label))+sDim.Render(" · input, output and cache per model · t to change period"))
	lines = append(lines, UsageDetail(*m.usage, p)...)
	lines = append(lines, "", sDim.Render("This session = the session shown in the header ("+format.Short(s.ID)+"). Tokens include cache reads and writes."))
	view, sc := scrollWindow(lines, h-2, m.scroll)
	if len(lines) <= h-2 {
		view, sc = lines, 0
	}
	m.scroll = sc
	return box(title, cBorder, w, view, h-2)
}
