package cli

import (
	"fmt"

	"github.com/genshibangou16/confluence-cli/internal/cli/output"
)

// warningLog は、--format json のコマンドが出す警告を run に渡すために溜める。
// JSON の出力では標準エラーを JSON だけにするため、警告をテキストで出さずに、run が最後に 1 つの JSON にまとめて出す。
type warningLog struct {
	json     bool
	messages []string
}

// warn は警告を出す。--format json のコマンドでは溜めて、それ以外では "warning: " を付けて標準エラーに出す。
// 警告は設定ファイルのパスや値を含むため、テキストで出すときは制御文字を除く。
func (d Deps) warn(msg string) {
	if d.warnings != nil && d.warnings.json {
		d.warnings.messages = append(d.warnings.messages, msg)
		return
	}
	fmt.Fprintln(d.Stderr, "warning: "+output.SanitizeText(msg))
}
