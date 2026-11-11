package main

import (
	"os"

	"github.com/genshibangou16/confluence-cli/internal/cli"
)

// version はビルド時に -ldflags "-X main.version=..." で埋め込む。
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}
