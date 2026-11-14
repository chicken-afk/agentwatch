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
		return l
	case len(ver) == 0:
		return fam
	}
	return fam + " " + strings.Join(ver, ".")
}

// ModelFamily: keluarga model (opus/sonnet/…) dari id model, "" kalau tidak dikenal.
func ModelFamily(m string) string {
	l := strings.ToLower(m)
	for _, f := range families {
		if strings.Contains(l, f) {
			return f
		}
	}
	return ""
}
