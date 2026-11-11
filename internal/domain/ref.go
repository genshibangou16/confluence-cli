package domain

import (
	"encoding/base64"
	"encoding/binary"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	digitsRe = regexp.MustCompile(`^[0-9]+$`)
	// 編集画面の URL は /pages/edit-v2/<id> か /pages/edit/<id> になる。
	pagesPathRe = regexp.MustCompile(`/pages/(?:edit-v2/|edit/)?([0-9]+)(?:/|$)`)
	tinyPathRe  = regexp.MustCompile(`/x/([A-Za-z0-9_-]+)/?$`)
)

// ParsePageRef はページ ID、ページ URL、tiny link のいずれかからページ ID を取り出す。
// 前後の空白は無視する。
func ParsePageRef(s string) (PageID, error) {
	s = strings.TrimSpace(s)
	if digitsRe.MatchString(s) {
		return PageID(s), nil
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		if m := pagesPathRe.FindStringSubmatch(u.Path); m != nil {
			return PageID(m[1]), nil
		}
		if id := u.Query().Get("pageId"); digitsRe.MatchString(id) {
			return PageID(id), nil
		}
		if m := tinyPathRe.FindStringSubmatch(u.Path); m != nil {
			if id, ok := decodeTinyLink(m[1]); ok {
				return id, nil
			}
		}
	}
	return "", &Error{
		Kind:    ErrValidation,
		Message: "ページ ID を解釈できません: " + strconv.Quote(s),
		Hint:    "ページ ID（数字）、ページ URL、短縮 URL（/wiki/x/...）のいずれかを指定してください",
	}
}

// decodeTinyLink は tiny link のコードをページ ID に変換する。
// コードは、ページ ID を 8 バイトのリトルエンディアン整数とみなし、
// 末尾の 0 バイトを省いたうえで URL セーフな base64（パディングなし）にしたもの。
func decodeTinyLink(code string) (PageID, bool) {
	if pad := len(code) % 4; pad != 0 {
		code += strings.Repeat("=", 4-pad)
	}
	b, err := base64.URLEncoding.DecodeString(code)
	if err != nil || len(b) == 0 || len(b) > 8 {
		return "", false
	}
	var buf [8]byte
	copy(buf[:], b)
	return PageID(strconv.FormatUint(binary.LittleEndian.Uint64(buf[:]), 10)), true
}
