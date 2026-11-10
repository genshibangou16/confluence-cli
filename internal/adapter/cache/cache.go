// Package cache は cloudId をファイルに保存する adapter。
package cache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// File は Dir 配下に host ごとのファイルを作って値を保存する。
type File struct {
	Dir string
}

// DefaultDir は既定の保存先を返す。XDG_CACHE_HOME が設定されていればその下、
// なければ home 配下の .cache の下の confluence-cli になる。
func DefaultDir(env func(string) string, home string) string {
	if x := env("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "confluence-cli")
	}
	return filepath.Join(home, ".cache", "confluence-cli")
}

// path は host に対応するファイルのパスを返す。host がファイル名の一部になるため、
// パス区切りや ".." を含む host は Dir の外を指せるので対象外とする。
func (f File) path(host string) (string, bool) {
	host = strings.ToLower(host)
	if host == "" || strings.ContainsAny(host, `/\`+"\x00") || strings.Contains(host, "..") {
		return "", false
	}
	return filepath.Join(f.Dir, "cloud-id-"+host), true
}

// Load は host に保存された cloudId を返す。読めない場合と空の場合は false を返す。
func (f File) Load(host string) (string, bool) {
	p, ok := f.path(host)
	if !ok {
		return "", false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(b))
	return id, id != ""
}

// Store は host の cloudId を 0600 のファイルへ保存する。
func (f File) Store(host, cloudID string) error {
	p, ok := f.path(host)
	if !ok {
		return errors.New("キャッシュのキーに使えないホスト名です: " + host)
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.Dir, ".cloud-id-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(cloudID); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp は 0600 で作るが、umask や実装差に依存しないよう明示する。
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
