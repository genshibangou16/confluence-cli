package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileCache_RoundTrip(t *testing.T) {
	f := File{Dir: filepath.Join(t.TempDir(), "sub")}
	if _, ok := f.Load("example.atlassian.net"); ok {
		t.Fatal("empty cache returned a value")
	}
	if err := f.Store("Example.Atlassian.net", "abc-123"); err != nil {
		t.Fatal(err)
	}
	got, ok := f.Load("example.atlassian.net")
	if !ok || got != "abc-123" {
		t.Errorf("Load = %q, %v", got, ok)
	}
	fi, err := os.Stat(filepath.Join(f.Dir, "cloud-id-example.atlassian.net"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestFileCache_RejectsUnsafeHost(t *testing.T) {
	dir := t.TempDir()
	f := File{Dir: filepath.Join(dir, "c")}
	for _, h := range []string{"", "../x", "a/b", `a\b`, "a..b"} {
		if err := f.Store(h, "id"); err == nil {
			t.Errorf("Store(%q) succeeded", h)
		}
		if _, ok := f.Load(h); ok {
			t.Errorf("Load(%q) ok", h)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "x")); err == nil {
		t.Error("file escaped Dir")
	}
}

func TestDefaultDir(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := DefaultDir(env(map[string]string{"XDG_CACHE_HOME": "/x"}), "/h"); got != "/x/confluence-cli" {
		t.Errorf("got %q", got)
	}
	if got := DefaultDir(env(nil), "/h"); got != "/h/.cache/confluence-cli" {
		t.Errorf("got %q", got)
	}
}
