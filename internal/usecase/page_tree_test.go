package usecase

import (
	"context"
	"reflect"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type fakePageLister struct {
	ancestors  []domain.Ancestor
	pages      map[domain.PageID]domain.PageSummary
	byIDsCalls [][]domain.PageID
}

func (f *fakePageLister) ListPages(context.Context, string, int, bool) ([]domain.PageSummary, error) {
	return nil, nil
}

func (f *fakePageLister) Children(context.Context, domain.PageID, int, bool) ([]domain.PageSummary, error) {
	return nil, nil
}

func (f *fakePageLister) Ancestors(context.Context, domain.PageID) ([]domain.Ancestor, error) {
	return f.ancestors, nil
}

func (f *fakePageLister) GetPagesByIDs(_ context.Context, ids []domain.PageID) ([]domain.PageSummary, error) {
	f.byIDsCalls = append(f.byIDsCalls, ids)
	var out []domain.PageSummary
	for _, id := range ids {
		if p, ok := f.pages[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakePageLister) Versions(context.Context, domain.PageID, int, bool) ([]domain.Version, error) {
	return nil, nil
}

func TestPageTree_AncestorsWithTitles(t *testing.T) {
	l := &fakePageLister{
		ancestors: []domain.Ancestor{{ID: "1", Type: "folder"}, {ID: "2", Type: "page"}, {ID: "3", Type: "page"}},
		pages: map[domain.PageID]domain.PageSummary{
			"2": {ID: "2", Title: "親"},
			"3": {ID: "3", Title: "子"},
		},
	}
	got, err := NewPageTree(l).Ancestors(context.Background(), "4")
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]domain.PageID{{"2", "3"}}; !reflect.DeepEqual(l.byIDsCalls, want) {
		t.Fatalf("GetPagesByIDs calls = %v", l.byIDsCalls)
	}
	want := []domain.Ancestor{{ID: "1", Type: "folder"}, {ID: "2", Type: "page", Title: "親"}, {ID: "3", Type: "page", Title: "子"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestPageTree_AncestorsWithoutPages(t *testing.T) {
	l := &fakePageLister{ancestors: []domain.Ancestor{{ID: "1", Type: "folder"}}}
	got, err := NewPageTree(l).Ancestors(context.Background(), "4")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.byIDsCalls) != 0 || len(got) != 1 {
		t.Fatalf("calls = %v, got = %+v", l.byIDsCalls, got)
	}
}

func TestPageTree_Pages(t *testing.T) {
	l := &fakePageLister{pages: map[domain.PageID]domain.PageSummary{"2": {ID: "2", SpaceID: "10"}}}
	got, err := NewPageTree(l).Pages(context.Background(), []domain.PageID{"2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []domain.PageSummary{{ID: "2", SpaceID: "10"}}) {
		t.Fatalf("got %+v", got)
	}
	if _, err := NewPageTree(l).Pages(context.Background(), nil); err != nil || len(l.byIDsCalls) != 1 {
		t.Fatalf("empty ids: err = %v, calls = %v", err, l.byIDsCalls)
	}
}
