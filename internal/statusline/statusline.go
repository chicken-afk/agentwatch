// Package statusline menangani `ccwatch statusline`: mencetak status line
// ringkas untuk Claude Code dan mencatat pergantian model lead.
package statusline

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"ccwatch/internal/config"
	"ccwatch/internal/events"
	"ccwatch/internal/format"
	"ccwatch/internal/theme"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type input struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	Model          struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
}

func (in input) dir() string {
	if in.Workspace.CurrentDir != "" {
		return in.Workspace.CurrentDir
	}
	return in.Cwd
}

// Run dipanggil Claude Code untuk status line. Selain mencetak baris status,
// ini juga mencatat model lead saat berubah (mis. setelah /model), supaya
// dashboard tahu model aktif bahkan sebelum ada pesan baru.
func Run(stdin io.Reader, stdout io.Writer) {
	defer func() { _ = recover() }()
	// stdout status line berupa pipe → paksa warna tetap keluar
	lipgloss.SetColorProfile(termenv.TrueColor)

	raw, _ := io.ReadAll(io.LimitReader(stdin, 4<<20))
	var in input
	_ = json.Unmarshal(raw, &in)

	recordModelChange(in)
	fmt.Fprint(stdout, render(in))
}

// recordModelChange menulis event StatusLine bila model sesi berbeda dari catatan terakhir.
func recordModelChange(in input) {
	if in.SessionID == "" || in.Model.ID == "" {
		return
	}
	dir := config.DataDir()
	_ = os.MkdirAll(dir, 0o755)
	mark := filepath.Join(dir, "statusline-"+in.SessionID)
	if prev, _ := os.ReadFile(mark); string(prev) == in.Model.ID {
		return
	}
	_ = os.WriteFile(mark, []byte(in.Model.ID), 0o644)
	record, _ := json.Marshal(map[string]any{
		"session_id": in.SessionID, "transcript_path": in.TranscriptPath, "cwd": in.dir(),
		"hook_event_name": "StatusLine", "model": in.Model.ID,
		"ccwatch_ts": time.Now().UTC().Format(time.RFC3339Nano),
	})
	_ = events.Append(record)
}

func render(in input) string {
	cwd := in.dir()
	model := in.Model.ID
	if model == "" {
		model = in.Model.DisplayName
	}
	sep := theme.Dim.Render("  │  ")
	out := theme.ModelBold(model, "■ "+format.PrettyModel(model)) + theme.Dim.Render(" · lead")
	if adv := config.AdvisorModel(cwd); adv != "" {
		out += sep + theme.Model(adv, "◇ "+format.PrettyModel(adv)) + theme.Dim.Render(" · advisor")
	}
	out += sep + theme.Text.Render(format.BaseDir(cwd))
	if in.Cost.TotalCostUSD > 0 {
		out += theme.Dim.Render(fmt.Sprintf("  │  $%.2f", in.Cost.TotalCostUSD))
	}
	return out
}
