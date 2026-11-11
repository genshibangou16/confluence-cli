package confluence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// redirectTarget は受けたリクエストの数と、Authorization ヘッダーを受けた数を数える。
type redirectTarget struct {
	reqs, auths atomic.Int32
}

func (rt *redirectTarget) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rt.reqs.Add(1)
		if r.Header.Get("Authorization") != "" {
			rt.auths.Add(1)
		}
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}
}

func TestNewHTTPClient_RefusesUnsafeRedirects(t *testing.T) {
	httpTarget := &redirectTarget{}
	plain := httptest.NewServer(httpTarget.handler())
	t.Cleanup(plain.Close)
	otherTarget := &redirectTarget{}
	other := httptest.NewTLSServer(otherTarget.handler())
	t.Cleanup(other.Close)

	for name, tc := range map[string]struct {
		location string
		target   *redirectTarget
	}{
		"http":       {plain.URL + "/wiki/api/v2/pages/1", httpTarget},
		"other host": {other.URL + "/wiki/api/v2/pages/1", otherTarget},
	} {
		origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, tc.location, http.StatusFound)
		}))
		h := NewHTTPClient(5 * time.Second)
		// 自己署名の証明書を信頼するため、httptest の Transport を使う。other も同じ証明書を使う。
		h.Transport = origin.Client().Transport
		c := New(Options{Root: origin.URL, Email: "a@example.com", Token: "tok", HTTP: h, Sleep: func(time.Duration) {}})
		_, err := c.GetPage(context.Background(), "1", false, 0)
		origin.Close()
		if err == nil || !strings.Contains(err.Error(), "リダイレクト") {
			t.Errorf("%s: err = %v", name, err)
		}
		if n := tc.target.reqs.Load(); n != 0 {
			t.Errorf("%s: リダイレクト先が %d 件のリクエストを受けた（Authorization 付き %d 件）", name, n, tc.target.auths.Load())
		}
	}
}

func TestNewHTTPClient_FollowsSameHostHTTPSRedirect(t *testing.T) {
	var auth string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/wiki/api/v2/pages/1", http.StatusFound)
			return
		}
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	t.Cleanup(srv.Close)
	h := NewHTTPClient(5 * time.Second)
	h.Transport = srv.Client().Transport
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/old", nil)
	req.SetBasicAuth("a@example.com", "tok")
	resp, err := h.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || auth == "" {
		t.Fatalf("status = %d auth = %q", resp.StatusCode, auth)
	}
}
