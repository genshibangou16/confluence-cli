package usecase

import (
	"context"
	"fmt"
	"slices"
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
// 作成後に本文を読み戻し、送った本文から失われた語があれば、WriteResult とともに ErrVerifyFailed を返す。
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
	if strings.TrimSpace(r.Body) != "" && strings.TrimSpace(body) == "" {
		return WriteResult{}, emptyConvertedError()
	}
	if err := c.guard.CheckReadOnly(); err != nil {
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

// emptyConvertedError は、空でない入力が空の storage 形式に変換されたときのエラー。
// Markdown のリンク参照定義だけの入力などが該当し、そのまま送ると本文や節の中身が消える。
func emptyConvertedError() error {
	return validationError("本文を storage 形式に変換すると空になります。表示される内容を含む本文を指定してください")
}

// lostWordsInMessage はエラーメッセージに並べる失われた語の上限。残りは語数だけを示す。
const lostWordsInMessage = 20

// verify は書き込んだページの本文を読み戻し、sent から失われた語を WriteResult.Lost に入れる。
// 失われた語があるときと読み戻しに失敗したときは ErrVerifyFailed を返す。
// 読み戻しに失敗したときは、書き込みが済んでいることをメッセージで示す。利用者が失敗と判断して作成をやり直すと、同じページが 2 つできる。
func (c *PageCommand) verify(ctx context.Context, written domain.Page, sent string) (WriteResult, error) {
	res := WriteResult{Page: written}
	// 書き込みの直後に別の利用者が更新しても、書き込んだ版の本文と比べるため、版を指定して読み戻す。
	// 書き込みの応答に版番号が無ければ（0 なら）、最新版を読む。
	got, err := c.g.GetPage(ctx, written.ID, true, written.Version.Number)
	if err != nil {
		return res, &domain.Error{
			Kind:    domain.ErrVerifyFailed,
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
		Kind:    domain.ErrVerifyFailed,
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
// 置き換える範囲から外れるインラインコメントのマーカーと、Markdown で置き換える範囲にある Markdown で表せない要素は、
// 許可のフラグがなければ書き込まずにエラーにする。書き込み後は Create と同じく本文を読み戻して確認する。
// DryRun では書き込まないため、read_only と writable_spaces による書き込みの許可は確認しない。
func (c *PageCommand) Update(ctx context.Context, r UpdateRequest) (WriteResult, error) {
	if err := validateUpdate(r); err != nil {
		return WriteResult{}, err
	}
	if r.Body != nil {
		// 現在の本文を取得する前に、入力の誤りと、変換後の本文が空であることを確認する。
		in, err := toStorage(*r.Body, r.Input)
		if err != nil {
			return WriteResult{}, err
		}
		if strings.TrimSpace(in) == "" {
			return WriteResult{}, emptyConvertedError()
		}
	}
	if !r.DryRun {
		if err := c.guard.CheckReadOnly(); err != nil {
			return WriteResult{}, err
		}
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
	if !r.DryRun {
		if err := c.guard.Check(ctx, cur.SpaceID); err != nil {
			return WriteResult{}, err
		}
	}
	title := cur.Title
	if r.Title != nil {
		title = *r.Title
	}
	if r.Body != nil && r.Mode == ModeReplace {
		// CommentMarkerRefs と LossyElements は解析できない位置で数えるのをやめるため、整形式でない本文ではマーカーや要素を数え漏らす。
		// 節の置き換えでは FindSection が本文全体を解析するため、整形式でない本文はそこでエラーになる。
		if err := storage.Validate(cur.Body); err != nil {
			return WriteResult{}, &domain.Error{
				Kind:    domain.ErrValidation,
				Message: fmt.Sprintf("ページ %s の現在の本文が整形式でないため、置き換えで失われる要素を確認できません: %v", r.ID, err),
				Err:     err,
			}
		}
	}
	body, in, replaced, err := newBody(cur.Body, r)
	if err != nil {
		return WriteResult{}, err
	}
	// 追記と先頭への追加は現在の本文を解析しないため、現在の本文が整形式でなければ、組み立てた本文も整形式にならない。
	if r.Body != nil {
		if err := storage.Validate(body); err != nil {
			return WriteResult{}, &domain.Error{
				Kind:    domain.ErrValidation,
				Message: fmt.Sprintf("ページ %s の更新後の本文が整形式ではないため、更新しません: %v", r.ID, err),
				Err:     err,
			}
		}
	}

	var warnings []string
	// DryRun では、書き込まずに失われるものを確認できるように、エラーにせず警告として返す。
	refuse := func(kind domain.ErrorKind, msg string) error {
		if r.DryRun {
			warnings = append(warnings, msg)
			return nil
		}
		return &domain.Error{Kind: kind, Message: msg}
	}
	if n := droppedMarkers(replaced, in, r.Input); n > 0 && !r.AllowDropComments {
		msg := fmt.Sprintf("置き換える範囲にインラインコメントのマーカーが %d 個あり、更新するとコメントが本文から外れます。外してよい場合は --allow-drop-comments を指定してください", n)
		if err := refuse(domain.ErrCommentLoss, msg); err != nil {
			return WriteResult{}, err
		}
	}
	if r.Input == InputMarkdown && !r.AllowLossy {
		// インラインコメントは上の確認と --allow-drop-comments で扱うため、ここでは数えない。
		lossy := slices.DeleteFunc(storage.LossyElements(replaced), func(e string) bool { return e == "inline-comment" })
		if len(lossy) > 0 {
			msg := "置き換える範囲に Markdown で表せない要素があり、置き換えると失われます: " + strings.Join(lossy, ", ") + "。失ってよい場合は --allow-lossy を指定してください"
			if err := refuse(domain.ErrLossy, msg); err != nil {
				return WriteResult{}, err
			}
		}
	}

	if r.DryRun {
		diff := udiff.Unified("current", "new", splitBlocks(cur.Body), splitBlocks(body))
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

// droppedMarkers は、置き換える範囲 replaced にあるインラインコメントのマーカーのうち、更新で本文から外れるものの数を返す。
// storage 形式の入力 in に同じ ac:ref のマーカーがあれば、そのマーカーは残るため数えない。
// Markdown はマーカーを表せないため、Markdown の入力では範囲のマーカーをすべて数える。
func droppedMarkers(replaced, in string, input InputFormat) int {
	refs := storage.CommentMarkerRefs(replaced)
	if input != InputStorage {
		return len(refs)
	}
	kept := map[string]bool{}
	for _, ref := range storage.CommentMarkerRefs(in) {
		if ref != "" {
			kept[ref] = true
		}
	}
	n := 0
	for _, ref := range refs {
		if !kept[ref] {
			n++
		}
	}
	return n
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
	// 入力元のパイプが失敗して本文が空になったときに、ページの本文を消さないようにする。
	if r.Body != nil && r.Mode == ModeReplace && strings.TrimSpace(*r.Body) == "" {
		return validationError("本文が空です。本文全体を空にする更新はできません")
	}
	if r.Title == nil && r.Body == nil && r.Message == "" {
		return validationError("タイトル・本文・版コメントのいずれも指定されていないため、更新する内容がありません")
	}
	return nil
}

// diffLineBreaker は、改行を含まないことが多い storage 形式の本文を、diff の行がブロック要素ごとになるように改行する。
var diffLineBreaker = func() *strings.Replacer {
	var pairs []string
	for _, tag := range []string{"</p>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>", "</li>", "</tr>", "</table>", "</ul>", "</ol>", "</blockquote>", "</ac:structured-macro>", "</ac:layout-section>", "<hr />", "<hr/>"} {
		pairs = append(pairs, tag, tag+"\n")
	}
	return strings.NewReplacer(pairs...)
}()

// splitBlocks は dry-run の diff の表示だけに使う。送る本文には使わない。
func splitBlocks(body string) string { return diffLineBreaker.Replace(body) }

// newBody は r に従って新しい本文を作り、storage 形式にした入力と、現在の本文のうち置き換えられる範囲も返す。
// 置き換えられる範囲は、インラインコメントのマーカーが外れるかと、Markdown で表せない要素が失われるかの判定に使う。
// r.Body が nil なら、現在の本文をそのまま返し、置き換える範囲はない。
func newBody(cur string, r UpdateRequest) (body, in, replaced string, err error) {
	if r.Body == nil {
		return cur, "", "", nil
	}
	in, err = toStorage(*r.Body, r.Input)
	if err != nil {
		return "", "", "", err
	}
	switch r.Mode {
	case ModeAppend:
		return storage.Append(cur, in), in, "", nil
	case ModePrepend:
		return storage.Prepend(cur, in), in, "", nil
	case ModeSection:
		body, replaced, err := storage.ReplaceSection(cur, r.Section, in)
		if err != nil {
			return "", "", "", err
		}
		return body, in, replaced, nil
	default:
		return in, in, cur, nil
	}
}

// Delete はページを削除する。書き込みの許可はページのあるスペースで確認する。
func (c *PageCommand) Delete(ctx context.Context, id domain.PageID) error {
	if err := c.guard.checkPage(ctx, c.g, id); err != nil {
		return err
	}
	return c.w.DeletePage(ctx, id)
}
