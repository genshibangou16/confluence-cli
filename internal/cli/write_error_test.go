package cli

import (
	"errors"
	"strings"
	"testing"
)

var errWriteFailed = errors.New("write failed")

// failingWriter はすべての書き込みを失敗させる。
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

func TestOutputWriteErrorsReturned(t *testing.T) {
	cases := map[string]struct {
		env  func(t *testing.T) *env
		args []string
	}{
		"page get --outline": {headingEnv, []string{"page", "get", "1234567890", "--outline"}},
		"page delete":        {func(t *testing.T) *env { e, _ := newWriteEnv(t); return e }, []string{"page", "delete", "1"}},
		"page update":        {func(t *testing.T) *env { e, _ := newWriteEnv(t); return e }, []string{"page", "update", "1", "--body", "<p>b</p>"}},
		"label add":          {func(t *testing.T) *env { e, _ := newWriteEnv(t); return e }, []string{"label", "add", "1", "a", "b"}},
		"label remove":       {func(t *testing.T) *env { e, _ := newWriteEnv(t); return e }, []string{"label", "remove", "1", "a"}},
		"config show":        {func(t *testing.T) *env { return newEnv(t, nil) }, []string{"config", "show"}},
		"config path":        {func(t *testing.T) *env { return newEnv(t, nil) }, []string{"config", "path"}},
	}
	for name, tc := range cases {
		e := tc.env(t)
		e.deps.Stdout = failingWriter{}
		if code := e.run(tc.args...); code != 1 || !strings.Contains(e.stderr.String(), errWriteFailed.Error()) {
			t.Errorf("%s: code = %d, stderr = %q", name, code, e.stderr)
		}
	}
}
