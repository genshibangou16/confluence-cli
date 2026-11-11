package confluence

import (
	"context"
	"net/http"
	"net/url"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

var (
	scopeReadSpace = []string{"read:space:confluence"}

	epSpaces    = endpoint{method: http.MethodGet, path: "/wiki/api/v2/spaces", scopes: scopeReadSpace}
	epSpaceByID = endpoint{method: http.MethodGet, scopes: scopeReadSpace}
)

type spaceJSON struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (s spaceJSON) domain() domain.Space {
	return domain.Space{ID: s.ID, Key: s.Key, Name: s.Name}
}

// SpaceByKey はキーでスペースを取得する。該当がなければ ErrNotFound を返す。
func (c *Client) SpaceByKey(ctx context.Context, key string) (domain.Space, error) {
	items, err := collectV2[spaceJSON](ctx, c, epSpaces, url.Values{"keys": {key}}, 0, false)
	if err != nil {
		return domain.Space{}, err
	}
	if len(items) == 0 {
		return domain.Space{}, &domain.Error{Kind: domain.ErrNotFound, Message: "スペースが見つかりません: " + key}
	}
	return items[0].domain(), nil
}

func (c *Client) SpaceByID(ctx context.Context, id string) (domain.Space, error) {
	ep := epSpaceByID
	ep.path = "/wiki/api/v2/spaces/" + url.PathEscape(id)
	var s spaceJSON
	if _, err := c.do(ctx, ep, nil, nil, &s); err != nil {
		return domain.Space{}, err
	}
	return s.domain(), nil
}
