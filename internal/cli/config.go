package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/genshibangou16/confluence-cli/internal/adapter/config"
)

// config のサブコマンドは認証情報なしで動かすため、buildApp を呼ばない。
func newConfigCmd(d Deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "設定ファイルを作成・表示する",
	}
	c.AddCommand(newConfigInitCmd(d), newConfigInitLocalCmd(d), newConfigShowCmd(d), newConfigPathCmd(d))
	return c
}

// prompter は対話入力を読む。質問は標準出力を汚さないよう標準エラーに出す。
type prompter struct {
	d Deps
	r *bufio.Reader
}

func newPrompter(d Deps) *prompter {
	return &prompter{d: d, r: bufio.NewReader(d.Stdin)}
}

// line は 1 行読む。入力が空なら def を返す。
func (p *prompter) line(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.d.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(p.d.Stderr, "%s: ", label)
	}
	s, err := p.r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if s = strings.TrimSpace(s); s == "" {
		return def, nil
	}
	return s, nil
}

// secret は端末から入力を表示せずに読む。標準入力が端末でなければ 1 行読む。
func (p *prompter) secret(label string) (string, error) {
	if f, ok := p.d.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprintf(p.d.Stderr, "%s: ", label)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(p.d.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return p.line(label, "")
}

func (p *prompter) confirm(label string) (bool, error) {
	s, err := p.line(label+" [y/N]", "")
	if err != nil {
		return false, err
	}
	s = strings.ToLower(s)
	return s == "y" || s == "yes", nil
}

func newConfigInitCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "ユーザー設定ファイルを作成・更新する",
		Long: `ユーザー設定ファイルに base_url・email・api_token を書き込む。
既存のファイルがあれば、api_token_command・read_only・writable_spaces は残す。
api_token_command を設定済みなら、API トークンは尋ねない。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			path := d.Sources.UserConfigPath
			fc, exists, err := config.ReadUser(path)
			if err != nil {
				return err
			}
			p := newPrompter(d)
			if exists {
				ok, err := p.confirm(fmt.Sprintf("%s を更新しますか？", path))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(d.Stderr, "中止しました")
					return nil
				}
			}

			defBase := fc.BaseURL
			if defBase == "" {
				defBase = localBaseURL(d)
			}
			if fc.BaseURL, err = p.line("Base URL（例: https://example.atlassian.net/wiki）", defBase); err != nil {
				return err
			}
			if err := config.ValidateBaseURL(fc.BaseURL); err != nil {
				return err
			}
			if fc.Email, err = p.line("Email", fc.Email); err != nil {
				return err
			}
			if len(fc.APITokenCommand) > 0 {
				fmt.Fprintf(d.Stderr, "API トークンは api_token_command (%s) から取得するため入力しません\n", fc.APITokenCommand[0])
			} else {
				label := "API Token"
				if fc.APIToken != "" {
					label += "（空なら現在の値を残す）"
				}
				tok, err := p.secret(label)
				if err != nil {
					return err
				}
				if tok != "" {
					fc.APIToken = tok
				}
			}
			if err := config.SaveUser(path, fc); err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "設定を保存しました: %s\n", path)
			return nil
		},
	}
}

// localBaseURL は見つかったローカル設定の base_url を、ローカル設定として採用できる場合だけ返す。
func localBaseURL(d Deps) string {
	path, data, _ := config.FindLocal(d.Sources.Cwd, d.Sources.Home)
	if path == "" {
		return ""
	}
	var fc config.FileConfig
	if yaml.Unmarshal(data, &fc) != nil || !config.IsAtlassianNetHTTPS(fc.BaseURL) {
		return ""
	}
	return fc.BaseURL
}

func newConfigInitLocalCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init-local",
		Short: "カレントディレクトリにローカル設定ファイルを作成する（base_url だけ）",
		Long: `カレントディレクトリに ` + config.LocalConfigName + ` を作成し、base_url だけを書き込む。
ローカル設定の base_url は https の *.atlassian.net だけが使われる。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			path := filepath.Join(d.Sources.Cwd, config.LocalConfigName)
			p := newPrompter(d)
			li, err := os.Lstat(path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				return err
			case !li.Mode().IsRegular():
				// シンボリックリンクの先（ユーザー設定など）を書き換えないため、通常ファイル以外は上書きしない。
				return validationf("%s が通常のファイルではないため書き込みません", path)
			default:
				ok, err := p.confirm(fmt.Sprintf("%s を上書きしますか？", path))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(d.Stderr, "中止しました")
					return nil
				}
			}
			base, err := p.line("Base URL（例: https://example.atlassian.net/wiki）", "")
			if err != nil {
				return err
			}
			if !config.IsAtlassianNetHTTPS(base) {
				return validationf("ローカル設定の base_url には https の *.atlassian.net の URL を指定してください")
			}
			// email などの空のキーを書かないよう、base_url だけの構造体にする。
			data, err := yaml.Marshal(struct {
				BaseURL string `yaml:"base_url"`
			}{base})
			if err != nil {
				return err
			}
			if err := writeLocalConfig(path, data); err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "ローカル設定を保存しました: %s\n", path)
			return nil
		},
	}
}

// writeLocalConfig は同じディレクトリの一時ファイルに書いてから path へ rename する。
// rename は path がシンボリックリンクでもリンク自体を置き換えるため、Lstat の検査の後にリンクへ差し替えられても
// リンク先は書き換わらない。
// 他のユーザーが書き換えられるローカル設定は読み込み時にスキップされるため、パーミッションは 0644 にする。
func writeLocalConfig(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".confluence-cli-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func maskToken(t string) string {
	r := []rune(t)
	if len(r) <= 4 {
		return "****"
	}
	return string(r[:4]) + "****"
}

func newConfigShowCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "設定ファイルの場所と、読み込んだ設定を表示する（API トークンは先頭 4 文字だけ）",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			w := d.Stdout
			userPath := d.Sources.UserConfigPath
			user, exists, userErr := config.ReadUser(userPath)
			switch {
			case userErr != nil:
				fmt.Fprintf(w, "ユーザー設定:    %s（%v）\n", userPath, userErr)
			case exists:
				fmt.Fprintf(w, "ユーザー設定:    %s\n", userPath)
			default:
				fmt.Fprintf(w, "ユーザー設定:    %s（無し）\n", userPath)
			}
			if local, _, _ := config.FindLocal(d.Sources.Cwd, d.Sources.Home); local != "" {
				fmt.Fprintf(w, "ローカル設定:    %s\n", local)
			} else {
				fmt.Fprintln(w, "ローカル設定:    （無し）")
			}
			fmt.Fprintln(w)

			// 設定が不完全でも場所の確認には使えるため、読み込みのエラーは表示して正常終了する。
			s, warns, err := config.Load(d.Sources)
			for _, wn := range warns {
				fmt.Fprintln(d.Stderr, "warning: "+wn)
			}
			if err != nil {
				fmt.Fprintf(w, "設定を読み込めません: %v\n", err)
				return nil
			}

			token := maskToken(s.APIToken)
			switch {
			case d.Sources.Env("CONFLUENCE_API_TOKEN") != "":
				token += "（環境変数 CONFLUENCE_API_TOKEN）"
			case userErr == nil && len(user.APITokenCommand) > 0:
				token += fmt.Sprintf("（api_token_command: %s）", user.APITokenCommand[0])
			}
			spaces := "制限なし"
			switch {
			case s.WritableSpaces == nil:
			case len(s.WritableSpaces) == 0:
				spaces = "なし（どのスペースにも書き込めない）"
			default:
				spaces = strings.Join(s.WritableSpaces, ", ")
			}
			fmt.Fprintf(w, "Base URL:        %s\n", s.BaseURL)
			fmt.Fprintf(w, "Email:           %s\n", s.Email)
			fmt.Fprintf(w, "API Token:       %s\n", token)
			fmt.Fprintf(w, "Read only:       %t\n", s.ReadOnly)
			fmt.Fprintf(w, "Writable spaces: %s\n", spaces)
			return nil
		},
	}
}

func newConfigPathCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "設定ファイルのパスを表示する",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Fprintf(d.Stdout, "ユーザー設定:    %s\n", d.Sources.UserConfigPath)
			fmt.Fprintf(d.Stdout, "ローカル設定名:  %s\n", config.LocalConfigName)
			if local, _, _ := config.FindLocal(d.Sources.Cwd, d.Sources.Home); local != "" {
				fmt.Fprintf(d.Stdout, "見つかったローカル設定: %s\n", local)
			}
			return nil
		},
	}
}
