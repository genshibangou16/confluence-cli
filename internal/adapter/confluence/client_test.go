package confluence

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"net/url"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

type sleepRecorder struct{ calls []time.Duration }

func (s *sleepRecorder) sleep(d time.Duration) { s.calls = append(s.calls, d) }

func newTestClient(srv *httptest.Server, rec *sleepRecorder) *Client {
	return New(Options{Root: srv.URL, Email: "a@example.com", Token: "tok", HTTP: srv.Client(), Sleep: rec.sleep})
}

var testEP = endpoint{method: http.MethodGet, path: "/wiki/api/v2/pages", scopes: []string{"read:page:confluence", "read:space:confluence"}}

func TestDo_BasicAuthAndHeaders(t *testing.T) {
	var gotAuth, gotAccept, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := newTestClient(srv, &sleepRecorder{})

	var out struct{ OK bool }
	if _, err := c.do(context.Background(), testEP, nil, nil, &out); err != nil {
		t.Fatal(err)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte("a@example.com:tok")); gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if gotCT != "" || !out.OK {
		t.Errorf("Content-Type = %q, out = %+v", gotCT, out)
	}

	ep := endpoint{method: http.MethodPost, path: "/wiki/api/v2/pages"}
	if _, err := c.do(context.Background(), ep, nil, map[string]string{"a": "b"}, nil); err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/json" || gotBody != `{"a":"b"}` {
		t.Errorf("Content-Type = %q, body = %q", gotCT, gotBody)
	}
}

func TestDo_ErrorMapping(t *testing.T) {
	cases := []struct {
		status   int
		kind     domain.ErrorKind
		hintPart string
	}{
		{400, domain.ErrValidation, ""},
		{401, domain.ErrAuth, "read:page:confluence, read:space:confluence"},
		{403, domain.ErrForbidden, "read:page:confluence, read:space:confluence"},
		{404, domain.ErrNotFound, ""},
		{409, domain.ErrConflict, "page get"},
		{500, domain.ErrAPI, ""},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(`{"message":"boom"}`))
			}))
			defer srv.Close()
			_, err := newTestClient(srv, &sleepRecorder{}).do(context.Background(), testEP, nil, nil, nil)
			de, ok := err.(*domain.Error)
			if !ok {
				t.Fatalf("err = %T %v", err, err)
			}
			if de.Kind != tc.kind || de.Status != tc.status || de.Message != "boom" {
				t.Errorf("got %+v", de)
			}
			if !strings.Contains(de.Hint, tc.hintPart) || (tc.hintPart != "" && de.Hint == "") {
				t.Errorf("hint = %q", de.Hint)
			}
		})
	}
}

func TestDo_ErrorMessageFallsBackToBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(strings.Repeat("x", 800)))
	}))
	defer srv.Close()
	_, err := newTestClient(srv, &sleepRecorder{}).do(context.Background(), testEP, nil, nil, nil)
	if de := err.(*domain.Error); len(de.Message) != 500 {
		t.Errorf("len = %d", len(de.Message))
	}
}

func TestDo_RetryAfterSeconds(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	rec := &sleepRecorder{}
	if _, err := newTestClient(srv, rec).do(context.Background(), testEP, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 || rec.calls[0] != 2*time.Second {
		t.Errorf("sleeps = %v", rec.calls)
	}
}

func TestDo_RetryAfterHTTPDateAndCap(t *testing.T) {
	d := retryAfter(time.Now().Add(30*time.Second).UTC().Format(http.TimeFormat), time.Now())
	if d < 28*time.Second || d > 31*time.Second {
		t.Errorf("date d = %v", d)
	}
	if d := retryAfter("600", time.Now()); d != 60*time.Second {
		t.Errorf("cap d = %v", d)
	}
}

func TestDo_RetryBackoffWithoutHeader(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) <= 3 {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	rec := &sleepRecorder{}
	if _, err := newTestClient(srv, rec).do(context.Background(), testEP, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	ranges := [][2]time.Duration{{800 * time.Millisecond, 1200 * time.Millisecond}, {1600 * time.Millisecond, 2400 * time.Millisecond}, {3200 * time.Millisecond, 4800 * time.Millisecond}}
	if len(rec.calls) != 3 {
		t.Fatalf("sleeps = %v", rec.calls)
	}
	for i, r := range ranges {
		if rec.calls[i] < r[0] || rec.calls[i] > r[1] {
			t.Errorf("sleep[%d] = %v, want %v..%v", i, rec.calls[i], r[0], r[1])
		}
	}
}

func TestDo_RetryResendsBody(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	ep := endpoint{method: http.MethodPut, path: "/wiki/api/v2/pages/1"}
	if _, err := newTestClient(srv, &sleepRecorder{}).do(context.Background(), ep, nil, map[string]int{"a": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] || bodies[0] == "" {
		t.Errorf("bodies = %v", bodies)
	}
}

func TestDo_ConflictAfterRetryHintsEarlierAttempt(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(409)
	}))
	defer srv.Close()
	ep := endpoint{method: http.MethodPut, path: "/wiki/api/v2/pages/1"}
	_, err := newTestClient(srv, &sleepRecorder{}).do(context.Background(), ep, nil, map[string]int{"a": 1}, nil)
	var de *domain.Error
	if !errors.As(err, &de) || de.Kind != domain.ErrConflict || !strings.Contains(de.Hint, "page versions") || n != 2 {
		t.Fatalf("err = %#v, n = %d", err, n)
	}

	// 再送していない 409 には、前の試行についての案内を付けない。
	srv409 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(409) }))
	defer srv409.Close()
	_, err = newTestClient(srv409, &sleepRecorder{}).do(context.Background(), ep, nil, map[string]int{"a": 1}, nil)
	if !errors.As(err, &de) || de.Kind != domain.ErrConflict || strings.Contains(de.Hint, "page versions") {
		t.Fatalf("no retry: err = %#v", err)
	}
}

func TestDo_NoRetryForPOST(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(429)
	}))
	defer srv.Close()
	ep := endpoint{method: http.MethodPost, path: "/wiki/api/v2/pages"}
	_, err := newTestClient(srv, &sleepRecorder{}).do(context.Background(), ep, nil, nil, nil)
	if domain.KindOf(err) != domain.ErrRateLimit || n != 1 {
		t.Errorf("kind = %v, n = %d", domain.KindOf(err), n)
	}
}

func TestDo_GivesUpAfterMaxAttempts(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(429)
	}))
	defer srv.Close()
	_, err := newTestClient(srv, &sleepRecorder{}).do(context.Background(), testEP, nil, nil, nil)
	if domain.KindOf(err) != domain.ErrRateLimit || n != 4 {
		t.Errorf("kind = %v, n = %d", domain.KindOf(err), n)
	}
}

func TestDo_ContextCanceledBeforeRetry(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(429)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	c := New(Options{Root: srv.URL, HTTP: srv.Client(), Sleep: func(time.Duration) { cancel() }})
	_, err := c.do(ctx, testEP, nil, nil, nil)
	if err == nil || n != 1 {
		t.Errorf("err = %v, n = %d", err, n)
	}
}

type item struct {
	ID int `json:"id"`
}

func v2Server(t *testing.T, paths *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.RequestURI())
		if r.URL.Query().Get("cursor") == "" {
			w.Header().Set("Link", `</wiki/api/v2/pages?cursor=abc>; rel="next"`)
			w.Write([]byte(`{"results":[{"id":1},{"id":2}]}`))
			return
		}
		w.Write([]byte(`{"results":[{"id":3},{"id":4}]}`))
	}))
}

func TestCollectV2_FollowsLink(t *testing.T) {
	var paths []string
	srv := v2Server(t, &paths)
	defer srv.Close()
	got, err := collectV2[item](context.Background(), newTestClient(srv, &sleepRecorder{}), testEP, nil, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].ID != 3 {
		t.Errorf("got = %v", got)
	}
	if len(paths) != 2 || paths[1] != "/wiki/api/v2/pages?cursor=abc" || !strings.Contains(paths[0], "limit=3") {
		t.Errorf("paths = %v", paths)
	}
}

func TestCollectV2_All(t *testing.T) {
	var paths []string
	srv := v2Server(t, &paths)
	defer srv.Close()
	got, err := collectV2[item](context.Background(), newTestClient(srv, &sleepRecorder{}), testEP, nil, 0, true)
	if err != nil || len(got) != 4 {
		t.Errorf("got = %v, err = %v", got, err)
	}
}

func TestCollectV2_SinglePage(t *testing.T) {
	var paths []string
	srv := v2Server(t, &paths)
	defer srv.Close()
	got, _ := collectV2[item](context.Background(), newTestClient(srv, &sleepRecorder{}), testEP, url.Values{}, 0, false)
	if len(got) != 2 || len(paths) != 1 || strings.Contains(paths[0], "limit=") {
		t.Errorf("got = %v, paths = %v", got, paths)
	}
}

func TestCollectV1_PrefixesWiki(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		if r.URL.Query().Get("cursor") == "" {
			w.Write([]byte(`{"results":[{"id":1}],"_links":{"next":"/rest/api/search?cursor=x"}}`))
			return
		}
		w.Write([]byte(`{"results":[{"id":2}],"_links":{}}`))
	}))
	defer srv.Close()
	ep := endpoint{method: http.MethodGet, path: "/wiki/rest/api/search"}
	got, err := collectV1[item](context.Background(), newTestClient(srv, &sleepRecorder{}), ep, nil, 0, true)
	if err != nil || len(got) != 2 {
		t.Fatalf("got = %v, err = %v", got, err)
	}
	if len(paths) != 2 || paths[1] != "/wiki/rest/api/search?cursor=x" {
		t.Errorf("paths = %v", paths)
	}
}

func TestCollectV2_RejectsNextLinkWithoutLeadingSlash(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.Header().Set("Link", `<@evil.example.com/x>; rel="next"`)
		w.Write([]byte(`{"results":[{"id":1}]}`))
	}))
	defer srv.Close()
	_, err := collectV2[item](context.Background(), newTestClient(srv, &sleepRecorder{}), testEP, nil, 0, true)
	var de *domain.Error
	if !errors.As(err, &de) || de.Kind != domain.ErrAPI {
		t.Fatalf("err = %v, want *domain.Error(ErrAPI)", err)
	}
	if n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

func TestCollectV2_InvalidNextLinkIsDomainError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</wiki/%zz>; rel="next"`)
		w.Write([]byte(`{"results":[{"id":1}]}`))
	}))
	defer srv.Close()
	_, err := collectV2[item](context.Background(), newTestClient(srv, &sleepRecorder{}), testEP, nil, 0, true)
	var de *domain.Error
	if !errors.As(err, &de) || de.Kind != domain.ErrAPI {
		t.Fatalf("err = %v, want *domain.Error(ErrAPI)", err)
	}
}

func TestDefaultSleep_ReturnsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	New(Options{}).sleep(ctx, time.Minute)
	if d := time.Since(start); d > time.Second {
		t.Errorf("sleep took %v after cancel", d)
	}
}

func TestErrorMessage_V2Errors(t *testing.T) {
	body := []byte(`{"errors":[{"status":404,"code":"NOT_FOUND","title":"Cannot find a page with id [1]","detail":null},{"title":"second"}]}`)
	if got := errorMessage(body); got != "Cannot find a page with id [1]; second" {
		t.Fatalf("got %q", got)
	}
}
