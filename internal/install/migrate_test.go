package install

import (
	"os"
	"path/filepath"
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

func TestWriteSettingsSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	// an existing file in a different key order and spacing than agentwatch writes
	orig := `{"model":"sonnet",   "env":{"B":"2","A":"1"}, "n": 5}`
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	backups := func() int {
		m, _ := filepath.Glob(path + ".agentwatch-backup-*")
		return len(m)
	}
	write := func(change func(map[string]any)) {
		t.Helper()
		s, b, err := readSettings(path)
		if err != nil {
			t.Fatal(err)
		}
		change(s)
		if _, err := writeSettings(path, s, b); err != nil {
			t.Fatal(err)
		}
	}

	write(func(map[string]any) {})
	if got, _ := os.ReadFile(path); string(got) != orig || backups() != 0 {
		t.Fatalf("unchanged settings must not be rewritten or backed up (backups=%d)", backups())
	}
	write(func(s map[string]any) { s["hooks"] = map[string]any{"Stop": []any{}} })
	if backups() != 1 {
		t.Fatalf("a real change should back up once, got %d", backups())
	}
	write(func(map[string]any) {})
	if backups() != 1 {
		t.Errorf("writing the same settings again must not add a backup, got %d", backups())
	}
	if !sameJSON([]byte(`{"a":1,"b":[1,2]}`), []byte("{\n  \"b\": [1, 2],\n  \"a\": 1.0\n}")) {
		t.Error("sameJSON should ignore key order and formatting")
	}
}
