// Package storage は Confluence の storage 形式を XML として読むための処理を提供する。
package storage

import (
	"encoding/xml"
	"strings"
)

// storage 形式は ac:・ri:・at: の名前空間を宣言しないまま使うため、宣言したルート要素で包んでから解析する。
// 開始タグに改行を含めないことで、エラーの行番号が元の本文の行番号と一致する。
const (
	wrapperStart = `<root xmlns:ac="http://atlassian.com/content" xmlns:ri="http://atlassian.com/resource/identifier" xmlns:at="http://atlassian.com/template">`
	wrapperEnd   = `</root>`
)

// TokenReader は、本文をルート要素で包んで読む xml.Decoder に、ルート要素の外の検出を加えたもの。
// encoding/xml はルート要素が閉じた後の要素も受け付けるため、本文が </root> を含むと、
// 本文の一部がルート要素の外に出たまま整形式として読めてしまう。
// TokenReader は、ルート要素の終了タグが入力の末尾（Decoder が付けた終了タグ）より前にあれば *xml.SyntaxError を返す。
type TokenReader struct {
	d     *xml.Decoder
	depth int
	total int
}

// Decoder は body をルート要素で包んで読む TokenReader と、包むために前に付けたバイト数を返す。
// 呼び出し側は InputOffset からこのバイト数を引くと、元の本文でのオフセットを得られる。
// &nbsp; などの HTML の実体参照を受け付けるため、Entity に xml.HTMLEntity を設定する。
func Decoder(body string) (*TokenReader, int) {
	src := wrapperStart + body + wrapperEnd
	d := xml.NewDecoder(strings.NewReader(src))
	d.Strict = true
	d.Entity = xml.HTMLEntity
	return &TokenReader{d: d, total: len(src)}, len(wrapperStart)
}

// Token は xml.Decoder.Token と同じく次のトークンを返す。
func (r *TokenReader) Token() (xml.Token, error) {
	tok, err := r.d.Token()
	if err != nil {
		return tok, err
	}
	switch tok.(type) {
	case xml.StartElement:
		r.depth++
	case xml.EndElement:
		r.depth--
		if r.depth == 0 && int(r.d.InputOffset()) != r.total {
			line, _ := r.d.InputPos()
			return nil, &xml.SyntaxError{Msg: "</root> が本文のルート要素を閉じています", Line: line}
		}
	}
	return tok, nil
}

// InputOffset は xml.Decoder.InputOffset と同じく、包んだ入力でのオフセットを返す。
func (r *TokenReader) InputOffset() int64 { return r.d.InputOffset() }
