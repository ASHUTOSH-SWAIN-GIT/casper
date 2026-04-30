package main

import (
	"fmt"
	"os"
)

// main is intentionally tiny — every subcommand lives in its own file
// (validate.go, hash.go, policy.go, compile.go, propose.go, run.go),
// each registered with rootCmd in its init(). main just wires the
// process exit to whatever cobra returns.
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
