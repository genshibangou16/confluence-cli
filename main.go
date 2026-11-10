package main

import (
	"os"

	"github.com/genshibangou16/confluence-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
