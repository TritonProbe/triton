package main

import (
	"fmt"
	"os"

	"github.com/tritonprobe/triton/internal/cli"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	app := cli.NewApp(version, buildTime)
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
