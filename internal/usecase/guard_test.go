package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type fakeSpaceIDFinder struct {
	spaces map[string]domain.Space
	calls  int
}

func (f *fakeSpaceIDFinder) SpaceByID(_ context.Context, id string) (domain.Space, error) {
	f.calls++
	s, ok := f.spaces[id]
	if !ok {
		return domain.Space{}, &domain.Error{Kind: domain.ErrNotFound}
	}
	return s, nil
}

func newSpaces() *fakeSpaceIDFinder {
	return &fakeSpaceIDFinder{spaces: map[string]domain.Space{
		"100": {ID: "100", Key: "DOC"},
		"200": {ID: "200", Key: "OTHER"},
	}}
}

func TestGuard_ReadOnly(t *testing.T) {
	s := newSpaces()
	err := NewWriteGuard(WritePolicy{ReadOnly: true}, s).Check(context.Background(), "100")
	if domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("err = %v", err)
	}
	if s.calls != 0 {
		t.Fatalf("SpaceByID called %d times", s.calls)
	}
}

func TestGuard_NoRestriction(t *testing.T) {
	s := newSpaces()
	if err := NewWriteGuard(WritePolicy{}, s).Check(context.Background(), "100"); err != nil {
		t.Fatal(err)
	}
	if s.calls != 0 {
		t.Fatalf("SpaceByID called %d times", s.calls)
	}
}

func TestGuard_WritableSpaces(t *testing.T) {
	s := newSpaces()
	g := NewWriteGuard(WritePolicy{WritableSpaces: []string{"doc"}}, s)
	if err := g.Check(context.Background(), "100"); err != nil {
		t.Fatalf("allowed space: %v", err)
	}
	err := g.Check(context.Background(), "200")
	if domain.KindOf(err) != domain.ErrReadOnly || !strings.Contains(err.Error(), "OTHER") {
		t.Fatalf("disallowed space: %v", err)
	}

	err = NewWriteGuard(WritePolicy{WritableSpaces: []string{}}, s).Check(context.Background(), "100")
	if domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("empty list: %v", err)
	}

	// 許可リスト外のスペースへの作成では PageWriter が呼ばれない。
	w := &fakePageWriter{}
	c := NewPageCommand(&fakeWriteGetter{}, w, g)
	_, err = c.Create(context.Background(), CreateRequest{SpaceID: "200", Title: "t", Body: "<p>x</p>", Input: InputStorage})
	if domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("create: %v", err)
	}
	if w.creates != 0 {
		t.Fatalf("CreatePage called %d times", w.creates)
	}
}

func TestGuard_SpaceLookupError(t *testing.T) {
	err := NewWriteGuard(WritePolicy{WritableSpaces: []string{"DOC"}}, newSpaces()).Check(context.Background(), "999")
	if domain.KindOf(err) != domain.ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}
