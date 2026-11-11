package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

func TestTruncate_Runes(t *testing.T) {
	if got := Truncate("あいうえお", 3); got != "あいう…" {
		t.Errorf("got %q", got)
	}
	if got := Truncate("あいう", 3); got != "あいう" {
		t.Errorf("got %q", got)
	}
}

func TestCleanExcerpt(t *testing.T) {
	got := CleanExcerpt("議事録\n<b>x</b>")
	if got != "議事録 x" {
		t.Errorf("got %q", got)
	}
	if got := CleanExcerpt("a &amp; b&nbsp;&lt;c&gt;"); got != "a & b <c>" {
		t.Errorf("got %q", got)
	}
}

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	err := Table(&buf, []string{"ID", "TITLE"}, [][]string{{"1", "a\nb"}, {"22", "c\td"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "ID  TITLE\n1   a b\n22  c d\n"
	if buf.String() != want {
		t.Errorf("got %q want %q", buf.String(), want)
	}
}

func TestJSON_SingleLine(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, map[string]any{"a": 1, "b": "<x>"}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "{\"a\":1,\"b\":\"<x>\"}\n" {
		t.Errorf("got %q", buf.String())
	}
}

func TestWriteError_JSON(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, &domain.Error{Kind: domain.ErrNotFound, Status: 404, Message: "m"}, true, nil)
	want := `{"error":{"kind":"not_found","status":404,"message":"m"}}` + "\n"
	if buf.String() != want {
		t.Errorf("got %q", buf.String())
	}
	if strings.Count(buf.String(), "\n") != 1 {
		t.Error("not single line")
	}
}

func TestWriteError_JSON_NonDomain(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, errors.New("boom"), true, nil)
	want := `{"error":{"kind":"api","message":"boom"}}` + "\n"
	if buf.String() != want {
		t.Errorf("got %q", buf.String())
	}
}

func TestWriteError_Text(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, &domain.Error{Kind: domain.ErrAuth, Message: "m", Hint: "h"}, false, nil)
	if buf.String() != "Error: m\nHint: h\n" {
		t.Errorf("got %q", buf.String())
	}
	buf.Reset()
	WriteError(&buf, fmt.Errorf("wrap: %w", &domain.Error{Kind: domain.ErrAuth, Message: "m"}), false, nil)
	if buf.String() != "Error: wrap: m\n" {
		t.Errorf("got %q", buf.String())
	}
	buf.Reset()
	WriteError(&buf, errors.New("boom"), false, nil)
	if buf.String() != "Error: boom\n" {
		t.Errorf("got %q", buf.String())
	}
}

func TestSanitizeText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"\x1b[31mred", " [31mred"},
		{"a\u009bb", "a b"},
		{"a\u202eb\u2066c\u200fd\u061ce", "abcde"},
		{"日本語 テキスト", "日本語 テキスト"},
	} {
		if got := SanitizeText(c.in); got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
	if strings.Contains(CleanExcerpt("\x1b[31mred&#27;[0m"), "\x1b") {
		t.Error("escape remained in excerpt")
	}
	var buf bytes.Buffer
	_ = Table(&buf, []string{"T"}, [][]string{{"\x1b]0;x\u202e"}})
	if strings.Contains(buf.String(), "\x1b") || strings.Contains(buf.String(), "\u202e") {
		t.Error("unsanitized table cell")
	}
}

func TestTruncate_Negative(t *testing.T) {
	if got := Truncate("abc", -1); got != "…" {
		t.Errorf("got %q", got)
	}
}

func TestWriteError_TextSanitizesControlChars(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, &domain.Error{Kind: domain.ErrAPI, Message: "bad\x1b]0;title\x07msg", Hint: "h\x1b[2Jint"}, false, nil)
	got := buf.String()
	if strings.ContainsRune(got, 0x1b) || strings.ContainsRune(got, 0x07) {
		t.Fatalf("got %q", got)
	}
	if got != "Error: bad ]0;title msg\nHint: h [2Jint\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitizeMultiline(t *testing.T) {
	got := SanitizeMultiline("a\x1b[31mb\n\tc\u202ed\u0085e")
	if got != "a [31mb\n\tcd e" {
		t.Fatalf("got %q", got)
	}
}

// 端末の表示を変えうる C1 制御文字と双方向制御文字を並べた文字列。
const terminalControls = "a\u0080b\u009bc\u061Cd\u200Ee\u200Ff\u202Ag\u202Eh\u2066i\u2069j\u00A0k"

func hasTerminalControls(s string) bool {
	for _, r := range s {
		if (r >= 0x80 && r <= 0x9F) || r == 0x061C || r == 0x200E || r == 0x200F ||
			(r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			return true
		}
	}
	return false
}

func TestJSON_EscapesTerminalControls(t *testing.T) {
	var buf bytes.Buffer
	in := map[string]string{terminalControls: terminalControls}
	if err := JSON(&buf, in); err != nil {
		t.Fatal(err)
	}
	if hasTerminalControls(buf.String()) || !strings.Contains(buf.String(), `\u009b`) || !strings.Contains(buf.String(), `\u202e`) {
		t.Fatalf("got %q", buf.String())
	}
	var out map[string]string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out[terminalControls] != terminalControls {
		t.Fatalf("decoded %q", out)
	}
}

func TestWriteError_JSONEscapesTerminalControls(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, &domain.Error{Kind: domain.ErrAPI, Message: terminalControls, Hint: terminalControls}, true, nil)
	if hasTerminalControls(buf.String()) {
		t.Fatalf("got %q", buf.String())
	}
	var out ErrorJSON
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error.Message != terminalControls || out.Error.Hint != terminalControls {
		t.Fatalf("decoded %+v", out)
	}
}
