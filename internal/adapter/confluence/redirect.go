package confluence

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// maxRedirects は net/http の既定と同じリダイレクトの上限。
const maxRedirects = 10

// NewHTTPClient は、https で最初のリクエストと同じホストへのリダイレクトだけをたどる http.Client を返す。
// net/http はポートだけが違う同じホスト名へのリダイレクトや、http へのリダイレクトでも Authorization ヘッダーを送り直すため、
// リダイレクト先に API トークンが送られないよう、それ以外のリダイレクトはたどらずにエラーにする。
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: checkRedirect}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("リダイレクトが %d 回を超えたため中止しました", maxRedirects)
	}
	orig := via[0].URL
	if req.URL.Scheme != "https" || req.URL.Host != orig.Host {
		return errors.New("https で同じホストへのリダイレクトではないため、リダイレクト先 " + req.URL.Scheme + "://" + req.URL.Host + " には接続しません")
	}
	return nil
}
