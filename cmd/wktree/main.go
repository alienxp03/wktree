package main

import (
	"os"

	"github.com/alienxp03/wktree/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Options{}))
}
