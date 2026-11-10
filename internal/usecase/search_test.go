package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/cql"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// fakeSearcher は呼び出しごとに errs の先頭から順にエラーを返し、渡された CQL を記録する。
type fakeSearcher struct {
	errs  []error
	hits  []domain.SearchHit
	calls []string
}

func (f *fakeSearcher) Search(_ context.Context, q string, _ int, _ bool) ([]domain.SearchHit, error) {
	f.calls = append(f.calls, q)
	if i := len(f.calls) - 1; i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	return f.hits, nil
}

func TestSearch_FallsBackToTextOnValidationError(t *testing.T) {
	fs := &fakeSearcher{
		errs: []error{&domain.Error{Kind: domain.ErrValidation, Status: 400}},
		hits: []domain.SearchHit{{ID: "1"}},
	}
	hits, warnings, err := NewSearchService(fs).Search(context.Background(), cql.Filter{Mode: cql.ModeSite, Query: "foo"}, 25, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.calls) != 2 {
		t.Fatalf("calls = %q", fs.calls)
	}
	if !strings.HasPrefix(fs.calls[0], `siteSearch ~ "foo"`) || !strings.HasPrefix(fs.calls[1], `text ~ "foo"`) {
		t.Fatalf("calls = %q", fs.calls)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %q", warnings)
	}
	if len(hits) != 1 || hits[0].ID != "1" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestSearch_NoFallbackForOtherModes(t *testing.T) {
	want := &domain.Error{Kind: domain.ErrValidation, Status: 400}
	fs := &fakeSearcher{errs: []error{want}}
	_, warnings, err := NewSearchService(fs).Search(context.Background(), cql.Filter{Mode: cql.ModeTitle, Query: "foo"}, 25, false)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	if len(fs.calls) != 1 || len(warnings) != 0 {
		t.Fatalf("calls = %q, warnings = %q", fs.calls, warnings)
	}
}

func TestSearch_NoFallbackForNonValidationError(t *testing.T) {
	want := &domain.Error{Kind: domain.ErrAuth, Status: 401}
	fs := &fakeSearcher{errs: []error{want}}
	_, _, err := NewSearchService(fs).Search(context.Background(), cql.Filter{Mode: cql.ModeSite, Query: "foo"}, 25, false)
	if !errors.Is(err, want) || len(fs.calls) != 1 {
		t.Fatalf("err = %v, calls = %q", err, fs.calls)
	}
}

func TestSearch_NoFallbackForRawCQL(t *testing.T) {
	want := &domain.Error{Kind: domain.ErrValidation, Status: 400}
	fs := &fakeSearcher{errs: []error{want}}
	_, _, err := NewSearchService(fs).Search(context.Background(), cql.Filter{Mode: cql.ModeSite, RawCQL: "type = page"}, 25, false)
	if !errors.Is(err, want) || len(fs.calls) != 1 {
		t.Fatalf("err = %v, calls = %q", err, fs.calls)
	}
}

func TestSearch_BuildErrorDoesNotCallAPI(t *testing.T) {
	fs := &fakeSearcher{}
	_, _, err := NewSearchService(fs).Search(context.Background(), cql.Filter{}, 25, false)
	if domain.KindOf(err) != domain.ErrValidation || len(fs.calls) != 0 {
		t.Fatalf("err = %v, calls = %q", err, fs.calls)
	}
}
