package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../testdata/storage", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func wantValidation(t *testing.T, err error) *domain.Error {
	t.Helper()
	var de *domain.Error
	if !errors.As(err, &de) || de.Kind != domain.ErrValidation {
		t.Fatalf("err = %v, want *domain.Error with ErrValidation", err)
	}
	return de
}

func TestHeadings_IgnoresCDATA(t *testing.T) {
	body := `<ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[<h2>x</h2>]]></ac:plain-text-body></ac:structured-macro><h2>y</h2>`
	hs, err := Headings(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].Text != "y" {
		t.Fatalf("Headings = %+v, want only y", hs)
	}
	hs, err = Headings(readFixture(t, "rich-nolayout.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hs {
		if h.Text == "x" {
			t.Errorf("heading from CDATA: %+v", h)
		}
	}
}

func TestHeadings_NestedNotTopLevel(t *testing.T) {
	for _, name := range []string{"rich.xml", "rich-nolayout.xml"} {
		body := readFixture(t, name)
		hs, err := Headings(body)
		if err != nil {
			t.Fatal(err)
		}
		expandH2 := strings.Index(body, `<ac:rich-text-body><h2>背景</h2>`) + len(`<ac:rich-text-body>`)
		found := false
		for _, h := range hs {
			if h.Start == expandH2 {
				found = true
				if h.TopLevel {
					t.Errorf("%s: heading in expand is TopLevel", name)
				}
			}
			if h.Text == "セルの見出し" && h.TopLevel {
				t.Errorf("%s: heading in table cell is TopLevel", name)
			}
		}
		if !found {
			t.Errorf("%s: heading in expand not found: %+v", name, hs)
		}
	}
}

func TestHeadings_LayoutCellIsTopLevel(t *testing.T) {
	body := readFixture(t, "rich.xml")
	hs, err := Headings(body)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range hs {
		if h.TopLevel {
			got = append(got, "h"+strconv.Itoa(h.Level)+" "+h.Text)
		}
		if body[h.Start:h.End] != "<h"+strconv.Itoa(h.Level)+">"+h.Text+"</h"+strconv.Itoa(h.Level)+">" {
			t.Errorf("body[Start:End] = %q for %+v", body[h.Start:h.End], h)
		}
	}
	want := []string{"h1 概要", "h2 背景", "h3 詳細", "h2 手順", "h2 左の列", "h2 右の列", "h2 まとめ"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("top-level headings = %v, want %v", got, want)
	}
}

func TestHeadings_NormalizedText(t *testing.T) {
	hs, err := Headings("<h2> <strong>A</strong>&nbsp;B  </h2>")
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].Text != "A B" || hs[0].Level != 2 || !hs[0].TopLevel {
		t.Fatalf("Headings = %+v", hs)
	}
}

func TestHeadings_EmptyAndMalformed(t *testing.T) {
	hs, err := Headings("")
	if err != nil || len(hs) != 0 {
		t.Fatalf("Headings(\"\") = %+v, %v", hs, err)
	}
	_, err = Headings("<h2>a</h3>")
	wantValidation(t, err)
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Errorf("err does not wrap *SyntaxError: %v", err)
	}
}

func sectionContent(t *testing.T, body, title string) string {
	t.Helper()
	s, err := FindSection(body, title)
	if err != nil {
		t.Fatalf("FindSection(%q) = %v", title, err)
	}
	if s.ContentStart != s.Heading.End {
		t.Errorf("ContentStart = %d, Heading.End = %d", s.ContentStart, s.Heading.End)
	}
	return body[s.ContentStart:s.ContentEnd]
}

func TestFindSection_Boundaries(t *testing.T) {
	body := "<h1>A</h1><p>1</p><h2>B</h2><p>2</p><h3>C</h3><p>3</p><h2>D</h2><p>4</p>"
	cases := map[string]string{
		"A": "<p>1</p><h2>B</h2><p>2</p><h3>C</h3><p>3</p><h2>D</h2><p>4</p>",
		"B": "<p>2</p><h3>C</h3><p>3</p>",
		"C": "<p>3</p>",
		"D": "<p>4</p>",
	}
	for title, want := range cases {
		if got := sectionContent(t, body, title); got != want {
			t.Errorf("section %q = %q, want %q", title, got, want)
		}
	}
}

func TestFindSection_StopsAtMacroContainingHeading(t *testing.T) {
	macro := func(h string) string {
		return `<ac:structured-macro ac:name="expand"><ac:rich-text-body>` + h + `</ac:rich-text-body></ac:structured-macro>`
	}
	cases := []struct {
		name, body, want string
	}{
		{"same level", "<h2>B</h2><p>x</p>" + macro("<h2>E</h2>") + "<p>y</p>", "<p>x</p>"},
		{"higher level", "<h2>B</h2><p>x</p>" + macro("<h1>E</h1>") + "<p>y</p>", "<p>x</p>"},
		{"lower level", "<h2>B</h2><p>x</p>" + macro("<h3>E</h3>") + "<p>y</p>", "<p>x</p>" + macro("<h3>E</h3>") + "<p>y</p>"},
		{"layout after root heading",
			"<h2>B</h2><p>x</p><ac:layout><ac:layout-section><ac:layout-cell><h2>E</h2></ac:layout-cell></ac:layout-section></ac:layout>",
			"<p>x</p>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sectionContent(t, c.body, "B"); got != c.want {
				t.Errorf("section = %q, want %q", got, c.want)
			}
		})
	}

	body := readFixture(t, "rich.xml")
	got := sectionContent(t, body, "手順")
	if !strings.HasPrefix(got, `<ac:structured-macro ac:name="code"`) || !strings.HasSuffix(got, `</table>`) {
		t.Errorf("rich.xml 手順 section should end before expand: %q", got)
	}
	// 表のセルにある h2 も、表をトップレベルの要素として節の終わりにする。
	body = readFixture(t, "rich-nolayout.xml")
	got = sectionContent(t, body, "手順")
	if !strings.HasPrefix(got, `<ac:structured-macro ac:name="code"`) || !strings.HasSuffix(got, `</ac:structured-macro>`) {
		t.Errorf("rich-nolayout.xml 手順 section should end before table: %q", got)
	}
}

func TestFindSection_EndsAtLayoutCell(t *testing.T) {
	body := readFixture(t, "rich.xml")
	cases := map[string]string{
		"左の列": "<p>左の本文。</p>",
		"まとめ": "<blockquote><p>引用の段落。</p></blockquote><p>最後の段落。</p>",
	}
	for title, want := range cases {
		if got := sectionContent(t, body, title); got != want {
			t.Errorf("section %q = %q, want %q", title, got, want)
		}
	}
}

func TestFindSection_Ambiguous(t *testing.T) {
	body := "<h2>X</h2><p>1</p><h2> X </h2>"
	_, err := FindSection(body, "X")
	de := wantValidation(t, err)
	for _, off := range []string{"0", "18"} {
		if !strings.Contains(de.Message, off) {
			t.Errorf("message %q does not contain offset %s", de.Message, off)
		}
	}
	if strings.Count(de.Message, "h2") < 2 {
		t.Errorf("message %q does not list both headings", de.Message)
	}
}

func TestFindSection_IgnoresNestedSameName(t *testing.T) {
	for _, name := range []string{"rich.xml", "rich-nolayout.xml"} {
		body := readFixture(t, name)
		s, err := FindSection(body, "背景")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := strings.Index(body, "<h2>背景</h2>"); s.Heading.Start != want {
			t.Errorf("%s: Heading.Start = %d, want %d", name, s.Heading.Start, want)
		}
		if got := body[s.ContentStart:s.ContentEnd]; !strings.Contains(got, "<h3>詳細</h3>") || strings.Contains(got, "手順</h2>") {
			t.Errorf("%s: section = %q", name, got)
		}
	}
}

func TestFindSection_EmptyTitle(t *testing.T) {
	body := "<h2></h2><p>x</p>"
	for _, title := range []string{"", "  ", "\u00a0\t"} {
		_, err := FindSection(body, title)
		de := wantValidation(t, err)
		if !strings.Contains(de.Message, "見出しを指定") {
			t.Errorf("message = %q", de.Message)
		}
		_, _, err = ReplaceSection(body, title, "<p/>")
		wantValidation(t, err)
	}
}

func TestFindSection_NotFound(t *testing.T) {
	for _, body := range []string{"", "<p>x</p>", `<ac:structured-macro ac:name="expand"><ac:rich-text-body><h2>X</h2></ac:rich-text-body></ac:structured-macro>`} {
		_, err := FindSection(body, "X")
		de := wantValidation(t, err)
		if !strings.Contains(de.Message, "見出しがありません") {
			t.Errorf("message = %q", de.Message)
		}
	}
}

func TestReplaceSection_PreservesOutsideBytes(t *testing.T) {
	const content = "<p>新しい本文</p>"
	for _, name := range []string{"rich.xml", "rich-nolayout.xml"} {
		body := readFixture(t, name)
		hs, err := Headings(body)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hs {
			if !h.TopLevel {
				continue
			}
			s, err := FindSection(body, h.Text)
			if err != nil {
				t.Fatalf("%s %q: %v", name, h.Text, err)
			}
			got, old, err := ReplaceSection(body, h.Text, content)
			if err != nil {
				t.Fatalf("%s %q: %v", name, h.Text, err)
			}
			if old != body[s.ContentStart:s.ContentEnd] {
				t.Errorf("%s %q: replaced content = %q", name, h.Text, old)
			}
			if got[:s.ContentStart] != body[:s.ContentStart] {
				t.Errorf("%s %q: prefix changed", name, h.Text)
			}
			if got[len(got)-(len(body)-s.ContentEnd):] != body[s.ContentEnd:] {
				t.Errorf("%s %q: suffix changed", name, h.Text)
			}
			if got[s.ContentStart:s.ContentStart+len(content)] != content {
				t.Errorf("%s %q: new content not at ContentStart", name, h.Text)
			}
			if err := Validate(got); err != nil {
				t.Errorf("%s %q: result is not well-formed: %v", name, h.Text, err)
			}
		}
	}
}

func TestReplaceSection_Errors(t *testing.T) {
	_, _, err := ReplaceSection("<h2>X</h2><h2>X</h2>", "X", "<p/>")
	wantValidation(t, err)
	_, _, err = ReplaceSection("", "X", "<p/>")
	wantValidation(t, err)
}

func TestAppendPrepend_EmptyBody(t *testing.T) {
	if got := Append("", "<p>x</p>"); got != "<p>x</p>" {
		t.Errorf("Append = %q", got)
	}
	if got := Prepend("", "<p>x</p>"); got != "<p>x</p>" {
		t.Errorf("Prepend = %q", got)
	}
	if got := Append("<p>a</p>", "<p>b</p>"); got != "<p>a</p><p>b</p>" {
		t.Errorf("Append = %q", got)
	}
	if got := Prepend("<p>a</p>", "<p>b</p>"); got != "<p>b</p><p>a</p>" {
		t.Errorf("Prepend = %q", got)
	}
}

// testdata/private には実際のページの storage 形式を置く。公開できないため git の管理外で、無い環境ではスキップする。
func TestSections_PrivateFixtures(t *testing.T) {
	files, err := filepath.Glob("../../testdata/private/*.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("testdata/private にフィクスチャがありません")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		body := string(b)
		hs, err := Headings(body)
		if err != nil {
			t.Errorf("%s: Headings = %v", filepath.Base(f), err)
			continue
		}
		count := map[string]int{}
		for _, h := range hs {
			if h.TopLevel {
				count[h.Text]++
			}
		}
		t.Logf("%s: %d headings, %d distinct top-level texts", filepath.Base(f), len(hs), len(count))
		for _, h := range hs {
			if !h.TopLevel {
				continue
			}
			s, err := FindSection(body, h.Text)
			if h.Text == "" || count[h.Text] > 1 {
				wantValidation(t, err)
				continue
			}
			if err != nil {
				t.Errorf("%s: FindSection(heading at %d) = %v", filepath.Base(f), h.Start, err)
				continue
			}
			same, _, err := ReplaceSection(body, h.Text, body[s.ContentStart:s.ContentEnd])
			if err != nil || same != body {
				t.Errorf("%s: ReplaceSection with the same content changed the body (heading at %d, err %v)", filepath.Base(f), h.Start, err)
			}
			replaced, _, err := ReplaceSection(body, h.Text, "<p>x</p>")
			if err != nil {
				t.Errorf("%s: ReplaceSection(heading at %d) = %v", filepath.Base(f), h.Start, err)
				continue
			}
			if err := Validate(replaced); err != nil {
				t.Errorf("%s: replacing heading at %d makes the body malformed: %v", filepath.Base(f), h.Start, err)
			}
		}
	}
}

func TestHeadingsAndFindSection_ClosingWrapperIsError(t *testing.T) {
	body := `<p>a</p></root><h2>X</h2><root>`
	_, err := Headings(body)
	wantValidation(t, err)
	_, err = FindSection(body, "X")
	wantValidation(t, err)
}

func TestHeadings_BrIsSpace(t *testing.T) {
	hs, err := Headings(`<h2>A<br/>B</h2><h2>C<br />D</h2>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 2 || hs[0].Text != "A B" || hs[1].Text != "C D" {
		t.Fatalf("headings = %+v", hs)
	}
	if _, err := FindSection(`<h2>A<br/>B</h2><p>x</p>`, "A B"); err != nil {
		t.Fatal(err)
	}
}
