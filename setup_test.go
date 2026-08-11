package main

import (
	"io"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "kapt")
}

// using an expectation would hide the backtrace if it goes wrong
func noError(err error) {
	if err != nil {
		panic(err)
	}
}

// run main with the given arguments and return exit code and stdout
func runMain(argv ...string) (code int, stdout string) {
	code = -1
	exitFunction = func(got int) { code = got }
	defer func() { exitFunction = os.Exit }()

	oldArgs := os.Args
	os.Args = append([]string{"kapt"}, argv...)
	defer func() { os.Args = oldArgs }()

	stdout = captureStdout(main)
	return code, stdout
}

func captureStdout(fn func()) string {
	read, write, err := os.Pipe()
	noError(err)

	old := os.Stdout
	os.Stdout = write
	defer func() { os.Stdout = old }()

	fn()
	noError(write.Close())
	captured, err := io.ReadAll(read)
	noError(err)
	return string(captured)
}
