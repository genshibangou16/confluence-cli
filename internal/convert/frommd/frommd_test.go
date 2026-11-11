package frommd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/convert/tomd"
	"github.com/genshibangou16/confluence-cli/internal/storage"
)

var update = flag.Bool("update", false, "ゴールデンファイルを現在の出力で書き直す")

var goldenDir = filepath.Join("..", "..", "..", "testdata", "convert", "frommd")

// goldenNames はスキルの変換リファレンスの例を入力にしたゴールデンファイルの名前。
var goldenNames = []string{"headings", "inline", "lists", "table", "code", "alerts", "tasks"}

func mustConvert(t *testing.T, md string) string {
	t.Helper()
	got, err := Convert(md)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return got
}

func TestConvert_ReferenceExamples(t *testing.T) {
	for _, name := range goldenNames {
		t.Run(name, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join(goldenDir, name+".md"))
			if err != nil {
				t.Fatal(err)
			}
			got := mustConvert(t, string(in))
			goldenPath := filepath.Join(goldenDir, name+".xml")
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

func TestConvert_OutputIsWellFormed(t *testing.T) {
	for _, name := range goldenNames {
		in, err := os.ReadFile(filepath.Join(goldenDir, name+".md"))
		if err != nil {
			t.Fatal(err)
		}
		if err := storage.Validate(mustConvert(t, string(in))); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestConvert_AlertMapping(t *testing.T) {
	for kind, macro := range map[string]string{"NOTE": "info", "IMPORTANT": "info", "TIP": "tip", "WARNING": "note", "CAUTION": "warning"} {
		got := mustConvert(t, "> [!"+kind+"]\n> 本文\n")
		want := `<ac:structured-macro ac:name="` + macro + `" ac:schema-version="1">` + "\n<ac:rich-text-body>\n<p>本文</p>\n</ac:rich-text-body>\n</ac:structured-macro>\n"
		if got != want {
			t.Errorf("%s: got %q, want %q", kind, got, want)
		}
	}
	if got, want := mustConvert(t, "> 普通の引用\n"), "<blockquote>\n<p>普通の引用</p>\n</blockquote>\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// [!TYPE] が最初の段落の 1 行目の全体でなければパネルにしない。
	got := mustConvert(t, "> [!NOTE] 本文\n")
	if !strings.HasPrefix(got, "<blockquote>") {
		t.Errorf("got %q, want blockquote", got)
	}
}

func TestConvert_CodeLanguageAlias(t *testing.T) {
	for lang, want := range map[string]string{"sh": "bash", "golang": "go", "foo": "text", "Python": "python"} {
		got := mustConvert(t, "```"+lang+"\nx\n```\n")
		if !strings.Contains(got, `<ac:parameter ac:name="language">`+want+`</ac:parameter>`) {
			t.Errorf("%s: got %q, want language %s", lang, got, want)
		}
	}
	if got := mustConvert(t, "```\nx\n```\n"); strings.Contains(got, "language") {
		t.Errorf("言語の指定がないのに language パラメータがある: %q", got)
	}
}

func TestConvert_CDATASplit(t *testing.T) {
	code := `if a[b[0]]> 1 { return "]]>" }`
	got := mustConvert(t, "```go\n"+code+"\n```\n")
	if err := storage.Validate(got); err != nil {
		t.Fatalf("Validate: %v\n%s", err, got)
	}
	md, err := tomd.Convert(got)
	if err != nil {
		t.Fatal(err)
	}
	if want := "```go\n" + code + "\n```\n"; md != want {
		t.Errorf("tomd で戻した結果: got %q, want %q", md, want)
	}
}

func TestConvert_EscapesRawHTML(t *testing.T) {
	for _, md := range []string{"<script>x</script>\n", "前 <b>x</b> & 後\n"} {
		got := mustConvert(t, md)
		if strings.Contains(got, "<script>") || strings.Contains(got, "<b>") {
			t.Errorf("生の HTML が残っている: %q", got)
		}
		if err := storage.Validate(got); err != nil {
			t.Errorf("Validate: %v", err)
		}
	}
	if got := mustConvert(t, "<script>x</script>\n"); !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("got %q", got)
	}
}

func TestConvert_TaskIDsSequential(t *testing.T) {
	got := mustConvert(t, "- [ ] a\n  - [x] b\n- [ ] c\n\n段落\n\n- [x] d\n")
	for i, id := range []string{"1", "2", "3", "4"} {
		if !strings.Contains(got, "<ac:task-id>"+id+"</ac:task-id>") {
			t.Errorf("%d 番目のタスクの ID %s がない: %q", i+1, id, got)
		}
	}
	if strings.Contains(got, "<ac:task-id>5") {
		t.Errorf("ID が余分にある: %q", got)
	}
	// 入れ子のタスクは親のタスクの ac:task-body の中に置く。
	if !strings.Contains(got, "<ac:task-body>a<ac:task-list>") {
		t.Errorf("入れ子のタスクが親の本文の中にない: %q", got)
	}
}

func TestConvert_Images(t *testing.T) {
	got := mustConvert(t, "![図](images/a.png)\n")
	if strings.Contains(got, "ac:image") || !strings.Contains(got, `<a href="images/a.png">図</a>`) {
		t.Errorf("相対パスの画像: got %q", got)
	}
}

func TestConvert_EscapesText(t *testing.T) {
	got := mustConvert(t, "a & b < c > d [x](https://example.com/?a=1&b=\"2\")\n")
	if want := `<p>a &amp; b &lt; c &gt; d <a href="https://example.com/?a=1&amp;b=%222%22">x</a></p>` + "\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvert_DropsCharsInvalidInXML(t *testing.T) {
	got := mustConvert(t, "a\x01b &#1; c\x00d\n\n```\nx\x02y\n```\n")
	want := "<p>ab  cd</p>\n" + `<ac:structured-macro ac:name="code" ac:schema-version="1">` + "\n<ac:plain-text-body><![CDATA[xy]]></ac:plain-text-body>\n</ac:structured-macro>\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// 入力の制御文字を除いた結果で "]]>" ができても、CDATA の分割で本文に戻る。
func TestConvert_InvalidCharInsideCDATAEnd(t *testing.T) {
	got := mustConvert(t, "```\n]]\x01>\n```\n")
	if err := storage.Validate(got); err != nil {
		t.Fatalf("Validate: %v\n%s", err, got)
	}
	md, err := tomd.Convert(got)
	if err != nil {
		t.Fatal(err)
	}
	if want := "```\n]]>\n```\n"; md != want {
		t.Errorf("got %q, want %q", md, want)
	}
}

func TestConvert_DangerousURLs(t *testing.T) {
	for _, dest := range []string{"&#106;avascript:alert(1)", "JaVaScRiPt:alert(1)", "vbscript:msgbox(1)", "data:text/html,x"} {
		for _, md := range []string{"[x](" + dest + ")\n", "![x](" + dest + ")\n"} {
			got := mustConvert(t, md)
			if strings.Contains(got, "href") || strings.Contains(got, "ri:url") {
				t.Errorf("%q: 危険な URL がリンクになっている: %q", md, got)
			}
		}
	}
	if got := mustConvert(t, "<javascript:alert(1)>\n"); strings.Contains(got, "href") {
		t.Errorf("自動リンク: %q", got)
	}
}

func TestConvert_SoftLineBreak(t *testing.T) {
	for md, want := range map[string]string{
		"これは長い文で\n途中で折り返す。\n": "<p>これは長い文で途中で折り返す。</p>\n",
		"English\nline\n": "<p>English line</p>\n",
		// 両側が東アジアの全角文字のときだけ空白を入れない。片側が英字なら空白を入れる。
		"日本語\nEnglish\n": "<p>日本語 English</p>\n",
		"日本語\n**太字**\n":  "<p>日本語<strong>太字</strong></p>\n",
	} {
		if got := mustConvert(t, md); got != want {
			t.Errorf("%q: got %q, want %q", md, got, want)
		}
	}
}

func TestConvert_CRLF(t *testing.T) {
	got := mustConvert(t, "```go\r\nx\r\ny\r\n```\r\n\r\npara\r\nnext\r\n")
	want := `<ac:structured-macro ac:name="code" ac:schema-version="1">` + "\n" + `<ac:parameter ac:name="language">go</ac:parameter>` +
		"\n<ac:plain-text-body><![CDATA[x\ny]]></ac:plain-text-body>\n</ac:structured-macro>\n<p>para next</p>\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvert_OrderedListStart(t *testing.T) {
	if got := mustConvert(t, "3. a\n4. b\n"); !strings.HasPrefix(got, `<ol start="3">`) {
		t.Errorf("got %q", got)
	}
	if got := mustConvert(t, "1. a\n"); !strings.HasPrefix(got, "<ol>\n") {
		t.Errorf("got %q", got)
	}
}

func TestConvert_Empty(t *testing.T) {
	if got := mustConvert(t, ""); got != "" {
		t.Errorf("got %q", got)
	}
}
