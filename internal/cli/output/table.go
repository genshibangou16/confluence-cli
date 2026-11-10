// Package output は CLI の標準出力・標準エラー出力の整形を担う。
package output

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"
	"unicode"
)

// SanitizeText は Confluence 由来の文字列を端末へ出力する前に、端末の制御に使える文字を除く。
// ESC や C1 を含む制御文字（改行・タブも含む）は空白に置き換え、表示順を変える
// 双方向制御文字は取り除く。
func SanitizeText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r):
			return ' '
		case r == 0x061C, r == 0x200E, r == 0x200F,
			r >= 0x202A && r <= 0x202E,
			r >= 0x2066 && r <= 0x2069:
			return -1
		}
		return r
	}, s)
}

// Table は header と rows を列揃えして書き出す。区切り行は出さない。
// セルは SanitizeText を通すため、改行とタブは空白になり行がずれない。
func Table(w io.Writer, header []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	write := func(cells []string) {
		out := make([]string, len(cells))
		for i, c := range cells {
			out[i] = SanitizeText(c)
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
	if n < 0 {
		n = 0
	}
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
	s = SanitizeText(html.UnescapeString(s))
	return strings.Join(strings.Fields(s), " ")
}
