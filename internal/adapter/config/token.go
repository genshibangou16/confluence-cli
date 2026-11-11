package config

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/genshibangou16/confluence-cli/internal/domain"
)

// resolveToken は api_token_command を実行し、標準出力の 1 行目を API トークンとして返す。
// エラーメッセージにはコマンド名だけを含め、コマンドの出力（トークンを含みうる）は含めない。
func resolveToken(run func(string, ...string) ([]byte, error), cmd []string) (string, error) {
	fail := func(reason string, err error) error {
		return &domain.Error{
			Kind:    domain.ErrValidation,
			Message: fmt.Sprintf("api_token_command (%s) %s", cmd[0], reason),
			Err:     err,
		}
	}
	out, err := run(cmd[0], cmd[1:]...)
	if err != nil {
		return "", fail("の実行に失敗しました", nil)
	}
	first, _, _ := strings.Cut(string(out), "\n")
	token := strings.TrimSpace(first)
	if token == "" {
		return "", fail("の出力の 1 行目が空です", nil)
	}
	return token, nil
}

// runExec はシェルを介さずにコマンドを実行する。
func runExec(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
