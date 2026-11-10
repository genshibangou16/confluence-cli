package output

import (
	"errors"
	"fmt"
	"io"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// ErrorJSON は --json 指定時のエラー出力の形。
type ErrorJSON struct {
	Error struct {
		Kind    string `json:"kind"`
		Status  int    `json:"status,omitempty"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
	} `json:"error"`
}

// WriteError は err を w に書き出す。*domain.Error を含む場合は Message・Status・Hint を使い、
// Message が空なら err.Error() を使う。
func WriteError(w io.Writer, err error, asJSON bool) {
	var de *domain.Error
	hasDomain := errors.As(err, &de)

	msg := err.Error()
	hint := ""
	if hasDomain {
		if de.Message != "" {
			msg = de.Message
		}
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
		_ = JSON(w, e)
		return
	}
	fmt.Fprintf(w, "Error: %s\n", msg)
	if hint != "" {
		fmt.Fprintf(w, "Hint: %s\n", hint)
	}
}
