package storage

import (
	"reflect"
	"testing"
)

func TestCountCommentMarkers(t *testing.T) {
	if got := CountCommentMarkers(readFixture(t, "rich.xml")); got != 2 {
		t.Errorf("rich.xml = %d, want 2", got)
	}
	frag := `<p><ac:inline-comment-marker ac:ref="a">x</ac:inline-comment-marker></p><p>y</p><p><ac:inline-comment-marker ac:ref="b">z</ac:inline-comment-marker></p>`
	if got := CountCommentMarkers(frag); got != 2 {
		t.Errorf("fragment = %d, want 2", got)
	}
	for _, body := range []string{"", "<p>x</p>", "<p>unclosed"} {
		if got := CountCommentMarkers(body); got != 0 {
			t.Errorf("%q = %d, want 0", body, got)
		}
	}
}

func TestLossyElements_Rich(t *testing.T) {
	want := []string{"ac:layout", "macro:status", "inline-comment", "macro:anchor", "table:merged-cells", "macro:expand"}
	if got := LossyElements(readFixture(t, "rich.xml")); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	got := LossyElements(readFixture(t, "rich-nolayout.xml"))
	for _, g := range got {
		if g == "ac:layout" {
			t.Errorf("rich-nolayout reports ac:layout: %v", got)
		}
	}
}

func TestLossyElements_ConvertibleOnly(t *testing.T) {
	body := `<ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[x]]></ac:plain-text-body></ac:structured-macro>` +
		`<ac:structured-macro ac:name="info"><ac:rich-text-body><p>a</p></ac:rich-text-body></ac:structured-macro>` +
		`<ac:structured-macro ac:name="toc"/><ac:task-list><ac:task><ac:task-body>t</ac:task-body></ac:task></ac:task-list>` +
		`<table><tr><td colspan="1">a</td></tr></table>`
	if got := LossyElements(body); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestLossyElements_Dedup(t *testing.T) {
	body := `<ac:structured-macro ac:name="status"/><ac:structured-macro ac:name="status"/><table><tr><td rowspan="2"/><td colspan="3"/></tr></table>`
	want := []string{"macro:status", "table:merged-cells"}
	if got := LossyElements(body); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLostText_IgnoresAttributeChanges(t *testing.T) {
	sent := `<p>hello&nbsp;world</p><ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter><ac:plain-text-body><![CDATA[a := 1]]></ac:plain-text-body></ac:structured-macro>`
	got := `<p>hello world</p><ac:structured-macro ac:name="code" ac:macro-id="x"><ac:parameter ac:name="language">go</ac:parameter><ac:plain-text-body><![CDATA[a := 1]]></ac:plain-text-body></ac:structured-macro>`
	if lost := LostText(sent, got); len(lost) != 0 {
		t.Errorf("lost = %v", lost)
	}
	rich := readFixture(t, "rich.xml")
	if lost := LostText(rich, rich); len(lost) != 0 {
		t.Errorf("same body lost = %v", lost)
	}
}

func TestLostText_ReportsMissing(t *testing.T) {
	sent := `<p>one two</p><p>three four</p>`
	got := `<p>one two</p>`
	want := []string{"three", "four"}
	if lost := LostText(sent, got); !reflect.DeepEqual(lost, want) {
		t.Errorf("lost = %v, want %v", lost, want)
	}
}

func TestLostText_Multiplicity(t *testing.T) {
	if lost := LostText("<p>a b a</p>", "<p>a b</p>"); !reflect.DeepEqual(lost, []string{"a"}) {
		t.Errorf("lost = %v", lost)
	}
	if lost := LostText("<p>a</p>", "<p>a a</p>"); len(lost) != 0 {
		t.Errorf("lost = %v", lost)
	}
}

func TestLostText_ParameterAndMalformed(t *testing.T) {
	sent := `<ac:structured-macro ac:name="status"><ac:parameter ac:name="title">done</ac:parameter></ac:structured-macro>`
	if lost := LostText(sent, "<p/>"); !reflect.DeepEqual(lost, []string{"done"}) {
		t.Errorf("lost = %v", lost)
	}
	// 解析できない本文は、取り出せた範囲のテキストで比べる。
	if lost := LostText("<p>a b", "<p>a</p>"); !reflect.DeepEqual(lost, []string{"b"}) {
		t.Errorf("lost = %v", lost)
	}
}
