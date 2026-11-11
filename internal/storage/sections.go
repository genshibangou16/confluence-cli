package storage

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// Heading は本文中の見出し要素。Start と End は開始タグの先頭と終了タグの末尾のバイト位置で、
// Text はタグを除き、br を空白にして空白をまとめた文字列。TopLevel は、親が本文のルートか ac:layout-cell であることを表す。
type Heading struct {
	Level      int
	Text       string
	Start, End int
	TopLevel   bool
}

// Section は見出しと、その見出しの節の中身のバイト範囲 [ContentStart, ContentEnd)。
type Section struct {
	Heading                  Heading
	ContentStart, ContentEnd int
}

// noHeading は、見出しを含まない要素の minLevel。どの見出しのレベルより大きい。
const noHeading = 7

type element struct {
	name       xml.Name
	parent     int
	start, end int
	closeStart int // 終了タグの先頭。自己終了タグでは end と同じ。
	level      int // 見出しなら 1〜6、それ以外は 0
	minLevel   int // 自身と子孫の見出しの最小レベル
	children   []int
	text       strings.Builder
}

// tree は本文の要素を文書順に並べたもの。elems[0] は Decoder が包んだルート要素で、
// その範囲は本文全体（closeStart が len(body)）になる。
type tree struct {
	elems    []*element
	headings []int
}

func parse(body string) (*tree, error) {
	d, prefix := Decoder(body)
	t := &tree{}
	var stack []int
	inHeading := -1
	for {
		off := int(d.InputOffset()) - prefix
		tok, err := d.Token()
		if err == io.EOF {
			return t, nil
		}
		if err != nil {
			se := syntaxError(body, d, prefix, err)
			return nil, &domain.Error{Kind: domain.ErrValidation, Message: se.Error(), Err: se}
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			e := &element{name: tok.Name, parent: -1, start: off, level: headingLevel(tok.Name), minLevel: noHeading}
			idx := len(t.elems)
			if len(stack) > 0 {
				e.parent = stack[len(stack)-1]
				t.elems[e.parent].children = append(t.elems[e.parent].children, idx)
			}
			// tomd は見出しの中の br を空白にするため、見出しの文字列でも空白として扱う。
			if inHeading >= 0 && tok.Name.Space == "" && tok.Name.Local == "br" {
				t.elems[inHeading].text.WriteByte(' ')
			}
			if e.level > 0 {
				e.minLevel = e.level
				t.headings = append(t.headings, idx)
				if inHeading < 0 {
					inHeading = idx
				}
			}
			t.elems = append(t.elems, e)
			stack = append(stack, idx)
		case xml.EndElement:
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			e := t.elems[idx]
			e.closeStart = off
			e.end = int(d.InputOffset()) - prefix
			if e.parent >= 0 {
				p := t.elems[e.parent]
				p.minLevel = min(p.minLevel, e.minLevel)
			}
			if idx == inHeading {
				inHeading = -1
			}
		case xml.CharData:
			if inHeading >= 0 {
				t.elems[inHeading].text.Write(tok)
			}
		}
	}
}

func headingLevel(n xml.Name) int {
	if n.Space == "" && len(n.Local) == 2 && n.Local[0] == 'h' && n.Local[1] >= '1' && n.Local[1] <= '6' {
		return int(n.Local[1] - '0')
	}
	return 0
}

// normalize は見出しの文字列を比べるために、空白（&nbsp; を含む）の連続を 1 つの空白にまとめ、前後の空白を除く。
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (t *tree) heading(idx int) Heading {
	e := t.elems[idx]
	top := e.parent == 0 || t.elems[e.parent].name.Local == "layout-cell" && t.elems[e.parent].name.Space == acNamespace
	return Heading{Level: e.level, Text: normalize(e.text.String()), Start: e.start, End: e.end, TopLevel: top}
}

// acNamespace は Decoder が ac: の接頭辞に割り当てた名前空間。
const acNamespace = "http://atlassian.com/content"

// Headings は本文中のすべての見出しを文書順に返す。CDATA の中の文字列は見出しとして扱わない。
// 本文が整形式でなければ ErrValidation の *domain.Error（*SyntaxError を包む）を返す。
func Headings(body string) ([]Heading, error) {
	t, err := parse(body)
	if err != nil {
		return nil, err
	}
	hs := make([]Heading, 0, len(t.headings))
	for _, idx := range t.headings {
		hs = append(hs, t.heading(idx))
	}
	return hs, nil
}

// FindSection は、正規化した文字列が title と一致するトップレベルの見出しの節を返す。
// 節の中身は見出しの終了タグの直後から、見出しと同じ親を持つ後続の要素のうち、
// 同じかより上のレベルの見出しであるか、そのような見出しを子孫に含む最初の要素の開始位置まで。
// 該当する要素が無ければ親の終了タグの先頭（親がルートなら本文の末尾、ac:layout-cell ならそのセルの終わり）まで。
// 範囲を親の子要素の境界で区切るため、範囲を置き換えても要素の入れ子は崩れない。
func FindSection(body, title string) (Section, error) {
	want := normalize(title)
	if want == "" {
		return Section{}, &domain.Error{Kind: domain.ErrValidation, Message: "見出しを指定してください"}
	}
	t, err := parse(body)
	if err != nil {
		return Section{}, err
	}
	var matches []int
	for _, idx := range t.headings {
		if h := t.heading(idx); h.TopLevel && h.Text == want {
			matches = append(matches, idx)
		}
	}
	switch len(matches) {
	case 0:
		return Section{}, &domain.Error{
			Kind:    domain.ErrValidation,
			Message: fmt.Sprintf("トップレベルに「%s」という見出しがありません（マクロや表の中の見出しは対象外です）", want),
		}
	case 1:
	default:
		list := make([]string, 0, len(matches))
		for _, idx := range matches {
			h := t.heading(idx)
			list = append(list, fmt.Sprintf("h%d「%s」（バイト位置 %d）", h.Level, h.Text, h.Start))
		}
		return Section{}, &domain.Error{
			Kind:    domain.ErrValidation,
			Message: fmt.Sprintf("「%s」に一致する見出しが %d 個あります: %s", want, len(matches), strings.Join(list, "、")),
		}
	}

	idx := matches[0]
	e := t.elems[idx]
	p := t.elems[e.parent]
	end := p.closeStart
	for _, c := range p.children {
		if ce := t.elems[c]; ce.start >= e.end && ce.minLevel <= e.level {
			end = ce.start
			break
		}
	}
	return Section{Heading: t.heading(idx), ContentStart: e.end, ContentEnd: end}, nil
}

// ReplaceSection は title の節の中身を content に置き換えた本文と、置き換える前の節の中身を返す。
// 見出し自体と節の外側は元のバイト列のまま残す。
func ReplaceSection(body, title, content string) (replaced, old string, err error) {
	s, err := FindSection(body, title)
	if err != nil {
		return "", "", err
	}
	return body[:s.ContentStart] + content + body[s.ContentEnd:], body[s.ContentStart:s.ContentEnd], nil
}

// Append は本文の末尾に add を足す。
func Append(body, add string) string { return body + add }

// Prepend は本文の先頭に add を足す。
func Prepend(body, add string) string { return add + body }
