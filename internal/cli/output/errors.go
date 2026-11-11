package output

import (
	"errors"
	"fmt"
	"io"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// ErrorJSON は --format json 指定時のエラー出力の形。Warnings は、失敗するまでに出た警告。
type ErrorJSON struct {
	Error struct {
		Kind    string `json:"kind"`
		Status  int    `json:"status,omitempty"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
	} `json:"error"`
	Warnings []string `json:"warnings,omitempty"`
}

// WarningsJSON は --format json のコマンドが成功したときに、警告を標準エラーに出す形。
type WarningsJSON struct {
	Warnings []string `json:"warnings"`
}

// WriteWarnings は warnings を 1 行の WarningsJSON で w に書き出す。
func WriteWarnings(w io.Writer, warnings []string) error {
	return JSON(w, WarningsJSON{Warnings: warnings})
}

// WriteError は err を w に書き出す。メッセージは err.Error() を使い、
// ラップした側の文脈を残す。*domain.Error を含む場合は Status と Hint をその値から取る。
// warnings は JSON のときだけ ErrorJSON.Warnings に入れる。テキストのときは呼び出し側が警告を出し済みのため使わない。
func WriteError(w io.Writer, err error, asJSON bool, warnings []string) {
	var de *domain.Error
	hasDomain := errors.As(err, &de)

	msg := err.Error()
	hint := ""
	if hasDomain {
		hint = de.Hint
	}

	if asJSON {
		var e ErrorJSON
		e.Error.Kind = string(domain.KindOf(err))
		if hasDomain {
			e.Error.Status = de.Status
		}
		e.Error.Message = msg
		e.Error.Hint = hint
		e.Warnings = warnings
		_ = JSON(w, e)
		return
	}
	// メッセージには Confluence の応答本文の文字列が入りうるため、端末へ出す前に制御文字を除く。
	fmt.Fprintf(w, "Error: %s\n", SanitizeText(msg))
	if hint != "" {
		fmt.Fprintf(w, "Hint: %s\n", SanitizeText(hint))
	}
}
