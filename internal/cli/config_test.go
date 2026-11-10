package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/adapter/config"
)

func TestConfigInit_WritesUserConfigAndKeepsPolicy(t *testing.T) {
	e := newEnv(t, nil)
	path := e.deps.Sources.UserConfigPath
	if err := config.SaveUser(path, config.FileConfig{BaseURL: "https://old.example.com", Email: "old@example.com", APIToken: "oldtok", ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	e.deps.Stdin = strings.NewReader("y\nhttps://new.example.com/wiki\nnew@example.com\nnewtok\n")
	if code := e.run("config", "init"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	fc, ok, err := config.ReadUser(path)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if fc.BaseURL != "https://new.example.com/wiki" || fc.Email != "new@example.com" || fc.APIToken != "newtok" || !fc.ReadOnly {
		t.Fatalf("got %+v", fc)
	}
}

func TestConfigInit_RejectsInvalidBaseURL(t *testing.T) {
	e := newEnv(t, nil)
	e.deps.Stdin = strings.NewReader("http://a.example.com\ne@example.com\ntok\n")
	if code := e.run("config", "init"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if _, err := os.Stat(e.deps.Sources.UserConfigPath); !os.IsNotExist(err) {
		t.Fatalf("config written: %v", err)
	}
}

func TestConfigInitLocal_WritesBaseURLOnly(t *testing.T) {
	e := newEnv(t, nil)
	e.deps.Stdin = strings.NewReader("https://example.atlassian.net/wiki\n")
	if code := e.run("config", "init-local"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	p := filepath.Join(e.deps.Sources.Cwd, config.LocalConfigName)
	data, err := os.ReadFile(p)
	if err != nil || strings.TrimSpace(string(data)) != "base_url: https://example.atlassian.net/wiki" {
		t.Fatalf("data = %q, err = %v", data, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v", fi.Mode())
	}
}

func TestConfigInitLocal_RefusesSymlink(t *testing.T) {
	e := newEnv(t, nil)
	target := filepath.Join(t.TempDir(), "target.yaml")
	if err := os.WriteFile(target, []byte("secret: keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(e.deps.Sources.Cwd, config.LocalConfigName)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	e.deps.Stdin = strings.NewReader("y\nhttps://example.atlassian.net/wiki\n")
	if code := e.run("config", "init-local"); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(e.stderr.String(), "Error: ") {
		t.Fatalf("stderr = %q", e.stderr)
	}
	data, err := os.ReadFile(target)
	fi, _ := os.Stat(target)
	if err != nil || string(data) != "secret: keep\n" || fi.Mode().Perm() != 0o600 {
		t.Fatalf("target changed: %q, %v, %v", data, fi.Mode(), err)
	}
	if li, _ := os.Lstat(link); li.Mode()&os.ModeSymlink == 0 {
		t.Fatal("link replaced")
	}
}

func TestConfigInitLocal_OverwritesRegularFileAfterConfirm(t *testing.T) {
	e := newEnv(t, nil)
	p := filepath.Join(e.deps.Sources.Cwd, config.LocalConfigName)
	if err := os.WriteFile(p, []byte("base_url: https://old.atlassian.net\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.deps.Stdin = strings.NewReader("y\nhttps://new.atlassian.net\n")
	if code := e.run("config", "init-local"); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, e.stderr)
	}
	data, _ := os.ReadFile(p)
	fi, _ := os.Stat(p)
	if strings.TrimSpace(string(data)) != "base_url: https://new.atlassian.net" || fi.Mode().Perm() != 0o644 {
		t.Fatalf("data = %q, mode = %v", data, fi.Mode())
	}
	if entries, _ := os.ReadDir(e.deps.Sources.Cwd); len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}
