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

// Decoder は body をルート要素で包んで読む Decoder と、包むために前に付けたバイト数を返す。
// 呼び出し側は InputOffset からこのバイト数を引くと、元の本文でのオフセットを得られる。
// &nbsp; などの HTML の実体参照を受け付けるため、Entity に xml.HTMLEntity を設定する。
func Decoder(body string) (*xml.Decoder, int) {
	d := xml.NewDecoder(strings.NewReader(wrapperStart + body + wrapperEnd))
	d.Strict = true
	d.Entity = xml.HTMLEntity
	return d, len(wrapperStart)
}
