package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeSite は書き込みのコマンドが呼ぶ Confluence の API を、メモリ上のページで再現する。
// ページ 1 はスペース 10（キー DEV）にあり、作成したページは ID 900 になる。
type fakeSite struct {
	mu     sync.Mutex
	writes []string // 書き込みのリクエストの "METHOD path?query"
	sent   map[string]any
	title  string
	body   string
	ver    int
	// readBack が空でなければ、書き込み後の読み戻しで本文の代わりに返す。
	readBack string
}

func newFakeSite() *fakeSite {
	return &fakeSite{title: "T", body: "<h1>A</h1><p>a1</p>", ver: 7}
}

func (f *fakeSite) pageJSON(id string) string {
	body := f.body
	if f.readBack != "" && len(f.writes) > 0 {
		body = f.readBack
	}
	b, _ := json.Marshal(map[string]any{
		"id": id, "title": f.title, "status": "current", "spaceId": "10",
		"version": map[string]any{"number": f.ver},
		"body":    map[string]any{"storage": map[string]any{"value": body}},
		"_links":  map[string]any{"base": "https://example.atlassian.net/wiki", "webui": "/spaces/DEV/pages/" + id},
	})
	return string(b)
}

func (f *fakeSite) record(r *http.Request) {
	f.writes = append(f.writes, r.Method+" "+r.URL.RequestURI())
	f.sent = nil
	if b, _ := io.ReadAll(r.Body); len(b) > 0 {
		var v any
		_ = json.Unmarshal(b, &v)
		if m, ok := v.(map[string]any); ok {
			f.sent = m
		} else {
			f.sent = map[string]any{"list": v}
		}
	}
}

func (f *fakeSite) page(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
		case http.MethodPut:
			f.record(r)
			f.title = f.sent["title"].(string)
			f.body = f.sent["body"].(map[string]any)["value"].(string)
			f.ver = int(f.sent["version"].(map[string]any)["number"].(float64))
		case http.MethodDelete:
			f.record(r)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(f.pageJSON(id)))
	}
}

func (f *fakeSite) routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/wiki/api/v2/pages/1":   f.page("1"),
		"/wiki/api/v2/pages/900": f.page("900"),
		"/wiki/api/v2/pages": func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.record(r)
			f.title = f.sent["title"].(string)
			f.body = f.sent["body"].(map[string]any)["value"].(string)
			f.ver = 1
			_, _ = w.Write([]byte(f.pageJSON("900")))
		},
		"/wiki/api/v2/spaces":    reply(`{"results":[{"id":"10","key":"DEV","name":"Dev"}]}`),
		"/wiki/api/v2/spaces/10": reply(`{"id":"10","key":"DEV","name":"Dev"}`),
		"/wiki/rest/api/content/1/label": func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.record(r)
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_, _ = w.Write([]byte(`{"results":[]}`))
		},
	}
}

func newWriteEnv(t *testing.T) (*env, *fakeSite) {
	t.Helper()
	f := newFakeSite()
	return newEnv(t, f.routes()), f
}

func (e *env) setEnv(key, value string) {
	prev := e.deps.Sources.Env
	e.deps.Sources.Env = func(k string) string {
		if k == key {
			return value
		}
		return prev(k)
	}
}

// addUserConfig はユーザー設定のファイルの末尾に YAML の行を足す。
func (e *env) addUserConfig(t *testing.T, line string) {
	t.Helper()
	f, err := os.OpenFile(e.deps.Sources.UserConfigPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func TestPageUpdate_BodyFileStdin(t *testing.T) {
	e, f := newWriteEnv(t)
	e.deps.Stdin = strings.NewReader("<p>標準入力の本文</p>")
	if code := e.run("page", "update", "1", "--body-file", "-"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !reflect.DeepEqual(f.writes, []string{"PUT /wiki/api/v2/pages/1"}) {
		t.Fatalf("writes = %v", f.writes)
	}
	if f.body != "<p>標準入力の本文</p>" || f.title != "T" || f.ver != 8 {
		t.Fatalf("body = %q, title = %q, version = %d", f.body, f.title, f.ver)
	}
	want := "id: 1\ntitle: T\nversion: 8\nurl: https://example.atlassian.net/wiki/spaces/DEV/pages/1\n"
	if e.stdout.String() != want {
		t.Fatalf("stdout = %q", e.stdout)
	}
}

func TestPageUpdate_BodyFilePath(t *testing.T) {
	e, f := newWriteEnv(t)
	p := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(p, []byte("# 見出し\n\n本文\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := e.run("page", "update", "1", "--body-file", p, "--input-format", "markdown", "--title", "新しい題", "--message", "m"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !strings.Contains(f.body, "<h1>見出し</h1>") || f.title != "新しい題" {
		t.Fatalf("body = %q, title = %q", f.body, f.title)
	}
	if f.sent["version"].(map[string]any)["message"] != "m" {
		t.Fatalf("sent = %v", f.sent)
	}
}

func TestPageUpdate_TitleOnlyKeepsBody(t *testing.T) {
	e, f := newWriteEnv(t)
	before := f.body
	if code := e.run("page", "update", "1", "--title", "U"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if f.body != before || f.title != "U" {
		t.Fatalf("body = %q, title = %q", f.body, f.title)
	}
}

func TestPageUpdate_SectionAndAppend(t *testing.T) {
	e, f := newWriteEnv(t)
	if code := e.run("page", "update", "1", "--section", "A", "--body", "<p>z</p>"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if f.body != "<h1>A</h1><p>z</p>" {
		t.Fatalf("section body = %q", f.body)
	}
	if code := e.run("page", "update", "1", "--append", "--body", "<p>end</p>"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !strings.HasSuffix(f.body, "<p>end</p>") {
		t.Fatalf("append body = %q", f.body)
	}
}

func TestPageUpdate_FlagsExclusive(t *testing.T) {
	cases := [][]string{
		{"--append", "--section", "A", "--body", "<p>x</p>"},
		{"--append", "--prepend", "--body", "<p>x</p>"},
		{"--body", "<p>x</p>", "--body-file", "-"},
		{"--append"},
		{"--body", "<p>x</p>", "--input-format", "html"},
		{"--body", "<p>x</p>", "--expect-version", "-1"},
		{"--body", "<p>x</p>", "--expect-version", "0"},
	}
	for _, c := range cases {
		e, _ := newWriteEnv(t)
		args := append([]string{"page", "update", "1", "--format", "json"}, c...)
		if code := e.run(args...); code != 1 {
			t.Fatalf("%v: code = %d", c, code)
		}
		if !strings.Contains(e.stderr.String(), `"kind":"validation"`) {
			t.Fatalf("%v: stderr = %s", c, e.stderr)
		}
		if n := len(e.rec.reqs); n != 0 {
			t.Fatalf("%v: %d requests sent: %v", c, n, e.rec.reqs)
		}
	}
}

func TestPageUpdate_DryRun(t *testing.T) {
	e, f := newWriteEnv(t)
	f.body = `<h1>A</h1><p><ac:inline-comment-marker ac:ref="r">a1</ac:inline-comment-marker></p>`
	if code := e.run("page", "update", "1", "--body", "<p>b</p>", "--dry-run"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if len(f.writes) != 0 {
		t.Fatalf("writes = %v", f.writes)
	}
	out := e.stdout.String()
	if !strings.Contains(out, "-<h1>A</h1>") || !strings.Contains(out, "+<p>b</p>") || strings.Contains(out, "id: ") {
		t.Fatalf("stdout = %q", out)
	}
	if !strings.HasPrefix(e.stderr.String(), "warning: ") || !strings.Contains(e.stderr.String(), "インラインコメント") {
		t.Fatalf("stderr = %q", e.stderr)
	}

	e, _ = newWriteEnv(t)
	if code := e.run("page", "update", "1", "--body", "<p>b</p>", "--dry-run", "--format", "json"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	var got struct {
		DryRun   bool     `json:"dryRun"`
		Diff     string   `json:"diff"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(e.stdout.Bytes(), &got); err != nil || !got.DryRun || !strings.Contains(got.Diff, "+<p>b</p>") || got.Warnings == nil {
		t.Fatalf("stdout = %s, err = %v", e.stdout, err)
	}
}

func TestPageUpdate_LostTextPrintsPageAndFails(t *testing.T) {
	e, f := newWriteEnv(t)
	f.readBack = "<p>other</p>"
	if code := e.run("page", "update", "1", "--body", "<p>消える語</p>"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.HasPrefix(e.stdout.String(), "id: 1\n") || !strings.Contains(e.stderr.String(), "消える語") {
		t.Fatalf("stdout = %q, stderr = %q", e.stdout, e.stderr)
	}
}

func TestPageCreate_TextOutput4Lines(t *testing.T) {
	e, f := newWriteEnv(t)
	if code := e.run("page", "create", "--parent", "1", "--title", "新規", "--body", "<p>x</p>"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	want := "id: 900\ntitle: 新規\nversion: 1\nurl: https://example.atlassian.net/wiki/spaces/DEV/pages/900\n"
	if e.stdout.String() != want {
		t.Fatalf("stdout = %q", e.stdout)
	}
	if !reflect.DeepEqual(f.writes, []string{"POST /wiki/api/v2/pages"}) || f.sent["parentId"] != "1" || f.sent["spaceId"] != "10" {
		t.Fatalf("writes = %v, sent = %v", f.writes, f.sent)
	}
}

func TestPageCreate_SpaceKeyAndParentIDAlias(t *testing.T) {
	e, f := newWriteEnv(t)
	if code := e.run("page", "create", "--space-key", "DEV", "--title", "新規", "--body", "<p>x</p>", "--format", "json"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if u := e.rec.find("/wiki/api/v2/spaces"); u == nil || u.Query().Get("keys") != "DEV" {
		t.Fatalf("spaces request = %v", u)
	}
	if f.sent["spaceId"] != "10" {
		t.Fatalf("sent = %v", f.sent)
	}
	var got map[string]any
	if err := json.Unmarshal(e.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"id": "900", "title": "新規", "version": float64(1), "url": "https://example.atlassian.net/wiki/spaces/DEV/pages/900"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("json = %v", got)
	}

	e, f = newWriteEnv(t)
	if code := e.run("page", "create", "--parent-id", "1", "--title", "新規"); code != 0 {
		t.Fatalf("--parent-id: code = %d, stderr = %s", code, e.stderr)
	}
	if f.sent["parentId"] != "1" {
		t.Fatalf("sent = %v", f.sent)
	}

	e, _ = newWriteEnv(t)
	if code := e.run("page", "create", "--parent-id", "1", "--parent", "2", "--title", "新規"); code != 1 || len(e.rec.reqs) != 0 {
		t.Fatalf("both parents: code = %d, reqs = %v", code, e.rec.reqs)
	}
}

func TestReadOnly_AllWriteCommandsRefused(t *testing.T) {
	cmds := [][]string{
		{"page", "create", "--parent", "1", "--title", "t", "--body", "<p>x</p>"},
		{"page", "create", "--space-key", "DEV", "--title", "t", "--body", "<p>x</p>"},
		{"page", "update", "1", "--body", "<p>x</p>"},
		{"page", "update", "1", "--body", "<p>x</p>", "--dry-run"},
		{"page", "delete", "1"},
		{"label", "add", "1", "a", "b"},
		{"label", "remove", "1", "a"},
	}
	for _, c := range cmds {
		e, _ := newWriteEnv(t)
		e.setEnv("CONFLUENCE_READ_ONLY", "1")
		if code := e.run(append(c, "--format", "json")...); code != 1 {
			t.Fatalf("%v: code = %d", c, code)
		}
		if n := len(e.rec.reqs); n != 0 {
			t.Fatalf("%v: %d requests sent: %v", c, n, e.rec.reqs)
		}
		var got struct {
			Error struct {
				Kind string `json:"kind"`
			} `json:"error"`
		}
		if err := json.Unmarshal(e.stderr.Bytes(), &got); err != nil || got.Error.Kind != "read_only" {
			t.Fatalf("%v: stderr = %s", c, e.stderr)
		}
	}
}

func TestDelete_GuardChecked(t *testing.T) {
	e, f := newWriteEnv(t)
	e.addUserConfig(t, "writable_spaces: [OTHER]")
	if code := e.run("page", "delete", "1", "--format", "json"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if len(f.writes) != 0 || !strings.Contains(e.stderr.String(), `"kind":"read_only"`) {
		t.Fatalf("writes = %v, stderr = %s", f.writes, e.stderr)
	}

	e, f = newWriteEnv(t)
	e.addUserConfig(t, "writable_spaces: [dev]")
	if code := e.run("page", "delete", "1"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !reflect.DeepEqual(f.writes, []string{"DELETE /wiki/api/v2/pages/1"}) || e.stdout.String() != "deleted: 1\n" {
		t.Fatalf("writes = %v, stdout = %q", f.writes, e.stdout)
	}
}

func TestLabel_AddRemove(t *testing.T) {
	e, f := newWriteEnv(t)
	if code := e.run("label", "add", "1", "a", "b"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	want := []any{map[string]any{"prefix": "global", "name": "a"}, map[string]any{"prefix": "global", "name": "b"}}
	if !reflect.DeepEqual(f.writes, []string{"POST /wiki/rest/api/content/1/label"}) || !reflect.DeepEqual(f.sent["list"], want) {
		t.Fatalf("writes = %v, sent = %v", f.writes, f.sent)
	}
	if e.stdout.String() != "labels:\n- a\n- b\n" {
		t.Fatalf("stdout = %q", e.stdout)
	}

	e, f = newWriteEnv(t)
	if code := e.run("label", "remove", "1", "a/b"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	if !reflect.DeepEqual(f.writes, []string{"DELETE /wiki/rest/api/content/1/label?name=a%2Fb"}) || e.stdout.String() != "removed: a/b\n" {
		t.Fatalf("writes = %v, stdout = %q", f.writes, e.stdout)
	}
}

func TestPageUpdate_BodyFileTooLarge(t *testing.T) {
	prev := maxBodyBytes
	maxBodyBytes = 8
	t.Cleanup(func() { maxBodyBytes = prev })

	p := filepath.Join(t.TempDir(), "body.html")
	if err := os.WriteFile(p, []byte("<p>123456</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"-", p} {
		e, _ := newWriteEnv(t)
		e.deps.Stdin = strings.NewReader("<p>123456</p>")
		if code := e.run("page", "update", "1", "--body-file", src, "--format", "json"); code != 1 {
			t.Fatalf("%s: code = %d", src, code)
		}
		if !strings.Contains(e.stderr.String(), `"kind":"validation"`) || len(e.rec.reqs) != 0 {
			t.Fatalf("%s: stderr = %s, reqs = %v", src, e.stderr, e.rec.reqs)
		}
	}

	e, f := newWriteEnv(t)
	e.deps.Stdin = strings.NewReader("<p>1</p>")
	if code := e.run("page", "update", "1", "--body-file", "-"); code != 0 || f.body != "<p>1</p>" {
		t.Fatalf("at the limit: code = %d, body = %q, stderr = %s", code, f.body, e.stderr)
	}
}
