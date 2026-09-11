// Command rolle is the Rolle CLI. It is a thin client for the daemon.
package main

import (
	"fmt"
	"os"

	"github.com/nateships/rolle/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: rolle <status>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "status":
		fmt.Printf("rolle %s: daemon not running\n", version.Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}
