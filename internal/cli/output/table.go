// Package output は CLI の標準出力・標準エラー出力の整形を担う。
package output

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"
)

var cellReplacer = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")

// Table は header と rows を列揃えして書き出す。区切り行は出さない。
// セル内の改行とタブは、行がずれないよう空白に置き換える。
func Table(w io.Writer, header []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	write := func(cells []string) {
		out := make([]string, len(cells))
		for i, c := range cells {
			out[i] = cellReplacer.Replace(c)
		}
		fmt.Fprintln(tw, strings.Join(out, "\t"))
	}
	write(header)
	for _, r := range rows {
		write(r)
	}
	return tw.Flush()
}

// Truncate は s を n 文字（rune 単位）に切り詰め、超えた場合は末尾に「…」を付ける。
// 日本語のマルチバイト文字を途中で切らないため、バイト数ではなく rune 数で数える。
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// CleanExcerpt は検索結果の抜粋から、ハイライト用のマーカーと HTML タグを除き、
// HTML エンティティを戻して、連続する空白を 1 つにまとめる。
func CleanExcerpt(s string) string {
	s = strings.NewReplacer("@@@hl@@@", "", "@@@endhl@@@", "").Replace(s)
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}
