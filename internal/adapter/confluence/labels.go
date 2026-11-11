package confluence

import (
	"context"
	"net/http"
	"net/url"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

var (
	scopeReadLabel  = []string{"read:label:confluence"}
	scopeWriteLabel = []string{"write:label:confluence"}

	epLabels      = endpoint{method: http.MethodGet, path: "/wiki/api/v2/labels", scopes: scopeReadLabel}
	epPageLabels  = endpoint{method: http.MethodGet, scopes: scopeReadLabel}
	epLabelPages  = endpoint{method: http.MethodGet, scopes: []string{"read:page:confluence"}}
	epAddLabels   = endpoint{method: http.MethodPost, scopes: []string{"read:label:confluence", "write:label:confluence"}}
	epRemoveLabel = endpoint{method: http.MethodDelete, scopes: scopeWriteLabel}
)

type labelJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

func labels(items []labelJSON) []domain.Label {
	out := make([]domain.Label, 0, len(items))
	for _, l := range items {
		out = append(out, domain.Label{ID: l.ID, Name: l.Name, Prefix: l.Prefix})
	}
	return out
}

// Labels はサイト内のラベルを返す。prefix が空なら絞り込まない。
func (c *Client) Labels(ctx context.Context, prefix string, limit int, all bool) ([]domain.Label, error) {
	q := url.Values{}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	items, err := collectV2[labelJSON](ctx, c, epLabels, q, limit, all)
	if err != nil {
		return nil, err
	}
	return labels(items), nil
}

func (c *Client) PageLabels(ctx context.Context, id domain.PageID, limit int, all bool) ([]domain.Label, error) {
	ep := epPageLabels
	ep.path = pagePath(id, "/labels")
	items, err := collectV2[labelJSON](ctx, c, ep, nil, limit, all)
	if err != nil {
		return nil, err
	}
	return labels(items), nil
}

// PagesWithLabel は labelID のラベルが付いたページを返す。spaceID が空なら全スペースが対象。
func (c *Client) PagesWithLabel(ctx context.Context, labelID, spaceID string, limit int, all bool) ([]domain.PageSummary, error) {
	ep := epLabelPages
	ep.path = "/wiki/api/v2/labels/" + url.PathEscape(labelID) + "/pages"
	q := url.Values{}
	if spaceID != "" {
		q.Set("space-id", spaceID)
	}
	items, err := collectV2[summaryJSON](ctx, c, ep, q, limit, all)
	if err != nil {
		return nil, err
	}
	return summaries(items), nil
}

func contentLabelPath(id domain.PageID) string {
	return "/wiki/rest/api/content/" + url.PathEscape(string(id)) + "/label"
}

type addLabelRequest struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

// AddLabels はページに names のラベルを付ける。names が空なら API を呼ばず ErrValidation を返す。
func (c *Client) AddLabels(ctx context.Context, id domain.PageID, names []string) error {
	if len(names) == 0 {
		return &domain.Error{Kind: domain.ErrValidation, Message: "ラベル名を 1 つ以上指定してください"}
	}
	in := make([]addLabelRequest, 0, len(names))
	for _, n := range names {
		in = append(in, addLabelRequest{Prefix: "global", Name: n})
	}
	ep := epAddLabels
	ep.path = contentLabelPath(id)
	_, err := c.do(ctx, ep, nil, in, nil)
	return err
}

// RemoveLabel はページからラベルを外す。ラベル名に "/" が含まれうるため、パスではなくクエリで渡す。
func (c *Client) RemoveLabel(ctx context.Context, id domain.PageID, name string) error {
	ep := epRemoveLabel
	ep.path = contentLabelPath(id)
	_, err := c.do(ctx, ep, url.Values{"name": {name}}, nil, nil)
	return err
}
