package confluence

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

const maxMessageBytes = 500

// apiError は Confluence の非 2xx 応答を domain.Error に変換する。
func apiError(ep endpoint, status int, body []byte) *domain.Error {
	e := &domain.Error{Kind: domain.ErrAPI, Status: status, Message: errorMessage(body)}
	scopes := strings.Join(ep.scopes, ", ")
	switch status {
	case http.StatusBadRequest:
		e.Kind = domain.ErrValidation
	case http.StatusUnauthorized:
		e.Kind = domain.ErrAuth
		e.Hint = "API トークンが失効しているか、必要なスコープがありません（API トークンの有効期限は最長 1 年）。この操作に必要なスコープ: " + scopes
	case http.StatusForbidden:
		e.Kind = domain.ErrForbidden
		e.Hint = "この操作の権限がありません。この操作に必要なスコープ: " + scopes
	case http.StatusNotFound:
		e.Kind = domain.ErrNotFound
	case http.StatusConflict:
		e.Kind = domain.ErrConflict
		e.Hint = "版の競合か、公開前の承認が必要なスペースです。page get で最新の版を確認してください。"
	case http.StatusTooManyRequests:
		e.Kind = domain.ErrRateLimit
	}
	return e
}

// errorMessage は応答の message を返す。message が無ければ本文の先頭 500 バイトを返す。
func errorMessage(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		return m.Message
	}
	if len(body) > maxMessageBytes {
		body = body[:maxMessageBytes]
	}
	return string(body)
}
