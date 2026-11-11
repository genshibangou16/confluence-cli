package usecase

import (
	"context"
	"fmt"
	"strings"

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
