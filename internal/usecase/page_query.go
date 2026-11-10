package usecase

import (
	"context"
	"fmt"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatStorage  Format = "storage"
	FormatJSON     Format = "json"
)

// GetRequest の Version が 0 なら最新版を取得する。
type GetRequest struct {
	Format  Format
	Version int
	Outline bool
	Section string
}

// PageView の Content は Format に応じた本文。
type PageView struct {
	Page    domain.Page
	Content string
}

type PageQuery struct {
	g PageGetter
}

func NewPageQuery(g PageGetter) *PageQuery {
	return &PageQuery{g: g}
}

// Get は storage と json の形式だけを扱う。どちらの形式でも Content は storage 形式の本文で、
// json での出力の組み立ては呼び出し側が Page から行う。
// markdown・Outline・Section は未実装のため、API を呼ばずに ErrValidation を返す。
func (q *PageQuery) Get(ctx context.Context, id domain.PageID, r GetRequest) (PageView, error) {
	if r.Format != FormatStorage && r.Format != FormatJSON {
		return PageView{}, &domain.Error{Kind: domain.ErrValidation, Message: fmt.Sprintf("未対応の形式: %q", r.Format)}
	}
	if r.Outline || r.Section != "" {
		return PageView{}, &domain.Error{Kind: domain.ErrValidation, Message: "--outline と --section はまだ使えない"}
	}
	p, err := q.g.GetPage(ctx, id, true, r.Version)
	if err != nil {
		return PageView{}, err
	}
	return PageView{Page: p, Content: p.Body}, nil
}
