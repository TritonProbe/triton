package main

import (
	"fmt"
	"io"
	"os"

	"github.com/tritonprobe/triton/internal/cli"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	app := cli.NewApp(version, buildTime)
	app.SetStdout(stdout)
	if err := app.Run(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
