package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zack-nova/harnessyard/cmd/hyard/cli"
)

func main() {
	if err := cli.Execute(context.Background()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		if exitCode, ok := cli.ErrorExitCode(err); ok {
			os.Exit(exitCode)
		}
		os.Exit(1)
	}
}
