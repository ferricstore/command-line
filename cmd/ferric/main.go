// Command ferric is the command-line interface for FerricStore.
package main

import (
	"fmt"
	"os"

	"github.com/ferricstore/command-line/internal/buildinfo"
	"github.com/ferricstore/command-line/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	info := buildinfo.ResolveRuntime(buildinfo.Info{
		Version: version,
		Commit:  commit,
		Date:    date,
	})

	if err := cli.Execute(info); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
