package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ccwatch/internal/config"
	"ccwatch/internal/format"
	"ccwatch/internal/store"

	"github.com/charmbracelet/lipgloss"
)

var tabNames = [tabCount]string{"1 dashboard", "2 model sequence", "3 log", "4 subagents"}

func (m *model) View() string {
	w, h := m.w, m.h
	if w == 0 {
		w, h = 120, 40
	}
	s, idx, n := m.current()
	if s == nil {
		return m.emptyView(w, h)
	}
	head := m.header(s, idx, n, w)
	foot := m.footer(w)
	bodyH := h - lipgloss.Height(head) - 1

	var body string
	switch m.tab {
	case 0:
		body = m.dashboard(s, w, bodyH)
	case 1:
		body = m.flowView(s, w, bodyH)
	case 2:
		body = m.logView(s, w, bodyH)
	case 3:
		body = m.agentsView(s, w, bodyH)
	}
	lines := strings.Split(body, "\n")
	if len(lines) > bodyH {
		lines = lines[:bodyH]
	}
	for len(lines) < bodyH {
		lines = append(lines, "")
	}
	return head + "\n" + strings.Join(lines, "\n") + "\n" + foot
}

func (m *model) emptyView(w, h int) string {
	lines := []string{
		sBold.Render("CCWATCH") + sDim.Render(" · live dashboard for AI coding agents"),
		"",
		sText.Render("No agent sessions found yet."),
		sDim.Render("Watching: Claude Code (hooks), Codex, Gemini CLI, OpenCode, omp (session files)."),
		"",
	}
	if d := m.st.Options().Dir; d != "" {
		lines = append(lines,
			sDim.Render("Only sessions in folder: ")+sText.Render(d),
			sDim.Render("Show all folders:        ")+"ccwatch --all",
			"",
		)
	}
	lines = append(lines,
		sDim.Render("Claude Code: ")+"ccwatch install"+sDim.Render(", then start ")+"claude",
		sDim.Render("Codex / Gemini / OpenCode / omp: just start them in this folder."),
		sDim.Render("Any other tool: pipe events to ")+"ccwatch hook --agent <name>",
		"",
		sDim.Render("Without hooks: ")+"ccwatch --recent 30m"+sDim.Render("  (reads recently active transcripts)"),
		sDim.Render("Try the demo:  ")+"ccwatch demo",
		"",
		sDim.Render("events: "+config.EventsPath()),
	)
	b := box("", cBorder, min(w, 76), lines, 0)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, b)
}

func (m *model) statusBadge(s *store.Session) string {
	switch {
	case s.Status == "ended":
		return sDim.Render("■ ended")
	case s.Status == "working" || s.Running() > 0:
		return sGreen.Render(m.spin() + " working")
	case !s.HasHooks && s.IsClaude():
		return sYel.Render("○ transcript-only")
	default:
		return sDim.Render("○ idle")
	}
}

func (m *model) header(s *store.Session, idx, n, w int) string {
	el := ""
	if !s.Start.IsZero() {
		end := time.Now()
		if s.Status == "ended" {
			end = s.Last
		}
		el = format.Dur(end.Sub(s.Start))
	}
	left := sBold.Render("CCWATCH") + sDim.Render("  ·  ") + agentBadge(s.Agent) + sDim.Render(" · ") +
		sText.Render("session "+format.Short(s.ID)) + sDim.Render(" · ") + sText.Render(format.BaseDir(s.Cwd)) +
		sDim.Render(" · ") + sText.Render(el) + "   " + m.statusBadge(s)
	follow := "follow"
	if m.pinned != "" {
		follow = "pinned"
	}
	scope := "all folders"
	if d := m.st.Options().Dir; d != "" {
		scope = "folder " + format.BaseDir(d)
	}
	if a := m.st.Options().Agents; len(a) > 0 {
		scope += " · " + strings.Join(a, ",")
	}
	right := sDim.Render(fmt.Sprintf("session %d/%d · %s · %s", idx+1, n, scope, follow))
	line1 := left
	if gap := w - lipgloss.Width(left) - lipgloss.Width(right); gap > 1 {
		line1 += strings.Repeat(" ", gap) + right
	}

	var tabs []string
	for i, t := range tabNames {
		if i == m.tab {
			tabs = append(tabs, lipgloss.NewStyle().Foreground(lipgloss.Color("#18181B")).Background(cText).Render(" "+t+" "))
		} else {
			tabs = append(tabs, sDim.Render(" "+t+" "))
		}
	}
	// model legend
	var legend []string
	for _, st := range s.StatsOrdered() {
		legend = append(legend, mc(st.Model, "■ ")+sText.Render(format.PrettyModel(st.Model))+sDim.Render(" · "+rolesSummary(st)))
	}
	line2 := strings.Join(tabs, "")
	if len(legend) > 0 {
		line2 += "   " + strings.Join(legend, "   ")
	}
	rule := lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", w))
	return fit(line1, w) + "\n" + fit(line2, w) + "\n" + rule
}

// rolesSummary: the top three roles using model st (by message count); the rest as "+N".
func rolesSummary(st *store.ModelStat) string {
	type roleCount struct {
		role  string
		count int
	}
	var rs []roleCount
	for r, c := range st.Roles {
		rs = append(rs, roleCount{r, c})
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].count != rs[j].count {
			return rs[i].count > rs[j].count
		}
		return rs[i].role < rs[j].role // stable: map iteration order is random
	})
	var names []string
	for i, r := range rs {
		if i == 3 {
			names = append(names, fmt.Sprintf("+%d", len(rs)-3))
			break
		}
		names = append(names, r.role)
	}
	return strings.Join(names, ", ")
}

func (m *model) footer(w int) string {
	keys := "1-4/tab view · [ ] switch session · f follow · j/k scroll · q quit"
	return sDim.Render(fit(keys, w))
}
