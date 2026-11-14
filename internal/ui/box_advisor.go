package ui

import (
	"fmt"
	"strconv"
	"time"

	"ccwatch/internal/format"
	"ccwatch/internal/store"
)

const advisorActiveWindow = 8 * time.Second // setelah panggilan terakhir, advisor dianggap masih "memberi saran"

// advisorBox: kotak ADVISOR. h > 0 memaksa tinggi total kotak (riwayat mengisi sisanya).
func (m *model) advisorBox(s *store.Session, w, h int) string {
	if s.AdvisorCfg == "" && len(s.Advisor) == 0 {
		lines := []string{
			sDim.Render("tidak terdeteksi"),
			"",
			sDim.Render("aktifkan dengan:"),
			sText.Render("claude --advisor opus"),
			sDim.Render("atau label manual:"),
			sText.Render("ccwatch --advisor opus"),
		}
		return box(sBold.Render("ADVISOR"), cBorder, w, lines, max(0, h-2))
	}

	model := s.AdvisorLabel()
	if n := len(s.Advisor); n > 0 {
		model = s.Advisor[n-1].Model
	}
	state := sDim.Render("◇ ready")
	active := false
	if n := len(s.Advisor); n > 0 {
		if time.Since(s.Advisor[n-1].T) < advisorActiveWindow {
			active = true
			state = mc(model, m.spin()+" memberi saran")
		} else {
			state = sDim.Render("◇ ready · terakhir " + format.Clock(s.Advisor[n-1].T))
		}
	}
	total := s.AdvisorUsage()
	lines := []string{
		kv("model", mcb(model, format.PrettyModel(model))),
		kv("status", state),
		kv("calls", sText.Render(strconv.Itoa(len(s.Advisor)))),
	}
	if total.In+total.Out > 0 {
		lines = append(lines, kv("token", fmt.Sprintf("%s in · %s out", format.Human(total.InputTotal()), format.Human(total.Out))))
	}
	lines = append(lines, "", sDim.Render("riwayat panggilan"))
	room := 30
	if h > 0 {
		room = h - 2 - len(lines)
	}
	for i := len(s.Advisor) - 1; i >= max(0, len(s.Advisor)-room); i-- {
		lines = append(lines, advisorHistoryLine(s.Advisor[i]))
	}
	if len(s.Advisor) == 0 {
		lines = append(lines, sDim.Render("belum dipanggil"))
	}
	title := sBold.Render("ADVISOR") + sDim.Render(" · ") + mc(model, format.PrettyModel(model))
	border := modelColor(model)
	if active {
		border = pulse(border)
	}
	return box(title, border, w, lines, max(0, h-2))
}

func advisorHistoryLine(c store.AdvisorCall) string {
	from := "lead"
	if c.FromSub != "" {
		from = c.FromSub
	}
	ctx := from
	if c.After != "" {
		ctx += " · " + c.After
	}
	return mc(c.Model, "◇ ") + sDim.Render(format.Clock(c.T)) + " " + sText.Render(ctx)
}
