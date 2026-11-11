package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/convert/frommd"
	"github.com/genshibangou16/confluence-cli/internal/domain"
	"github.com/genshibangou16/confluence-cli/internal/storage"
)

// fakePageWriter は CreatePage で受け取った本文をそのまま持つページを返す。
type fakePageWriter struct {
	creates            int
	gotSpace, gotTitle string
	gotParent          domain.PageID
	gotBody            string
	updates, deletes   int
}

func (f *fakePageWriter) CreatePage(_ context.Context, spaceID string, parentID domain.PageID, title, body string) (domain.Page, error) {
	f.creates++
	f.gotSpace, f.gotParent, f.gotTitle, f.gotBody = spaceID, parentID, title, body
	return domain.Page{ID: "900", SpaceID: spaceID, Title: title}, nil
}

func (f *fakePageWriter) UpdatePage(context.Context, domain.PageID, string, string, int, string) (domain.Page, error) {
	f.updates++
	return domain.Page{}, nil
}

func (f *fakePageWriter) DeletePage(context.Context, domain.PageID) error {
	f.deletes++
	return nil
}

// fakeWriteGetter は ID ごとのページを返す。readBack が空でない ID は、その本文を読み戻しの本文として返す。
type fakeWriteGetter struct {
	pages       map[domain.PageID]domain.Page
	readBack    map[domain.PageID]string
	readBackErr error
	w           *fakePageWriter
	calls       []domain.PageID
}

func (f *fakeWriteGetter) GetPage(_ context.Context, id domain.PageID, body bool, _ int) (domain.Page, error) {
	f.calls = append(f.calls, id)
	if p, ok := f.pages[id]; ok {
		return p, nil
	}
	if f.w != nil && id == "900" {
		if f.readBackErr != nil {
			return domain.Page{}, f.readBackErr
		}
		p := domain.Page{ID: id, SpaceID: f.w.gotSpace, Title: f.w.gotTitle}
		if body {
			p.Body = f.w.gotBody
			if b, ok := f.readBack[id]; ok {
				p.Body = b
			}
		}
		return p, nil
	}
	return domain.Page{}, &domain.Error{Kind: domain.ErrNotFound}
}

func newCommand(policy WritePolicy) (*PageCommand, *fakeWriteGetter, *fakePageWriter) {
	w := &fakePageWriter{}
	g := &fakeWriteGetter{w: w, pages: map[domain.PageID]domain.Page{"10": {ID: "10", SpaceID: "100"}}}
	return NewPageCommand(g, w, NewWriteGuard(policy, newSpaces())), g, w
}

func TestCreate_InvalidStorageNoAPICall(t *testing.T) {
	c, g, w := newCommand(WritePolicy{})
	_, err := c.Create(context.Background(), CreateRequest{ParentID: "10", Title: "t", Body: "<p>x", Input: InputStorage})
	var se *storage.SyntaxError
	if !errors.As(err, &se) || domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("err = %v", err)
	}
	if w.creates != 0 || len(g.calls) != 0 {
		t.Fatalf("API called: creates=%d gets=%v", w.creates, g.calls)
	}
}

func TestCreate_MarkdownConverted(t *testing.T) {
	c, _, w := newCommand(WritePolicy{})
	md := "# 見出し\n\n本文の *段落*\n"
	res, err := c.Create(context.Background(), CreateRequest{SpaceID: "100", Title: "t", Body: md, Input: InputMarkdown})
	if err != nil {
		t.Fatal(err)
	}
	want, err := frommd.Convert(md)
	if err != nil {
		t.Fatal(err)
	}
	if w.gotBody != want {
		t.Fatalf("body = %q, want %q", w.gotBody, want)
	}
	if res.Page.ID != "900" || len(res.Lost) != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestCreate_VerifyReportsLostText(t *testing.T) {
	c, g, _ := newCommand(WritePolicy{})
	g.readBack = map[domain.PageID]string{"900": "<p>残る</p>"}
	res, err := c.Create(context.Background(), CreateRequest{SpaceID: "100", Title: "t", Body: "<p>残る</p><p>消える 段落</p>", Input: InputStorage})
	if domain.KindOf(err) != domain.ErrVerifyFailed || !strings.Contains(err.Error(), "書き込み後の本文から次のテキストが失われました: 消える 段落") {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(res.Lost, []string{"消える", "段落"}) || res.Page.ID != "900" {
		t.Fatalf("result = %+v", res)
	}
}

func TestCreate_LostTextMessageTruncated(t *testing.T) {
	c, g, _ := newCommand(WritePolicy{})
	g.readBack = map[domain.PageID]string{"900": ""}
	words := make([]string, 30)
	for i := range words {
		words[i] = "w" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
	}
	_, err := c.Create(context.Background(), CreateRequest{SpaceID: "100", Title: "t", Body: "<p>" + strings.Join(words, " ") + "</p>", Input: InputStorage})
	if err == nil || strings.Contains(err.Error(), words[20]) || !strings.Contains(err.Error(), words[19]) || !strings.Contains(err.Error(), "ほか 10 語") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreate_ParentSpace(t *testing.T) {
	c, g, w := newCommand(WritePolicy{WritableSpaces: []string{"DOC"}})
	if _, err := c.Create(context.Background(), CreateRequest{ParentID: "10", Title: "t", Body: "<p>x</p>", Input: InputStorage}); err != nil {
		t.Fatal(err)
	}
	if w.gotSpace != "100" || w.gotParent != "10" || g.calls[0] != "10" {
		t.Fatalf("space = %q parent = %q gets = %v", w.gotSpace, w.gotParent, g.calls)
	}

	c, _, w = newCommand(WritePolicy{})
	_, err := c.Create(context.Background(), CreateRequest{SpaceID: "200", ParentID: "10", Title: "t", Body: "<p>x</p>", Input: InputStorage})
	if domain.KindOf(err) != domain.ErrValidation || w.creates != 0 {
		t.Fatalf("space mismatch: %v, creates=%d", err, w.creates)
	}
}

func TestCreate_Validation(t *testing.T) {
	for name, r := range map[string]CreateRequest{
		"no space":    {Title: "t", Body: "<p>x</p>", Input: InputStorage},
		"blank title": {SpaceID: "100", Title: " \t", Body: "<p>x</p>", Input: InputStorage},
		"no title":    {SpaceID: "100", Body: "<p>x</p>", Input: InputStorage},
		"unknown fmt": {SpaceID: "100", Title: "t", Body: "<p>x</p>", Input: "html"},
	} {
		c, g, w := newCommand(WritePolicy{})
		_, err := c.Create(context.Background(), r)
		if domain.KindOf(err) != domain.ErrValidation || w.creates != 0 || len(g.calls) != 0 {
			t.Errorf("%s: err=%v creates=%d gets=%v", name, err, w.creates, g.calls)
		}
	}
}

func TestCreate_ReadOnlyNoAPICall(t *testing.T) {
	c, g, w := newCommand(WritePolicy{ReadOnly: true})
	_, err := c.Create(context.Background(), CreateRequest{ParentID: "10", Title: "t", Body: "<p>x</p>", Input: InputStorage})
	if domain.KindOf(err) != domain.ErrReadOnly || w.creates != 0 || len(g.calls) != 0 {
		t.Fatalf("err=%v creates=%d gets=%v", err, w.creates, g.calls)
	}
}

func TestCreate_DisallowedSpaceNoCreate(t *testing.T) {
	c, _, w := newCommand(WritePolicy{WritableSpaces: []string{"DOC"}})
	_, err := c.Create(context.Background(), CreateRequest{SpaceID: "200", Title: "t", Body: "<p>x</p>", Input: InputStorage})
	if domain.KindOf(err) != domain.ErrReadOnly || w.creates != 0 {
		t.Fatalf("err=%v creates=%d", err, w.creates)
	}
}

func TestCreate_ReadBackFailure(t *testing.T) {
	c, g, w := newCommand(WritePolicy{})
	cause := &domain.Error{Kind: domain.ErrRateLimit, Message: "too many"}
	g.readBackErr = cause
	res, err := c.Create(context.Background(), CreateRequest{SpaceID: "100", Title: "t", Body: "<p>x</p>", Input: InputStorage})
	if w.creates != 1 || res.Page.ID != "900" {
		t.Fatalf("creates=%d result=%+v", w.creates, res)
	}
	if domain.KindOf(err) != domain.ErrVerifyFailed || !errors.Is(err, cause) || !strings.Contains(err.Error(), "900") {
		t.Fatalf("err = %v", err)
	}
}

func TestDelete_GuardChecked(t *testing.T) {
	c, _, w := newCommand(WritePolicy{WritableSpaces: []string{"OTHER"}})
	if err := c.Delete(context.Background(), "10"); domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("err = %v", err)
	}
	if w.deletes != 0 {
		t.Fatalf("DeletePage called %d times", w.deletes)
	}

	c, _, w = newCommand(WritePolicy{WritableSpaces: []string{"DOC"}})
	if err := c.Delete(context.Background(), "10"); err != nil || w.deletes != 1 {
		t.Fatalf("err = %v, deletes = %d", err, w.deletes)
	}
}

func TestDelete_ReadOnlyNoAPICall(t *testing.T) {
	c, g, w := newCommand(WritePolicy{ReadOnly: true})
	if err := c.Delete(context.Background(), "10"); domain.KindOf(err) != domain.ErrReadOnly {
		t.Fatalf("err = %v", err)
	}
	if len(g.calls) != 0 || w.deletes != 0 {
		t.Fatalf("API called: get %v, deletes %d", g.calls, w.deletes)
	}
}

func TestDelete_NotFound(t *testing.T) {
	c, _, w := newCommand(WritePolicy{})
	if err := c.Delete(context.Background(), "404"); domain.KindOf(err) != domain.ErrNotFound || w.deletes != 0 {
		t.Fatalf("err = %v, deletes = %d", err, w.deletes)
	}
}

func TestCreate_EmptyConvertedBodyRefused(t *testing.T) {
	c, g, w := newCommand(WritePolicy{})
	_, err := c.Create(context.Background(), CreateRequest{ParentID: "10", Title: "t", Body: "[a]: http://example.com\n", Input: InputMarkdown})
	if domain.KindOf(err) != domain.ErrValidation || w.creates != 0 || len(g.calls) != 0 {
		t.Fatalf("err = %v creates = %d gets = %v", err, w.creates, g.calls)
	}
}

func TestCreate_NoBodyAllowed(t *testing.T) {
	c, _, w := newCommand(WritePolicy{})
	if _, err := c.Create(context.Background(), CreateRequest{SpaceID: "100", Title: "t", Input: InputMarkdown}); err != nil || w.creates != 1 {
		t.Fatalf("err = %v creates = %d", err, w.creates)
	}
}
