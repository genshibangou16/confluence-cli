package usecase

import (
	"context"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type LabelQuery struct {
	r LabelReader
}

func NewLabelQuery(r LabelReader) *LabelQuery {
	return &LabelQuery{r: r}
}

// List は prefix が空なら全ラベルを返す。
func (q *LabelQuery) List(ctx context.Context, prefix string, limit int, all bool) ([]domain.Label, error) {
	return q.r.Labels(ctx, prefix, limit, all)
}

func (q *LabelQuery) OfPage(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.Label, error) {
	return q.r.PageLabels(ctx, id, limit, all)
}

// Pages は spaceID が空なら全スペースのページを返す。
func (q *LabelQuery) Pages(ctx context.Context, labelID, spaceID string, limit int, all bool) ([]domain.PageSummary, error) {
	return q.r.PagesWithLabel(ctx, labelID, spaceID, limit, all)
}
