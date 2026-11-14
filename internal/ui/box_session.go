package ui

import (
	"fmt"
	"strconv"

	"ccwatch/internal/format"
	"ccwatch/internal/store"
)

func (m *model) leadBox(s *store.Session, w int) string {
	status := m.statusBadge(s)
	if s.CurrentTool != "" {
		status += sDim.Render("  ⟶ ") + sText.Render(s.CurrentTool)
	}
	prompt := sDim.Render("—")
	if s.LastPrompt != "" {
		prompt = sText.Render("“" + s.LastPrompt + "”")
	}
	u := s.MainUsage
	lines := []string{
		kv("status", status),
		kv("prompt", sDim.Render(fmt.Sprintf("#%d ", s.Prompts))+prompt),
		kv("aktivit", fmt.Sprintf("%s pesan · %s tool · %s turn · %s subagent",
			sText.Render(strconv.Itoa(s.MainMsgs)), sText.Render(strconv.Itoa(s.MainTools)), sText.Render(strconv.Itoa(s.Turns)),
			sText.Render(strconv.Itoa(len(s.Agents))))+failSuffix(s.Failures)),
		kv("token", fmt.Sprintf("in %s · out %s · cache read %s · write %s",
			format.Human(u.In), format.Human(u.Out), format.Human(u.CacheR), format.Human(u.CacheW))),
	}
	if s.PermMode != "" || s.Compacts > 0 {
		lines = append(lines, kv("mode", sDim.Render(s.PermMode)+sDim.Render(fmt.Sprintf(" · compact %d", s.Compacts))))
	}
	title := sBold.Render("LEAD") + sDim.Render(" · ") + mcb(s.Lead, format.PrettyModel(s.Lead)) + sDim.Render(" · main session")
	border := modelColor(s.Lead)
	if s.Status == "working" {
		border = pulse(border)
	}
	return box(title, border, w, lines, 0)
}

func failSuffix(n int) string {
	if n == 0 {
		return ""
	}
	return sRed.Render(fmt.Sprintf(" · %d gagal", n))
}

func (m *model) usageBox(s *store.Session, w int) string {
	stats := s.StatsOrdered()
	var totalOut int64
	for _, st := range stats {
		totalOut += st.Usage.Out
	}
	lines := []string{sDim.Render(fmt.Sprintf("%-13s %-24s %6s %8s %8s %9s %8s  %-16s %s",
		"MODEL", "PERAN", "PESAN", "IN", "OUT", "CACHE R", "CACHE W", "PORSI OUTPUT", "AKTIF"))}
	for _, st := range stats {
		frac := 0.0
		if totalOut > 0 {
			frac = float64(st.Usage.Out) / float64(totalOut)
		}
		lines = append(lines,
			mc(st.Model, fmt.Sprintf("%-13s", "● "+format.PrettyModel(st.Model)))+" "+
				sText.Render(fmt.Sprintf("%-24s %6d %8s %8s %9s %8s", format.Clip(rolesSummary(st), 24), st.Msgs,
					format.Human(st.Usage.In), format.Human(st.Usage.Out), format.Human(st.Usage.CacheR), format.Human(st.Usage.CacheW)))+"  "+
				bar(frac, 10, modelColor(st.Model))+sDim.Render(fmt.Sprintf(" %3.0f%%  ", frac*100))+
				sDim.Render(format.Clock(st.First)+"–"+format.Clock(st.Last)))
	}
	if len(stats) == 0 {
		lines = append(lines, sDim.Render("belum ada data model"))
	}
	return box(sBold.Render("PEMAKAIAN MODEL"), cBorder, w, lines, 0)
}

func logLine(e store.LogEntry, w int) string {
	var who string
	switch e.Who {
	case "sys":
		who = sDim.Render(fmt.Sprintf("%-12s", "·"))
	case "user":
		who = sYel.Render(fmt.Sprintf("%-12s", "you"))
	default:
		who = mc(e.Mdl, fmt.Sprintf("%-12s", format.Clip(e.Who, 12)))
	}
	txt := sText.Render(e.Text)
	if e.Bad {
		txt = sRed.Render("✗ " + e.Text)
	}
	return fit(sDim.Render(format.Clock(e.T))+"  "+who+" "+txt, w)
}

func (m *model) logBox(s *store.Session, w, rows int) string {
	var lines []string
	start := max(0, len(s.Log)-rows)
	for _, e := range s.Log[start:] {
		lines = append(lines, logLine(e, w-4))
	}
	if len(s.Log) == 0 {
		if s.HasHooks {
			lines = []string{sDim.Render("menunggu event…")}
		} else {
			lines = []string{sYel.Render("hooks belum terpasang untuk sesi ini — jalankan `ccwatch install` untuk log live")}
		}
	}
	return box(sBold.Render("SESSION LOG"), cBorder, w, lines, rows)
}

// logView: tab 3, semua event sesi.
func (m *model) logView(s *store.Session, w, h int) string {
	var lines []string
	for _, e := range s.Log {
		lines = append(lines, logLine(e, w-4))
	}
	view, sc := scrollWindow(lines, h-2, m.scroll)
	m.scroll = sc
	title := sBold.Render("SESSION LOG") + sDim.Render(fmt.Sprintf(" · %d event · j/k scroll", len(s.Log)))
	return box(title, cBorder, w, view, h-2)
}
