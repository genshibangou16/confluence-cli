package usecase

import (
	"context"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type PageTree struct {
	l PageLister
}

func NewPageTree(l PageLister) *PageTree {
	return &PageTree{l: l}
}

func (t *PageTree) List(ctx context.Context, spaceID string, limit int, all bool) ([]domain.PageSummary, error) {
	return t.l.ListPages(ctx, spaceID, limit, all)
}

func (t *PageTree) Children(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.PageSummary, error) {
	return t.l.Children(ctx, id, limit, all)
}

// Ancestors は祖先を API の返す順で返す。ancestors API はタイトルを返さないため、
// Type が page の祖先だけ GetPagesByIDs でまとめて取得して Title を埋める。
// フォルダーなどページ以外の祖先は Title が空のまま。
func (t *PageTree) Ancestors(ctx context.Context, id domain.PageID) ([]domain.Ancestor, error) {
	as, err := t.l.Ancestors(ctx, id)
	if err != nil {
		return nil, err
	}
	var ids []domain.PageID
	for _, a := range as {
		if a.Type == "page" {
			ids = append(ids, a.ID)
		}
	}
	if len(ids) == 0 {
		return as, nil
	}
	pages, err := t.l.GetPagesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	titles := make(map[domain.PageID]string, len(pages))
	for _, p := range pages {
		titles[p.ID] = p.Title
	}
	for i := range as {
		if as[i].Type == "page" {
			as[i].Title = titles[as[i].ID]
		}
	}
	return as, nil
}

// Pages は ids のページの概要を返す。取得できなかった ID は結果に含まれない。
func (t *PageTree) Pages(ctx context.Context, ids []domain.PageID) ([]domain.PageSummary, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return t.l.GetPagesByIDs(ctx, ids)
}

func (t *PageTree) Versions(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.Version, error) {
	return t.l.Versions(ctx, id, limit, all)
}
