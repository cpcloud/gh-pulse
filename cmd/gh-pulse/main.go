package main

import (
	"context"
	"os"

	"github.com/cpcloud/gh-pulse/internal/command"
)

var version = "dev"

func main() {
	os.Exit(command.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr, version))
}
