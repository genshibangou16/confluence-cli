package confluence

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

var epSearch = endpoint{method: http.MethodGet, path: "/wiki/rest/api/search", scopes: []string{"search:confluence"}}

type searchResultJSON struct {
	Content *struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Expandable struct {
			Space string `json:"space"`
		} `json:"_expandable"`
	} `json:"content"`
	Title                 string `json:"title"`
	Excerpt               string `json:"excerpt"`
	URL                   string `json:"url"`
	LastModified          string `json:"lastModified"`
	ResultGlobalContainer struct {
		DisplayURL string `json:"displayUrl"`
	} `json:"resultGlobalContainer"`
}

func (r searchResultJSON) spaceKey() string {
	if key, ok := strings.CutPrefix(r.Content.Expandable.Space, "/rest/api/space/"); ok && key != "" {
		return key
	}
	if key, ok := strings.CutPrefix(r.ResultGlobalContainer.DisplayURL, "/spaces/"); ok {
		key, _, _ = strings.Cut(key, "/")
		return key
	}
	return ""
}

// highlightMarkers は検索語に一致した箇所を示すマーカーを除く。マーカーは excerpt に入り、タイトル検索では title にも入る。
var highlightMarkers = strings.NewReplacer("@@@hl@@@", "", "@@@endhl@@@", "")

// Search は CQL で検索する。content を持たない結果（ユーザーなど）は含めない。
// 日時を解釈できない結果は LastModified をゼロ値にする。
func (c *Client) Search(ctx context.Context, cql string, limit int, all bool) ([]domain.SearchHit, error) {
	q := url.Values{"cql": {cql}, "excerpt": {"highlight_unescaped"}}
	var hits []domain.SearchHit
	// 結果の url は相対パスで、基点は応答の _links.base にある。
	_, err := collectV1WithBase(ctx, c, epSearch, q, limit, all, func(base string, results []searchResultJSON) {
		for _, r := range results {
			if r.Content == nil {
				continue
			}
			t, _ := time.Parse(time.RFC3339, r.LastModified)
			hits = append(hits, domain.SearchHit{
				ID:           domain.PageID(r.Content.ID),
				Title:        highlightMarkers.Replace(r.Title),
				Type:         r.Content.Type,
				SpaceKey:     r.spaceKey(),
				Excerpt:      highlightMarkers.Replace(r.Excerpt),
				URL:          base + r.URL,
				LastModified: t,
			})
		}
	})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}
