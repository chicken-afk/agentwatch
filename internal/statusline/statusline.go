// Package statusline handles `agentwatch statusline`: it prints a compact
// status line for Claude Code and records lead model changes.
package statusline

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/chicken-afk/agentwatch/internal/config"
	"github.com/chicken-afk/agentwatch/internal/events"
	"github.com/chicken-afk/agentwatch/internal/format"
	"github.com/chicken-afk/agentwatch/internal/theme"

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

// Run is called by Claude Code for the status line. Besides printing the status line,
// it also records the lead model when it changes (e.g. after /model), so the
// dashboard knows the active model even before a new message arrives.
func Run(stdin io.Reader, stdout io.Writer) {
	defer func() { _ = recover() }()
	// status line stdout is a pipe → force colors on anyway
	lipgloss.SetColorProfile(termenv.TrueColor)

	raw, _ := io.ReadAll(io.LimitReader(stdin, 4<<20))
	var in input
	_ = json.Unmarshal(raw, &in)

	recordModelChange(in)
	fmt.Fprint(stdout, render(in))
}

// recordModelChange writes a StatusLine event when the session's model differs from the last record.
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
		"agentwatch_ts": time.Now().UTC().Format(time.RFC3339Nano),
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
