package output

import (
	"encoding/json"
	"io"
)

// JSON は v をインデントなしの 1 行で書き出す。読み手が消費するトークンを減らすため。
// HTML 用のエスケープは行わない。
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
