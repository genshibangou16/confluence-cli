package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/adapter/config"
)

// useLocalConfig は、名前に ESC を含むディレクトリをカレントディレクトリにし、そこに content のローカル設定を置く。
func useLocalConfig(t *testing.T, e *env, content string) {
	t.Helper()
	cwd := filepath.Join(e.deps.Sources.Home, "proj\x1b[31m")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, config.LocalConfigName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e.deps.Sources.Cwd = cwd
}

func TestConfigShow_SanitizesConfigValues(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{})
	useLocalConfig(t, e, "writable_spaces: [\"A\\eB\"]\nemail: x@example.com\n")
	if code := e.run("config", "show"); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, e.stderr)
	}
	if !strings.Contains(e.stdout.String(), "Writable spaces: A") || !strings.Contains(e.stderr.String(), "warning: ") {
		t.Fatalf("stdout = %q stderr = %q", e.stdout, e.stderr)
	}
	if strings.ContainsRune(e.stdout.String(), 0x1b) || strings.ContainsRune(e.stderr.String(), 0x1b) {
		t.Fatalf("ESC が出力に残る: stdout = %q stderr = %q", e.stdout, e.stderr)
	}
}

func TestConfigPath_SanitizesLocalPath(t *testing.T) {
	e := newEnv(t, nil)
	useLocalConfig(t, e, "base_url: https://example.atlassian.net/wiki\n")
	if code := e.run("config", "path"); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, e.stderr)
	}
	if strings.ContainsRune(e.stdout.String(), 0x1b) || !strings.Contains(e.stdout.String(), "[31m/") {
		t.Fatalf("stdout = %q", e.stdout)
	}
}

func TestSearch_TextWarningsSanitized(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/rest/api/search": reply(searchBody)})
	useLocalConfig(t, e, "email: x@example.com\n")
	if code := e.run("search", "議事"); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, e.stderr)
	}
	if !strings.HasPrefix(e.stderr.String(), "warning: ") || strings.ContainsRune(e.stderr.String(), 0x1b) {
		t.Fatalf("stderr = %q", e.stderr)
	}
}

type warningsJSON struct {
	Warnings []string `json:"warnings"`
	Error    *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeSingleJSON は s が 1 行の JSON だけであることを確認して読む。
func decodeSingleJSON(t *testing.T, s string) warningsJSON {
	t.Helper()
	if strings.Count(s, "\n") != 1 || !strings.HasSuffix(s, "\n") {
		t.Fatalf("1 行の JSON ではない: %q", s)
	}
	var got warningsJSON
	if err := json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatalf("JSON として読めない: %v: %q", err, s)
	}
	return got
}

func TestSearch_JSONWarningsInErrorJSON(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/rest/api/search": func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
	}})
	useLocalConfig(t, e, "email: x@example.com\n")
	if code := e.run("search", "議事", "--format", "json"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	got := decodeSingleJSON(t, e.stderr.String())
	if got.Error == nil || got.Error.Kind == "" || len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "email") {
		t.Fatalf("stderr = %q", e.stderr)
	}
	if strings.ContainsRune(e.stderr.String(), 0x1b) {
		t.Fatalf("ESC がエスケープされずに残る: %q", e.stderr)
	}
}

func TestSearch_JSONWarningsOnSuccess(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/rest/api/search": reply(searchBody)})
	useLocalConfig(t, e, "email: x@example.com\n")
	if code := e.run("search", "議事", "--format", "json"); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, e.stderr)
	}
	got := decodeSingleJSON(t, e.stderr.String())
	if got.Error != nil || len(got.Warnings) != 1 {
		t.Fatalf("stderr = %q", e.stderr)
	}
	var hits []map[string]any
	if err := json.Unmarshal(e.stdout.Bytes(), &hits); err != nil || len(hits) != 1 {
		t.Fatalf("stdout = %q", e.stdout)
	}
}

func TestSearch_JSONNoWarningsNoStderr(t *testing.T) {
	e := newEnv(t, map[string]http.HandlerFunc{"/wiki/rest/api/search": reply(searchBody)})
	if code := e.run("search", "議事", "--format", "json"); code != 0 || e.stderr.Len() != 0 {
		t.Fatalf("code = %d, stderr = %q", code, e.stderr)
	}
}
