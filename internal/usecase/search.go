package usecase

import (
	"context"

	"github.com/genshibangou16/confluence-cli/internal/cql"
	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type SearchService struct {
	s Searcher
}

func NewSearchService(s Searcher) *SearchService {
	return &SearchService{s: s}
}

// Search は f から CQL を組み立てて検索する。
// siteSearch はサイトの設定によって 400 で拒否されることがあるため、検索語での siteSearch が
// ErrValidation で失敗したときだけ、同じ検索語の text ~ で 1 回だけ検索し直し、warnings に記録する。
// RawCQL を指定した検索は Mode を使わないため、検索し直さない。
func (s *SearchService) Search(ctx context.Context, f cql.Filter, limit int, all bool) (hits []domain.SearchHit, warnings []string, err error) {
	q, err := cql.Build(f)
	if err != nil {
		return nil, nil, err
	}
	hits, err = s.s.Search(ctx, q, limit, all)
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
