//go:build !unix

package config

import "io/fs"

// ownedByCurrentUser は所有者を取得できない環境では常に true を返す。
func ownedByCurrentUser(fi fs.FileInfo) bool { return true }
