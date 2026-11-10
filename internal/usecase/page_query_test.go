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

func TestPageQuery_JSONContent(t *testing.T) {
	g := &fakePageGetter{page: domain.Page{ID: "1", Body: "<p>hello</p>"}}
	v, err := NewPageQuery(g).Get(context.Background(), "1", GetRequest{Format: FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	if v.Content != "<p>hello</p>" {
		t.Fatalf("view = %+v", v)
	}
}

func TestPageQuery_UnsupportedRequest(t *testing.T) {
	for _, r := range []GetRequest{
		{Format: FormatMarkdown},
		{Format: "html"},
		{Format: FormatStorage, Outline: true},
		{Format: FormatStorage, Section: "概要"},
	} {
		g := &fakePageGetter{}
		_, err := NewPageQuery(g).Get(context.Background(), "1", r)
		if domain.KindOf(err) != domain.ErrValidation || g.calledTimes != 0 {
			t.Errorf("%+v: err = %v, calls = %d", r, err, g.calledTimes)
		}
	}
}
