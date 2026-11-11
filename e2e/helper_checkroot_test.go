//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckRoot_EmptyBodyRestoresTitle は、開始時の本文が空のテストルートで本文とタイトルの両方が変わったときに、
// CLI で戻せない本文はエラーとして報告し、タイトルは戻すことを確かめる。
// 実際の Confluence は使わず、呼ばれた引数を記録する偽のバイナリを使う。
func TestCheckRoot_EmptyBodyRestoresTitle(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	bin := filepath.Join(dir, "fake-cli")
	script := `#!/bin/sh
echo "$*" >> "$FAKE_CLI_LOG"
case "$1 $2" in
"page get") printf '%s' '{"id":"5","title":"変わったタイトル","version":3,"content":"<p>変わった本文</p>"}' ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	e := &env{bin: bin, root: "5", rootTitle: "元のタイトル", rootBody: "", rootVersion: 3, extraEnv: []string{"FAKE_CLI_LOG=" + log}}
	var errs []string
	e.checkRoot(func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) })

	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "page update 5 --title 元のタイトル") {
		t.Errorf("タイトルを戻していない。呼び出し:\n%s", data)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "本文") {
		t.Errorf("本文のエラーが 1 件だけ報告されていない: %q", errs)
	}
}
