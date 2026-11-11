package tomd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			if len(got)*10 > len(in)*6 {
				t.Errorf("出力 %d バイトが storage %d バイトの 60%% を超える", len(got), len(in))
			}
			if strings.TrimSpace(got) == "" {
				t.Error("出力が空")
			}
		})
	}
}
