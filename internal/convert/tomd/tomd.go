// Package tomd は Confluence の storage 形式を Markdown に変換する。
// 出力の主な読み手は LLM のため、少ないバイト数で内容を残すことを優先し、厳密な CommonMark への準拠より読みやすさを選ぶ。
package tomd

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/genshibangou16/confluence-cli/internal/convert"
	"github.com/genshibangou16/confluence-cli/internal/domain"
	"github.com/genshibangou16/confluence-cli/internal/storage"
)

// storage.Decoder が ac: と ri: の接頭辞に割り当てる名前空間。
const (
	acNS = "http://atlassian.com/content"
	riNS = "http://atlassian.com/resource/identifier"
)

// adfPanelToAlert は ADF のパネル（ac:adf-node type="panel"）の種類から GitHub Alerts の種類への対応。
// ADF のパネルは storage 形式のパネルマクロと種類も色も違うため、convert.PanelToAlert とは別に、色と名前が合う種類に対応させる。
// custom は対応する種類がないため、ここに含めずに普通の引用にする。
var adfPanelToAlert = map[string]string{"info": "NOTE", "note": "NOTE", "success": "TIP", "warning": "WARNING", "error": "CAUTION"}

// placeholderSkipParams は、変換規則のないマクロのプレースホルダーに出さないパラメータ。
// 表示の装飾と接続先の識別子で、読み手が内容を理解するのに使わない。
var placeholderSkipParams = []string{
	"server", "serverId", "atlassian-macro-output-type", "breakoutWidth", "breakoutMode",
	"bgColor", "borderColor", "borderStyle", "borderWidth", "titleBGColor", "titleColor",
	"panelIcon", "panelIconId", "panelIconText",
}

type node struct {
	name     xml.Name
	attr     []xml.Attr
	children []*node
	text     string
	isText   bool
}

// Convert は storage 形式の本文を Markdown に変換する。
// 本文が整形式でなければ ErrValidation の *domain.Error（*storage.SyntaxError を包む）を返す。
func Convert(body string) (string, error) {
	root, err := parse(body)
	if err != nil {
		return "", err
	}
	out := strings.Join(renderBlocks(root.children), "\n\n")
	if out == "" {
		return "", nil
	}
	return out + "\n", nil
}

func parse(body string) (*node, error) {
	d, _ := storage.Decoder(body)
	doc := &node{}
	stack := []*node{doc}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 位置付きのエラーメッセージを Validate と同じ形で返す。
			verr := storage.Validate(body)
			if verr == nil {
				verr = err
			}
			return nil, &domain.Error{Kind: domain.ErrValidation, Message: verr.Error(), Err: verr}
		}
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name, attr: t.Attr}
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if k := len(top.children); k > 0 && top.children[k-1].isText {
				top.children[k-1].text += string(t)
			} else {
				top.children = append(top.children, &node{isText: true, text: string(t)})
			}
		}
	}
	// storage.Decoder が包んだルート要素の中身を返す。
	if len(doc.children) == 0 {
		return &node{}, nil
	}
	return doc.children[0], nil
}

func (n *node) isHTML(locals ...string) bool {
	return !n.isText && n.name.Space == "" && slices.Contains(locals, n.name.Local)
}

func (n *node) isAC(local string) bool {
	return !n.isText && n.name.Space == acNS && n.name.Local == local
}

func (n *node) isRI(local string) bool {
	return !n.isText && n.name.Space == riNS && n.name.Local == local
}

// attrValue は名前空間を問わずローカル名で属性を探す。storage 形式では同じ属性が ac: 付きと無しの両方で現れるため。
func (n *node) attrValue(local string) string {
	for _, a := range n.attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func (n *node) child(match func(*node) bool) *node {
	for _, c := range n.children {
		if match(c) {
			return c
		}
	}
	return nil
}

func (n *node) acChild(local string) *node {
	return n.child(func(c *node) bool { return c.isAC(local) })
}

// rawText は子孫の文字データをそのまま連結する。br は改行にする。
func (n *node) rawText() string {
	var sb strings.Builder
	var walk func(*node)
	walk = func(m *node) {
		for _, c := range m.children {
			switch {
			case c.isText:
				sb.WriteString(c.text)
			case c.isHTML("br"):
				sb.WriteByte('\n')
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return sb.String()
}

func macroName(n *node) string { return n.attrValue("name") }

func macroParam(n *node, key string) string {
	for _, c := range n.children {
		if c.isAC("parameter") && c.attrValue("name") == key {
			return strings.TrimSpace(c.rawText())
		}
	}
	return ""
}

// isBlock は n をブロックとして描画するかを返す。ブロックでない要素は、前後の文字と同じ段落に入る。
// 本文を持たないマクロ（status など）は段落の途中に現れるため、インラインとして扱う。
func isBlock(n *node) bool {
	if n.isText {
		return false
	}
	switch n.name.Space {
	case "":
		return slices.Contains([]string{"p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "table", "blockquote", "pre", "hr", "div", "section", "colgroup"}, n.name.Local)
	case acNS:
		switch n.name.Local {
		case "layout", "layout-section", "layout-cell", "task-list", "adf-extension", "rich-text-body", "adf-content":
			return true
		case "structured-macro":
			return slices.Contains(storage.ConvertibleMacros, macroName(n)) ||
				n.acChild("rich-text-body") != nil || n.acChild("plain-text-body") != nil
		}
	}
	return false
}

// renderBlocks は子要素をブロックの並びにする。連続したインラインの要素と文字は 1 つの段落にまとめる。空のブロックは含めない。
func renderBlocks(ns []*node) []string {
	var out []string
	var run []*node
	flush := func() {
		if p := paragraph(run); p != "" {
			out = append(out, p)
		}
		run = nil
	}
	for i, n := range ns {
		if !isBlock(n) {
			if standalone(ns, i) && blank(run) {
				flush()
				out = append(out, paragraph([]*node{n}))
				continue
			}
			run = append(run, n)
			continue
		}
		flush()
		for _, b := range block(n) {
			if b != "" {
				out = append(out, b)
			}
		}
	}
	flush()
	return out
}

// standalone は ns[i] が、文字と同じ段落に入らない単独のマクロかを返す。
// 段落の外に並んだ本文のないマクロ（view-file など）を、1 つの行につなげずに別の行にするため。
func standalone(ns []*node, i int) bool {
	if !ns[i].isAC("structured-macro") {
		return false
	}
	for _, next := range ns[i+1:] {
		if next.isText && strings.Trim(next.text, " \t\r\n") == "" {
			continue
		}
		return isBlock(next) || next.isAC("structured-macro")
	}
	return true
}

func blank(run []*node) bool {
	for _, n := range run {
		if !n.isText || strings.Trim(n.text, " \t\r\n") != "" {
			return false
		}
	}
	return true
}

func block(n *node) []string {
	if n.name.Space == acNS {
		switch n.name.Local {
		case "structured-macro":
			return macroBlock(n)
		case "task-list":
			return []string{taskList(n)}
		case "adf-extension":
			return adfExtension(n)
		}
		return renderBlocks(n.children)
	}
	switch l := n.name.Local; l {
	case "p":
		return []string{paragraph(n.children)}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		text := strings.Join(lines(inline(n.children)), " ")
		if text == "" {
			return nil
		}
		return []string{strings.Repeat("#", int(l[1]-'0')) + " " + text}
	case "ul", "ol":
		return []string{list(n)}
	case "table":
		return []string{table(n)}
	case "blockquote":
		return []string{quote(strings.Join(renderBlocks(n.children), "\n\n"))}
	case "pre":
		return []string{fence("", n.rawText())}
	case "hr":
		return []string{"---"}
	case "colgroup":
		return nil
	}
	return renderBlocks(n.children)
}

var (
	blockMarkup   = regexp.MustCompile("^(#{1,6}( |$)|>|[-*+]( |$)|`{3}|~{3}|(- *)+$|(\\* *)+$|(_ *)+$|=+$)")
	orderedMarkup = regexp.MustCompile(`^(\d+)([.)])( |$)`)
)

// paragraph はインラインの並びを段落にする。行頭と行末の空白と、空の行を除く。
// 行頭が見出し・引用・リスト・コードブロックのフェンスの記号に見える行と、水平線や見出しの下線に見える行は、
// その記法として読まれないように \ でエスケープする。
func paragraph(ns []*node) string {
	ls := lines(inline(ns))
	for i, l := range ls {
		if blockMarkup.MatchString(l) {
			ls[i] = `\` + l
		} else {
			ls[i] = orderedMarkup.ReplaceAllString(l, `$1\$2$3`)
		}
	}
	return strings.Join(ls, "\n")
}

// lines は s を行に分け、各行の前後の空白を除き、空の行を除いて返す。全角空白は内容として残す。
func lines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.Trim(l, " \t"); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// collapseSpace は HTML と同じく、連続した空白を 1 つの空白にする。&nbsp; も空白として扱う。
func collapseSpace(s string) string {
	var sb strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u00a0' {
			if !space {
				sb.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		sb.WriteRune(r)
	}
	return sb.String()
}

// inline はインラインの並びを文字列にする。
// マクロの [...] は前後の文字とつながると 1 語に見えるため、前後に空白がなければ空白を入れる。
func inline(ns []*node) string {
	var sb strings.Builder
	prevMacro := false
	for _, n := range ns {
		s := inlineNode(n)
		if s == "" {
			continue
		}
		macro := n.isAC("structured-macro") && macroName(n) != "code"
		if (macro || prevMacro) && sb.Len() > 0 && !endsWithSpace(sb.String()) && !startsWithSpace(s) {
			sb.WriteByte(' ')
		}
		sb.WriteString(s)
		prevMacro = macro
	}
	return sb.String()
}

func startsWithSpace(s string) bool { return s[0] == ' ' || s[0] == '\n' }

func endsWithSpace(s string) bool { c := s[len(s)-1]; return c == ' ' || c == '\n' }

func inlineNode(n *node) string {
	if n.isText {
		return collapseSpace(n.text)
	}
	if n.name.Space == acNS {
		switch n.name.Local {
		case "link":
			return acLink(n)
		case "image":
			return image(n)
		case "emoticon":
			return ":" + n.attrValue("name") + ":"
		case "structured-macro":
			return macroInline(n)
		case "placeholder":
			// 編集画面にだけ表示される入力の案内で、ページの内容ではない。
			return ""
		}
	}
	if n.name.Space == "" {
		switch n.name.Local {
		case "strong", "b":
			return wrap("**", inline(n.children))
		case "em", "i":
			return wrap("*", inline(n.children))
		case "s", "del", "strike":
			return wrap("~~", inline(n.children))
		case "span":
			if strings.Contains(n.attrValue("style"), "line-through") {
				return wrap("~~", inline(n.children))
			}
		case "code":
			return codeSpan(collapseSpace(n.rawText()))
		case "a":
			return htmlLink(n)
		case "br":
			return "\n"
		case "time":
			return n.attrValue("datetime")
		}
	}
	if isBlock(n) {
		return "\n" + strings.Join(block(n), "\n") + "\n"
	}
	return inline(n.children)
}

// wrap は s の各行を強調の記号で囲む。記号の内側に空白があると強調として読まれないため、前後の空白は記号の外に出す。
func wrap(marker, s string) string {
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		core := strings.Trim(p, " ")
		if core == "" {
			continue
		}
		lead := p[:strings.Index(p, core)]
		trail := p[len(lead)+len(core):]
		parts[i] = lead + marker + core + marker + trail
	}
	return strings.Join(parts, "\n")
}

func codeSpan(s string) string {
	if strings.Trim(s, " ") == "" {
		return s
	}
	ticks := strings.Repeat("`", longestRun(s, '`')+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return ticks + s + ticks
}

func longestRun(s string, r rune) int {
	best, cur := 0, 0
	for _, c := range s {
		if c == r {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

// dest はリンク先を Markdown のリンク先として書く。空白と括弧を含むと Markdown のリンクとして読めないため <> で囲む。
func dest(u string) string {
	if strings.ContainsAny(u, " ()<>") {
		return "<" + u + ">"
	}
	return u
}

func htmlLink(n *node) string {
	href := n.attrValue("href")
	text := strings.Join(lines(inline(n.children)), " ")
	switch {
	case href == "":
		return text
	case text == "" || text == href:
		return href
	}
	return "[" + escapeLinkText(text) + "](" + dest(href) + ")"
}

// resource は ri: の要素が指す先を、リンク先の文字列と既定の表示名にする。
func resource(r *node) (target, label string) {
	switch r.name.Local {
	case "user":
		id := r.attrValue("account-id")
		if id == "" {
			id = r.attrValue("userkey")
		}
		return "@user(" + id + ")", ""
	case "page", "blog-post":
		title := r.attrValue("content-title")
		return "page:" + r.attrValue("space-key") + "/" + title, title
	case "attachment":
		f := r.attrValue("filename")
		return "attachment:" + f, f
	case "url":
		v := r.attrValue("value")
		return v, v
	case "space":
		k := r.attrValue("space-key")
		return "space:" + k, k
	}
	return "link:" + describe(r), ""
}

// describe は変換規則のない ri: の要素を、要素名と属性（版番号と local-id を除く）の 1 行にする。
// リンクの先を捨てたことと、元の参照先を読み手が分かるようにするため。
func describe(r *node) string {
	s := r.name.Local
	for _, a := range r.attr {
		if a.Name.Local == "version-at-save" || a.Name.Local == "local-id" {
			continue
		}
		s += " " + a.Name.Local + "=" + a.Value
	}
	return s
}

// escapeLinkText はリンクの表示名の角括弧をエスケープする。角括弧があるとリンクの終わりの位置が変わるため。
func escapeLinkText(s string) string {
	return strings.NewReplacer("[", `\[`, "]", `\]`).Replace(s)
}

func acLink(n *node) string {
	var label string
	if b := n.acChild("link-body"); b != nil {
		label = strings.Join(lines(inline(b.children)), " ")
	} else if b := n.acChild("plain-text-link-body"); b != nil {
		label = strings.TrimSpace(collapseSpace(b.rawText()))
	}
	target, defaultLabel := "", ""
	if r := n.child(func(c *node) bool { return !c.isText && c.name.Space == riNS }); r != nil {
		target, defaultLabel = resource(r)
		if r.isRI("user") {
			return target
		}
	}
	if a := n.attrValue("anchor"); a != "" {
		target += "#" + a
	}
	if label == "" {
		label = defaultLabel
	}
	if target == "" {
		return label
	}
	if label == "" && strings.HasPrefix(target, "link:") {
		return "[link: " + strings.TrimPrefix(target, "link:") + "]"
	}
	return "[" + escapeLinkText(label) + "](" + dest(target) + ")"
}

func image(n *node) string {
	target := ""
	if r := n.child(func(c *node) bool { return !c.isText && c.name.Space == riNS }); r != nil {
		target, _ = resource(r)
	}
	s := "![" + escapeLinkText(n.attrValue("alt")) + "](" + dest(target) + ")"
	if c := n.acChild("caption"); c != nil {
		if caption := strings.Join(lines(inline(c.children)), " "); caption != "" {
			s += " " + caption
		}
	}
	return s
}

func macroBlock(n *node) []string {
	name := macroName(n)
	var body []string
	if rb := n.acChild("rich-text-body"); rb != nil {
		body = renderBlocks(rb.children)
	}
	switch name {
	case "code":
		return []string{titled(macroParam(n, "title"), fence(macroParam(n, "language"), plainBody(n)))}
	case "toc":
		return []string{"[TOC]"}
	}
	if kind, ok := convert.PanelToAlert[name]; ok {
		return []string{alert(kind, macroParam(n, "title"), body)}
	}
	out := []string{placeholder(n)}
	if pb := n.acChild("plain-text-body"); pb != nil {
		out = append(out, fence("", pb.rawText()))
	}
	return append(out, body...)
}

func macroInline(n *node) string {
	name := macroName(n)
	var body string
	if rb := n.acChild("rich-text-body"); rb != nil {
		body = inline(rb.children)
	}
	switch name {
	case "code":
		return codeSpan(collapseSpace(plainBody(n)))
	case "toc":
		return "[TOC]"
	}
	if kind, ok := convert.PanelToAlert[name]; ok {
		if body = strings.Join(lines(body), " "); body == "" {
			return "[" + kind + "]"
		}
		return "[" + kind + ": " + body + "]"
	}
	s := placeholder(n)
	if pb := n.acChild("plain-text-body"); pb != nil {
		if code := codeSpan(collapseSpace(pb.rawText())); strings.Trim(code, " ") != "" {
			s += " " + code
		}
	}
	if body != "" {
		s += " " + body
	}
	return s
}

func plainBody(n *node) string {
	if pb := n.acChild("plain-text-body"); pb != nil {
		return pb.rawText()
	}
	return ""
}

// placeholder は変換規則のないマクロの名前とパラメータを 1 行で示す。マクロの内容を変換しなかったことを読み手に示すため。
func placeholder(n *node) string {
	var sb strings.Builder
	sb.WriteString("[macro: " + macroName(n))
	for _, p := range n.children {
		if !p.isAC("parameter") {
			continue
		}
		key := p.attrValue("name")
		if slices.Contains(placeholderSkipParams, key) {
			continue
		}
		value := strings.TrimSpace(collapseSpace(p.rawText()))
		if r := p.child(func(c *node) bool { return !c.isText && c.name.Space == riNS }); r != nil {
			value, _ = resource(r)
		}
		if value == "" {
			continue
		}
		if strings.ContainsAny(value, " \"]") {
			value = strconv.Quote(value)
		}
		sb.WriteByte(' ')
		if key != "" {
			sb.WriteString(key + "=")
		}
		sb.WriteString(value)
	}
	sb.WriteByte(']')
	return sb.String()
}

func titled(title, s string) string {
	if title == "" {
		return s
	}
	return "**" + title + "**\n" + s
}

// fence は body をフェンス付きコードブロックにする。body がバッククォートの並びを含むときは、それより長いフェンスにする。
func fence(lang, body string) string {
	body = strings.TrimSuffix(body, "\n")
	f := strings.Repeat("`", max(3, longestRun(body, '`')+1))
	return f + lang + "\n" + body + "\n" + f
}

func alert(kind, title string, body []string) string {
	s := "[!" + kind + "]"
	if title != "" {
		s += "\n**" + title + "**"
	}
	if len(body) > 0 {
		s += "\n" + strings.Join(body, "\n\n")
	}
	return quote(s)
}

func quote(s string) string {
	return prefixLines(s, "> ", ">")
}

// prefixLines は s の各行の先頭に prefix を付ける。空の行には、行末に空白を残さないよう emptyPrefix を付ける。
func prefixLines(s, prefix, emptyPrefix string) string {
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		if l == "" {
			ls[i] = emptyPrefix
		} else {
			ls[i] = prefix + l
		}
	}
	return strings.Join(ls, "\n")
}

// item はリストの項目を marker で始め、2 行目以降を indent 文字だけ字下げする。
// 字下げをリストの記号（"- " や "1. "）の幅に合わせるのは、番号付きリストの中の入れ子が Markdown で入れ子として読まれるようにするため。
func item(marker string, indent int, content string) string {
	first, rest, _ := strings.Cut(content, "\n")
	s := strings.TrimRight(marker+first, " ")
	if rest != "" {
		s += "\n" + prefixLines(rest, strings.Repeat(" ", indent), "")
	}
	return s
}

func list(n *node) string {
	ordered := n.name.Local == "ol"
	num := 1
	if v, err := strconv.Atoi(n.attrValue("start")); err == nil {
		num = v
	}
	var items []string
	for _, c := range n.children {
		if c.isText {
			continue
		}
		content := strings.Join(renderBlocks(c.children), "\n")
		if !c.isHTML("li") {
			// li の外に置かれた要素（入れ子のリストなど）は、直前の項目の続きとして字下げする。
			content = strings.Join(block(c), "\n")
			if len(items) > 0 && content != "" {
				items[len(items)-1] += "\n" + prefixLines(content, "  ", "")
				continue
			}
		}
		marker := "- "
		if ordered {
			marker = fmt.Sprintf("%d. ", num)
			num++
		}
		items = append(items, item(marker, len(marker), content))
	}
	return strings.Join(items, "\n")
}

func taskList(n *node) string {
	var items []string
	for _, t := range n.children {
		if !t.isAC("task") {
			continue
		}
		marker := "- [ ] "
		if s := t.acChild("task-status"); s != nil && strings.TrimSpace(s.rawText()) == "complete" {
			marker = "- [x] "
		}
		var content string
		if b := t.acChild("task-body"); b != nil {
			content = strings.Join(renderBlocks(b.children), "\n")
		}
		// "[ ]" は項目の内容の一部として読まれるため、入れ子の字下げは "- " の幅にする。
		items = append(items, item(marker, 2, content))
	}
	return strings.Join(items, "\n")
}

func adfExtension(n *node) []string {
	fallback := n.acChild("adf-fallback")
	nd := n.acChild("adf-node")
	if nd == nil {
		if fallback == nil {
			return nil
		}
		return renderBlocks(fallback.children)
	}
	var body []string
	if c := nd.acChild("adf-content"); c != nil {
		body = renderBlocks(c.children)
	} else if fallback != nil {
		body = renderBlocks(fallback.children)
	}
	typ := nd.attrValue("type")
	if typ == "panel" {
		var panelType string
		for _, a := range nd.children {
			if a.isAC("adf-attribute") && a.attrValue("key") == "panel-type" {
				panelType = strings.TrimSpace(a.rawText())
			}
		}
		if kind, ok := adfPanelToAlert[panelType]; ok {
			return []string{alert(kind, "", body)}
		}
		return []string{quote(strings.Join(body, "\n\n"))}
	}
	return append([]string{"[adf: " + typ + "]"}, body...)
}

// table は GFM のテーブルにする。1 行目を見出しの行にする。
// セルの前後に空白を入れない（|a|b|）。GFM として読めて、セルの多い表でバイト数が減るため。
// セルの中の複数の行は、テーブルの 1 行に収めるため " / " でつなぐ。
// 結合したセルの値は左上のセルにだけ置き、結合したことを表の直前の [table: merged cells] で示す。
func table(n *node) string {
	var rows []*node
	for _, c := range n.children {
		if c.isHTML("tr") {
			rows = append(rows, c)
		} else if c.isHTML("thead", "tbody", "tfoot") {
			for _, r := range c.children {
				if r.isHTML("tr") {
					rows = append(rows, r)
				}
			}
		}
	}
	if len(rows) == 0 {
		return ""
	}
	// 行数と列数はページの内容が決めるため、結合で埋まるセルの数か格子の大きさが上限を超える表は、格子を作らずに省略する。
	tooLarge := "[table: too large]"
	grid := make([][]string, len(rows))
	occupied := make([][]bool, len(rows))
	cells := 0
	merged := false
	ncols := 0
	for r, row := range rows {
		col := 0
		for _, cell := range row.children {
			if !cell.isHTML("th", "td") {
				continue
			}
			for col < len(occupied[r]) && occupied[r][col] {
				col++
			}
			rs, cs := min(span(cell, "rowspan"), len(rows)-r), span(cell, "colspan")
			if cs > maxColumns || col+cs > maxColumns {
				return tooLarge
			}
			if cells += rs * cs; cells > maxCells {
				return tooLarge
			}
			if rs > 1 || cs > 1 {
				merged = true
			}
			for dr := range rs {
				occ := occupied[r+dr]
				for len(occ) < col+cs {
					occ = append(occ, false)
				}
				for dc := range cs {
					occ[col+dc] = true
				}
				occupied[r+dr] = occ
			}
			for len(grid[r]) <= col {
				grid[r] = append(grid[r], "")
			}
			text := cellText(cell)
			if r == 0 {
				text = unbold(text)
			}
			grid[r][col] = text
			col += cs
		}
		ncols = max(ncols, len(occupied[r]), len(grid[r]))
	}
	if len(rows)*ncols > maxCells {
		return tooLarge
	}
	var sb strings.Builder
	if merged {
		sb.WriteString("[table: merged cells]\n")
	}
	writeRow := func(cells []string) {
		sb.WriteString("|")
		for c := range ncols {
			v := ""
			if c < len(cells) {
				v = cells[c]
			}
			sb.WriteString(v + "|")
		}
		sb.WriteByte('\n')
	}
	writeRow(grid[0])
	sb.WriteString(strings.Repeat("|---", ncols) + "|\n")
	for _, cells := range grid[1:] {
		writeRow(cells)
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// maxColumns は格子を作る表の列数の上限で、HTML の colspan の上限と同じ値。
// maxCells は格子のセルの数の上限。
const (
	maxColumns = 1000
	maxCells   = 100000
)

func span(cell *node, attr string) int {
	if v, err := strconv.Atoi(strings.TrimSpace(cell.attrValue(attr))); err == nil && v > 1 {
		return v
	}
	return 1
}

func cellText(cell *node) string {
	s := strings.Join(cellLines(strings.Join(renderBlocks(cell.children), "\n")), " / ")
	return strings.ReplaceAll(s, "|", `\|`)
}

// cellLines は lines と同じく行に分けるが、コードブロックの中の行は字下げを残す。
func cellLines(s string) []string {
	var out []string
	inCode := false
	for l := range strings.SplitSeq(s, "\n") {
		t := strings.Trim(l, " \t")
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inCode = !inCode
		} else if inCode {
			t = strings.TrimRight(l, " \t")
		}
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// unbold は見出しの行のセル全体が太字なら太字の記号を除く。見出しの行は太字で表示されるため、記号は情報を増やさない。
func unbold(s string) string {
	if strings.HasPrefix(s, "**") && strings.HasSuffix(s, "**") && len(s) > 4 && strings.Count(s, "**") == 2 {
		return s[2 : len(s)-2]
	}
	return s
}
