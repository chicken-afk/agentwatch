package format

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var families = []string{"opus", "sonnet", "haiku", "fable", "mythos"}

var bracketRe = regexp.MustCompile(`\[.*?\]`)

// PrettyModel: "claude-sonnet-5-5" → "sonnet 5.5", "claude-3-5-haiku-20241022" → "haiku 3.5".
func PrettyModel(m string) string {
	if m == "" {
		return "?"
	}
	l := strings.ToLower(bracketRe.ReplaceAllString(m, ""))
	l = strings.TrimPrefix(l, "claude-")
	fam := ""
	var ver []string
	for _, p := range strings.Split(l, "-") {
		if slices.Contains(families, p) {
			fam = p
			continue
		}
		if _, err := strconv.Atoi(p); err == nil && len(p) <= 2 {
			ver = append(ver, p)
		}
	}
	switch {
	case fam == "":
		// not a Claude family name: keep the id as written ("claude-only" stays "claude-only")
		return strings.ToLower(strings.TrimSpace(bracketRe.ReplaceAllString(m, "")))
	case len(ver) == 0:
		return fam
	}
	return fam + " " + strings.Join(ver, ".")
}

var openaiRe = regexp.MustCompile(`(^|[/-])(gpt|codex|o[1-9])([-.]|$)`)

// ModelFamily: the model family from a model id: opus/sonnet/… for Claude, plus "gpt"
// (gpt-*, codex, o1/o3/o4 models) and "gemini"; "" if unknown.
func ModelFamily(m string) string {
	l := strings.ToLower(m)
	for _, f := range families {
		if strings.Contains(l, f) {
			return f
		}
	}
	switch {
	case strings.Contains(l, "gemini"):
		return "gemini"
	case openaiRe.MatchString(l):
		return "gpt"
	}
	return ""
}
