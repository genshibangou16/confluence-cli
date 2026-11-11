package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// JSON は v をインデントなしの 1 行で書き出す。読み手が消費するトークンを減らすため。
// HTML 用のエスケープは行わない。
// encoding/json がエスケープしない C1 制御文字と双方向制御文字は \uXXXX にエスケープする。
// 出力を端末に表示したときに、Confluence 由来の文字列で端末の表示を変えられないようにするため。
// デコードすると元の文字列に戻る。
func JSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := io.WriteString(w, escapeTerminalControls(buf.Bytes()))
	return err
}

// escapeTerminalControls は encoding/json の出力にある C1 制御文字と双方向制御文字を \uXXXX に置き換える。
// JSON の文字列の外には ASCII 以外の文字が現れないため、置き換えるのは文字列の中だけになる。
func escapeTerminalControls(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if isTerminalControl(r) {
			fmt.Fprintf(&sb, `\u%04x`, r)
		} else {
			sb.Write(b[:size])
		}
		b = b[size:]
	}
	return sb.String()
}

func isTerminalControl(r rune) bool {
	return (r >= 0x80 && r <= 0x9F) || r == 0x061C || r == 0x200E || r == 0x200F ||
		(r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}
