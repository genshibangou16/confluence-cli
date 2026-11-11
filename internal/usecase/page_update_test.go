package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/convert/frommd"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

const commentMarker = `<ac:inline-comment-marker ac:ref="r1">注釈</ac:inline-comment-marker>`

// fakeUpdateStore は 1 ページだけを持ち、UpdatePage で受け取った値を記録する。
// 読み戻しでは、readBack が空でなければその本文を、空なら UpdatePage で受け取った本文を返す。
type fakeUpdateStore struct {
	page        domain.Page
	gets        int
	updates     int
	updateErr   error
	readBack    string
	readBackErr error

	gotID      domain.PageID
	gotTitle   string
	gotBody    string
	gotVersion int
	gotMessage string
}

func (f *fakeUpdateStore) GetPage(_ context.Context, id domain.PageID, body bool, _ int) (domain.Page, error) {
	f.gets++
	if id != f.page.ID {
		return domain.Page{}, &domain.Error{Kind: domain.ErrNotFound}
	}
	if f.updates == 0 {
		p := f.page
		if !body {
			p.Body = ""
		}
		return p, nil
	}
	if f.readBackErr != nil {
		return domain.Page{}, f.readBackErr
	}
	p := domain.Page{ID: id, SpaceID: f.page.SpaceID, Title: f.gotTitle, Version: domain.Version{Number: f.gotVersion}, Body: f.gotBody}
	if f.readBack != "" {
		p.Body = f.readBack
	}
	return p, nil
}

func (f *fakeUpdateStore) UpdatePage(_ context.Context, id domain.PageID, title, body string, version int, message string) (domain.Page, error) {
	f.updates++
	if f.updateErr != nil {
		return domain.Page{}, f.updateErr
	}
	f.gotID, f.gotTitle, f.gotBody, f.gotVersion, f.gotMessage = id, title, body, version, message
	return domain.Page{ID: id, SpaceID: f.page.SpaceID, Title: title}, nil
}

func (f *fakeUpdateStore) CreatePage(context.Context, string, domain.PageID, string, string) (domain.Page, error) {
	panic("CreatePage は呼ばれない")
}

func (f *fakeUpdateStore) DeletePage(context.Context, domain.PageID) error {
	panic("DeletePage は呼ばれない")
}

func newUpdateCommand(policy WritePolicy, body string) (*PageCommand, *fakeUpdateStore) {
	s := &fakeUpdateStore{page: domain.Page{ID: "20", SpaceID: "100", Title: "現在のタイトル", Version: domain.Version{Number: 5}, Body: body}}
	return NewPageCommand(s, s, NewWriteGuard(policy, newSpaces())), s
}

func ptr(s string) *string { return &s }

func TestUpdate_TitleOnlyKeepsBodyBytes(t *testing.T) {
	// 空白・実体参照・属性の順序を含む本文が、変換や再整形を経ずにそのまま送られることを確認する。
	body := `<p>a&nbsp;b</p>  <ac:structured-macro ac:name="status" ac:schema-version="1"><ac:parameter ac:name="title">X</ac:parameter></ac:structured-macro>` + commentMarker
	c, s := newUpdateCommand(WritePolicy{}, body)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Title: ptr("新しいタイトル"), Input: InputMarkdown}); err != nil {
		t.Fatal(err)
	}
	if s.gotBody != body || s.gotTitle != "新しいタイトル" {
		t.Fatalf("title = %q body = %q", s.gotTitle, s.gotBody)
	}
}

func TestUpdate_BodyOnlyKeepsTitle(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage}); err != nil {
		t.Fatal(err)
	}
	if s.gotTitle != "現在のタイトル" || s.gotBody != "<p>新</p>" {
		t.Fatalf("title = %q body = %q", s.gotTitle, s.gotBody)
	}
}

func TestUpdate_ExpectedVersionMismatch(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, ExpectedVersion: 4})
	if domain.KindOf(err) != domain.ErrVersionMismatch || !strings.Contains(err.Error(), "4") || !strings.Contains(err.Error(), "5") {
		t.Fatalf("err = %v", err)
	}
	if s.updates != 0 {
		t.Fatalf("updates = %d", s.updates)
	}
}

func TestUpdate_ExpectedVersionZeroNotChecked(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage}); err != nil {
		t.Fatal(err)
	}
	if s.updates != 1 {
		t.Fatalf("updates = %d", s.updates)
	}
	c, s = newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, ExpectedVersion: 5}); err != nil || s.updates != 1 {
		t.Fatalf("matching version: err=%v updates=%d", err, s.updates)
	}
}

func TestUpdate_SendsMessageAndNextVersion(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Message: "誤字の修正"})
	if err != nil {
		t.Fatal(err)
	}
	if s.gotID != "20" || s.gotVersion != 6 || s.gotMessage != "誤字の修正" {
		t.Fatalf("id = %q version = %d message = %q", s.gotID, s.gotVersion, s.gotMessage)
	}
	if res.Page.Version.Number != 6 || res.Page.Body != "<p>新</p>" || len(res.Lost) != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestUpdate_AppendPrependSection(t *testing.T) {
	cur := "<h2>A</h2><p>a</p><h2>B</h2><p>b</p>"
	for name, tc := range map[string]struct {
		mode    UpdateMode
		section string
		want    string
	}{
		"append":  {ModeAppend, "", cur + "<p>新</p>"},
		"prepend": {ModePrepend, "", "<p>新</p>" + cur},
		"section": {ModeSection, "A", "<h2>A</h2><p>新</p><h2>B</h2><p>b</p>"},
	} {
		c, s := newUpdateCommand(WritePolicy{}, cur)
		if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: tc.mode, Section: tc.section}); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if s.gotBody != tc.want {
			t.Errorf("%s: body = %q, want %q", name, s.gotBody, tc.want)
		}
	}
}

func TestUpdate_SectionNotFound(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<h2>A</h2><p>a</p>")
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: ModeSection, Section: "Z"})
	if domain.KindOf(err) != domain.ErrValidation || s.updates != 0 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_RequestValidation(t *testing.T) {
	for name, r := range map[string]UpdateRequest{
		"append without body":  {ID: "20", Input: InputStorage, Mode: ModeAppend},
		"section without name": {ID: "20", Body: ptr("<p>x</p>"), Input: InputStorage, Mode: ModeSection, Section: " "},
		"unknown mode":         {ID: "20", Body: ptr("<p>x</p>"), Input: InputStorage, Mode: UpdateMode(99)},
		"blank title":          {ID: "20", Title: ptr(" "), Input: InputStorage},
		"invalid storage":      {ID: "20", Body: ptr("<p>x"), Input: InputStorage},
	} {
		c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
		_, err := c.Update(context.Background(), r)
		if domain.KindOf(err) != domain.ErrValidation || s.updates != 0 {
			t.Errorf("%s: err = %v updates = %d", name, err, s.updates)
		}
	}
}

func TestUpdate_ReadOnlyNoAPICall(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{ReadOnly: true}, "<p>旧</p>")
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage})
	if domain.KindOf(err) != domain.ErrReadOnly || s.gets != 0 || s.updates != 0 {
		t.Fatalf("err = %v gets = %d updates = %d", err, s.gets, s.updates)
	}
}

func TestUpdate_DisallowedSpaceNoUpdate(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{WritableSpaces: []string{"OTHER"}}, "<p>旧</p>")
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage})
	if domain.KindOf(err) != domain.ErrReadOnly || s.updates != 0 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_SectionWithCommentMarkerRefused(t *testing.T) {
	cur := "<h2>A</h2><p>" + commentMarker + "</p><h2>B</h2><p>b</p>"
	c, s := newUpdateCommand(WritePolicy{}, cur)
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: ModeSection, Section: "A"})
	if domain.KindOf(err) != domain.ErrCommentLoss || !strings.Contains(err.Error(), "1") || !strings.Contains(err.Error(), "--allow-drop-comments") {
		t.Fatalf("err = %v", err)
	}
	if s.updates != 0 {
		t.Fatalf("updates = %d", s.updates)
	}

	// 対象外の節にあるマーカーは置き換えないため、拒否しない。
	c, s = newUpdateCommand(WritePolicy{}, cur)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: ModeSection, Section: "B"}); err != nil || s.updates != 1 {
		t.Fatalf("other section: err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_ReplaceWithCommentMarkerRefused(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>"+commentMarker+"</p>")
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage})
	if domain.KindOf(err) != domain.ErrCommentLoss || s.updates != 0 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_AppendWithCommentMarkerNotChecked(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>"+commentMarker+"</p>")
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: ModeAppend}); err != nil || s.updates != 1 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_SectionWithCommentMarkerAllowed(t *testing.T) {
	cur := "<h2>A</h2><p>" + commentMarker + "</p>"
	c, s := newUpdateCommand(WritePolicy{}, cur)
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: ModeSection, Section: "A", AllowDropComments: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.gotBody != "<h2>A</h2><p>新</p>" {
		t.Fatalf("body = %q", s.gotBody)
	}
}

func TestUpdate_DryRunWarnsOnComments(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<h2>A</h2><p>"+commentMarker+"</p>")
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: ModeSection, Section: "A", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "--allow-drop-comments") || !res.DryRun || s.updates != 0 {
		t.Fatalf("result = %+v updates = %d", res, s.updates)
	}
}

func TestUpdate_MarkdownReplaceLossyRefused(t *testing.T) {
	cur := `<p>状態</p><ac:structured-macro ac:name="status"><ac:parameter ac:name="title">完了</ac:parameter></ac:structured-macro>`
	c, s := newUpdateCommand(WritePolicy{}, cur)
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新しい本文\n"), Input: InputMarkdown})
	if domain.KindOf(err) != domain.ErrLossy || !strings.Contains(err.Error(), "macro:status") || !strings.Contains(err.Error(), "--allow-lossy") {
		t.Fatalf("err = %v", err)
	}
	if s.updates != 0 {
		t.Fatalf("updates = %d", s.updates)
	}

	c, s = newUpdateCommand(WritePolicy{}, cur)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新しい本文\n"), Input: InputMarkdown, AllowLossy: true}); err != nil || s.updates != 1 {
		t.Fatalf("allow lossy: err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_MarkdownAppendNotLossyChecked(t *testing.T) {
	cur := `<ac:structured-macro ac:name="status"><ac:parameter ac:name="title">完了</ac:parameter></ac:structured-macro>`
	c, s := newUpdateCommand(WritePolicy{}, cur)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("追記\n"), Input: InputMarkdown, Mode: ModeAppend}); err != nil {
		t.Fatal(err)
	}
	add, err := frommd.Convert("追記\n")
	if err != nil {
		t.Fatal(err)
	}
	if s.gotBody != cur+add {
		t.Fatalf("body = %q", s.gotBody)
	}
}

func TestUpdate_DryRunNoWrite(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>\n<p>共通</p>\n")
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Title: ptr("新タイトル"), Body: ptr("<p>新</p>\n<p>共通</p>\n"), Input: InputStorage, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.updates != 0 || !res.DryRun {
		t.Fatalf("updates = %d result = %+v", s.updates, res)
	}
	for _, want := range []string{"-<p>旧</p>", "+<p>新</p>", " <p>共通</p>", "現在のタイトル", "新タイトル"} {
		if !strings.Contains(res.Diff, want) {
			t.Errorf("diff does not contain %q:\n%s", want, res.Diff)
		}
	}
}

func TestUpdate_ConflictPassedThrough(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	conflict := &domain.Error{Kind: domain.ErrConflict, Status: 409, Hint: "承認"}
	s.updateErr = conflict
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage})
	if domain.KindOf(err) != domain.ErrConflict || !errors.Is(err, conflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdate_EmptyBodyAppend(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "")
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>追加</p>"), Input: InputStorage, Mode: ModeAppend}); err != nil {
		t.Fatal(err)
	}
	if s.gotBody != "<p>追加</p>" {
		t.Fatalf("body = %q", s.gotBody)
	}
}

func TestUpdate_VerifyReportsLostText(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<h2>A</h2><p>a</p>")
	s.readBack = "<h2>A</h2>"
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>追加</p>"), Input: InputStorage, Mode: ModeAppend})
	if domain.KindOf(err) != domain.ErrVerifyFailed || !strings.Contains(err.Error(), "a 追加") {
		t.Fatalf("err = %v", err)
	}
	if len(res.Lost) != 2 {
		t.Fatalf("lost = %v", res.Lost)
	}
}

func TestUpdate_ReadBackFailure(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	cause := &domain.Error{Kind: domain.ErrRateLimit}
	s.readBackErr = cause
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage})
	if domain.KindOf(err) != domain.ErrVerifyFailed || !errors.Is(err, cause) || !strings.Contains(err.Error(), "20") || res.Page.ID != "20" || s.updates != 1 {
		t.Fatalf("err = %v result = %+v", err, res)
	}
}

func TestUpdate_MarkdownReplaceCommentOnlyAllowedByDropComments(t *testing.T) {
	c, s := newUpdateCommand(WritePolicy{}, "<p>"+commentMarker+"</p>")
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新しい本文\n"), Input: InputMarkdown, AllowDropComments: true})
	if err != nil || s.updates != 1 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_DryRunWarnsOnLossy(t *testing.T) {
	cur := `<ac:structured-macro ac:name="status"><ac:parameter ac:name="title">完了</ac:parameter></ac:structured-macro>`
	c, s := newUpdateCommand(WritePolicy{}, cur)
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新しい本文\n"), Input: InputMarkdown, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "macro:status") || !strings.Contains(res.Warnings[0], "--allow-lossy") || s.updates != 0 {
		t.Fatalf("result = %+v updates = %d", res, s.updates)
	}
}

func TestUpdate_MalformedCurrentBodyRefused(t *testing.T) {
	// 解析できない位置より後ろのマーカーを数え漏らさないように、現在の本文が整形式でなければ書き込まない。
	cur := "<p>a<p>" + commentMarker
	for name, r := range map[string]UpdateRequest{
		"storage replace":  {ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage},
		"markdown replace": {ID: "20", Body: ptr("新\n"), Input: InputMarkdown},
	} {
		c, s := newUpdateCommand(WritePolicy{}, cur)
		_, err := c.Update(context.Background(), r)
		if domain.KindOf(err) != domain.ErrValidation || s.updates != 0 {
			t.Errorf("%s: err = %v updates = %d", name, err, s.updates)
		}
	}
}

func TestUpdate_EmptyOrNoopRefused(t *testing.T) {
	for name, r := range map[string]UpdateRequest{
		"empty replace body": {ID: "20", Body: ptr(" \n\t"), Input: InputStorage},
		"nothing to change":  {ID: "20", Input: InputStorage},
	} {
		c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
		_, err := c.Update(context.Background(), r)
		if domain.KindOf(err) != domain.ErrValidation || s.updates != 0 || s.gets != 0 {
			t.Errorf("%s: err = %v updates = %d gets = %d", name, err, s.updates, s.gets)
		}
	}
	c, s := newUpdateCommand(WritePolicy{}, "<p>旧</p>")
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Input: InputStorage, Message: "版コメントだけ"}); err != nil || s.gotBody != "<p>旧</p>" || s.gotMessage != "版コメントだけ" {
		t.Fatalf("message only: err = %v body = %q message = %q", err, s.gotBody, s.gotMessage)
	}
}

func TestUpdate_DryRunDiffSplitsBlocks(t *testing.T) {
	cur := "<h2>A</h2><p>a1</p><p>a2</p><h2>B</h2><p>b</p>"
	c, _ := newUpdateCommand(WritePolicy{}, cur)
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: ModeSection, Section: "A", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	var minus, plus []string
	for _, l := range strings.Split(res.Diff, "\n") {
		switch {
		case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"):
		case strings.HasPrefix(l, "-"):
			minus = append(minus, l)
		case strings.HasPrefix(l, "+"):
			plus = append(plus, l)
		}
	}
	if strings.Join(minus, "|") != "-<p>a1</p>|-<p>a2</p>" || strings.Join(plus, "|") != "+<p>新</p>" {
		t.Fatalf("diff:\n%s", res.Diff)
	}
}

func TestUpdate_MarkdownSectionLossyRefused(t *testing.T) {
	status := `<ac:structured-macro ac:name="status"><ac:parameter ac:name="title">完了</ac:parameter></ac:structured-macro>`
	cur := "<h2>A</h2>" + status + "<h2>B</h2><p>b</p>"
	c, s := newUpdateCommand(WritePolicy{}, cur)
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新\n"), Input: InputMarkdown, Mode: ModeSection, Section: "A"})
	if domain.KindOf(err) != domain.ErrLossy || !strings.Contains(err.Error(), "macro:status") || !strings.Contains(err.Error(), "--allow-lossy") || s.updates != 0 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}

	c, s = newUpdateCommand(WritePolicy{}, cur)
	res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新\n"), Input: InputMarkdown, Mode: ModeSection, Section: "A", DryRun: true})
	if err != nil || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "macro:status") || s.updates != 0 {
		t.Fatalf("dry run: err = %v result = %+v", err, res)
	}

	c, s = newUpdateCommand(WritePolicy{}, cur)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新\n"), Input: InputMarkdown, Mode: ModeSection, Section: "A", AllowLossy: true}); err != nil || s.updates != 1 {
		t.Fatalf("allow lossy: err = %v updates = %d", err, s.updates)
	}

	// 表せない要素が置き換えない節にだけあるときは拒否しない。
	c, s = newUpdateCommand(WritePolicy{}, cur)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("新\n"), Input: InputMarkdown, Mode: ModeSection, Section: "B"}); err != nil || s.updates != 1 {
		t.Fatalf("other section: err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_StorageKeepingMarkersNotRefused(t *testing.T) {
	cur := "<h2>A</h2><p>" + commentMarker + "</p>"
	c, s := newUpdateCommand(WritePolicy{}, cur)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr(cur + "<p>追記</p>"), Input: InputStorage}); err != nil || s.updates != 1 {
		t.Fatalf("replace: err = %v updates = %d", err, s.updates)
	}
	c, s = newUpdateCommand(WritePolicy{}, cur)
	if _, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>前 " + commentMarker + "</p>"), Input: InputStorage, Mode: ModeSection, Section: "A"}); err != nil || s.updates != 1 {
		t.Fatalf("section: err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_StorageDroppingOneMarkerRefused(t *testing.T) {
	second := `<ac:inline-comment-marker ac:ref="r2">別の注釈</ac:inline-comment-marker>`
	cur := "<p>" + commentMarker + "</p><p>" + second + "</p>"
	c, s := newUpdateCommand(WritePolicy{}, cur)
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>" + commentMarker + "</p><p>別の注釈</p>"), Input: InputStorage})
	if domain.KindOf(err) != domain.ErrCommentLoss || !strings.Contains(err.Error(), "マーカーが 1 個") || s.updates != 0 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_MarkdownCountsAllMarkers(t *testing.T) {
	// Markdown はマーカーを表せないため、入力にマーカーと同じ文字列があっても置き換える範囲のマーカーはすべて数える。
	c, s := newUpdateCommand(WritePolicy{}, "<h2>A</h2><p>"+commentMarker+"</p>")
	_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr(commentMarker + "\n"), Input: InputMarkdown, Mode: ModeSection, Section: "A"})
	if domain.KindOf(err) != domain.ErrCommentLoss || !strings.Contains(err.Error(), "マーカーが 1 個") || s.updates != 0 {
		t.Fatalf("err = %v updates = %d", err, s.updates)
	}
}

func TestUpdate_DryRunAllowedWithoutWritePermission(t *testing.T) {
	for name, p := range map[string]WritePolicy{
		"read_only":       {ReadOnly: true},
		"writable_spaces": {WritableSpaces: []string{"OTHER"}},
	} {
		c, s := newUpdateCommand(p, "<p>旧</p>")
		res, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, DryRun: true})
		if err != nil || !res.DryRun || !strings.Contains(res.Diff, "+<p>新</p>") || s.updates != 0 {
			t.Errorf("%s: err = %v result = %+v updates = %d", name, err, res, s.updates)
		}
	}
}

func TestUpdate_MalformedAssembledBodyRefused(t *testing.T) {
	// 追記と先頭への追加は現在の本文を解析しないため、組み立てた本文を送る前に確認する。
	for _, cur := range []string{"<p>a", `<p>a</p></root><h2>X</h2><root>`} {
		for name, mode := range map[string]UpdateMode{"append": ModeAppend, "prepend": ModePrepend} {
			c, s := newUpdateCommand(WritePolicy{}, cur)
			_, err := c.Update(context.Background(), UpdateRequest{ID: "20", Body: ptr("<p>新</p>"), Input: InputStorage, Mode: mode})
			if domain.KindOf(err) != domain.ErrValidation || s.updates != 0 {
				t.Errorf("%s %q: err = %v updates = %d", name, cur, err, s.updates)
			}
		}
	}
}
