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

var tabNames = [tabCount]string{"1 dashboard", "2 urutan model", "3 log", "4 subagent"}

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
		sBold.Render("CLAUDE CODE WATCH"),
		"",
		sText.Render("Belum ada sesi Claude Code yang terlihat."),
		"",
	}
	if d := m.st.Options().Dir; d != "" {
		lines = append(lines,
			sDim.Render("Hanya sesi di folder: ")+sText.Render(d),
			sDim.Render("Lihat semua folder:   ")+"ccwatch --all",
			"",
		)
	}
	lines = append(lines,
		sDim.Render("1. pasang hooks:      ")+"ccwatch install",
		sDim.Render("2. buka sesi baru:    ")+"claude --model sonnet --advisor opus",
		sDim.Render("3. dashboard ini akan ikut otomatis."),
		"",
		sDim.Render("Tanpa hooks: ")+"ccwatch --recent 30m"+sDim.Render("  (baca transcript yang baru aktif)"),
		sDim.Render("Coba tampilan: ")+"ccwatch demo",
		"",
		sDim.Render("events: "+config.EventsPath()),
	)
	b := box("", cBorder, min(w, 76), lines, 0)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, b)
}

func (m *model) statusBadge(s *store.Session) string {
	switch {
	case s.Status == "ended":
		return sDim.Render("■ selesai")
	case s.Status == "working" || s.Running() > 0:
		return sGreen.Render(m.spin() + " working")
	case !s.HasHooks:
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
	left := sBold.Render("CLAUDE CODE WATCH") + sDim.Render("  ·  ") +
		sText.Render("sesi "+format.Short(s.ID)) + sDim.Render(" · ") + sText.Render(format.BaseDir(s.Cwd)) +
		sDim.Render(" · ") + sText.Render(el) + "   " + m.statusBadge(s)
	follow := "follow"
	if m.pinned != "" {
		follow = "pinned"
	}
	scope := "semua folder"
	if d := m.st.Options().Dir; d != "" {
		scope = "folder " + format.BaseDir(d)
	}
	right := sDim.Render(fmt.Sprintf("sesi %d/%d · %s · %s", idx+1, n, scope, follow))
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
	// legenda model
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

// rolesSummary: tiga peran teratas pemakai model st (urut jumlah pesan), sisanya "+N".
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
		return rs[i].role < rs[j].role // stabil: iterasi map acak
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
	keys := "1-4/tab tampilan · [ ] ganti sesi · f follow · j/k scroll · q keluar"
	return sDim.Render(fit(keys, w))
}
