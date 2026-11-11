//go:build unix

package config

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByCurrentUser はファイルの所有者が実行ユーザーかを返す。所有者を取得できないときは true を返す。
func ownedByCurrentUser(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return int(st.Uid) == os.Getuid()
}
