// Command rolled is the Rolle daemon. It serves the CLI and desktop app over a local socket.
package main

import (
	"fmt"

	"github.com/nateships/rolle/internal/version"
)

func main() {
	fmt.Println("rolled", version.Version)
}
