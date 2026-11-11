package confluence

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxRetryAfter = 60 * time.Second

// retryable は再送しても副作用が重複しない GET・PUT・DELETE の 429 と 503 だけを対象にする。
func retryable(method string, status int) bool {
	if status != http.StatusTooManyRequests && status != http.StatusServiceUnavailable {
		return false
	}
	return method == http.MethodGet || method == http.MethodPut || method == http.MethodDelete
}

// retryDelay は次の試行までの待機時間を返す。attempt は直前に失敗した試行の番号（1 始まり）。
// Retry-After があればそれを使い、なければ 1 秒・2 秒・4 秒に ±20% の jitter を加える。
func retryDelay(header string, attempt int, now time.Time) time.Duration {
	if d := retryAfter(header, now); d > 0 {
		return d
	}
	base := time.Second << (attempt - 1)
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(base) * jitter)
}

// retryAfter は Retry-After ヘッダー（秒数または HTTP 日付）を解釈し、上限 60 秒に切り詰める。
// 解釈できない場合と過去の日付の場合は 0 を返す。
func retryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(v); err == nil {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = t.Sub(now)
	}
	if d <= 0 {
		return 0
	}
	return min(d, maxRetryAfter)
}
