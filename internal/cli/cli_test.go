package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/adapter/config"
)

// recorder は httptest の Confluence が受けたリクエストのパスとクエリを記録する。
type recorder struct {
	mu   sync.Mutex
	reqs []*url.URL
}

func (r *recorder) add(u *url.URL) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, u)
}

func (r *recorder) find(path string) *url.URL {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.reqs {
		if u.Path == path {
			return u
		}
	}
	return nil
}

type env struct {
	deps           Deps
	stdout, stderr *bytes.Buffer
	rec            *recorder
}

// newEnv は routes を返す TLS の httptest サーバーと、そのサーバーを base_url にしたユーザー設定を用意する。
// config.Load が https だけを受け付けるため TLS にする。routes が nil なら設定ファイルも作らない。
func newEnv(t *testing.T, routes map[string]http.HandlerFunc) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, rec: &recorder{}}
	userPath := filepath.Join(dir, "config", "config.yaml")
	httpClient := http.DefaultClient

	if routes != nil {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			e.rec.add(r.URL)
			h, ok := routes[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			h(w, r)
		}))
		t.Cleanup(srv.Close)
		httpClient = srv.Client()
		if err := config.SaveUser(userPath, config.FileConfig{BaseURL: srv.URL + "/wiki", Email: "a@example.com", APIToken: "tok-123456"}); err != nil {
			t.Fatal(err)
		}
	}

	cwd := filepath.Join(dir, "proj")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	e.deps = Deps{
		Sources: config.Sources{
			Env:            func(string) string { return "" },
			Cwd:            cwd,
			Home:           dir,
			UserConfigPath: userPath,
			RunCommand:     func(string, ...string) ([]byte, error) { return nil, errors.New("unexpected") },
		},
		CacheDir: filepath.Join(dir, "cache"),
		HTTP:     httpClient,
		Stdout:   e.stdout,
		Stderr:   e.stderr,
		Stdin:    strings.NewReader(""),
	}
	return e
}

func (e *env) run(args ...string) int {
	return run(context.Background(), e.deps, args)
}

func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }
}

const searchBody = `{"results":[
{"content":{"id":"123","type":"page","_expandable":{"space":"/rest/api/space/DEV"}},
 "title":"会議メモ","excerpt":"前 @@@hl@@@議事@@@endhl@@@ 後","url":"/spaces/DEV/pages/123",
 "lastModified":"2026-06-10T01:03:37.000Z"}
],"_links":{"base":"https://example.atlassian.net/wiki"}}`

func TestSearch_TableColumns(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/rest/api/search": reply(searchBody)})
	if code := e.run("search", "議事"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	lines := strings.Split(strings.TrimRight(e.stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q", e.stdout)
	}
	if got := strings.Fields(lines[0]); strings.Join(got, " ") != "ID TITLE SPACE MODIFIED EXCERPT" {
		t.Fatalf("header = %q", lines[0])
	}
	if got := strings.Fields(lines[1]); strings.Join(got, " ") != "123 会議メモ DEV 2026-06-10 前 議事 後" {
		t.Fatalf("row = %q", lines[1])
	}
	u := e.rec.find("/wiki/rest/api/search")
	if u == nil || !strings.HasPrefix(u.Query().Get("cql"), `siteSearch ~ "議事"`) || u.Query().Get("limit") != "10" {
		t.Fatalf("request = %v", u)
	}
}

func TestSearch_TitleShowsSpaceKeyAndID(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{
		"/wiki/rest/api/search": reply(searchBody),
		"/wiki/api/v2/pages":    reply(`{"results":[{"id":"123","title":"会議メモ","status":"current","spaceId":"98765"}]}`),
	})
	if code := e.run("search", "議事", "--title"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	out := e.stdout.String()
	if !strings.Contains(out, "SPACE ID") || !strings.Contains(out, "DEV") || !strings.Contains(out, "98765") {
		t.Fatalf("stdout = %q", out)
	}
	if u := e.rec.find("/wiki/rest/api/search"); u == nil || !strings.HasPrefix(u.Query().Get("cql"), `title ~ "議事"`) {
		t.Fatalf("request = %v", u)
	}
	if u := e.rec.find("/wiki/api/v2/pages"); u == nil || u.Query().Get("id") != "123" {
		t.Fatalf("pages request = %v", u)
	}
}

const pageBody = `{"id":"1234567890","title":"T","status":"current","spaceId":"10","version":{"number":7},"body":{"storage":{"value":"<p>本文</p>"}}}`

func TestPageGet_AcceptsTinyLink(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1234567890": reply(pageBody)})
	if code := e.run("page", "get", "https://example.atlassian.net/wiki/x/0gKWSQ", "--format", "storage"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if e.rec.find("/wiki/api/v2/pages/1234567890") == nil {
		t.Fatalf("requests = %v", e.rec.reqs)
	}
}

func TestPageGet_StorageToStdoutVersionToStderr(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1234567890": reply(pageBody)})
	if code := e.run("page", "get", "1234567890", "--format", "storage", "--version", "7"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	// 本文は末尾が改行でない。page update --body-file - に渡して往復できるよう、改行を足さずにそのまま出す。
	if e.stdout.String() != "<p>本文</p>" {
		t.Fatalf("stdout = %q", e.stdout)
	}
	if !strings.Contains(e.stderr.String(), "version: 7\n") {
		t.Fatalf("stderr = %q", e.stderr)
	}
	u := e.rec.find("/wiki/api/v2/pages/1234567890")
	if u.Query().Get("version") != "7" || u.Query().Get("body-format") != "storage" {
		t.Fatalf("query = %v", u.Query())
	}
}

const headingPageBody = `{"id":"1234567890","title":"T","status":"current","spaceId":"10","version":{"number":7},"_links":{"webui":"/spaces/X/pages/1234567890"},"body":{"storage":{"value":"<h1>A</h1><p>a1</p><h2>A-1</h2><p>a2</p><h1>B</h1><p>b1</p>"}}}`

func headingEnv(t *testing.T) *env {
	return newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1234567890": reply(headingPageBody)})
}

func TestPageGet_MarkdownHeaderToStderr(t *testing.T) {
	// 標準出力をそのまま page update --input-format markdown に渡せるよう、標準出力には本文だけを出す。
	e := headingEnv(t)
	if code := e.run("page", "get", "1234567890"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !strings.HasPrefix(e.stdout.String(), "# A\n") || strings.Contains(e.stdout.String(), "id: ") || strings.Contains(e.stdout.String(), "---") {
		t.Fatalf("stdout = %q", e.stdout)
	}
	want := "id: 1234567890\ntitle: T\nversion: 7\nurl: /spaces/X/pages/1234567890\n---\n"
	if e.stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", e.stderr, want)
	}
}

func TestPageGet_Outline(t *testing.T) {
	e := headingEnv(t)
	if code := e.run("page", "get", "1234567890", "--outline"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if e.stdout.String() != "- A\n  - A-1\n- B\n" {
		t.Fatalf("stdout = %q", e.stdout)
	}
	if !strings.HasPrefix(e.stderr.String(), "id: 1234567890\n") || !strings.HasSuffix(e.stderr.String(), "---\n") {
		t.Fatalf("stderr = %q", e.stderr)
	}
}

func TestPageGet_SectionStorageExact(t *testing.T) {
	e := headingEnv(t)
	if code := e.run("page", "get", "1234567890", "--format", "storage", "--section", "B"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if e.stdout.String() != "<p>b1</p>" || !strings.Contains(e.stderr.String(), "version: 7\n") {
		t.Fatalf("stdout = %q, stderr = %q", e.stdout, e.stderr)
	}
}

func TestPageGet_InvalidCombinations(t *testing.T) {
	for _, args := range [][]string{
		{"--outline", "--section", "A"},
		{"--content-format", "storage"},
		{"--format", "storage", "--content-format", "storage"},
		{"--format", "json", "--content-format", "html"},
		{"--section", "none"},
		{"--section", ""},
	} {
		e := headingEnv(t)
		if code := e.run(append([]string{"page", "get", "1234567890"}, args...)...); code != 1 {
			t.Errorf("%v: code = %d", args, code)
		}
		if e.stdout.Len() != 0 {
			t.Errorf("%v: stdout = %q", args, e.stdout)
		}
	}
}

func TestPageGet_OutlineJSONEmptyBody(t *testing.T) {
	empty := `{"id":"1234567890","title":"T","status":"current","spaceId":"10","version":{"number":7},"body":{"storage":{"value":""}}}`
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1234567890": reply(empty)})
	if code := e.run("page", "get", "1234567890", "--format", "json", "--outline"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !strings.Contains(e.stdout.String(), `"outline":[]`) || strings.Contains(e.stdout.String(), `"content"`) {
		t.Fatalf("stdout = %q", e.stdout)
	}
	e = newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1234567890": reply(empty)})
	if code := e.run("page", "get", "1234567890", "--format", "json"); code != 0 || strings.Contains(e.stdout.String(), `"outline"`) {
		t.Fatalf("code = %d, stdout = %q", code, e.stdout)
	}
}

func TestPageGet_MarkdownBodySanitized(t *testing.T) {
	body := `{"id":"1","title":"T","status":"current","spaceId":"10","version":{"number":1},"body":{"storage":{"value":"<h1>A</h1><p>x\u009b31mred</p><p>y</p>"}}}`
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1": reply(body)})
	for _, args := range [][]string{{"page", "get", "1"}, {"page", "get", "1", "--section", "A"}} {
		e.stdout.Reset()
		if code := e.run(args...); code != 0 {
			t.Fatalf("code = %d, stderr = %s", code, e.stderr)
		}
		out := e.stdout.String()
		if strings.Contains(out, "\u009b") || !strings.Contains(out, "x 31mred\n\ny\n") {
			t.Fatalf("%v: stdout = %q", args, out)
		}
	}
}

func TestPageGet_JSON(t *testing.T) {
	type out struct {
		ID, Title, Status, SpaceID, URL string
		Version                         int
		Content                         *string
		Outline                         []struct {
			Level int
			Text  string
		}
	}
	run := func(args ...string) (out, string) {
		e := headingEnv(t)
		if code := e.run(append([]string{"page", "get", "1234567890", "--format", "json"}, args...)...); code != 0 {
			t.Fatalf("%v: code = %d, stderr = %s", args, code, e.stderr)
		}
		var got out
		if err := json.Unmarshal(e.stdout.Bytes(), &got); err != nil {
			t.Fatalf("%v: %q", err, e.stdout)
		}
		if strings.Count(strings.TrimRight(e.stdout.String(), "\n"), "\n") != 0 {
			t.Fatalf("not single line: %q", e.stdout)
		}
		return got, e.stdout.String()
	}
	got, raw := run()
	if got.ID != "1234567890" || got.Version != 7 || got.SpaceID != "10" || got.Status != "current" ||
		got.Content == nil || !strings.HasPrefix(*got.Content, "# A\n") || !strings.Contains(raw, `"spaceId"`) {
		t.Fatalf("got %+v", got)
	}
	got, _ = run("--content-format", "storage")
	if got.Content == nil || !strings.HasPrefix(*got.Content, "<h1>A</h1>") {
		t.Fatalf("storage: %+v", got)
	}
	got, raw = run("--section", "B")
	if got.Content == nil || *got.Content != "b1\n" {
		t.Fatalf("section: %+v", got)
	}
	got, raw = run("--outline")
	if got.Content != nil || len(got.Outline) != 3 || strings.Contains(raw, `"content"`) || got.Outline[1].Level != 2 {
		t.Fatalf("outline: %+v", got)
	}
}

func TestError_NoUsageAndJSON(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{})
	if code := e.run("page", "get", "999", "--format", "json"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if e.stdout.Len() != 0 {
		t.Fatalf("stdout = %q", e.stdout)
	}
	errOut := e.stderr.String()
	if strings.Contains(errOut, "Usage") || strings.Count(strings.TrimRight(errOut, "\n"), "\n") != 0 {
		t.Fatalf("stderr = %q", errOut)
	}
	var got struct {
		Error struct {
			Kind   string `json:"kind"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errOut), &got); err != nil || got.Error.Kind != "not_found" || got.Error.Status != 404 {
		t.Fatalf("stderr = %q, err = %v", errOut, err)
	}
}

func TestError_TextWithoutUsage(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{})
	if code := e.run("page", "get", "not-a-page"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.HasPrefix(e.stderr.String(), "Error: ") || strings.Contains(e.stderr.String(), "Usage") {
		t.Fatalf("stderr = %q", e.stderr)
	}
	if len(e.rec.reqs) != 0 {
		t.Fatalf("requests = %v", e.rec.reqs)
	}
}

func TestConfigShow_NoCredentialsNeeded(t *testing.T) {
	e := newEnv(t, nil)
	if code := e.run("config", "show"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !strings.Contains(e.stdout.String(), e.deps.Sources.UserConfigPath) {
		t.Fatalf("stdout = %q", e.stdout)
	}
}

func TestConfigShow_MasksToken(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{})
	if code := e.run("config", "show"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	out := e.stdout.String()
	if strings.Contains(out, "tok-123456") || !strings.Contains(out, "tok-****") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestPageList_NoBodyRequested(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{
		"/wiki/api/v2/spaces": reply(`{"results":[{"id":"10","key":"DEV","name":"Dev"}]}`),
		"/wiki/api/v2/pages":  reply(`{"results":[{"id":"1","title":"A","status":"current","spaceId":"10"}]}`),
	})
	if code := e.run("page", "list", "--space-key", "DEV"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	u := e.rec.find("/wiki/api/v2/pages")
	if u == nil || u.Query().Has("body-format") || u.Query().Get("space-id") != "10" {
		t.Fatalf("request = %v", u)
	}
	if !strings.Contains(e.stdout.String(), "A") {
		t.Fatalf("stdout = %q", e.stdout)
	}
}

func TestPageAncestors_ShowsTypeAndTitle(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{
		"/wiki/api/v2/pages/5/ancestors": reply(`{"results":[{"id":"1","type":"folder"},{"id":"2","type":"page"}]}`),
		"/wiki/api/v2/pages":             reply(`{"results":[{"id":"2","title":"親","status":"current","spaceId":"10"}]}`),
	})
	if code := e.run("page", "ancestors", "5"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	lines := strings.Split(strings.TrimRight(e.stdout.String(), "\n"), "\n")
	if len(lines) != 3 || strings.Join(strings.Fields(lines[0]), " ") != "ID TYPE TITLE" || strings.Join(strings.Fields(lines[2]), " ") != "2 page 親" {
		t.Fatalf("stdout = %q", e.stdout)
	}
}

func TestLabelList_JSON(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{
		"/wiki/api/v2/labels": reply(`{"results":[{"id":"9","name":"x","prefix":"global"}]}`),
	})
	if code := e.run("label", "list", "--format", "json", "--limit", "3"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if strings.TrimSpace(e.stdout.String()) != `[{"id":"9","name":"x","prefix":"global"}]` {
		t.Fatalf("stdout = %q", e.stdout)
	}
	if u := e.rec.find("/wiki/api/v2/labels"); u.Query().Get("limit") != "3" {
		t.Fatalf("query = %v", u.Query())
	}
}

func TestSearch_TitleAndTextExclusive(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{})
	if code := e.run("search", "x", "--title", "--text"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if len(e.rec.reqs) != 0 {
		t.Fatalf("requests = %v", e.rec.reqs)
	}
}

func TestSearch_RequiresQueryOrFilter(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{})
	if code := e.run("search"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if len(e.rec.reqs) != 0 || !strings.Contains(e.stderr.String(), "Error: ") {
		t.Fatalf("requests = %v, stderr = %q", e.rec.reqs, e.stderr)
	}
}

func TestSearch_CQLTypeOnlyWhenExplicit(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"search", "--cql", "space = DEV"}, "space = DEV"},
		{[]string{"search", "--cql", "space = DEV", "--type", "blogpost"}, `(space = DEV) AND type = "blogpost"`},
	} {
		e := newEnv(t, map[string]http.HandlerFunc{"/wiki/rest/api/search": reply(`{"results":[]}`)})
		if code := e.run(tt.args...); code != 0 {
			t.Fatalf("%v: code = %d, stderr = %s", tt.args, code, e.stderr)
		}
		if got := e.rec.find("/wiki/rest/api/search").Query().Get("cql"); got != tt.want {
			t.Errorf("%v: cql = %q", tt.args, got)
		}
	}
}

func TestJSON_KeysCamelCase(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{
		"/wiki/rest/api/search":           reply(searchBody),
		"/wiki/api/v2/pages":              reply(`{"results":[{"id":"123","title":"会議メモ","status":"current","spaceId":"98765"}]}`),
		"/wiki/api/v2/pages/123/versions": reply(`{"results":[{"number":2,"message":"m","createdAt":"2026-06-10T01:03:37.000Z","authorId":"u1","minorEdit":true}]}`),
		"/wiki/api/v2/spaces":             reply(`{"results":[{"id":"98765","key":"DEV","name":"Dev"}]}`),
	})
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"search", "議事", "--title"}, []string{`"spaceKey":"DEV"`, `"spaceId":"98765"`, `"lastModified":"2026-06-10T01:03:37Z"`}},
		{[]string{"page", "list", "--space-key", "DEV"}, []string{`"spaceId":"98765"`}},
		{[]string{"page", "versions", "123"}, []string{`"createdAt":"2026-06-10T01:03:37Z"`, `"authorId":"u1"`, `"minorEdit":true`}},
	} {
		e.stdout.Reset()
		if code := e.run(append(c.args, "--format", "json")...); code != 0 {
			t.Fatalf("%v: code = %d, stderr = %s", c.args, code, e.stderr)
		}
		out := e.stdout.String()
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v: stdout に %s がない: %s", c.args, w, out)
			}
		}
		if strings.Contains(out, `_`) {
			t.Errorf("%v: stdout に _ を含むキーがある: %s", c.args, out)
		}
	}
}

func TestRoot_Version(t *testing.T) {
	e := newEnv(t, nil)
	e.deps.Version = "v1.2.3"
	if code := e.run("--version"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if e.stdout.String() != "confluence-cli v1.2.3\n" {
		t.Fatalf("stdout = %q", e.stdout)
	}
}

func TestPageGet_StorageSanitizedOnTerminal(t *testing.T) {
	body := `{"id":"1","title":"T","status":"current","spaceId":"10","version":{"number":1},"body":{"storage":{"value":"<h1>A</h1><p>x\u009b31m\u202Ered</p>"}}}`
	for _, args := range [][]string{
		{"page", "get", "1", "--format", "storage"},
		{"page", "get", "1", "--format", "storage", "--section", "A"},
	} {
		// 端末に出すときは制御文字を除き、末尾に改行を足す。
		e := newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1": reply(body)})
		e.deps.IsTerminal = func(io.Writer) bool { return true }
		if code := e.run(args...); code != 0 {
			t.Fatalf("%v: code = %d, stderr = %s", args, code, e.stderr)
		}
		out := e.stdout.String()
		if strings.ContainsAny(out, "\u009b\u202E") || !strings.HasSuffix(out, "x 31mred</p>\n") {
			t.Fatalf("%v: terminal stdout = %q", args, out)
		}

		// 端末でなければバイト単位でそのまま出す。
		e = newEnv(t, map[string]http.HandlerFunc{"/wiki/api/v2/pages/1": reply(body)})
		e.deps.IsTerminal = func(io.Writer) bool { return false }
		if code := e.run(args...); code != 0 {
			t.Fatalf("%v: code = %d, stderr = %s", args, code, e.stderr)
		}
		if out := e.stdout.String(); !strings.HasSuffix(out, "<p>x\u009b31m\u202Ered</p>") {
			t.Fatalf("%v: pipe stdout = %q", args, out)
		}
	}
}

func TestIsTerminal_NonFileWriter(t *testing.T) {
	if isTerminal(&bytes.Buffer{}) {
		t.Fatal("bytes.Buffer を端末と判定した")
	}
}
