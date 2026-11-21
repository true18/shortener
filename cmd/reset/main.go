// Command reset generates Reset methods for annotated structs.
package main

import (
	"fmt"
	"os"

	"github.com/true18/shortener/cmd/reset/resetgen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitFailure()
	}
}

func run() error {
	return resetgen.Generate(".")
}

func exitFailure() {
	os.Exit(1)
}
