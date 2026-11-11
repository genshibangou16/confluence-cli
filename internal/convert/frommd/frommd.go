// Package frommd は Markdown（GFM）を Confluence の storage 形式に変換する。
// 変換規則は、スキルの Markdown → storage 形式の変換リファレンスに合わせる。GitHub Alerts は convert.AlertToPanel の対応に従う。
package frommd

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/genshibangou16/confluence-cli/internal/storage"
)

// Convert は Markdown を storage 形式に変換する。
// 出力が整形式でなければ *storage.SyntaxError を返す。
func Convert(md string) (string, error) {
	src := []byte(md)
	r := &renderer{src: src}
	p := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList)).Parser()
	r.blocks(p.Parse(text.NewReader(src)))
	out := dropInvalidXMLChars(r.buf.String())
	if err := storage.Validate(out); err != nil {
		return "", err
	}
	return out, nil
}
