package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ccwatch/internal/config"
)

// AgentDef: definisi subagent dari .claude/agents/*.md (frontmatter).
type AgentDef struct {
	Name, Model, Desc string
	Source            string // project | user
	Path              string
}

// ModelLabel: label model untuk definisi agent ("inherit"/kosong = ikut lead).
func (d AgentDef) ModelLabel() string {
	if d.Model == "" || d.Model == "inherit" {
		return "inherit"
	}
	return d.Model
}

// readAgentDefs: subagent yang terkonfigurasi untuk sesi di cwd.
// Project (.claude/agents terdekat dari cwd ke atas) menimpa user (~/.claude/agents)
// kalau namanya sama, sama seperti Claude Code.
func readAgentDefs(cwd string) []AgentDef {
	byName := map[string]AgentDef{}
	for _, d := range loadAgentDir(filepath.Join(config.ClaudeDir(), "agents"), "user") {
		byName[d.Name] = d
	}
	if dir := projectAgentsDir(cwd); dir != "" {
		for _, d := range loadAgentDir(dir, "project") {
			byName[d.Name] = d
		}
	}
	out := make([]AgentDef, 0, len(byName))
	for _, d := range byName {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// projectAgentsDir: cari <dir>/.claude/agents dari cwd naik ke atas (berhenti sebelum home).
func projectAgentsDir(cwd string) string {
	if cwd == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	userDir := filepath.Clean(filepath.Join(config.ClaudeDir(), "agents"))
	p := filepath.Clean(cwd)
	for {
		if p == home {
			return ""
		}
		d := filepath.Join(p, ".claude", "agents")
		if d != userDir {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				return d
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
}

func loadAgentDir(dir, source string) []AgentDef {
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	var out []AgentDef
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		fm := frontmatter(string(b))
		name := fm["name"]
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(f), ".md")
		}
		out = append(out, AgentDef{Name: name, Model: fm["model"], Desc: fm["description"], Source: source, Path: f})
	}
	return out
}

// frontmatter: parse sederhana "key: value" di antara baris --- pertama dan kedua.
func frontmatter(s string) map[string]string {
	m := map[string]string{}
	s = strings.TrimPrefix(s, "\uFEFF")
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return m
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if l == "" || l[0] == ' ' || l[0] == '\t' || l[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		m[strings.TrimSpace(k)] = v
	}
	return m
}

// IdleDefs: subagent terkonfigurasi yang belum pernah dipanggil di sesi ini.
func (s *Session) IdleDefs() []AgentDef {
	used := map[string]bool{}
	for _, a := range s.Agents {
		used[a.Type] = true
	}
	var out []AgentDef
	for _, d := range s.AgentDefs {
		if !used[d.Name] {
			out = append(out, d)
		}
	}
	return out
}
