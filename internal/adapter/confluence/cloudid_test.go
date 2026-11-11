package confluence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type memCache struct {
	m      map[string]string
	stored []string
}

func (c *memCache) Load(h string) (string, bool) { v, ok := c.m[h]; return v, ok }
func (c *memCache) Store(h, id string) error {
	if c.m == nil {
		c.m = map[string]string{}
	}
	c.m[h] = id
	c.stored = append(c.stored, h+"="+id)
	return nil
}

func tenantServer(status int, body string, n *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(n, 1)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

func toServer(srv *httptest.Server) func(string) string {
	return func(string) string { return srv.URL + "/_edge/tenant_info" }
}

const siteURL = "https://example.atlassian.net/wiki"

func TestResolveRoot_UsesCache(t *testing.T) {
	var n int32
	srv := tenantServer(200, `{"cloudId":"zzz"}`, &n)
	defer srv.Close()
	c := &memCache{m: map[string]string{"example.atlassian.net": "abc-1"}}
	root, warn := resolveRoot(context.Background(), srv.Client(), siteURL, c, toServer(srv))
	if root != "https://api.atlassian.com/ex/confluence/abc-1" || warn != "" || n != 0 {
		t.Errorf("root=%q warn=%q n=%d", root, warn, n)
	}
}

func TestResolveRoot_FetchesAndStores(t *testing.T) {
	var n int32
	srv := tenantServer(200, `{"cloudId":"abc-1"}`, &n)
	defer srv.Close()
	c := &memCache{}
	root, warn := resolveRoot(context.Background(), srv.Client(), siteURL, c, toServer(srv))
	if root != "https://api.atlassian.com/ex/confluence/abc-1" || warn != "" || n != 1 {
		t.Errorf("root=%q warn=%q n=%d", root, warn, n)
	}
	if len(c.stored) != 1 || c.stored[0] != "example.atlassian.net=abc-1" {
		t.Errorf("stored = %v", c.stored)
	}
}

func TestResolveRoot_FallbackOnError(t *testing.T) {
	var n int32
	srv := tenantServer(500, `x`, &n)
	defer srv.Close()
	c := &memCache{}
	root, warn := resolveRoot(context.Background(), srv.Client(), siteURL, c, toServer(srv))
	if root != "https://example.atlassian.net" || warn != cloudIDFallback || len(c.stored) != 0 {
		t.Errorf("root=%q warn=%q stored=%v", root, warn, c.stored)
	}
}

func TestResolveRoot_InvalidCloudID(t *testing.T) {
	for _, body := range []string{`{"cloudId":""}`, `{"cloudId":"a/b"}`, `{"cloudId":"a?b"}`, `not json`} {
		var n int32
		srv := tenantServer(200, body, &n)
		c := &memCache{}
		root, warn := resolveRoot(context.Background(), srv.Client(), siteURL, c, toServer(srv))
		srv.Close()
		if root != "https://example.atlassian.net" || warn == "" || len(c.stored) != 0 {
			t.Errorf("%s: root=%q warn=%q stored=%v", body, root, warn, c.stored)
		}
	}
}

func TestResolveRoot_NonAtlassianHost(t *testing.T) {
	c := &memCache{}
	for in, want := range map[string]string{
		"https://confluence.example.com/wiki":  "https://confluence.example.com",
		"https://confluence.example.com/wiki/": "https://confluence.example.com",
		"https://confluence.example.com":       "https://confluence.example.com",
	} {
		root, warn := ResolveRoot(context.Background(), http.DefaultClient, in, c)
		if root != want || warn != "" {
			t.Errorf("%s: root=%q warn=%q", in, root, warn)
		}
	}
	if strings.Join(c.stored, "") != "" {
		t.Error("stored")
	}
}
