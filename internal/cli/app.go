package cli

import (
	"context"
	"fmt"

	"github.com/genshibangou16/confluence-cli/internal/adapter/cache"
	"github.com/genshibangou16/confluence-cli/internal/adapter/config"
	"github.com/genshibangou16/confluence-cli/internal/adapter/confluence"
	"github.com/genshibangou16/confluence-cli/internal/usecase"
)

type App struct {
	Search   *usecase.SearchService
	Spaces   *usecase.SpaceService
	Pages    *usecase.PageQuery
	Tree     *usecase.PageTree
	Labels   *usecase.LabelQuery
	Settings config.Settings
}

// buildApp は設定を読み込み、Confluence のクライアントを 1 つ作ってユースケースに渡す。
// 認証情報が無いと失敗するため、API を呼ぶコマンドの RunE の中でだけ呼ぶ。
// 設定の警告と cloudId の警告は標準エラーに出す。
func buildApp(ctx context.Context, d Deps) (*App, error) {
	s, warns, err := config.Load(d.Sources)
	for _, w := range warns {
		fmt.Fprintln(d.Stderr, "Warning: "+w)
	}
	if err != nil {
		return nil, err
	}
	root, warn := confluence.ResolveRoot(ctx, d.HTTP, s.BaseURL, cache.File{Dir: d.CacheDir})
	if warn != "" {
		fmt.Fprintln(d.Stderr, "Warning: "+warn)
	}
	c := confluence.New(confluence.Options{Root: root, Email: s.Email, Token: s.APIToken, HTTP: d.HTTP})
	return &App{
		Search:   usecase.NewSearchService(c),
		Spaces:   usecase.NewSpaceService(c),
		Pages:    usecase.NewPageQuery(c),
		Tree:     usecase.NewPageTree(c),
		Labels:   usecase.NewLabelQuery(c),
		Settings: s,
	}, nil
}
