package cli

import (
	"context"

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
	Commands *usecase.PageCommand
	LabelCmd *usecase.LabelCommand
	Guard    *usecase.WriteGuard
}

// buildApp は設定を読み込み、Confluence のクライアントを 1 つ作ってユースケースに渡す。
// 認証情報が無いと失敗するため、API を呼ぶコマンドの RunE の中でだけ呼ぶ。
// 設定の警告と cloudId の警告は Deps.warn で出す。
func buildApp(ctx context.Context, d Deps) (*App, error) {
	s, warns, err := config.Load(d.Sources)
	for _, w := range warns {
		d.warn(w)
	}
	if err != nil {
		return nil, err
	}
	root, warn := confluence.ResolveRoot(ctx, d.HTTP, s.BaseURL, cache.File{Dir: d.CacheDir})
	if warn != "" {
		d.warn(warn)
	}
	c := confluence.New(confluence.Options{Root: root, Email: s.Email, Token: s.APIToken, HTTP: d.HTTP})
	guard := usecase.NewWriteGuard(usecase.WritePolicy{ReadOnly: s.ReadOnly, WritableSpaces: s.WritableSpaces}, c)
	return &App{
		Search:   usecase.NewSearchService(c, c),
		Spaces:   usecase.NewSpaceService(c),
		Pages:    usecase.NewPageQuery(c),
		Tree:     usecase.NewPageTree(c),
		Labels:   usecase.NewLabelQuery(c),
		Commands: usecase.NewPageCommand(c, c, guard),
		LabelCmd: usecase.NewLabelCommand(c, c, guard),
		Guard:    guard,
	}, nil
}
