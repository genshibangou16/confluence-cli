// Package confluence は Confluence Cloud の REST API を呼び出す adapter。
package confluence

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

const defaultMaxAttempts = 4

// Options は Client の生成パラメーター。Root はサイトの root で、/wiki を含まない。
// HTTP・Sleep・MaxAttempts は省略するとそれぞれ既定値（タイムアウト 30 秒の http.Client、
// ctx のキャンセルで中断する待機、4 回）になる。Sleep はテストでリトライの待機を差し替えるために公開している。
type Options struct {
	Root        string
	Email       string
	Token       string
	HTTP        *http.Client
	Sleep       func(time.Duration)
	MaxAttempts int
}

type Client struct {
	root        string
	email       string
	token       string
	http        *http.Client
	sleep       func(context.Context, time.Duration)
	maxAttempts int
}

func New(o Options) *Client {
	c := &Client{
		root:        strings.TrimRight(o.Root, "/"),
		email:       o.Email,
		token:       o.Token,
		http:        o.HTTP,
		maxAttempts: o.MaxAttempts,
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Sleep != nil {
		sleep := o.Sleep
		c.sleep = func(_ context.Context, d time.Duration) { sleep(d) }
	} else {
		c.sleep = sleepContext
	}
	if c.maxAttempts <= 0 {
		c.maxAttempts = defaultMaxAttempts
	}
	return c
}

// sleepContext は d だけ待つ。ctx がキャンセルされたら、待機の残りを待たずに戻る。
func sleepContext(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// endpoint は 1 つの API 操作。scopes はエラーの Hint に表示する、操作に必要な OAuth スコープ。
// path は常に /wiki/ から書く。
type endpoint struct {
	method string
	path   string
	scopes []string
}

// do は 1 回の API 呼び出しを行う。in が nil でなければ JSON として送信し、
// out が nil でなければ応答本文を JSON として読み込む。
func (c *Client) do(ctx context.Context, ep endpoint, q url.Values, in, out any) (http.Header, error) {
	var payload []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, &domain.Error{Kind: domain.ErrValidation, Message: "リクエスト本文を JSON に変換できません", Err: err}
		}
		payload = b
	}
	u := c.root + ep.path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, &domain.Error{Kind: domain.ErrAPI, Message: err.Error(), Err: err}
		}
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, ep.method, u, body)
		if err != nil {
			return nil, &domain.Error{Kind: domain.ErrAPI, Message: err.Error(), Err: err}
		}
		req.SetBasicAuth(c.email, c.token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, &domain.Error{Kind: domain.ErrAPI, Message: err.Error(), Err: err}
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, &domain.Error{Kind: domain.ErrAPI, Status: resp.StatusCode, Message: readErr.Error(), Err: readErr}
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil && len(data) > 0 {
				if err := json.Unmarshal(data, out); err != nil {
					return nil, &domain.Error{Kind: domain.ErrAPI, Status: resp.StatusCode, Message: "応答の JSON を解釈できません", Err: err}
				}
			}
			return resp.Header, nil
		}

		if attempt < c.maxAttempts && retryable(ep.method, resp.StatusCode) {
			c.sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), attempt, time.Now()))
			continue
		}
		return nil, apiError(ep, resp.StatusCode, data)
	}
}
