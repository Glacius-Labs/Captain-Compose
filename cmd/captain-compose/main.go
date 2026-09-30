package main

import (
	"fmt"
	"os"

	"github.com/glacius-labs/captain-compose/internal/operator"
)

var version = "dev"
var commit = "unknown"
var buildDate = "unknown"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(versionText(version, commit, buildDate))
		return
	}
	os.Exit(operator.Run(os.Args[1:], os.Stdout, os.Stderr))
}

func versionText(version, commit, buildDate string) string {
	return fmt.Sprintf("captain-compose %s (commit %s, built %s)", version, commit, buildDate)
}
