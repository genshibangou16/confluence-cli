//go:build unix

package config

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFindLocal_SkipsFIFOWithoutBlocking(t *testing.T) {
	home := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(home, LocalConfigName), 0o600); err != nil {
		t.Skip("FIFO を作れない")
	}
	type result struct {
		path  string
		warns []string
	}
	done := make(chan result, 1)
	go func() {
		p, _, w := FindLocal(home, home)
		done <- result{p, w}
	}()
	select {
	case r := <-done:
		if r.path != "" || len(r.warns) != 1 || !strings.Contains(r.warns[0], "通常のファイルではない") {
			t.Fatalf("path=%q warns=%v", r.path, r.warns)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO を開こうとして戻らない")
	}
}
