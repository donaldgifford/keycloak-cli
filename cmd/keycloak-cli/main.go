// Package main is the entry point for the keycloak-cli
package main

import (
	"log/slog"
	"os"

	"github.com/donaldgifford/keycloak-cli/cmd"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := cmd.Execute(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}
