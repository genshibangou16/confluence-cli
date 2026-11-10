package output

import (
	"bytes"
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
	got := CleanExcerpt("@@@hl@@@議事@@@endhl@@@録\n<b>x</b>")
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
	WriteError(&buf, &domain.Error{Kind: domain.ErrNotFound, Status: 404, Message: "m"}, true)
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
	WriteError(&buf, errors.New("boom"), true)
	want := `{"error":{"kind":"api","message":"boom"}}` + "\n"
	if buf.String() != want {
		t.Errorf("got %q", buf.String())
	}
}

func TestWriteError_Text(t *testing.T) {
	var buf bytes.Buffer
	WriteError(&buf, &domain.Error{Kind: domain.ErrAuth, Message: "m", Hint: "h"}, false)
	if buf.String() != "Error: m\nHint: h\n" {
		t.Errorf("got %q", buf.String())
	}
	buf.Reset()
	WriteError(&buf, fmt.Errorf("wrap: %w", &domain.Error{Kind: domain.ErrAuth, Message: "m"}), false)
	if buf.String() != "Error: wrap: m\n" {
		t.Errorf("got %q", buf.String())
	}
	buf.Reset()
	WriteError(&buf, errors.New("boom"), false)
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
