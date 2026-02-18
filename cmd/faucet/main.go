package main

import (
	"fmt"
	"os"

	"github.com/faucetdb/faucet/cmd/faucet/cli"
)

// Set via -ldflags at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := cli.Execute(version, commit, date); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
