package ui

import (
	"fmt"

	"ccwatch/internal/format"
	"ccwatch/internal/store"

	"github.com/charmbracelet/lipgloss"
)

const (
	maxCurrentSkills = 6
	maxContextLead   = 5
	maxContextSub    = 3
)

// skillsBox: skill di prompt/task saat ini, skill lead yang masih di konteks, dan riwayat subagent.
func (m *model) skillsBox(s *store.Session, w int) string {
	lines, anyActive := currentSkillLines(s)
	ctxLead, ctxSub := s.ContextSkills()
	lines = append(lines, leadContextLines(ctxLead)...)
	lines = append(lines, subContextLines(ctxSub)...)

	title := sBold.Render("SKILLS")
	if n := len(s.PromptTimes); n > 0 {
		title += sDim.Render(fmt.Sprintf(" · prompt #%d", n))
	}
	border := cBorder
	if anyActive {
		border = pulse(cGreen)
	}
	return box(title, border, w, lines, 0)
}

// currentSkillLines: skill sejak prompt terakhir; anyActive true kalau ada yang masih berjalan.
func currentSkillLines(s *store.Session) (lines []string, anyActive bool) {
	cur := s.CurrentSkills()
	if len(cur) == 0 {
		lines = append(lines, sDim.Render("belum ada skill di prompt ini"))
	}
	start := max(0, len(cur)-maxCurrentSkills)
	if start > 0 {
		lines = append(lines, sDim.Render(fmt.Sprintf("… +%d sebelumnya", start)))
	}
	for _, u := range cur[start:] {
		mark := sDim.Render("✓ ")
		name := sText.Render(u.Name)
		sub := ""
		if u.Sub != "" {
			sub = " " + sText.Render(u.Sub)
		}
		if s.SkillActive(u) {
			anyActive = true
			mark = lipgloss.NewStyle().Foreground(pulse(modelColor(u.Model))).Render("● ")
			name = mcb(u.Model, u.Name)
			if u.Sub != "" {
				sub = " " + sBold.Render(u.Sub)
			}
		}
		who := u.Who
		if u.Source == "slash" {
			who = "user · slash"
		}
		lines = append(lines, mark+name+sub)
		if u.Args != "" {
			lines = append(lines, "  "+sDim.Render("“"+u.Args+"”"))
		}
		lines = append(lines, "  "+sDim.Render(format.Clock(u.T)+" · ")+mc(u.Model, who))
	}
	return lines, anyActive
}

// leadContextLines: skill lead dari prompt sebelumnya yang masih ada di konteks lead.
func leadContextLines(ctx []store.ContextSkill) []string {
	if len(ctx) == 0 {
		return nil
	}
	lines := []string{"", sDim.Render("masih di konteks (lead)")}
	for i, c := range ctx {
		if i == maxContextLead {
			lines = append(lines, sDim.Render(fmt.Sprintf("+%d lain", len(ctx)-maxContextLead)))
			break
		}
		txt := c.Name
		if sl := c.SubsLabel(); sl != "" {
			txt += " · " + sl
		} else if c.Count > 1 {
			txt += fmt.Sprintf(" ×%d", c.Count)
		}
		if c.LastPrompt > 0 {
			txt += fmt.Sprintf(" · #%d", c.LastPrompt)
		} else if c.FromCompact {
			txt += " · dari compact"
		}
		lines = append(lines, mc(c.Model, "◆ ")+sText.Render(txt))
	}
	return lines
}

// subContextLines: riwayat skill subagent dari prompt sebelumnya.
func subContextLines(ctx []store.ContextSkill) []string {
	if len(ctx) == 0 {
		return nil
	}
	lines := []string{"", sDim.Render("riwayat subagent")}
	for i, c := range ctx {
		if i == maxContextSub {
			lines = append(lines, sDim.Render(fmt.Sprintf("+%d lain", len(ctx)-maxContextSub)))
			break
		}
		txt := c.Name
		if sl := c.SubsLabel(); sl != "" {
			txt += " · " + sl
		}
		txt += " · " + c.Who
		if c.LastPrompt > 0 {
			txt += fmt.Sprintf(" · #%d", c.LastPrompt)
		}
		lines = append(lines, mc(c.Model, "◇ ")+sDim.Render(txt))
	}
	return lines
}
