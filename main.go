package main

import (
	"fmt"
	"os"

	"github.com/shichao402/pkv/cmd"
)

func main() {
	if err := cmd.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
