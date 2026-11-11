package usecase

import (
	"context"

	"github.com/genshibangou16/confluence-cli/internal/cql"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type SearchService struct {
	s Searcher
	p PagesByIDGetter
}

func NewSearchService(s Searcher, p PagesByIDGetter) *SearchService {
	return &SearchService{s: s, p: p}
}

// Search は f から CQL を組み立てて検索する。
// siteSearch はサイトの設定によって 400 で拒否されることがあるため、検索語での siteSearch が
// ErrValidation で失敗したときだけ、同じ検索語の text ~ で 1 回だけ検索し直し、warnings に記録する。
// RawCQL を指定した検索は Mode を使わないため、検索し直さない。
// タイトル検索では、結果の SpaceID をページ API で埋める。
func (s *SearchService) Search(ctx context.Context, f cql.Filter, limit int, all bool) (hits []domain.SearchHit, warnings []string, err error) {
	q, err := cql.Build(f)
	if err != nil {
		return nil, nil, err
	}
	hits, err = s.s.Search(ctx, q, limit, all)
	if err == nil && f.Mode == cql.ModeTitle && f.RawCQL == "" {
		return hits, nil, s.fillSpaceIDs(ctx, hits)
	}
	if err == nil || f.Mode != cql.ModeSite || f.RawCQL != "" || domain.KindOf(err) != domain.ErrValidation {
		return hits, nil, err
	}

	f.Mode = cql.ModeText
	q, err = cql.Build(f)
	if err != nil {
		return nil, nil, err
	}
	hits, err = s.s.Search(ctx, q, limit, all)
	if err != nil {
		return nil, nil, err
	}
	return hits, []string{"siteSearch で検索できなかったため、text ~ で検索し直しました"}, nil
}

// fillSpaceIDs は hits の SpaceID を埋める。検索 API は space-id を返さないため、ページ API でまとめて取得する。
// 取得できなかったページの SpaceID は空のまま残す。
func (s *SearchService) fillSpaceIDs(ctx context.Context, hits []domain.SearchHit) error {
	if len(hits) == 0 {
		return nil
	}
	ids := make([]domain.PageID, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ID)
	}
	pages, err := s.p.GetPagesByIDs(ctx, ids)
	if err != nil {
		return err
	}
	spaceIDs := make(map[domain.PageID]string, len(pages))
	for _, p := range pages {
		spaceIDs[p.ID] = p.SpaceID
	}
	for i := range hits {
		hits[i].SpaceID = spaceIDs[hits[i].ID]
	}
	return nil
}
