package frommd

import (
	"bufio"
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"

	"github.com/genshibangou16/confluence-cli/internal/convert"
)

// alertLine は GitHub Alerts の 1 行目（"[!NOTE]" など）。
var alertLine = regexp.MustCompile(`^\[!([A-Za-z]+)\]$`)

type renderer struct {
	src    []byte
	buf    bytes.Buffer
	taskID int
}

func (r *renderer) write(ss ...string) {
	for _, s := range ss {
		r.buf.WriteString(s)
	}
}

// writeText は Markdown のテキストを XHTML のテキストとして書く。
// バックスラッシュのエスケープと文字参照の解決は goldmark の HTML レンダラーと同じ処理を使い、&・<・>・" をエスケープする。
// "&#1;" のような文字参照は解決すると XML で使えない文字になるため、書く前に除く。
func (r *renderer) writeText(v []byte) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	html.DefaultWriter.Write(w, v)
	_ = w.Flush()
	r.buf.WriteString(dropInvalidXMLChars(b.String()))
}

// writeEscaped は v を解釈せずに XHTML のテキストとしてエスケープして書く。
func (r *renderer) writeEscaped(v []byte) {
	r.buf.Write(util.EscapeHTML(v))
}

func (r *renderer) blocks(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.block(c)
	}
}

func (r *renderer) block(n ast.Node) {
	switch n := n.(type) {
	case *ast.Heading:
		tag := "h" + strconv.Itoa(n.Level)
		r.write("<", tag, ">")
		r.inlines(n.FirstChild())
		r.write("</", tag, ">\n")
	case *ast.Paragraph:
		if r.lineText(n, 0) == "[TOC]" && n.Lines().Len() == 1 {
			r.write(`<ac:structured-macro ac:name="toc" ac:schema-version="1">`+"\n",
				`<ac:parameter ac:name="maxLevel">3</ac:parameter>`+"\n", "</ac:structured-macro>\n")
			return
		}
		r.paragraph(n.FirstChild())
	case *ast.TextBlock:
		// 詰めたリストの項目の本文。変換リファレンスに合わせて <p> で包まない。
		r.inlines(n.FirstChild())
	case *ast.ThematicBreak:
		r.write("<hr />\n")
	case *ast.FencedCodeBlock:
		r.code(n, convert.NormalizeLanguage(string(n.Language(r.src))))
	case *ast.CodeBlock:
		r.code(n, "")
	case *ast.Blockquote:
		r.blockquote(n)
	case *ast.List:
		r.list(n)
	case *ast.HTMLBlock:
		r.htmlBlock(n)
	case *east.Table:
		r.table(n)
	default:
		r.blocks(n)
	}
}

// lineText は n の i 行目の、前後の空白を除いた原文。
func (r *renderer) lineText(n ast.Node, i int) string {
	if n.Lines().Len() <= i {
		return ""
	}
	seg := n.Lines().At(i)
	return strings.TrimSpace(string(seg.Value(r.src)))
}

// paragraph は first から始まるインラインを <p> で包んで書く。インラインがなければ何も書かない。
func (r *renderer) paragraph(first ast.Node) {
	if first == nil {
		return
	}
	r.write("<p>")
	r.inlines(first)
	r.write("</p>\n")
}

func (r *renderer) code(n ast.Node, lang string) {
	var b strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		b.Write(seg.Value(r.src))
	}
	body := strings.TrimSuffix(b.String(), "\n")
	r.write(`<ac:structured-macro ac:name="code" ac:schema-version="1">` + "\n")
	if lang != "" {
		r.write(`<ac:parameter ac:name="language">`, lang, "</ac:parameter>\n")
	}
	// CDATA は "]]>" で終わるため、本文の "]]>" は "]]" と ">" を別の CDATA に分けて書く。
	r.write("<ac:plain-text-body><![CDATA[", strings.ReplaceAll(body, "]]>", "]]]]><![CDATA[>"), "]]></ac:plain-text-body>\n", "</ac:structured-macro>\n")
}

// blockquote は GitHub Alerts をパネルマクロにし、それ以外の引用を blockquote にする。
// Alerts とみなすのは、最初の段落の 1 行目が "[!TYPE]" だけで、TYPE が convert.AlertToPanel にあるときだけ。
func (r *renderer) blockquote(n *ast.Blockquote) {
	p, ok := n.FirstChild().(*ast.Paragraph)
	var panel string
	if ok {
		if m := alertLine.FindStringSubmatch(r.lineText(p, 0)); m != nil {
			panel = convert.AlertToPanel[strings.ToUpper(m[1])]
		}
	}
	if panel == "" {
		r.write("<blockquote>\n")
		r.blocks(n)
		r.write("</blockquote>\n")
		return
	}
	r.write(`<ac:structured-macro ac:name="`, panel, `" ac:schema-version="1">`+"\n", "<ac:rich-text-body>\n")
	// 1 行目の "[!TYPE]" はテキストのノードとして解析されるため、1 行目に収まるテキストのノードを除いて書く。
	end := p.Lines().At(0).Stop
	first := p.FirstChild()
	for first != nil {
		t, ok := first.(*ast.Text)
		if !ok || t.Segment.Stop > end {
			break
		}
		first = first.NextSibling()
	}
	r.paragraph(first)
	for c := p.NextSibling(); c != nil; c = c.NextSibling() {
		r.block(c)
	}
	r.write("</ac:rich-text-body>\n", "</ac:structured-macro>\n")
}

// taskCheckBox は項目 li の最初のブロックの先頭にあるチェックボックスを返す。なければ nil を返す。
func taskCheckBox(li ast.Node) *east.TaskCheckBox {
	if li.FirstChild() == nil || li.FirstChild().FirstChild() == nil {
		return nil
	}
	cb, _ := li.FirstChild().FirstChild().(*east.TaskCheckBox)
	return cb
}

func (r *renderer) list(n *ast.List) {
	isTask := n.FirstChild() != nil
	for li := n.FirstChild(); li != nil; li = li.NextSibling() {
		if taskCheckBox(li) == nil {
			isTask = false
		}
	}
	if isTask {
		r.taskList(n)
		return
	}
	tag, open := "ul", "<ul>\n"
	if n.IsOrdered() {
		tag, open = "ol", "<ol>\n"
		if n.Start != 1 {
			open = `<ol start="` + strconv.Itoa(n.Start) + `">` + "\n"
		}
	}
	r.write(open)
	for li := n.FirstChild(); li != nil; li = li.NextSibling() {
		r.write("<li>")
		r.blocks(li)
		r.write("</li>\n")
	}
	r.write("</", tag, ">\n")
}

// taskList はすべての項目にチェックボックスがあるリストを ac:task-list にする。
// ac:task-id は文書全体で 1 からの連番にし、入れ子のリストは親のタスクの ac:task-body の中に置く。
func (r *renderer) taskList(n *ast.List) {
	r.write("<ac:task-list>\n")
	for li := n.FirstChild(); li != nil; li = li.NextSibling() {
		cb := taskCheckBox(li)
		r.taskID++
		status := "incomplete"
		if cb.IsChecked {
			status = "complete"
		}
		r.write("<ac:task>\n<ac:task-id>", strconv.Itoa(r.taskID), "</ac:task-id>\n<ac:task-status>", status, "</ac:task-status>\n<ac:task-body>")
		r.inlines(cb.NextSibling())
		for c := li.FirstChild().NextSibling(); c != nil; c = c.NextSibling() {
			r.block(c)
		}
		r.write("</ac:task-body>\n</ac:task>\n")
	}
	r.write("</ac:task-list>\n")
}

// htmlBlock は生の HTML のブロックを、タグを含めてテキストとして書く。
// storage 形式に Markdown 中の HTML タグを混ぜないため。
func (r *renderer) htmlBlock(n *ast.HTMLBlock) {
	var lines []string
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		lines = append(lines, string(seg.Value(r.src)))
	}
	if n.HasClosure() {
		lines = append(lines, string(n.ClosureLine.Value(r.src)))
	}
	r.write("<p>")
	for i, l := range lines {
		if i > 0 {
			r.write("<br />")
		}
		r.writeEscaped([]byte(strings.TrimRight(l, "\r\n")))
	}
	r.write("</p>\n")
}

func (r *renderer) table(n *east.Table) {
	r.write("<table>\n<tbody>\n")
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		tag := "td"
		if _, ok := row.(*east.TableHeader); ok {
			tag = "th"
		}
		r.write("<tr>\n")
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			r.write("<", tag, ">")
			r.inlines(cell.FirstChild())
			r.write("</", tag, ">\n")
		}
		r.write("</tr>\n")
	}
	r.write("</tbody>\n</table>\n")
}

// inlines は first とその後ろの兄弟のインラインを書く。
func (r *renderer) inlines(first ast.Node) {
	for c := first; c != nil; c = c.NextSibling() {
		r.inline(c)
	}
}

func (r *renderer) inline(n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		if n.IsRaw() {
			r.writeEscaped(n.Segment.Value(r.src))
		} else {
			r.writeText(n.Segment.Value(r.src))
		}
		if n.HardLineBreak() {
			r.write("<br />")
		} else if n.SoftLineBreak() && !joinsWithoutSpace(n, r.src) {
			r.write(" ")
		}
	case *ast.String:
		if n.IsCode() || n.IsRaw() {
			r.writeEscaped(n.Value)
		} else {
			r.writeText(n.Value)
		}
	case *ast.CodeSpan:
		r.write("<code>")
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				r.writeEscaped(bytes.ReplaceAll(t.Segment.Value(r.src), []byte("\n"), []byte(" ")))
			} else if s, ok := c.(*ast.String); ok {
				r.writeEscaped(s.Value)
			}
		}
		r.write("</code>")
	case *ast.Emphasis:
		tag := "em"
		if n.Level == 2 {
			tag = "strong"
		}
		r.write("<", tag, ">")
		r.inlines(n.FirstChild())
		r.write("</", tag, ">")
	case *east.Strikethrough:
		r.write(`<span style="text-decoration: line-through;">`)
		r.inlines(n.FirstChild())
		r.write("</span>")
	case *ast.Link:
		r.link(n.Destination, func() { r.inlines(n.FirstChild()) })
	case *ast.AutoLink:
		u := n.URL(r.src)
		if n.AutoLinkType == ast.AutoLinkEmail && !bytes.HasPrefix(bytes.ToLower(u), []byte("mailto:")) {
			u = append([]byte("mailto:"), u...)
		}
		r.link(u, func() { r.writeEscaped(n.Label(r.src)) })
	case *ast.Image:
		r.image(n)
	case *ast.RawHTML:
		// 生の HTML のタグは、storage 形式に混ぜないためテキストとして書く。
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			r.writeEscaped(seg.Value(r.src))
		}
	case *east.TaskCheckBox:
		if n.IsChecked {
			r.write("[x] ")
		} else {
			r.write("[ ] ")
		}
	default:
		r.inlines(n.FirstChild())
	}
}

// link は dest へのリンクを書く。javascript: などの危険な URL はリンクにせず、表示テキストだけを書く。
// 危険かどうかは、"&#106;avascript:" のような文字参照を解決した後の URL で判定する。
func (r *renderer) link(dest []byte, label func()) {
	u := util.URLEscape(dest, true)
	if html.IsDangerousURL(u) {
		label()
		return
	}
	r.write(`<a href="`)
	r.writeEscaped(u)
	r.write(`">`)
	label()
	r.write("</a>")
}

// image は外部 URL の画像を ac:image にする。
// それ以外（相対パスなど）は Confluence 上で画像の参照先を解決できないため、代替テキストを表示テキストにしたリンクにする。
func (r *renderer) image(n *ast.Image) {
	alt := plainText(n, r.src)
	dest := string(n.Destination)
	lower := strings.ToLower(dest)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		if alt == "" {
			alt = dest
		}
		r.link(n.Destination, func() { r.writeEscaped([]byte(alt)) })
		return
	}
	r.write("<ac:image")
	if alt != "" {
		r.write(` ac:alt="`)
		r.writeEscaped([]byte(alt))
		r.write(`"`)
	}
	r.write(`><ri:url ri:value="`)
	r.writeEscaped(util.URLEscape(n.Destination, true))
	r.write(`" /></ac:image>`)
}

// joinsWithoutSpace は、ソフト改行の前の文字と後の文字がどちらも東アジアの全角文字かを返す。
// 日本語の文の途中の改行を空白にすると、Confluence の表示で文の途中に空白が入るため、両側が全角文字のときは空白を入れずにつなげる。
// 判定は goldmark の EastAsianLineBreaksSimple と同じ。
func joinsWithoutSpace(t *ast.Text, src []byte) bool {
	v := t.Segment.Value(src)
	if len(v) == 0 {
		return false
	}
	next := firstRune(t.NextSibling(), src)
	return next >= 0 && util.IsEastAsianWideRune(util.ToRune(v, len(v)-1)) && util.IsEastAsianWideRune(next)
}

// firstRune は n から後ろの兄弟と子孫をたどって、最初に現れるテキストの先頭の文字を返す。テキストがなければ -1 を返す。
func firstRune(n ast.Node, src []byte) rune {
	for ; n != nil; n = n.NextSibling() {
		var v []byte
		switch c := n.(type) {
		case *ast.Text:
			v = c.Segment.Value(src)
		case *ast.String:
			v = c.Value
		default:
			if r := firstRune(n.FirstChild(), src); r >= 0 {
				return r
			}
		}
		if len(v) > 0 {
			r, _ := utf8.DecodeRune(v)
			return r
		}
	}
	return -1
}

// plainText は n の子孫のテキストを装飾を除いてつなげる。
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(src))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// dropInvalidXMLChars は XML 1.0 で使えない文字（タブ・改行・復帰以外の制御文字など）を除く。
// Markdown には制御文字や "&#1;" のような文字参照を書けるが、storage 形式に含めると整形式の XML にならないため。
func dropInvalidXMLChars(s string) string {
	return strings.Map(func(c rune) rune {
		switch {
		case c == '\t' || c == '\n' || c == '\r',
			c >= 0x20 && c <= 0xD7FF,
			c >= 0xE000 && c <= 0xFFFD,
			c >= 0x10000 && c <= 0x10FFFF:
			return c
		}
		return -1
	}, s)
}
