package confluence

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

const pagesByIDsChunk = 250

var (
	scopeReadPage = []string{"read:page:confluence"}

	epGetPage    = endpoint{method: http.MethodGet, scopes: scopeReadPage}
	epListPages  = endpoint{method: http.MethodGet, path: "/wiki/api/v2/pages", scopes: scopeReadPage}
	epCreatePage = endpoint{method: http.MethodPost, path: "/wiki/api/v2/pages", scopes: []string{"write:page:confluence"}}
	epUpdatePage = endpoint{method: http.MethodPut, scopes: []string{"write:page:confluence"}}
	epDeletePage = endpoint{method: http.MethodDelete, scopes: []string{"delete:page:confluence"}}
	epChildren   = endpoint{method: http.MethodGet, scopes: []string{"read:hierarchical-content:confluence"}}
	epAncestors  = endpoint{method: http.MethodGet, scopes: []string{"read:content.metadata:confluence"}}
	epVersions   = endpoint{method: http.MethodGet, scopes: scopeReadPage}
)

func pagePath(id domain.PageID, suffix string) string {
	return "/wiki/api/v2/pages/" + url.PathEscape(string(id)) + suffix
}

type versionJSON struct {
	Number    int    `json:"number"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
	AuthorID  string `json:"authorId"`
	MinorEdit bool   `json:"minorEdit"`
}

func (v versionJSON) domain() domain.Version {
	// 解釈できない日時はゼロ値のままにする。版の表示を日時の不備で失敗させない。
	t, _ := time.Parse(time.RFC3339, v.CreatedAt)
	return domain.Version{Number: v.Number, Message: v.Message, CreatedAt: t, AuthorID: v.AuthorID, MinorEdit: v.MinorEdit}
}

type pageJSON struct {
	ID       string      `json:"id"`
	Status   string      `json:"status"`
	Title    string      `json:"title"`
	SpaceID  string      `json:"spaceId"`
	ParentID string      `json:"parentId"`
	Version  versionJSON `json:"version"`
	Body     struct {
		Storage struct {
			Value string `json:"value"`
		} `json:"storage"`
	} `json:"body"`
	Links struct {
		WebUI string `json:"webui"`
		Base  string `json:"base"`
	} `json:"_links"`
}

func (p pageJSON) domain() domain.Page {
	return domain.Page{
		ID:       domain.PageID(p.ID),
		Title:    p.Title,
		Status:   p.Status,
		SpaceID:  p.SpaceID,
		ParentID: p.ParentID,
		WebURL:   p.Links.Base + p.Links.WebUI,
		Version:  p.Version.domain(),
		Body:     p.Body.Storage.Value,
	}
}

type summaryJSON struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	SpaceID string `json:"spaceId"`
}

func (s summaryJSON) domain() domain.PageSummary {
	return domain.PageSummary{ID: domain.PageID(s.ID), Title: s.Title, Status: s.Status, SpaceID: s.SpaceID}
}

func summaries(in []summaryJSON) []domain.PageSummary {
	out := make([]domain.PageSummary, 0, len(in))
	for _, s := range in {
		out = append(out, s.domain())
	}
	return out
}

// GetPage は 1 ページを取得する。body が true なら storage 形式の本文を含め、version が正ならその版を取得する。
func (c *Client) GetPage(ctx context.Context, id domain.PageID, body bool, version int) (domain.Page, error) {
	q := url.Values{}
	if body {
		q.Set("body-format", "storage")
	}
	if version > 0 {
		q.Set("version", strconv.Itoa(version))
	}
	ep := epGetPage
	ep.path = pagePath(id, "")
	var p pageJSON
	if _, err := c.do(ctx, ep, q, nil, &p); err != nil {
		return domain.Page{}, err
	}
	return p.domain(), nil
}

// GetPagesByIDs は ids のページを入力の順で返す。取得できなかった ID は結果に含まれない。
func (c *Client) GetPagesByIDs(ctx context.Context, ids []domain.PageID) ([]domain.PageSummary, error) {
	found := map[domain.PageID]domain.PageSummary{}
	for start := 0; start < len(ids); start += pagesByIDsChunk {
		chunk := ids[start:min(start+pagesByIDsChunk, len(ids))]
		parts := make([]string, len(chunk))
		for i, id := range chunk {
			parts[i] = string(id)
		}
		q := url.Values{"id": {strings.Join(parts, ",")}, "limit": {strconv.Itoa(len(chunk))}}
		items, err := collectV2[summaryJSON](ctx, c, epListPages, q, 0, false)
		if err != nil {
			return nil, err
		}
		for _, s := range items {
			found[domain.PageID(s.ID)] = s.domain()
		}
	}
	out := make([]domain.PageSummary, 0, len(found))
	for _, id := range ids {
		if s, ok := found[id]; ok {
			out = append(out, s)
			delete(found, id)
		}
	}
	return out, nil
}

// ListPages はスペース内のページを本文なしで取得する。spaceID が空なら全スペースが対象。
func (c *Client) ListPages(ctx context.Context, spaceID string, limit int, all bool) ([]domain.PageSummary, error) {
	q := url.Values{}
	if spaceID != "" {
		q.Set("space-id", spaceID)
	}
	items, err := collectV2[summaryJSON](ctx, c, epListPages, q, limit, all)
	if err != nil {
		return nil, err
	}
	return summaries(items), nil
}

type bodyJSON struct {
	Representation string `json:"representation"`
	Value          string `json:"value"`
}

type createRequest struct {
	SpaceID  string   `json:"spaceId"`
	Status   string   `json:"status"`
	Title    string   `json:"title"`
	ParentID string   `json:"parentId,omitempty"`
	Body     bodyJSON `json:"body"`
}

func (c *Client) CreatePage(ctx context.Context, spaceID string, parentID domain.PageID, title, storage string) (domain.Page, error) {
	in := createRequest{SpaceID: spaceID, Status: "current", Title: title, ParentID: string(parentID), Body: bodyJSON{"storage", storage}}
	var p pageJSON
	if _, err := c.do(ctx, epCreatePage, nil, in, &p); err != nil {
		return domain.Page{}, err
	}
	return p.domain(), nil
}

type updateRequest struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Title   string   `json:"title"`
	Body    bodyJSON `json:"body"`
	Version struct {
		Number  int    `json:"number"`
		Message string `json:"message,omitempty"`
	} `json:"version"`
}

// UpdatePage はページを更新する。version は更新後の版番号で、呼び出し側が現在の版に 1 を足して渡す。
func (c *Client) UpdatePage(ctx context.Context, id domain.PageID, title, storage string, version int, message string) (domain.Page, error) {
	in := updateRequest{ID: string(id), Status: "current", Title: title, Body: bodyJSON{"storage", storage}}
	in.Version.Number = version
	in.Version.Message = message
	ep := epUpdatePage
	ep.path = pagePath(id, "")
	var p pageJSON
	if _, err := c.do(ctx, ep, nil, in, &p); err != nil {
		return domain.Page{}, err
	}
	return p.domain(), nil
}

func (c *Client) DeletePage(ctx context.Context, id domain.PageID) error {
	ep := epDeletePage
	ep.path = pagePath(id, "")
	_, err := c.do(ctx, ep, nil, nil, nil)
	return err
}

// Children は直下の子コンテンツを返す。ページ以外（フォルダーなど）も含まれる。
func (c *Client) Children(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.PageSummary, error) {
	ep := epChildren
	ep.path = pagePath(id, "/direct-children")
	items, err := collectV2[summaryJSON](ctx, c, ep, nil, limit, all)
	if err != nil {
		return nil, err
	}
	return summaries(items), nil
}

// Ancestors は祖先をルートに近い順で返す。API はカーソルによるページングを持たず、limit の既定が 25 なので、上限の 250 を指定する。API の応答は ID と種別だけなので Title は空になる。
func (c *Client) Ancestors(ctx context.Context, id domain.PageID) ([]domain.Ancestor, error) {
	ep := epAncestors
	ep.path = pagePath(id, "/ancestors")
	type item struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	items, err := collectV2[item](ctx, c, ep, url.Values{"limit": {strconv.Itoa(maxPageSizeV2)}}, 0, true)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Ancestor, 0, len(items))
	for _, it := range items {
		out = append(out, domain.Ancestor{ID: domain.PageID(it.ID), Type: it.Type})
	}
	return out, nil
}

func (c *Client) Versions(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.Version, error) {
	ep := epVersions
	ep.path = pagePath(id, "/versions")
	items, err := collectV2[versionJSON](ctx, c, ep, nil, limit, all)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Version, 0, len(items))
	for _, v := range items {
		out = append(out, v.domain())
	}
	return out, nil
}
