package store

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ccwatch/internal/format"
)

// SkillUse: satu pemakaian skill, entah dipanggil Claude lewat tool Skill
// atau diketik user sebagai slash command (/nama).
type SkillUse struct {
	T       time.Time
	Name    string
	Sub     string // subcommand: kata pertama args (mis. "layout" di /impeccable layout …)
	Args    string // sisa args setelah subcommand
	Source  string // tool | slash
	AgentID string // "" = lead
	Who     string // lead | tipe subagent | user
	Model   string
	ID      string // tool_use_id (untuk dedupe hook vs transcript)
}

// fill: lengkapi data yang kosong dari sumber lain (hook lama tanpa args, dsb.).
func (c *SkillUse) fill(u SkillUse) {
	if c.Model == "" {
		c.Model = u.Model
	}
	if c.Sub == "" && c.Args == "" {
		c.Sub, c.Args = u.Sub, u.Args
	}
}

// slash command bawaan Claude Code: bukan skill, jangan dicatat.
var builtinSlash = map[string]bool{
	"clear": true, "compact": true, "model": true, "help": true, "config": true, "cost": true,
	"exit": true, "quit": true, "resume": true, "login": true, "logout": true, "mcp": true,
	"permissions": true, "hooks": true, "agents": true, "memory": true, "status": true,
	"doctor": true, "reload-skills": true, "reload-plugins": true, "plugin": true, "plugins": true,
	"context": true, "usage": true, "add-dir": true, "vim": true, "theme": true, "terminal-setup": true,
	"bug": true, "rewind": true, "export": true, "ide": true, "fast": true, "effort": true,
}

// slashSkill: "/nama args" → nama, args. ok=false kalau bukan slash command skill.
func slashSkill(prompt string) (name, args string, ok bool) {
	p := strings.TrimSpace(prompt)
	if !strings.HasPrefix(p, "/") || len(p) < 2 {
		return "", "", false
	}
	name, args, _ = strings.Cut(p[1:], " ")
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "/\\") || builtinSlash[name] {
		return "", "", false
	}
	return name, strings.TrimSpace(args), true
}

// systemPrompt: kalau "prompt" sebenarnya pesan otomatis dari Claude Code
// (bukan ketikan user), kembalikan jenisnya; selain itu "".
func systemPrompt(p string) string {
	p = strings.TrimSpace(p)
	for _, tag := range []string{"task-notification", "agent-message", "bash-notification", "local-command-stdout", "system-reminder"} {
		if strings.HasPrefix(p, "<"+tag+">") || strings.HasPrefix(p, "<"+tag+" ") {
			return tag
		}
	}
	return ""
}

var subRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)

// splitSub: "layout page http://…" → ("layout", "page http://…").
// Kata pertama dianggap subcommand kalau berbentuk kata biasa (bukan path/url/flag).
func splitSub(args string) (sub, rest string) {
	args = strings.TrimSpace(args)
	first, rest, _ := strings.Cut(args, " ")
	if subRe.MatchString(first) {
		return first, strings.TrimSpace(rest)
	}
	return "", args
}

var (
	cmdNameRe = regexp.MustCompile(`<command-name>\s*/?([^<\s]+)\s*</command-name>`)
	cmdArgsRe = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
)

// transcriptSlash: slash command dari pesan user di transcript
// (<command-name>/impeccable</command-name><command-args>layout …</command-args>).
func transcriptSlash(raw []byte) (name, args string, ok bool) {
	if len(raw) == 0 || !strings.Contains(string(raw), "command-name") {
		return "", "", false
	}
	var txt string
	if json.Unmarshal(raw, &txt) != nil {
		var blocks []map[string]any
		_ = json.Unmarshal(raw, &blocks)
		for _, b := range blocks {
			if t, _ := b["text"].(string); strings.Contains(t, "command-name") {
				txt = t
				break
			}
		}
	}
	mm := cmdNameRe.FindStringSubmatch(txt)
	if mm == nil {
		return "", "", false
	}
	if a := cmdArgsRe.FindStringSubmatch(txt); a != nil {
		args = strings.TrimSpace(a[1])
	}
	return slashSkill("/" + mm[1] + " " + args)
}

// dedupeWindow: dua catatan skill yang sama dari jalur berbeda (hook vs transcript) dalam jendela ini digabung.
const dedupeWindow = 5 * time.Second

// addSkill: catat pemakaian skill, dedupe antara hook dan transcript.
func (s *Session) addSkill(u SkillUse) {
	if u.Name == "" {
		return
	}
	if u.Sub == "" && u.Args != "" {
		u.Sub, u.Args = splitSub(u.Args)
	}
	u.Args = format.OneLine(u.Args)
	for i := range s.Skills {
		c := &s.Skills[i]
		if u.ID != "" && c.ID == u.ID {
			c.fill(u)
			return
		}
		// sumber yang sama dari jalur lain (hook tanpa id / transcript) dalam jendela pendek
		if c.Name == u.Name && c.AgentID == u.AgentID && c.Source == u.Source &&
			(c.ID == "" || u.ID == "") && absDur(c.T.Sub(u.T)) < dedupeWindow {
			if c.ID == "" {
				c.ID = u.ID
			}
			c.fill(u)
			return
		}
	}
	s.Skills = append(s.Skills, u)
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// promptOf: nomor prompt (1-based) yang sedang berjalan pada waktu t; 0 kalau belum ada.
func (s *Session) promptOf(t time.Time) int {
	n := 0
	for _, pt := range s.PromptTimes {
		if !pt.After(t) {
			n++
		}
	}
	return n
}

// CurrentSkills: skill yang dipakai sejak prompt terakhir (task saat ini).
func (s *Session) CurrentSkills() []SkillUse {
	var start time.Time
	if n := len(s.PromptTimes); n > 0 {
		start = s.PromptTimes[n-1]
	}
	var out []SkillUse
	for _, u := range s.Skills {
		if !u.T.Before(start) {
			out = append(out, u)
		}
	}
	return out
}

// SkillActive: skill masih "jalan" selama pemanggilnya masih bekerja di prompt ini.
func (s *Session) SkillActive(u SkillUse) bool {
	if s.Status == "ended" {
		return false
	}
	if u.AgentID != "" {
		a, ok := s.Agents[u.AgentID]
		return ok && a.Status == "running"
	}
	return s.Status == "working"
}

// ContextSkill: skill dari prompt sebelumnya, dikelompokkan per nama.
type ContextSkill struct {
	Name, Model string
	Who         string   // lead atau tipe subagent
	Subs        []string // subcommand unik, urut pertama dipakai
	SubCount    map[string]int
	Count       int
	Last        time.Time
	LastPrompt  int
	FromCompact bool // hanya diketahui dari invoked_skills (dipertahankan setelah compact)
}

// SubsLabel: "layout ×2, extract".
func (c ContextSkill) SubsLabel() string {
	var ps []string
	for _, sb := range c.Subs {
		if n := c.SubCount[sb]; n > 1 {
			ps = append(ps, sb+" ×"+strconv.Itoa(n))
		} else {
			ps = append(ps, sb)
		}
	}
	return strings.Join(ps, ", ")
}

// ContextSkills: skill dari prompt-prompt sebelumnya.
//   - lead: skill yang dipanggil lead tetap ada di konteks lead sepanjang sesi
//     (Claude Code menyertakannya lagi lewat invoked_skills setelah compact).
//   - sub: skill yang dipanggil subagent hanya ada di konteks subagent itu → riwayat.
func (s *Session) ContextSkills() (lead, sub []ContextSkill) {
	cur := len(s.PromptTimes)
	if cur == 0 {
		// tanpa batas prompt, semua sudah tampil sebagai "saat ini"
		return nil, nil
	}
	curNames := map[string]bool{}
	for _, u := range s.CurrentSkills() {
		if u.AgentID == "" {
			curNames[u.Name] = true
		}
	}
	li, si := map[string]int{}, map[string]int{}
	for _, u := range s.Skills {
		p := s.promptOf(u.T)
		if p >= cur {
			continue // prompt saat ini, sudah tampil di atas
		}
		if u.AgentID == "" {
			if curNames[u.Name] {
				continue // dipakai lagi di prompt ini
			}
			addContextSkill(&lead, li, u.Name, u, p)
		} else {
			addContextSkill(&sub, si, u.Who+"\x00"+u.Name, u, p)
		}
	}
	// skill yang hanya diketahui dari invoked_skills (mis. dipanggil sebelum hooks/tail)
	for name := range s.InvokedSkills {
		if _, ok := li[name]; ok || curNames[name] {
			continue
		}
		li[name] = len(lead)
		lead = append(lead, ContextSkill{Name: name, Who: "lead", Model: s.Lead, FromCompact: true})
	}
	byLast := func(x []ContextSkill) {
		sort.SliceStable(x, func(i, j int) bool { return x[i].Last.After(x[j].Last) })
	}
	byLast(lead)
	byLast(sub)
	return lead, sub
}

// addContextSkill menambah pemakaian u (di prompt ke-p) ke grup key di list.
func addContextSkill(list *[]ContextSkill, idx map[string]int, key string, u SkillUse, p int) {
	i, ok := idx[key]
	if !ok {
		idx[key] = len(*list)
		*list = append(*list, ContextSkill{Name: u.Name, Who: u.Who, SubCount: map[string]int{}})
		i = len(*list) - 1
	}
	c := &(*list)[i]
	c.Count++
	if u.Sub != "" {
		if c.SubCount[u.Sub] == 0 {
			c.Subs = append(c.Subs, u.Sub)
		}
		c.SubCount[u.Sub]++
	}
	if !u.T.Before(c.Last) {
		c.Last, c.LastPrompt, c.Model = u.T, p, u.Model
	}
}
