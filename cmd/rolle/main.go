// Command rolle is the rolle CLI.
package main

import (
	"fmt"
	"os"

	"github.com/nateships/rolle/cmd/rolle/cli"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "rolle:", err)
		os.Exit(1)
	}
}
