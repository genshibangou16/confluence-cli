package confluence

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// CloudIDCache は host ごとの cloudId の保存先。
type CloudIDCache interface {
	Load(host string) (string, bool)
	Store(host, cloudID string) error
}

const (
	gatewayRoot     = "https://api.atlassian.com/ex/confluence/"
	cloudIDFallback = "cloudId を取得できなかったため、サイトの URL に直接送信します"
	maxTenantBytes  = 64 * 1024
)

// cloudIDRe は tenant_info の cloudId として受け付ける文字。gateway の URL に連結するため、
// パス区切りや "?" などを含む値は使わない。
var cloudIDRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// ResolveRoot は API の root（/wiki を含まない）を返す。
// *.atlassian.net のサイトは cloudId を使った gateway の URL を返し、取得できなければサイトの URL と警告を返す。
// それ以外のホストは baseURL から末尾の /wiki を除いた値を返す。
func ResolveRoot(ctx context.Context, h *http.Client, baseURL string, cache CloudIDCache) (root string, warn string) {
	return resolveRoot(ctx, h, baseURL, cache, func(host string) string {
		return "https://" + host + "/_edge/tenant_info"
	})
}

func resolveRoot(ctx context.Context, h *http.Client, baseURL string, cache CloudIDCache, tenantURL func(host string) string) (string, string) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" {
		return strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/wiki"), ""
	}
	host := strings.ToLower(u.Hostname())
	if !isAtlassianNet(host) {
		return strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/wiki"), ""
	}

	if id, ok := cache.Load(host); ok && cloudIDRe.MatchString(id) {
		return gatewayRoot + id, ""
	}
	if id, err := fetchCloudID(ctx, h, tenantURL(host)); err == nil {
		// 保存に失敗しても、今回の呼び出しには取得した cloudId を使える。
		_ = cache.Store(host, id)
		return gatewayRoot + id, ""
	}
	return "https://" + host, cloudIDFallback
}

func isAtlassianNet(host string) bool {
	sub, ok := strings.CutSuffix(host, ".atlassian.net")
	return ok && sub != "" && !strings.Contains(sub, ".")
}

type cloudIDError string

func (e cloudIDError) Error() string { return string(e) }

func fetchCloudID(ctx context.Context, h *http.Client, tenantURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tenantURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := h.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", cloudIDError("tenant_info の応答が 200 ではありません")
	}
	var info struct {
		CloudID string `json:"cloudId"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTenantBytes)).Decode(&info); err != nil {
		return "", err
	}
	if !cloudIDRe.MatchString(info.CloudID) {
		return "", cloudIDError("cloudId が不正です")
	}
	return info.CloudID, nil
}
