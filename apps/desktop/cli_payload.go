package main

import "embed"

// cliPayload carries the rolle command on Windows and Linux. The release
// tasks put it in build/cli before the app compiles. A dev build has only the
// README there, so embeddedCLI returns nil and the app reports the command
// as unavailable.
//
//go:embed build/cli
var cliPayload embed.FS

func embeddedCLI() []byte {
	for _, name := range []string{"build/cli/rolle.exe", "build/cli/rolle"} {
		if b, err := cliPayload.ReadFile(name); err == nil && len(b) > 0 {
			return b
		}
	}
	return nil
}
