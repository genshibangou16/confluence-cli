package usecase

import (
	"context"
	"reflect"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type fakeLabelReader struct {
	got []any
}

func (f *fakeLabelReader) Labels(_ context.Context, prefix string, limit int, all bool) ([]domain.Label, error) {
	f.got = []any{prefix, limit, all}
	return []domain.Label{{ID: "10", Name: "a"}}, nil
}

func (f *fakeLabelReader) PageLabels(_ context.Context, id domain.PageID, limit int, all bool) ([]domain.Label, error) {
	f.got = []any{id, limit, all}
	return []domain.Label{{ID: "11", Name: "b"}}, nil
}

func (f *fakeLabelReader) PagesWithLabel(_ context.Context, labelID, spaceID string, limit int, all bool) ([]domain.PageSummary, error) {
	f.got = []any{labelID, spaceID, limit, all}
	return []domain.PageSummary{{ID: "1"}}, nil
}

func TestLabelQuery_PassesArguments(t *testing.T) {
	r := &fakeLabelReader{}
	q := NewLabelQuery(r)
	ctx := context.Background()

	if ls, err := q.List(ctx, "global", 10, false); err != nil || ls[0].ID != "10" || !reflect.DeepEqual(r.got, []any{"global", 10, false}) {
		t.Fatalf("List: %+v, %v, %v", ls, err, r.got)
	}
	if ls, err := q.OfPage(ctx, "5", 20, true); err != nil || ls[0].ID != "11" || !reflect.DeepEqual(r.got, []any{domain.PageID("5"), 20, true}) {
		t.Fatalf("OfPage: %+v, %v, %v", ls, err, r.got)
	}
	if ps, err := q.Pages(ctx, "10", "100", 30, false); err != nil || ps[0].ID != "1" || !reflect.DeepEqual(r.got, []any{"10", "100", 30, false}) {
		t.Fatalf("Pages: %+v, %v, %v", ps, err, r.got)
	}
}

type fakeLabelWriter struct {
	added   []string
	removed []string
	calls   int
}

func (f *fakeLabelWriter) AddLabels(_ context.Context, _ domain.PageID, names []string) error {
	f.calls++
	f.added = names
	return nil
}

func (f *fakeLabelWriter) RemoveLabel(_ context.Context, _ domain.PageID, name string) error {
	f.calls++
	f.removed = append(f.removed, name)
	return nil
}

func newLabelCommand(policy WritePolicy) (*LabelCommand, *fakeWriteGetter, *fakeLabelWriter) {
	g := &fakeWriteGetter{pages: map[domain.PageID]domain.Page{"10": {ID: "10", SpaceID: "100"}, "20": {ID: "20", SpaceID: "200"}}}
	w := &fakeLabelWriter{}
	return NewLabelCommand(g, w, NewWriteGuard(policy, newSpaces())), g, w
}

func TestLabelCommand_AddRemove(t *testing.T) {
	c, _, w := newLabelCommand(WritePolicy{WritableSpaces: []string{"DOC"}})
	ctx := context.Background()
	if err := c.Add(ctx, "10", []string{"a", "b"}); err != nil || !reflect.DeepEqual(w.added, []string{"a", "b"}) {
		t.Fatalf("Add: %v, %v", err, w.added)
	}
	if err := c.Remove(ctx, "10", "a"); err != nil || !reflect.DeepEqual(w.removed, []string{"a"}) {
		t.Fatalf("Remove: %v, %v", err, w.removed)
	}
}

func TestLabelCommand_ReadOnlyNoAPICall(t *testing.T) {
	c, g, w := newLabelCommand(WritePolicy{ReadOnly: true})
	ctx := context.Background()
	if err := c.Add(ctx, "10", []string{"a"}); domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("Add: %v", err)
	}
	if err := c.Remove(ctx, "10", "a"); domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("Remove: %v", err)
	}
	if len(g.calls) != 0 || w.calls != 0 {
		t.Fatalf("API called: get %v, write %d", g.calls, w.calls)
	}
}

func TestLabelCommand_DisallowedSpace(t *testing.T) {
	c, _, w := newLabelCommand(WritePolicy{WritableSpaces: []string{"DOC"}})
	ctx := context.Background()
	if err := c.Add(ctx, "20", []string{"a"}); domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("Add: %v", err)
	}
	if err := c.Remove(ctx, "20", "a"); domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("Remove: %v", err)
	}
	if w.calls != 0 {
		t.Fatalf("writer called %d times", w.calls)
	}
}

func TestLabelCommand_Validation(t *testing.T) {
	c, g, w := newLabelCommand(WritePolicy{})
	ctx := context.Background()
	if err := c.Add(ctx, "10", nil); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("Add no names: %v", err)
	}
	if err := c.Add(ctx, "10", []string{"a", " "}); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("Add blank name: %v", err)
	}
	if err := c.Remove(ctx, "10", ""); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("Remove blank: %v", err)
	}
	if len(g.calls) != 0 || w.calls != 0 {
		t.Fatalf("API called: get %v, write %d", g.calls, w.calls)
	}
}
