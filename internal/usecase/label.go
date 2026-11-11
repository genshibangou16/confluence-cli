package usecase

import (
	"context"
	"strings"

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

// LabelCommand はページのラベルを付け外しする。書き込みの許可はページのあるスペースで確認する。
type LabelCommand struct {
	g     PageGetter
	w     LabelWriter
	guard *WriteGuard
}

func NewLabelCommand(g PageGetter, w LabelWriter, guard *WriteGuard) *LabelCommand {
	return &LabelCommand{g: g, w: w, guard: guard}
}

func (c *LabelCommand) Add(ctx context.Context, id domain.PageID, names []string) error {
	if len(names) == 0 {
		return validationError("ラベル名を 1 つ以上指定してください")
	}
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			return validationError("空のラベル名は指定できません")
		}
	}
	if err := c.guard.checkPage(ctx, c.g, id); err != nil {
		return err
	}
	return c.w.AddLabels(ctx, id, names)
}

func (c *LabelCommand) Remove(ctx context.Context, id domain.PageID, name string) error {
	if strings.TrimSpace(name) == "" {
		return validationError("ラベル名を指定してください")
	}
	if err := c.guard.checkPage(ctx, c.g, id); err != nil {
		return err
	}
	return c.w.RemoveLabel(ctx, id, name)
}
