package usecase

import (
	"context"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type fakePageGetter struct {
	page        domain.Page
	gotBody     bool
	gotVersion  int
	calledTimes int
}

func (f *fakePageGetter) GetPage(_ context.Context, _ domain.PageID, body bool, version int) (domain.Page, error) {
	f.calledTimes++
	f.gotBody, f.gotVersion = body, version
	return f.page, nil
}

func TestPageQuery_StorageContent(t *testing.T) {
	g := &fakePageGetter{page: domain.Page{ID: "1", Body: "<p>hello</p>"}}
	v, err := NewPageQuery(g).Get(context.Background(), "1", GetRequest{Format: FormatStorage, Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	if v.Content != "<p>hello</p>" || v.Page.ID != "1" {
		t.Fatalf("view = %+v", v)
	}
	if !g.gotBody || g.gotVersion != 3 {
		t.Fatalf("body = %v, version = %d", g.gotBody, g.gotVersion)
	}
}

const headingBody = "<h1>A</h1><p>a1</p><h2>A-1</h2><p>a2</p><h1>B</h1><p>b1</p>"

func get(t *testing.T, body string, r GetRequest) (PageView, error) {
	t.Helper()
	g := &fakePageGetter{page: domain.Page{ID: "1", Body: body}}
	return NewPageQuery(g).Get(context.Background(), "1", r)
}

func TestPageQuery_MarkdownDefault(t *testing.T) {
	v, err := get(t, "<h1>A</h1><p>hello</p>", GetRequest{Format: FormatMarkdown})
	if err != nil {
		t.Fatal(err)
	}
	if v.Content != "# A\n\nhello\n" {
		t.Fatalf("content = %q", v.Content)
	}
}

func TestPageQuery_JSONContentFormat(t *testing.T) {
	v, err := get(t, "<p>hello</p>", GetRequest{Format: FormatJSON})
	if err != nil || v.Content != "hello\n" {
		t.Fatalf("markdown default: %q, %v", v.Content, err)
	}
	v, err = get(t, "<p>hello</p>", GetRequest{Format: FormatJSON, ContentFormat: FormatStorage})
	if err != nil || v.Content != "<p>hello</p>" {
		t.Fatalf("storage: %q, %v", v.Content, err)
	}
}

func TestPageQuery_Outline(t *testing.T) {
	v, err := get(t, headingBody, GetRequest{Format: FormatMarkdown, Outline: true})
	if err != nil {
		t.Fatal(err)
	}
	if v.Content != "" || len(v.Outline) != 3 || v.Outline[1] != (OutlineItem{2, "A-1"}) {
		t.Fatalf("view = %+v", v)
	}
	v, err = get(t, "<p>x</p><ac:structured-macro ac:name=\"info\"><ac:rich-text-body><h2>M</h2></ac:rich-text-body></ac:structured-macro>", GetRequest{Format: FormatMarkdown, Outline: true})
	if err != nil || len(v.Outline) != 0 {
		t.Fatalf("macro heading: %+v, %v", v, err)
	}
	v, err = get(t, "", GetRequest{Format: FormatMarkdown, Outline: true})
	if err != nil || v.Outline == nil || len(v.Outline) != 0 {
		t.Fatalf("empty: %+v, %v", v, err)
	}
}

func TestPageQuery_SectionMarkdown(t *testing.T) {
	v, err := get(t, headingBody, GetRequest{Format: FormatMarkdown, Section: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Content != "a1\n\n## A-1\n\na2\n" {
		t.Fatalf("content = %q", v.Content)
	}
}

func TestPageQuery_SectionStorageExact(t *testing.T) {
	v, err := get(t, headingBody, GetRequest{Format: FormatStorage, Section: "B"})
	if err != nil || v.Content != "<p>b1</p>" {
		t.Fatalf("content = %q, %v", v.Content, err)
	}
}

func TestPageQuery_SectionNotFound(t *testing.T) {
	for _, body := range []string{headingBody, ""} {
		_, err := get(t, body, GetRequest{Format: FormatMarkdown, Section: "none"})
		if domain.KindOf(err) != domain.ErrValidation {
			t.Errorf("err = %v", err)
		}
	}
}

func TestPageQuery_InvalidRequest(t *testing.T) {
	for _, r := range []GetRequest{
		{Format: "html"},
		{Format: FormatJSON, ContentFormat: "html"},
		{Format: FormatMarkdown, ContentFormat: FormatStorage},
		{Format: FormatStorage, ContentFormat: FormatMarkdown},
		{Format: FormatMarkdown, Outline: true, Section: "A"},
		{Format: FormatStorage, Outline: true},
	} {
		g := &fakePageGetter{}
		_, err := NewPageQuery(g).Get(context.Background(), "1", r)
		if domain.KindOf(err) != domain.ErrValidation || g.calledTimes != 0 {
			t.Errorf("%+v: err = %v, calls = %d", r, err, g.calledTimes)
		}
	}
}
