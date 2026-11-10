package storage

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// SyntaxError は storage 形式が整形式でないことを表す。Line と Column は元の本文での 1 始まりの位置で、
// パーサーが誤りを検出した時点で読み終えた位置を指す。Column は文字（rune）単位で数える。
type SyntaxError struct {
	Line, Column int
	Msg          string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("storage 形式が整形式ではありません（%d 行 %d 列）: %s", e.Line, e.Column, e.Msg)
}

// Validate は body が storage 形式として整形式かを確認する。整形式でなければ *SyntaxError を返す。
func Validate(body string) error {
	d, prefix := Decoder(body)
	for {
		_, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return syntaxError(body, d, prefix, err)
		}
	}
}

// syntaxError は Decoder.Token が返した err を、元の本文での位置を持つ *SyntaxError にする。
func syntaxError(body string, d *xml.Decoder, prefix int, err error) *SyntaxError {
	msg := err.Error()
	var xe *xml.SyntaxError
	if errors.As(err, &xe) {
		msg = xe.Msg
	}
	// 閉じていない要素は末尾に付けた終了タグで検出されるため、オフセットを本文の範囲に収める。
	off := int(d.InputOffset()) - prefix
	off = max(0, min(off, len(body)))
	line, col := position(body[:off])
	return &SyntaxError{Line: line, Column: col, Msg: msg}
}

// position は s を読み終えた位置の行と列を返す。
func position(s string) (line, col int) {
	line = strings.Count(s, "\n") + 1
	lastLine := s[strings.LastIndexByte(s, '\n')+1:]
	return line, utf8.RuneCountInString(lastLine) + 1
}
