package install

import (
	"strings"
	"testing"
)

func TestRulesReplaceLegacyBlock(t *testing.T) {
	legacy := "# Mine\n\n" + legacyStart + "\n## Skills (managed by ccwatch)\n- old\n" + legacyEnd + "\n\n## Other\n"
	out, changed := upsertRules(legacy)
	if !changed || strings.Contains(out, "ccwatch") || !strings.Contains(out, RulesBlock()) {
		t.Fatalf("legacy block not replaced:\n%s", out)
	}
	if !strings.HasPrefix(out, "# Mine\n\n") || !strings.HasSuffix(out, "## Other\n") {
		t.Errorf("surrounding content changed:\n%s", out)
	}
	if again, changed := upsertRules(out); changed || again != out {
		t.Error("second install should be a no-op")
	}

	both := RulesBlock() + "\n\n" + legacyStart + "\nx\n" + legacyEnd + "\n"
	if out, _ := upsertRules(both); strings.Contains(out, legacyStart) {
		t.Error("leftover legacy block should be dropped when the new one exists")
	}
	if out, changed := removeRules("keep\n\n" + both); !changed || out != "keep\n" {
		t.Errorf("uninstall should remove both blocks, got %q", out)
	}
}

func TestIsOursIncludesLegacyName(t *testing.T) {
	for _, c := range []string{"/u/.local/bin/ccwatch hook", "/u/.local/bin/agentwatch hook", `"/a b/agentwatch" hook`} {
		if !isOurs(c, "hook") {
			t.Errorf("isOurs(%q) = false", c)
		}
	}
	if isOurs("/u/bin/other hook", "hook") || isOurs("/u/bin/agentwatch statusline", "hook") {
		t.Error("foreign commands must not match")
	}
}
