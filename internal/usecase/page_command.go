package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/aymanbagabas/go-udiff"

	"github.com/genshibangou16/confluence-cli/internal/convert/frommd"
	"github.com/genshibangou16/confluence-cli/internal/domain"
	"github.com/genshibangou16/confluence-cli/internal/storage"
)

type InputFormat string

const (
	InputStorage  InputFormat = "storage"
	InputMarkdown InputFormat = "markdown"
)

// WriteResult の Lost は、書き込み後に読み戻した本文から失われた語。
type WriteResult struct {
	Page     domain.Page
	Lost     []string
	Diff     string
	DryRun   bool
	Warnings []string
}

type PageCommand struct {
	g     PageGetter
	w     PageWriter
	guard *WriteGuard
}

func NewPageCommand(g PageGetter, w PageWriter, guard *WriteGuard) *PageCommand {
	return &PageCommand{g: g, w: w, guard: guard}
}

// CreateRequest の ParentID を指定したときは、ページを親と同じスペースに作る。
// ParentID が無いときは SpaceID が必要で、両方あるときは親のスペースと一致しなければならない。
type CreateRequest struct {
	SpaceID  string
	ParentID domain.PageID
	Title    string
	Body     string
	Input    InputFormat
}

// Create は本文を storage 形式にして検証してから、書き込みの許可を確認してページを作る。
// 作成後に本文を読み戻し、送った本文から失われた語があれば、WriteResult とともに ErrAPI を返す。
func (c *PageCommand) Create(ctx context.Context, r CreateRequest) (WriteResult, error) {
	if strings.TrimSpace(r.Title) == "" {
		return WriteResult{}, validationError("タイトルを指定してください")
	}
	if r.ParentID == "" && r.SpaceID == "" {
		return WriteResult{}, validationError("スペースか親ページを指定してください")
	}
	body, err := toStorage(r.Body, r.Input)
	if err != nil {
		return WriteResult{}, err
	}
	if err := c.guard.checkReadOnly(); err != nil {
		return WriteResult{}, err
	}
	spaceID := r.SpaceID
	if r.ParentID != "" {
		parent, err := c.g.GetPage(ctx, r.ParentID, false, 0)
		if err != nil {
			return WriteResult{}, err
		}
		if spaceID != "" && spaceID != parent.SpaceID {
			return WriteResult{}, validationError("親ページ %s はスペース %s にありません", r.ParentID, spaceID)
		}
		spaceID = parent.SpaceID
	}
	if err := c.guard.Check(ctx, spaceID); err != nil {
		return WriteResult{}, err
	}
	page, err := c.w.CreatePage(ctx, spaceID, r.ParentID, r.Title, body)
	if err != nil {
		return WriteResult{}, err
	}
	return c.verify(ctx, page, body)
}

// toStorage は入力を storage 形式にする。整形式でなければ、*storage.SyntaxError を Err に持つ ErrValidation を返す。
func toStorage(body string, in InputFormat) (string, error) {
	var out string
	var err error
	switch in {
	case InputMarkdown:
		out, err = frommd.Convert(body)
	case InputStorage:
		out, err = body, storage.Validate(body)
	default:
		return "", validationError("未対応の入力形式: %q", in)
	}
	if err != nil {
		return "", &domain.Error{Kind: domain.ErrValidation, Message: err.Error(), Err: err}
	}
	return out, nil
}

// lostWordsInMessage はエラーメッセージに並べる失われた語の上限。残りは語数だけを示す。
const lostWordsInMessage = 20

// verify は書き込んだページの本文を読み戻し、sent から失われた語を WriteResult.Lost に入れる。
// 読み戻しに失敗したときは、書き込みが済んでいることをメッセージで示す。利用者が失敗と判断して作成をやり直すと、同じページが 2 つできる。
func (c *PageCommand) verify(ctx context.Context, written domain.Page, sent string) (WriteResult, error) {
	res := WriteResult{Page: written}
	got, err := c.g.GetPage(ctx, written.ID, true, 0)
	if err != nil {
		return res, &domain.Error{
			Kind:    domain.ErrAPI,
			Message: fmt.Sprintf("ページ %s への書き込みは完了しましたが、本文の読み戻しに失敗しました: %v", written.ID, err),
			Err:     err,
		}
	}
	res.Page = got
	res.Lost = storage.LostText(sent, got.Body)
	if len(res.Lost) == 0 {
		return res, nil
	}
	shown := res.Lost
	suffix := ""
	if len(shown) > lostWordsInMessage {
		suffix = fmt.Sprintf(" ほか %d 語", len(shown)-lostWordsInMessage)
		shown = shown[:lostWordsInMessage]
	}
	return res, &domain.Error{
		Kind:    domain.ErrAPI,
		Message: "書き込み後の本文から次のテキストが失われました: " + strings.Join(shown, " ") + suffix,
	}
}

// UpdateMode は新しい本文を現在の本文のどこに入れるかを表す。
type UpdateMode int

const (
	ModeReplace UpdateMode = iota
	ModeAppend
	ModePrepend
	ModeSection
)

// UpdateRequest の Title と Body が nil なら、現在のタイトルと本文を残す。
// Body が nil のときは、本文を送り直すだけなので Mode は ModeReplace でなければならない。
// ExpectedVersion が 0 なら版番号を確認しない。
type UpdateRequest struct {
	ID                domain.PageID
	Title             *string
	Body              *string
	Input             InputFormat
	Mode              UpdateMode
	Section           string
	ExpectedVersion   int
	Message           string
	DryRun            bool
	AllowLossy        bool
	AllowDropComments bool
}

// Update はページのタイトルと本文を更新する。新しい本文のうち r.Body で置き換えない部分は、現在の本文のバイト列をそのまま送る。
// 置き換える範囲のインラインコメントのマーカーと、Markdown で全体を置き換えるときに表せない要素は、
// 許可のフラグがなければ書き込まずにエラーにする。書き込み後は Create と同じく本文を読み戻して確認する。
func (c *PageCommand) Update(ctx context.Context, r UpdateRequest) (WriteResult, error) {
	if err := validateUpdate(r); err != nil {
		return WriteResult{}, err
	}
	if err := c.guard.checkReadOnly(); err != nil {
		return WriteResult{}, err
	}
	cur, err := c.g.GetPage(ctx, r.ID, true, 0)
	if err != nil {
		return WriteResult{}, err
	}
	if r.ExpectedVersion > 0 && r.ExpectedVersion != cur.Version.Number {
		return WriteResult{}, &domain.Error{
			Kind:    domain.ErrVersionMismatch,
			Message: fmt.Sprintf("ページ %s の現在の版は %d で、指定された版 %d と異なるため更新しません", r.ID, cur.Version.Number, r.ExpectedVersion),
		}
	}
	if err := c.guard.Check(ctx, cur.SpaceID); err != nil {
		return WriteResult{}, err
	}
	title := cur.Title
	if r.Title != nil {
		title = *r.Title
	}
	body, replaced, err := newBody(cur.Body, r)
	if err != nil {
		return WriteResult{}, err
	}

	var warnings []string
	if n := storage.CountCommentMarkers(replaced); n > 0 && !r.AllowDropComments {
		msg := fmt.Sprintf("置き換える範囲にインラインコメントのマーカーが %d 個あり、更新するとコメントが本文から外れます。外してよい場合は --allow-drop-comments を指定してください", n)
		if !r.DryRun {
			return WriteResult{}, &domain.Error{Kind: domain.ErrCommentLoss, Message: msg}
		}
		warnings = append(warnings, msg)
	}
	if r.Body != nil && r.Input == InputMarkdown && r.Mode == ModeReplace && !r.AllowLossy {
		if lossy := storage.LossyElements(cur.Body); len(lossy) > 0 {
			return WriteResult{}, &domain.Error{
				Kind:    domain.ErrLossy,
				Message: "現在の本文に Markdown で表せない要素があり、本文全体を置き換えると失われます: " + strings.Join(lossy, ", ") + "。失ってよい場合は --allow-lossy を指定してください",
			}
		}
	}

	if r.DryRun {
		diff := udiff.Unified("current", "new", cur.Body, body)
		if title != cur.Title {
			diff = fmt.Sprintf("タイトル: %q → %q\n", cur.Title, title) + diff
		}
		return WriteResult{Page: cur, DryRun: true, Diff: diff, Warnings: warnings}, nil
	}

	page, err := c.w.UpdatePage(ctx, r.ID, title, body, cur.Version.Number+1, r.Message)
	if err != nil {
		return WriteResult{}, err
	}
	return c.verify(ctx, page, body)
}

// validateUpdate は API を呼ばずに判定できる UpdateRequest の誤りを ErrValidation にする。
func validateUpdate(r UpdateRequest) error {
	if r.Title != nil && strings.TrimSpace(*r.Title) == "" {
		return validationError("タイトルを空にはできません")
	}
	switch r.Mode {
	case ModeReplace, ModeAppend, ModePrepend:
	case ModeSection:
		if strings.TrimSpace(r.Section) == "" {
			return validationError("置き換える節の見出しを指定してください")
		}
	default:
		return validationError("未対応の更新方法: %d", r.Mode)
	}
	if r.Body == nil && r.Mode != ModeReplace {
		return validationError("追記・先頭への追加・節の置き換えには本文を指定してください")
	}
	return nil
}

// newBody は r に従って新しい本文を作り、現在の本文のうち置き換えられる範囲も返す。
// 置き換えられる範囲は、インラインコメントのマーカーが外れるかの判定に使う。
// r.Body が nil なら、現在の本文をそのまま返し、置き換える範囲はない。
func newBody(cur string, r UpdateRequest) (body, replaced string, err error) {
	if r.Body == nil {
		return cur, "", nil
	}
	in, err := toStorage(*r.Body, r.Input)
	if err != nil {
		return "", "", err
	}
	switch r.Mode {
	case ModeAppend:
		return storage.Append(cur, in), "", nil
	case ModePrepend:
		return storage.Prepend(cur, in), "", nil
	case ModeSection:
		s, err := storage.FindSection(cur, r.Section)
		if err != nil {
			return "", "", err
		}
		return cur[:s.ContentStart] + in + cur[s.ContentEnd:], cur[s.ContentStart:s.ContentEnd], nil
	default:
		return in, cur, nil
	}
}
