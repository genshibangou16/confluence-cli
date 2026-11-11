// Package usecase はコマンドから呼ばれる処理を、Confluence API の呼び出し方から切り離して実装する。
// ポートの interface は、このパッケージのユースケースが呼ぶメソッドだけを持つ。
package usecase

import (
	"context"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type Searcher interface {
	Search(ctx context.Context, cql string, limit int, all bool) ([]domain.SearchHit, error)
}

type PagesByIDGetter interface {
	GetPagesByIDs(ctx context.Context, ids []domain.PageID) ([]domain.PageSummary, error)
}

type SpaceKeyFinder interface {
	SpaceByKey(ctx context.Context, key string) (domain.Space, error)
}

type PageGetter interface {
	GetPage(ctx context.Context, id domain.PageID, body bool, version int) (domain.Page, error)
}

type PageLister interface {
	ListPages(ctx context.Context, spaceID string, limit int, all bool) ([]domain.PageSummary, error)
	Children(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.PageSummary, error)
	Ancestors(ctx context.Context, id domain.PageID) ([]domain.Ancestor, error)
	GetPagesByIDs(ctx context.Context, ids []domain.PageID) ([]domain.PageSummary, error)
	Versions(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.Version, error)
}

type LabelReader interface {
	Labels(ctx context.Context, prefix string, limit int, all bool) ([]domain.Label, error)
	PageLabels(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.Label, error)
	PagesWithLabel(ctx context.Context, labelID, spaceID string, limit int, all bool) ([]domain.PageSummary, error)
}

type PageWriter interface {
	CreatePage(ctx context.Context, spaceID string, parentID domain.PageID, title, storage string) (domain.Page, error)
	UpdatePage(ctx context.Context, id domain.PageID, title, storage string, version int, message string) (domain.Page, error)
	DeletePage(ctx context.Context, id domain.PageID) error
}

type SpaceIDFinder interface {
	SpaceByID(ctx context.Context, id string) (domain.Space, error)
}

type LabelWriter interface {
	AddLabels(ctx context.Context, id domain.PageID, names []string) error
	RemoveLabel(ctx context.Context, id domain.PageID, name string) error
}
