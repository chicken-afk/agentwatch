package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// a tiny protobuf encoder for building test blobs
func pbV(v uint64) []byte {
	var b []byte
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func pbInt(num int, v int64) []byte { return append(pbV(uint64(num)<<3), pbV(uint64(v))...) }

func pbBytes(num int, b []byte) []byte {
	out := append(pbV(uint64(num)<<3|2), pbV(uint64(len(b)))...)
	return append(out, b...)
}

func pbStr(num int, s string) []byte { return pbBytes(num, []byte(s)) }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func agyMeta(t time.Time, extra ...[]byte) []byte {
	ts := pbBytes(1, cat(pbInt(1, t.Unix()), pbInt(2, int64(t.Nanosecond()))))
	return cat(append([][]byte{ts}, extra...)...)
}

func TestAntigravityConversation(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "antigravity")
	conv := "b00f0793-23e7-4cb7-93e4-3f3a544b6042"
	t0 := time.Now().Add(-10 * time.Minute).Truncate(time.Millisecond)

	// prompt text + folder: the newest prompts are logged without a conversationId, so they're
	// matched by time; slash commands and other conversations must not match
	write(t, filepath.Join(dir, "history.jsonl"),
		fmt.Sprintf(`{"display":"fix the login page","timestamp":%d,"workspace":"/work/pos"}`+"\n", t0.UnixMilli()+12)+
			fmt.Sprintf(`{"display":"/model","timestamp":%d,"workspace":"/work/pos","type":"slash_command","conversationId":%q}`+"\n", t0.UnixMilli()+5, conv)+
			fmt.Sprintf(`{"display":"other chat","timestamp":%d,"workspace":"/elsewhere","conversationId":"another"}`+"\n", t0.UnixMilli()))

	path := filepath.Join(dir, "conversations", conv+".db")
	write(t, path, "")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		"CREATE TABLE steps (idx integer PRIMARY KEY, step_type integer, status integer, has_subtrajectory numeric, metadata blob, error_details blob, step_payload blob)",
		"CREATE TABLE gen_metadata (idx integer PRIMARY KEY, data blob, size integer)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO gen_metadata (idx, data) VALUES (0, ?)", pbBytes(1, cat(pbInt(3, 1525), pbStr(19, "gemini-4-hydrogen")))); err != nil {
		t.Fatal(err)
	}
	usage := pbBytes(9, cat(pbInt(1, 1525), pbInt(2, 32195), pbInt(3, 780), pbInt(5, 1000)))
	tool := func(id, name, args string) []byte {
		return pbBytes(4, cat(pbStr(1, id), pbStr(2, name), pbStr(3, args)))
	}
	insert := func(idx, typ, status int, meta, errDetails []byte) {
		t.Helper()
		if _, err := db.Exec("INSERT INTO steps (idx, step_type, status, metadata, error_details) VALUES (?, ?, ?, ?, ?)", idx, typ, status, meta, errDetails); err != nil {
			t.Fatal(err)
		}
	}
	insert(0, 14, 3, agyMeta(t0), nil)
	insert(1, 15, 3, agyMeta(t0.Add(3*time.Second), usage, pbInt(11, 1525)), nil)
	insert(2, 132, 3, agyMeta(t0.Add(4*time.Second), tool("c1", "run_command", `{"CommandLine":"go test ./..."}`)), nil)
	insert(3, 132, 7, agyMeta(t0.Add(5*time.Second), tool("c2", "view_file", `{"AbsolutePath":"/work/pos/main.go"}`)), []byte{1})
	insert(4, 15, 3, agyMeta(t0.Add(6*time.Second), usage, pbInt(11, 1525)), nil)

	st := New(Options{})
	st.Refresh()
	s := only(t, st, AgentAntigravity)
	if s.ID != conv || s.Cwd != "/work/pos" || s.LastPrompt != "fix the login page" || s.Lead != "gemini-4-hydrogen" {
		t.Errorf("meta: id=%q cwd=%q prompt=%q lead=%q", s.ID, s.Cwd, s.LastPrompt, s.Lead)
	}
	if s.Prompts != 1 || s.MainMsgs != 2 || s.MainUsage.In != 64390 || s.MainUsage.Out != 1560 || s.MainUsage.CacheR != 2000 {
		t.Errorf("prompts=%d msgs=%d usage=%+v", s.Prompts, s.MainMsgs, s.MainUsage)
	}
	if s.MainTools != 2 || s.Failures != 1 {
		t.Errorf("tools=%d failures=%d", s.MainTools, s.Failures)
	}

	// a new step is picked up incrementally, without counting earlier steps twice
	insert(5, 15, 3, agyMeta(t0.Add(9*time.Second), usage, pbInt(11, 1525)), nil)
	st.lastProviderScan = time.Time{}
	st.Refresh()
	if s := only(t, st, AgentAntigravity); s.MainMsgs != 3 || s.MainTools != 2 || s.Prompts != 1 {
		t.Errorf("after an append: msgs=%d tools=%d prompts=%d", s.MainMsgs, s.MainTools, s.Prompts)
	}

	// the usage history counts it per model
	h := NewHistory()
	h.Scan(time.Now())
	rep := h.Report(time.Now(), []string{AgentAntigravity})
	if len(rep.Agents) != 1 || rep.Agents[0].Models[0].Model != "gemini-4-hydrogen" || rep.Total[0].Msgs != 3 {
		t.Errorf("usage history: %+v", rep)
	}
}

func TestPbParseRejectsGarbage(t *testing.T) {
	if _, ok := pbParse([]byte{0x0a, 0xff}); ok {
		t.Error("a truncated length-delimited field must not parse")
	}
	m := pbDecode(cat(pbInt(1, 42), pbStr(2, "hi"), pbBytes(3, pbInt(1, 7))))
	if m.int(1) != 42 || m.str(2) != "hi" || m.msg(3).int(1) != 7 || m.int(9) != 0 {
		t.Errorf("decode: %+v", m)
	}
}
