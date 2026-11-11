package tomd

import (
	"encoding/xml"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/genshibangou16/confluence-cli/internal/storage"
)

var update = flag.Bool("update", false, "ゴールデンファイルを現在の出力で書き直す")

func TestConvert_Golden(t *testing.T) {
	for _, name := range []string{"headings", "lists", "code", "panels", "tasks", "table", "macros", "mentions"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("..", "..", "..", "testdata", "convert", "tomd")
			in, err := os.ReadFile(filepath.Join(dir, name+".xml"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := Convert(string(in))
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			goldenPath := filepath.Join(dir, name+".md")
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("出力がゴールデンファイルと違う\n--- got\n%s\n--- want\n%s", got, want)
			}
		})
	}
}

func TestConvert_PanelAlert(t *testing.T) {
	in := `<ac:structured-macro ac:name="warning"><ac:parameter ac:name="title">T</ac:parameter><ac:rich-text-body><p>本文</p></ac:rich-text-body></ac:structured-macro>`
	got, err := Convert(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := "> [!CAUTION]\n> **T**\n> 本文\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvert_CodeKeepsCDATA(t *testing.T) {
	in := `<ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter><ac:plain-text-body><![CDATA[x := "<a> & ]]` + `]]><![CDATA[" ]]></ac:plain-text-body></ac:structured-macro>`
	got, err := Convert(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := "```go\nx := \"<a> & ]]\" \n```\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvert_InvalidXML(t *testing.T) {
	if _, err := Convert("<p>unclosed"); err == nil {
		t.Error("整形式でない本文でエラーにならない")
	}
}

func TestConvert_Empty(t *testing.T) {
	got, err := Convert("")
	if err != nil || got != "" {
		t.Errorf("got %q, %v; want empty", got, err)
	}
}

func TestConvert_PrivateFixtures(t *testing.T) {
	paths, _ := filepath.Glob(filepath.Join("..", "..", "..", "testdata", "private", "*.xml"))
	if len(paths) == 0 {
		t.Skip("testdata/private に storage 形式のファイルがない")
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			in, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Convert(string(in))
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			// 実在のページの内容をログに残さないため、バイト数だけを出す。
			t.Logf("storage %d バイト → markdown %d バイト（%.1f%%）", len(in), len(got), 100*float64(len(got))/float64(len(in)))
			// 上限は storage の 60% と、出力に残す内容のバイト数の 115% の大きいほう。
			// 日本語の文字が多くタグが少ないページでは、内容だけで storage の 60% を超えるため、
			// 内容に対するマークアップの上乗せでも上限を決める。
			content := contentBytes(t, string(in))
			limit := max(len(in)*6/10, content*115/100)
			t.Logf("内容 %d バイト、上限 %d バイト", content, limit)
			if len(got) > limit {
				t.Errorf("出力 %d バイトが上限 %d バイトを超える", len(got), limit)
			}
			if strings.TrimSpace(got) == "" {
				t.Error("出力が空")
			}
		})
	}
}

func TestConvert_HugeSpan(t *testing.T) {
	in := `<table><tbody><tr><td colspan="100000000" rowspan="100000000"><p>a</p></td></tr><tr><td><p>b</p></td></tr></tbody></table>`
	done := make(chan struct{})
	var got string
	var err error
	go func() {
		got, err = Convert(in)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Convert が 2 秒以内に終わらない")
	}
	if err != nil {
		t.Fatal(err)
	}
	if want := "[table: too large]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// contentBytes は、Markdown に残すべき内容のバイト数を返す。
// 内容は、連続した空白を 1 つにした文字データ（CDATA を含む）と、出力に残す属性値（ページのタイトル・添付のファイル名・日付・accountId）。
// 段落やセルなどの区切りの要素ごとに、区切りの空白として 1 バイトを足す。
func contentBytes(t *testing.T, body string) int {
	t.Helper()
	d, _ := storage.Decoder(body)
	n := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return n
		}
		if err != nil {
			t.Fatal(err)
		}
		switch tok := tok.(type) {
		case xml.CharData:
			n += len(strings.Join(strings.FieldsFunc(string(tok), func(r rune) bool {
				return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u00a0'
			}), " "))
		case xml.StartElement:
			if slices.Contains([]string{"p", "br", "li", "td", "th", "h1", "h2", "h3", "h4", "h5", "h6"}, tok.Name.Local) {
				n++
			}
			for _, a := range tok.Attr {
				if slices.Contains([]string{"content-title", "datetime", "filename", "account-id"}, a.Name.Local) {
					n += len(a.Value)
				}
			}
		}
	}
}
