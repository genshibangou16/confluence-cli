// Package config は環境変数・ユーザー設定・ローカル設定から接続設定を読み込む。
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

const LocalConfigName = ".confluence-cli.yaml"

// Settings は読み込み後の設定。WritableSpaces が nil のときは書き込み先のスペースを制限しない。
// 空のスライスのときは、どのスペースにも書き込めない。
type Settings struct {
	BaseURL        string
	Email          string
	APIToken       string
	ReadOnly       bool
	WritableSpaces []string
}

// FileConfig は設定ファイルの内容。
type FileConfig struct {
	BaseURL         string   `yaml:"base_url"`
	Email           string   `yaml:"email"`
	APIToken        string   `yaml:"api_token,omitempty"`
	APITokenCommand []string `yaml:"api_token_command,omitempty"`
	ReadOnly        bool     `yaml:"read_only,omitempty"`
	// WritableSpaces は nil（キー無し）と空（どのスペースにも書き込めない）を区別するためポインタにする。
	WritableSpaces *[]string `yaml:"writable_spaces,omitempty"`
}

// Sources は設定の読み込み元。テストで差し替えられるよう、環境・カレントディレクトリ・コマンド実行を注入する。
type Sources struct {
	Env            func(string) string
	Cwd            string
	Home           string
	UserConfigPath string
	RunCommand     func(name string, args ...string) ([]byte, error)
}

// DefaultSources は実環境の読み込み元を返す。
func DefaultSources() Sources {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return Sources{
		Env:            os.Getenv,
		Cwd:            cwd,
		Home:           home,
		UserConfigPath: userConfigPath(os.Getenv),
		RunCommand:     runExec,
	}
}

func userConfigPath(env func(string) string) string {
	if x := env("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "confluence-cli", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "confluence-cli", "config.yaml")
}

// FindLocal は cwd から親ディレクトリへ向かってローカル設定ファイルを探し、見つけたファイルのパスと内容を返す。
// 無ければパスと内容は空になる。
// 他のユーザーが置いたファイルや書き換えられるファイルを読まないよう、次の制限を課す。
//   - home より上は探さない（cwd と home はシンボリックリンクを解決してから比較する）。
//   - シンボリックリンク、所有者が実行ユーザーでないファイル、グループ・その他に書き込み権限があるファイルはスキップして警告を返す。
//
// 検査と読み込みの間にファイルを差し替えられないよう、ファイルは一度だけ開き、
// 検査も内容の読み込みも開いたファイルディスクリプタに対して行う。
func FindLocal(cwd, home string) (string, []byte, []string) {
	var warns []string
	dir := resolveDir(cwd)
	home = resolveDir(home)
	for dir != "" {
		p := filepath.Join(dir, LocalConfigName)
		data, warn, ok := readLocalFile(p)
		if ok {
			return p, data, warns
		}
		if warn != "" {
			warns = append(warns, warn)
		}
		parent := filepath.Dir(dir)
		if parent == dir || (home != "" && dir == home) {
			break
		}
		dir = parent
	}
	return "", nil, warns
}

// resolveDir はシンボリックリンクを解決したパスを返す。解決できないときは整形しただけのパスを返す。
func resolveDir(p string) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// readLocalFile は p が検査を通る通常ファイルのときだけ内容を返す。
// ファイルが無いときは警告も出さない。スキップするときは警告を返す。
func readLocalFile(p string) (data []byte, warn string, ok bool) {
	li, err := os.Lstat(p)
	if err != nil {
		return nil, "", false
	}
	if li.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Sprintf("ローカル設定 %s はシンボリックリンクのためスキップする", p), false
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Sprintf("ローカル設定 %s を開けないためスキップする", p), false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Sprintf("ローカル設定 %s の情報を取得できないためスキップする", p), false
	}
	// Lstat と Open の間にシンボリックリンクへ差し替えられた場合、開いたファイルは Lstat の結果と別物になる。
	if !fi.Mode().IsRegular() || !os.SameFile(li, fi) {
		return nil, fmt.Sprintf("ローカル設定 %s が検査中に変更されたためスキップする", p), false
	}
	switch {
	case !ownedByCurrentUser(fi):
		return nil, fmt.Sprintf("ローカル設定 %s の所有者が実行ユーザーではないためスキップする", p), false
	case fi.Mode().Perm()&0o022 != 0:
		return nil, fmt.Sprintf("ローカル設定 %s にグループまたはその他の書き込み権限があるためスキップする", p), false
	}
	data, err = io.ReadAll(f)
	if err != nil {
		return nil, fmt.Sprintf("ローカル設定 %s を読めないためスキップする", p), false
	}
	return data, "", true
}

// SaveUser はユーザー設定を書き込む。API トークンを含みうるため、ファイルは 0600、ディレクトリは 0700 にする。
func SaveUser(path string, s FileConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	// 既存ファイルへの WriteFile はパーミッションを変えないため明示する。
	return os.Chmod(path, 0o600)
}

func validation(format string, a ...any) error {
	return &domain.Error{Kind: domain.ErrValidation, Message: fmt.Sprintf(format, a...)}
}

// Load は環境変数 > ユーザー設定 > ローカル設定の優先順位で設定を読み込む。
// ローカル設定は base_url と、ユーザー設定より制限を緩めない書き込み制限だけを採用する。
// 2 番目の戻り値は標準エラーに出す警告。
func Load(src Sources) (Settings, []string, error) {
	var warns []string

	var user FileConfig
	// 読めないファイルを無視すると read_only と writable_spaces が失われるため、存在しない場合以外はエラーにする。
	data, err := os.ReadFile(src.UserConfigPath)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &user); err != nil {
			return Settings{}, nil, validation("ユーザー設定ファイル %s を解釈できない", src.UserConfigPath)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return Settings{}, nil, validation("ユーザー設定ファイル %s を読めない", src.UserConfigPath)
	}

	var local FileConfig
	localPath, localData, findWarns := FindLocal(src.Cwd, src.Home)
	warns = append(warns, findWarns...)
	if localPath != "" {
		if err := yaml.Unmarshal(localData, &local); err != nil {
			warns = append(warns, fmt.Sprintf("ローカル設定 %s を解釈できないため無視する", localPath))
			local = FileConfig{}
		}
	}
	if local.Email != "" || local.APIToken != "" {
		warns = append(warns, fmt.Sprintf("ローカル設定 %s の email と api_token は読まない。ユーザー設定か環境変数に書く", localPath))
	}
	if len(local.APITokenCommand) > 0 {
		warns = append(warns, fmt.Sprintf("ローカル設定 %s の api_token_command は読まない。ユーザー設定に書く", localPath))
	}
	localBase := local.BaseURL
	if localBase != "" && !isAtlassianNetHTTPS(localBase) {
		warns = append(warns, fmt.Sprintf("ローカル設定 %s の base_url は https の *.atlassian.net だけを採用するため無視する", localPath))
		localBase = ""
	}

	var s Settings
	if localBase != "" && src.Env("CONFLUENCE_BASE_URL") == "" && user.BaseURL == "" {
		host := localBase
		if u, err := url.Parse(localBase); err == nil {
			host = u.Host
		}
		warns = append(warns, fmt.Sprintf("ローカル設定 %s の base_url (%s) を使用する", localPath, host))
	}
	s.BaseURL = first(src.Env("CONFLUENCE_BASE_URL"), user.BaseURL, localBase)
	s.Email = first(src.Env("CONFLUENCE_EMAIL"), user.Email)
	s.APIToken = src.Env("CONFLUENCE_API_TOKEN")

	if s.BaseURL == "" {
		return Settings{}, warns, validation("base_url が未設定（設定ファイルか環境変数 CONFLUENCE_BASE_URL で指定する）")
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return Settings{}, warns, validation("base_url は認証情報を含まない https の URL だけを指定できる")
	}
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")

	if s.Email == "" {
		return Settings{}, warns, validation("email が未設定（設定ファイルか環境変数 CONFLUENCE_EMAIL で指定する）")
	}

	if s.APIToken == "" {
		switch {
		case len(user.APITokenCommand) > 0:
			tok, err := resolveToken(src.RunCommand, user.APITokenCommand)
			if err != nil {
				return Settings{}, warns, err
			}
			s.APIToken = tok
		default:
			s.APIToken = user.APIToken
		}
	}
	if s.APIToken == "" {
		return Settings{}, warns, validation("api_token が未設定（設定ファイル・api_token_command・環境変数 CONFLUENCE_API_TOKEN のいずれかで指定する）")
	}

	envRO, warn := parseBool(src.Env("CONFLUENCE_READ_ONLY"))
	if warn != "" {
		warns = append(warns, warn)
	}
	s.ReadOnly = user.ReadOnly || local.ReadOnly || envRO
	s.WritableSpaces = mergeSpaces(user.WritableSpaces, local.WritableSpaces)

	return s, warns, nil
}

func first(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func isAtlassianNetHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.HasSuffix(strings.ToLower(u.Hostname()), ".atlassian.net")
}

// parseBool は CONFLUENCE_READ_ONLY の値を解釈する。認識できない値は、
// 制限を外す方向に誤解釈しないよう true として扱い、警告を返す。
func parseBool(v string) (bool, string) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, ""
	case "", "0", "false", "no", "off":
		return false, ""
	}
	return true, "CONFLUENCE_READ_ONLY の値を解釈できないため true として扱う"
}

// mergeSpaces は片方だけ指定されていればその値、両方あれば共通部分を返す。
// ローカル設定がユーザー設定の許可範囲を広げないための規則。
func mergeSpaces(user, local *[]string) []string {
	switch {
	case user == nil && local == nil:
		return nil
	case user == nil:
		return *local
	case local == nil:
		return *user
	}
	inLocal := map[string]bool{}
	for _, k := range *local {
		inLocal[k] = true
	}
	out := []string{}
	for _, k := range *user {
		if inLocal[k] {
			out = append(out, k)
		}
	}
	return out
}
