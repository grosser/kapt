package main

import (
	"fmt"
	"os"

	"github.com/grosser/kapt/pkg/kapt"
)

// test injection point to enable test coverage of exit behavior
var exitFunction = os.Exit

// delegates to kapt.Run, so we have an easy to test method
func main() {
	argv := os.Args[1:]

	if len(argv) == 1 && argv[0] == "version" {
		fmt.Println(kapt.Version)
		exitFunction(0)
	} else { // wrapping in else in case exitFunction was stubbed
		exitFunction(kapt.Run(argv, os.Stdout, os.Stderr))
	}
}
