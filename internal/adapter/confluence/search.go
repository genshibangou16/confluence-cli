package confluence

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

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

// escapedSurrogatePair は、上位サロゲートと下位サロゲートの \uXXXX が隣り合う 12 文字。
var escapedSurrogatePair = regexp.MustCompile(`\\u([dD][89abAB][0-9a-fA-F]{2})\\u([dD][c-fC-F][0-9a-fA-F]{2})`)

// decodeEscapedSurrogatePairs は、s の中の \uXXXX\uXXXX の文字列のうち、サロゲートペアになる組を 1 文字に戻す。
// 検索 API は excerpt の中の絵文字などを、文字ではなく \uD83D\uDDD3 のような文字列で返すため。
// 組にならない \uXXXX とサロゲート以外の \uXXXX は、本文にその文字列が書かれている場合があるため変えない。
func decodeEscapedSurrogatePairs(s string) string {
	if !strings.Contains(s, `\u`) {
		return s
	}
	return escapedSurrogatePair.ReplaceAllStringFunc(s, func(m string) string {
		sub := escapedSurrogatePair.FindStringSubmatch(m)
		hi, _ := strconv.ParseUint(sub[1], 16, 16)
		lo, _ := strconv.ParseUint(sub[2], 16, 16)
		return string(utf16.DecodeRune(rune(hi), rune(lo)))
	})
}

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
				Excerpt:      decodeEscapedSurrogatePairs(highlightMarkers.Replace(r.Excerpt)),
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
