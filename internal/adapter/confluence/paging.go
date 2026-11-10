package confluence

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

const (
	maxPageSizeV2 = 250
	maxPageSizeV1 = 100
)

var linkNextRe = regexp.MustCompile(`<([^>]*)>\s*;[^,]*\brel="?next"?`)

// collectV2 は v2 API の一覧を取得する。次ページは Link ヘッダーの rel="next" で辿る。
// limit が正なら limit 件で打ち切り、all が false で limit が 0 以下なら 1 ページだけ取得する。
func collectV2[T any](ctx context.Context, c *Client, ep endpoint, q url.Values, limit int, all bool) ([]T, error) {
	return collect[T](ctx, c, ep, q, limit, all, maxPageSizeV2, true, nil)
}

// collectV1 は v1 API の一覧を取得する。次ページは応答の _links.next で辿り、
// 値が /wiki/ で始まらなければ /wiki を前に付ける。
func collectV1[T any](ctx context.Context, c *Client, ep endpoint, q url.Values, limit int, all bool) ([]T, error) {
	return collect[T](ctx, c, ep, q, limit, all, maxPageSizeV1, false, nil)
}

// collectV1WithBase は collectV1 に加えて、ページごとに応答の _links.base と結果を onPage へ渡す。
// 結果の相対 URL を絶対 URL にするために使う。
func collectV1WithBase[T any](ctx context.Context, c *Client, ep endpoint, q url.Values, limit int, all bool, onPage func(base string, results []T)) ([]T, error) {
	return collect[T](ctx, c, ep, q, limit, all, maxPageSizeV1, false, onPage)
}

type page[T any] struct {
	Results []T `json:"results"`
	Links   struct {
		Next string `json:"next"`
		Base string `json:"base"`
	} `json:"_links"`
}

func collect[T any](ctx context.Context, c *Client, ep endpoint, q url.Values, limit int, all bool, maxSize int, useHeader bool, onPage func(base string, results []T)) ([]T, error) {
	query := url.Values{}
	for k, v := range q {
		query[k] = v
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(min(limit, maxSize)))
	}

	var items []T
	for {
		var p page[T]
		h, err := c.do(ctx, ep, query, nil, &p)
		if err != nil {
			return nil, err
		}
		if onPage != nil {
			onPage(p.Links.Base, p.Results)
		}
		items = append(items, p.Results...)
		if limit > 0 && len(items) >= limit {
			return items[:limit], nil
		}
		if limit <= 0 && !all {
			return items, nil
		}

		var next string
		if useHeader {
			next = nextFromLinkHeader(h.Get("Link"))
		} else {
			next = p.Links.Next
			if next != "" && !strings.HasPrefix(next, "/wiki/") {
				next = "/wiki" + next
			}
		}
		if next == "" {
			return items, nil
		}
		u, err := url.Parse(next)
		if err != nil {
			return nil, &domain.Error{Kind: domain.ErrAPI, Message: "次ページのリンクを解釈できません", Err: err}
		}
		// root の直後に連結するため、"/" で始まらないと root のホストが書き換わり、
		// 認証情報が別のホストへ送られる。
		if !strings.HasPrefix(u.Path, "/") {
			return nil, &domain.Error{Kind: domain.ErrAPI, Message: "次ページのリンクが不正です: " + next}
		}
		ep.path = u.Path
		query = u.Query()
	}
}

func nextFromLinkHeader(v string) string {
	m := linkNextRe.FindStringSubmatch(v)
	if m == nil {
		return ""
	}
	return m[1]
}
