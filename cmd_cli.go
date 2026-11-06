package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"faxina/internal/runner"
	"faxina/internal/scan"
)

// runCLI trata os modos sem UI. Devolve false quando o app deve abrir normalmente.
func runCLI(args []string) (handled bool, code int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "--scan-json":
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return true, 1
		}
		result := scan.Run(context.Background(), scan.Options{Home: home, Runner: runner.Exec{}})
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return true, 1
		}
		return true, 0
	}
	return false, 0
}
