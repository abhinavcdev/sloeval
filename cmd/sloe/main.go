// Command sloe compiles AISLO specs into Prometheus rules, alerts and dashboards.
package main

import (
	"os"

	"github.com/abhinavcdev/sloeval/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.OSEnv()))
}
