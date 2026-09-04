package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	keycloakURL   string
	keycloakRealm string
	keycloakUser  string
	keycloakPass  string
)

var rootCmd = &cobra.Command{
	Use:   "keycloak-cli",
	Short: "CLI tool for managing Keycloak clients",
	Long: `A stopgap CLI tool for managing Keycloak clients via YAML definitions.

This tool provides a declarative way to manage Keycloak clients until
the Keycloak Operator supports Client CRs (expected in v26.6.0).

Environment variables:
  KEYCLOAK_URL       Keycloak server URL (e.g., https://auth.example.com)
  KEYCLOAK_REALM     Admin realm for authentication (default: master)
  KEYCLOAK_USER      Admin username
  KEYCLOAK_PASSWORD  Admin password`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&keycloakURL, "url", "", "Keycloak server URL (env: KEYCLOAK_URL)")
	rootCmd.PersistentFlags().StringVar(&keycloakRealm, "realm", "master", "Admin realm (env: KEYCLOAK_REALM)")
	rootCmd.PersistentFlags().StringVar(&keycloakUser, "user", "", "Admin username (env: KEYCLOAK_USER)")
	rootCmd.PersistentFlags().StringVar(&keycloakPass, "password", "", "Admin password (env: KEYCLOAK_PASSWORD)")
}

func getConfig() (url, realm, user, password string, err error) {
	url = keycloakURL
	if url == "" {
		url = os.Getenv("KEYCLOAK_URL")
	}
	if url == "" {
		return "", "", "", "", fmt.Errorf("keycloak URL required (--url or KEYCLOAK_URL)")
	}

	realm = keycloakRealm
	if realm == "" {
		realm = os.Getenv("KEYCLOAK_REALM")
	}
	if realm == "" {
		realm = "master"
	}

	user = keycloakUser
	if user == "" {
		user = os.Getenv("KEYCLOAK_USER")
	}
	if user == "" {
		return "", "", "", "", fmt.Errorf("keycloak user required (--user or KEYCLOAK_USER)")
	}

	password = keycloakPass
	if password == "" {
		password = os.Getenv("KEYCLOAK_PASSWORD")
	}
	if password == "" {
		return "", "", "", "", fmt.Errorf("keycloak password required (--password or KEYCLOAK_PASSWORD)")
	}

	return url, realm, user, password, nil
}
