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
