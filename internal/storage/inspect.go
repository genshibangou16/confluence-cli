package storage

import (
	"encoding/xml"
	"slices"
	"strconv"
	"strings"
)

// ConvertibleMacros は Markdown で表せるマクロの名前。これ以外のマクロは LossyElements が損失として報告する。
var ConvertibleMacros = []string{"code", "info", "tip", "note", "warning", "toc"}

// CountCommentMarkers は fragment に含まれる ac:inline-comment-marker 要素の数を返す。
// 解析できない箇所に達したら、そこまでの数を返す。
func CountCommentMarkers(fragment string) int {
	// 呼び出し側が先に Validate で整形式を確認するため、解析エラーでは部分的な結果を返してよい。
	d, _ := Decoder(fragment)
	n := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return n
		}
		if se, ok := tok.(xml.StartElement); ok && isAC(se.Name, "inline-comment-marker") {
			n++
		}
	}
}

func isAC(n xml.Name, local string) bool {
	return n.Space == acNamespace && n.Local == local
}

// LossyElements は、Markdown の変換規則で表せない要素の名前を、最初に現れた順に重複なしで返す。
// 名前は、ConvertibleMacros にないマクロの macro:<name>、ac:layout、
// rowspan か colspan が 2 以上のセルを持つ表の table:merged-cells、ac:inline-comment-marker の inline-comment。
// 解析できない箇所に達したら、そこまでの結果を返す。
func LossyElements(body string) []string {
	// 呼び出し側が先に Validate で整形式を確認するため、解析エラーでは部分的な結果を返してよい。
	d, _ := Decoder(body)
	var out []string
	add := func(name string) {
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch {
		case isAC(se.Name, "structured-macro"):
			for _, a := range se.Attr {
				if a.Name.Space == acNamespace && a.Name.Local == "name" && !slices.Contains(ConvertibleMacros, a.Value) {
					add("macro:" + a.Value)
				}
			}
		case isAC(se.Name, "layout"):
			add("ac:layout")
		case isAC(se.Name, "inline-comment-marker"):
			add("inline-comment")
		case se.Name.Space == "" && (se.Name.Local == "th" || se.Name.Local == "td"):
			for _, a := range se.Attr {
				if a.Name.Space == "" && (a.Name.Local == "rowspan" || a.Name.Local == "colspan") {
					if v, err := strconv.Atoi(strings.TrimSpace(a.Value)); err == nil && v > 1 {
						add("table:merged-cells")
					}
				}
			}
		}
	}
}

var blockHTML = []string{"p", "div", "li", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "td", "th", "tr", "table", "blockquote", "pre", "br", "hr"}

var blockAC = []string{"structured-macro", "parameter", "plain-text-body", "rich-text-body", "task", "task-body", "task-list", "layout", "layout-section", "layout-cell"}

// isBlock は、前後のテキストを別の語に分ける要素かを返す。
// インライン要素は語の途中にも現れ、保存時に境界が移ることがあるため、区切りに使わない。
func isBlock(n xml.Name) bool {
	switch n.Space {
	case "":
		return slices.Contains(blockHTML, n.Local)
	case acNamespace:
		return slices.Contains(blockAC, n.Local)
	}
	return false
}

// textWords は body の文字データ（CDATA と ac:parameter の値を含む）を空白で分けた語の並びを返す。
// &nbsp; は Decoder が U+00A0 に展開し、strings.Fields が空白として扱う。
// 解析できない箇所に達したら、そこまでの語を返す。
func textWords(body string) []string {
	d, _ := Decoder(body)
	var sb strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return strings.Fields(sb.String())
		}
		switch tok := tok.(type) {
		case xml.CharData:
			sb.Write(tok)
		case xml.StartElement:
			if isBlock(tok.Name) {
				sb.WriteByte(' ')
			}
		case xml.EndElement:
			if isBlock(tok.Name) {
				sb.WriteByte(' ')
			}
		}
	}
}

// LostText は、sent のテキストにあって got のテキストにない語を、sent での出現順に返す。
// 同じ語は出現回数も数え、sent に 2 回あって got に 1 回なら 1 回だけ返す。
// 属性だけの差は、テキストに現れないため返さない。
func LostText(sent, got string) []string {
	remaining := map[string]int{}
	for _, w := range textWords(got) {
		remaining[w]++
	}
	var lost []string
	for _, w := range textWords(sent) {
		if remaining[w] > 0 {
			remaining[w]--
			continue
		}
		lost = append(lost, w)
	}
	return lost
}
