package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chicken-afk/agentwatch/internal/store"
	"github.com/chicken-afk/agentwatch/internal/ui"
)

// periodIndex: "1h|hour", "24h|day", "7d|week", "30d|month" → index into store.UsagePeriods.
func periodIndex(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1h", "hour":
		return 0, nil
	case "24h", "1d", "day":
		return 1, nil
	case "7d", "week":
		return 2, nil
	case "30d", "month":
		return 3, nil
	}
	return 0, fmt.Errorf("unknown period %q (use 1h, 24h, 7d or 30d)", s)
}

// runUsage: `agentwatch usage [--period 24h] [--agent claude,codex] [--json]`.
func runUsage(args []string) error {
	fs := flag.NewFlagSet("usage", flag.ExitOnError)
	period := fs.String("period", "24h", "period for the input/output/cache breakdown: 1h, 24h, 7d or 30d")
	var agents listFlag
	fs.Var(&agents, "agent", "only these agents (claude, codex, gemini, antigravity, opencode, omp, …; comma-separated)")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := periodIndex(*period)
	if err != nil {
		return err
	}

	now := time.Now()
	h := store.NewHistory()
	h.Scan(now)
	rep := h.Report(now, agents)
	if *asJSON {
		return writeUsageJSON(rep)
	}
	fmt.Println(ui.UsageText(rep, p))
	return nil
}

type jsonTotals struct {
	Tokens     int64 `json:"tokens"`
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Messages   int   `json:"messages"`
}

func toJSONPeriods(ps []store.UsageTotals) map[string]jsonTotals {
	keys := []string{"1h", "24h", "7d", "30d"}
	out := map[string]jsonTotals{}
	for i, t := range ps {
		out[keys[i]] = jsonTotals{t.Tokens(), t.In, t.Out, t.CacheR, t.CacheW, t.Msgs}
	}
	return out
}

func writeUsageJSON(rep store.UsageReport) error {
	type jm struct {
		Model   string                `json:"model"`
		Periods map[string]jsonTotals `json:"periods"`
	}
	type ja struct {
		Agent   string                `json:"agent"`
		Periods map[string]jsonTotals `json:"periods"`
		Models  []jm                  `json:"models"`
	}
	out := struct {
		GeneratedAt time.Time             `json:"generated_at"`
		Total       map[string]jsonTotals `json:"total"`
		Agents      []ja                  `json:"agents"`
	}{GeneratedAt: rep.At, Total: toJSONPeriods(rep.Total), Agents: []ja{}}
	for _, a := range rep.Agents {
		x := ja{Agent: a.Agent, Periods: toJSONPeriods(a.Periods)}
		for _, m := range a.Models {
			x.Models = append(x.Models, jm{Model: m.Model, Periods: toJSONPeriods(m.Periods)})
		}
		out.Agents = append(out.Agents, x)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
