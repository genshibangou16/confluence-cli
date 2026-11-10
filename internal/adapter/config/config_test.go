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
		Home:           f.dir,
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
	if got, _ := FindLocal(src.Cwd, src.Home); filepath.Base(got) != LocalConfigName {
		t.Fatalf("got %q", got)
	}
	if got, _ := FindLocal(t.TempDir(), ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestFindLocal_StopsAtHome(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, LocalConfigName), "base_url: https://a.atlassian.net\n")
	home := filepath.Join(root, "home")
	cwd := filepath.Join(home, "proj")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := FindLocal(cwd, home); got != "" {
		t.Fatalf("home より上のファイルを採用した: %q", got)
	}
	write(t, filepath.Join(home, LocalConfigName), "base_url: https://a.atlassian.net\n")
	if got, _ := FindLocal(cwd, home); got != filepath.Join(home, LocalConfigName) {
		t.Fatalf("home 直下のファイルを採用していない: %q", got)
	}
}

func TestFindLocal_SkipsGroupWritable(t *testing.T) {
	_, src := newFixture(t, "", "base_url: https://a.atlassian.net\n")
	p := FindLocalPathForTest(t, src)
	if err := os.Chmod(p, 0o662); err != nil {
		t.Fatal(err)
	}
	got, warns := FindLocal(src.Cwd, src.Home)
	if got != "" || len(warns) != 1 || !strings.Contains(warns[0], p) {
		t.Fatalf("got=%q warns=%v", got, warns)
	}
	f, src2 := newFixture(t, "email: e\napi_token: t\nbase_url: https://u.example.com\n", "base_url: https://a.atlassian.net\n")
	_ = f
	if err := os.Chmod(filepath.Join(filepath.Dir(src2.Cwd), LocalConfigName), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, warns, err := Load(src2); err != nil || len(warns) == 0 {
		t.Fatalf("%v %v", warns, err)
	}
}

func FindLocalPathForTest(t *testing.T, src Sources) string {
	t.Helper()
	p, _ := FindLocal(src.Cwd, src.Home)
	if p == "" {
		t.Fatal("ローカル設定が見つからない")
	}
	return p
}

func TestLoad_WarnsWhenLocalBaseURLAdopted(t *testing.T) {
	_, src := newFixture(t, "email: e\napi_token: t\n", "base_url: https://x.atlassian.net/wiki\n")
	_, warns, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, LocalConfigName) && strings.Contains(w, "x.atlassian.net") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warns = %v", warns)
	}
}

func TestSaveUser_EmptyWritableSpacesRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	empty := []string{}
	if err := SaveUser(p, FileConfig{BaseURL: "https://a.example.com", Email: "e", APIToken: "t", WritableSpaces: &empty}); err != nil {
		t.Fatal(err)
	}
	_, src := newFixture(t, "", "")
	src.UserConfigPath = p
	s, _, err := Load(src)
	if err != nil || s.WritableSpaces == nil || len(s.WritableSpaces) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestLoad_UserEmptyWritableSpacesYAML(t *testing.T) {
	_, src := newFixture(t, "base_url: https://a.example.com\nemail: e\napi_token: t\nwritable_spaces: []\n", "")
	s, _, err := Load(src)
	if err != nil || s.WritableSpaces == nil || len(s.WritableSpaces) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestLoad_LocalBaseURLHostCheck(t *testing.T) {
	user := "email: e\napi_token: t\nbase_url: https://user.example.com\n"
	for _, bad := range []string{
		"https://x.atlassian.net@evil.com", "https://evil.com/.atlassian.net", "https://atlassian.net.evil.com",
		"https://evil.com#.atlassian.net", "https://atlassian.net", "https://u:p@x.atlassian.net/wiki",
	} {
		_, src := newFixture(t, user, "base_url: "+bad+"\n")
		s, warns, err := Load(src)
		if err != nil || s.BaseURL != "https://user.example.com" || len(warns) == 0 {
			t.Fatalf("%s: %+v %v %v", bad, s, warns, err)
		}
	}
	for good, want := range map[string]string{
		"HTTPS://X.ATLASSIAN.NET/wiki":      "HTTPS://X.ATLASSIAN.NET/wiki",
		"https://x.atlassian.net:8443/wiki": "https://x.atlassian.net:8443/wiki",
	} {
		_, src := newFixture(t, "email: e\napi_token: t\n", "base_url: "+good+"\n")
		s, _, err := Load(src)
		if err != nil || s.BaseURL != want {
			t.Fatalf("%s: %+v %v", good, s, err)
		}
	}
}

func TestLoad_RejectsUserinfoInBaseURL(t *testing.T) {
	_, src := newFixture(t, "base_url: https://u:p@x.atlassian.net/wiki\nemail: e\napi_token: t\n", "")
	if _, _, err := Load(src); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("user: %v", err)
	}
	f, src := newFixture(t, "email: e\napi_token: t\n", "")
	f.env["CONFLUENCE_BASE_URL"] = "https://u:p@x.atlassian.net/wiki"
	if _, _, err := Load(src); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("env: %v", err)
	}
}

func TestLoad_EnvReadOnlyValues(t *testing.T) {
	cases := map[string]struct {
		ro   bool
		warn bool
	}{"TRUE": {true, false}, "Yes": {true, false}, "on": {true, false}, "off": {false, false}, "No": {false, false}, "false": {false, false}, "banana": {true, true}}
	for v, c := range cases {
		f, src := newFixture(t, "base_url: https://a.example.com\nemail: e\napi_token: t\n", "")
		f.env["CONFLUENCE_READ_ONLY"] = v
		s, warns, err := Load(src)
		if err != nil || s.ReadOnly != c.ro || (len(warns) > 0) != c.warn {
			t.Fatalf("%q: %+v %v %v", v, s, warns, err)
		}
	}
}

func TestLoad_UnreadableUserConfigIsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root では読み取り権限を外せない")
	}
	_, src := newFixture(t, "base_url: https://a.example.com\nemail: e\napi_token: t\nread_only: true\n", "")
	if err := os.Chmod(src.UserConfigPath, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(src); domain.KindOf(err) != domain.ErrValidation {
		t.Fatalf("err = %v", err)
	}
}
