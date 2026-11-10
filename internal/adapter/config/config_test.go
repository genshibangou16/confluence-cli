package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	dir, user string
	env       map[string]string
	run       func(string, ...string) ([]byte, error)
}

func newFixture(t *testing.T, userYAML, localYAML string) (*fixture, Sources) {
	t.Helper()
	f := &fixture{dir: t.TempDir(), env: map[string]string{}}
	f.user = filepath.Join(f.dir, "user", "config.yaml")
	if userYAML != "" {
		write(t, f.user, userYAML)
	}
	cwd := filepath.Join(f.dir, "proj", "sub")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	if localYAML != "" {
		write(t, filepath.Join(f.dir, "proj", LocalConfigName), localYAML)
	}
	f.run = func(string, ...string) ([]byte, error) { return nil, errors.New("unexpected") }
	return f, Sources{
		Env:            func(k string) string { return f.env[k] },
		Cwd:            cwd,
		UserConfigPath: f.user,
		RunCommand:     func(n string, a ...string) ([]byte, error) { return f.run(n, a...) },
	}
}

func TestLoad_LocalConfigOnlyBaseURL(t *testing.T) {
	_, src := newFixture(t, "email: u@example.com\napi_token: usertok\n", `
base_url: https://example.atlassian.net/wiki/
email: evil@example.com
api_token: leaked
api_token_command: ["echo", "x"]
`)
	s, warns, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if s.BaseURL != "https://example.atlassian.net/wiki" || s.Email != "u@example.com" || s.APIToken != "usertok" {
		t.Fatalf("settings = %+v", s)
	}
	if len(warns) < 1 {
		t.Fatal("警告が返っていない")
	}
	for _, w := range warns {
		if strings.Contains(w, "leaked") {
			t.Fatalf("警告にトークンが含まれる: %s", w)
		}
	}
}

func TestLoad_LocalBaseURLMustBeAtlassianNet(t *testing.T) {
	_, src := newFixture(t, "email: u@example.com\napi_token: t\nbase_url: https://user.atlassian.net/wiki\n", "base_url: https://evil.example.com/wiki\n")
	s, warns, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if s.BaseURL != "https://user.atlassian.net/wiki" || len(warns) == 0 {
		t.Fatalf("s=%+v warns=%v", s, warns)
	}

	_, src = newFixture(t, "email: u@example.com\napi_token: t\n", "base_url: https://evil.example.com/wiki\n")
	_, _, err = Load(src)
	if domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("err = %v", err)
	}
}

func TestLoad_RejectsHTTP(t *testing.T) {
	_, src := newFixture(t, "base_url: http://a.example.com\nemail: e\napi_token: t\n", "")
	if _, _, err := Load(src); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("user: %v", err)
	}
	f, src := newFixture(t, "email: e\napi_token: t\n", "")
	f.env["CONFLUENCE_BASE_URL"] = "http://a.example.com"
	if _, _, err := Load(src); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("env: %v", err)
	}
	_, src = newFixture(t, "email: e\napi_token: t\n", "base_url: http://a.atlassian.net\n")
	if _, _, err := Load(src); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("local: %v", err)
	}
}

func TestLoad_UserConfigAnyHTTPSHost(t *testing.T) {
	_, src := newFixture(t, "base_url: https://confluence.example.com/wiki\nemail: e\napi_token: t\n", "")
	s, _, err := Load(src)
	if err != nil || s.BaseURL != "https://confluence.example.com/wiki" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}

func TestLoad_Precedence(t *testing.T) {
	f, src := newFixture(t, "base_url: https://user.example.com\nemail: user@example.com\napi_token: usertok\n", "base_url: https://local.atlassian.net\n")
	s, _, err := Load(src)
	if err != nil || s.BaseURL != "https://user.example.com" {
		t.Fatalf("user>local: %+v %v", s, err)
	}
	f.env["CONFLUENCE_BASE_URL"] = "https://env.example.com/"
	f.env["CONFLUENCE_EMAIL"] = "env@example.com"
	f.env["CONFLUENCE_API_TOKEN"] = "envtok"
	s, _, err = Load(src)
	if err != nil || s.BaseURL != "https://env.example.com" || s.Email != "env@example.com" || s.APIToken != "envtok" {
		t.Fatalf("env>user: %+v %v", s, err)
	}
}

func TestLoad_MissingFields(t *testing.T) {
	for _, y := range []string{
		"email: e\napi_token: t\n",
		"base_url: https://a.example.com\napi_token: t\n",
		"base_url: https://a.example.com\nemail: e\n",
	} {
		_, src := newFixture(t, y, "")
		if _, _, err := Load(src); domain.KindOf(err) != domain.ErrValidation {
			t.Fatalf("%q: %v", y, err)
		}
	}
}

func TestLoad_TokenCommand(t *testing.T) {
	f, src := newFixture(t, "base_url: https://a.example.com\nemail: e\napi_token: plain\napi_token_command: [security, find, -w]\n", "")
	var gotName string
	var gotArgs []string
	f.run = func(n string, a ...string) ([]byte, error) {
		gotName, gotArgs = n, a
		return []byte("tok\nextra\n"), nil
	}
	s, _, err := Load(src)
	if err != nil || s.APIToken != "tok" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
	if gotName != "security" || !reflect.DeepEqual(gotArgs, []string{"find", "-w"}) {
		t.Fatalf("name=%s args=%v", gotName, gotArgs)
	}
}

func TestLoad_TokenCommandFails(t *testing.T) {
	y := "base_url: https://a.example.com\nemail: e\napi_token_command: [mytokencmd, x]\n"
	for name, run := range map[string]func(string, ...string) ([]byte, error){
		"error": func(string, ...string) ([]byte, error) { return []byte("SECRETOUT"), errors.New("boom") },
		"empty": func(string, ...string) ([]byte, error) { return []byte("\nSECRETOUT\n"), nil },
	} {
		f, src := newFixture(t, y, "")
		f.run = run
		_, _, err := Load(src)
		if domain.KindOf(err) != domain.ErrValidation || !strings.Contains(err.Error(), "mytokencmd") || strings.Contains(err.Error(), "SECRETOUT") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestLoad_PolicyCannotBeLoosenedByLocal(t *testing.T) {
	base := "base_url: https://a.example.com\nemail: e\napi_token: t\n"
	cases := []struct {
		name, user, local string
		ro                bool
		spaces            []string
	}{
		{"read_only", base + "read_only: false\n", "read_only: true\n", true, nil},
		{"user read_only wins", base + "read_only: true\n", "read_only: false\n", true, nil},
		{"intersection", base + "writable_spaces: [A, B]\n", "writable_spaces: [B, C]\n", false, []string{"B"}},
		{"user unset", base, "writable_spaces: [C]\n", false, []string{"C"}},
		{"local unset", base + "writable_spaces: [A]\n", "", false, []string{"A"}},
	}
	for _, c := range cases {
		_, src := newFixture(t, c.user, c.local)
		s, _, err := Load(src)
		if err != nil {
			t.Fatal(c.name, err)
		}
		if s.ReadOnly != c.ro || !reflect.DeepEqual(s.WritableSpaces, c.spaces) {
			t.Fatalf("%s: %+v", c.name, s)
		}
	}
}

func TestLoad_DisjointWritableSpacesIsEmptyNotUnrestricted(t *testing.T) {
	_, src := newFixture(t, "base_url: https://a.example.com\nemail: e\napi_token: t\nwritable_spaces: [A]\n", "writable_spaces: [B]\n")
	s, _, err := Load(src)
	if err != nil || s.WritableSpaces == nil || len(s.WritableSpaces) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestLoad_EnvReadOnly(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "true": true, "0": false, "": false} {
		f, src := newFixture(t, "base_url: https://a.example.com\nemail: e\napi_token: t\n", "")
		f.env["CONFLUENCE_READ_ONLY"] = v
		s, _, err := Load(src)
		if err != nil || s.ReadOnly != want {
			t.Fatalf("%q: %+v %v", v, s, err)
		}
	}
}

func TestSaveUser_Permissions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "config.yaml")
	if err := SaveUser(p, FileConfig{BaseURL: "https://a.example.com", Email: "e", APIToken: "t"}); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file %v", fi.Mode())
	}
	di, _ := os.Stat(filepath.Dir(p))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("dir %v", di.Mode())
	}
}

func TestFindLocal(t *testing.T) {
	_, src := newFixture(t, "", "base_url: https://a.atlassian.net\n")
	if got := FindLocal(src.Cwd); filepath.Base(got) != LocalConfigName {
		t.Fatalf("got %q", got)
	}
	if got := FindLocal(t.TempDir()); got != "" {
		t.Fatalf("got %q", got)
	}
}
