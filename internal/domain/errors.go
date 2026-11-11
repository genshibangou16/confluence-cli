package domain

import "errors"

// ErrorKind はエラーの分類。値はエラーの JSON 出力の kind にそのまま出力される。
type ErrorKind string

const (
	ErrAuth            ErrorKind = "auth"
	ErrForbidden       ErrorKind = "forbidden"
	ErrNotFound        ErrorKind = "not_found"
	ErrConflict        ErrorKind = "conflict"
	ErrValidation      ErrorKind = "validation"
	ErrRateLimit       ErrorKind = "rate_limit"
	ErrReadOnly        ErrorKind = "read_only"
	ErrVersionMismatch ErrorKind = "version_mismatch"
	ErrLossy           ErrorKind = "lossy"
	ErrCommentLoss     ErrorKind = "comment_loss"
	// ErrVerifyFailed は、書き込みは成功したが、書き込み後に読み戻した本文の確認で失敗したことを表す。
	ErrVerifyFailed ErrorKind = "verify_failed"
	ErrAPI          ErrorKind = "api"
)

// Error は分類付きのエラー。Status は HTTP ステータス（HTTP 由来でなければ 0）、
// Hint は利用者に提示する対処方法。
type Error struct {
	Kind    ErrorKind
	Status  int
	Message string
	Hint    string
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e.Message != "":
		return e.Message
	case e.Err != nil:
		return e.Err.Error()
	default:
		return string(e.Kind)
	}
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf は err の連鎖から *Error を取り出して Kind を返す。*Error が無ければ ErrAPI を返す。
func KindOf(err error) ErrorKind {
	var de *Error
	if errors.As(err, &de) {
		return de.Kind
	}
	return ErrAPI
}
