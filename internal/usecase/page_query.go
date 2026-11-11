package usecase

import (
	"context"
	"fmt"

	"github.com/genshibangou16/confluence-cli/internal/convert/tomd"
	"github.com/genshibangou16/confluence-cli/internal/domain"
	"github.com/genshibangou16/confluence-cli/internal/storage"
)

type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatStorage  Format = "storage"
	FormatJSON     Format = "json"
)

// GetRequest の Version が 0 なら最新版を取得する。
// ContentFormat は Format が FormatJSON のときの content の形式で、空なら FormatMarkdown として扱う。
// Outline と Section は同時に指定できない。
type GetRequest struct {
	Format        Format
	ContentFormat Format
	Version       int
	Outline       bool
	Section       string
}

// OutlineItem は本文のトップレベルの見出し。
type OutlineItem struct {
	Level int
	Text  string
}

// PageView の Content は、Section があれば節の中身を、なければ本文を、Format（json では ContentFormat）の形式にしたもの。
// Outline を指定したときは Content が空で、Outline にトップレベルの見出しが文書順に入る（見出しが無ければ空）。
type PageView struct {
	Page    domain.Page
	Content string
	Outline []OutlineItem
}

type PageQuery struct {
	g PageGetter
}

func NewPageQuery(g PageGetter) *PageQuery {
	return &PageQuery{g: g}
}

func validationError(format string, a ...any) error {
	return &domain.Error{Kind: domain.ErrValidation, Message: fmt.Sprintf(format, a...)}
}

// Get は引数の組み合わせを API を呼ぶ前に検証し、ページを取得して Request に応じた Content か Outline を組み立てる。
func (q *PageQuery) Get(ctx context.Context, id domain.PageID, r GetRequest) (PageView, error) {
	switch r.Format {
	case FormatMarkdown, FormatStorage, FormatJSON:
	default:
		return PageView{}, validationError("未対応の形式: %q", r.Format)
	}
	contentFormat := r.Format
	if r.Format == FormatJSON {
		contentFormat = r.ContentFormat
		if contentFormat == "" {
			contentFormat = FormatMarkdown
		}
		if contentFormat != FormatMarkdown && contentFormat != FormatStorage {
			return PageView{}, validationError("未対応の content の形式: %q", r.ContentFormat)
		}
	} else if r.ContentFormat != "" {
		return PageView{}, validationError("content の形式は --format json のときだけ指定できます")
	}
	if r.Outline && r.Section != "" {
		return PageView{}, validationError("--outline と --section は同時に指定できません")
	}
	if r.Outline && r.Format == FormatStorage {
		return PageView{}, validationError("--outline は --format storage では使えません")
	}
	p, err := q.g.GetPage(ctx, id, true, r.Version)
	if err != nil {
		return PageView{}, err
	}
	v := PageView{Page: p}
	if r.Outline {
		hs, err := storage.Headings(p.Body)
		if err != nil {
			return PageView{}, err
		}
		v.Outline = []OutlineItem{}
		for _, h := range hs {
			if h.TopLevel {
				v.Outline = append(v.Outline, OutlineItem{Level: h.Level, Text: h.Text})
			}
		}
		return v, nil
	}
	body := p.Body
	if r.Section != "" {
		s, err := storage.FindSection(body, r.Section)
		if err != nil {
			return PageView{}, err
		}
		body = body[s.ContentStart:s.ContentEnd]
	}
	if contentFormat == FormatMarkdown {
		if body, err = tomd.Convert(body); err != nil {
			return PageView{}, err
		}
	}
	v.Content = body
	return v, nil
}
