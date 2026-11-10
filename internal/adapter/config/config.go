// Package config は環境変数・ユーザー設定・ローカル設定から接続設定を読み込む。
package config

import (
	"fmt"
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
	WritableSpaces  []string `yaml:"writable_spaces,omitempty"`
}

// Sources は設定の読み込み元。テストで差し替えられるよう、環境・カレントディレクトリ・コマンド実行を注入する。
type Sources struct {
	Env            func(string) string
	Cwd            string
	UserConfigPath string
	RunCommand     func(name string, args ...string) ([]byte, error)
}

// DefaultSources は実環境の読み込み元を返す。
func DefaultSources() Sources {
	cwd, _ := os.Getwd()
	return Sources{
		Env:            os.Getenv,
		Cwd:            cwd,
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

// FindLocal は cwd から親ディレクトリへ向かってローカル設定ファイルを探す。無ければ空文字列を返す。
func FindLocal(cwd string) string {
	dir := cwd
	for dir != "" {
		p := filepath.Join(dir, LocalConfigName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
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
	if data, err := os.ReadFile(src.UserConfigPath); err == nil {
		if err := yaml.Unmarshal(data, &user); err != nil {
			return Settings{}, nil, validation("ユーザー設定ファイル %s を解釈できない", src.UserConfigPath)
		}
	}

	var local FileConfig
	localPath := FindLocal(src.Cwd)
	if localPath != "" {
		if data, err := os.ReadFile(localPath); err == nil {
			if err := yaml.Unmarshal(data, &local); err != nil {
				warns = append(warns, fmt.Sprintf("ローカル設定 %s を解釈できないため無視する", localPath))
				local = FileConfig{}
			}
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
	s.BaseURL = first(src.Env("CONFLUENCE_BASE_URL"), user.BaseURL, localBase)
	s.Email = first(src.Env("CONFLUENCE_EMAIL"), user.Email)
	s.APIToken = src.Env("CONFLUENCE_API_TOKEN")

	if s.BaseURL == "" {
		return Settings{}, warns, validation("base_url が未設定（設定ファイルか環境変数 CONFLUENCE_BASE_URL で指定する）")
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Settings{}, warns, validation("base_url は https の URL だけを指定できる")
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

	envRO := src.Env("CONFLUENCE_READ_ONLY")
	s.ReadOnly = user.ReadOnly || local.ReadOnly || envRO == "1" || envRO == "true"
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
	return err == nil && u.Scheme == "https" && strings.HasSuffix(strings.ToLower(u.Hostname()), ".atlassian.net")
}

// mergeSpaces は片方だけ指定されていればその値、両方あれば共通部分を返す。
// ローカル設定がユーザー設定の許可範囲を広げないための規則。
func mergeSpaces(user, local []string) []string {
	switch {
	case user == nil:
		return local
	case local == nil:
		return user
	}
	inLocal := map[string]bool{}
	for _, k := range local {
		inLocal[k] = true
	}
	out := []string{}
	for _, k := range user {
		if inLocal[k] {
			out = append(out, k)
		}
	}
	return out
}
