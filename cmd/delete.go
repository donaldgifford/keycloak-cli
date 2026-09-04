package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/donaldgifford/keycloak-cli/internal/config"
	"github.com/donaldgifford/keycloak-cli/internal/keycloak"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	deleteFiles []string
	force       bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete resources from Keycloak",
	Long: `Delete removes Keycloak resources defined in YAML files.

Supports both clients and client scopes.

Examples:
  # Delete a client
  keycloak-cli delete -f clients/argocd.yaml

  # Delete a client scope
  keycloak-cli delete -f scopes/argocd-dedicated.yaml

  # Delete without confirmation (use with caution)
  keycloak-cli delete -f clients/argocd.yaml --force`,
	RunE: runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().StringArrayVarP(&deleteFiles, "file", "f", nil, "YAML file (can be specified multiple times)")
	deleteCmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	deleteCmd.MarkFlagRequired("file")
}

func runDelete(cmd *cobra.Command, args []string) error {
	url, realm, user, password, err := getConfig()
	if err != nil {
		return err
	}

	// Collect all YAML files
	var files []string
	for _, f := range deleteFiles {
		expanded, err := expandPath(f)
		if err != nil {
			return fmt.Errorf("failed to expand %s: %w", f, err)
		}
		files = append(files, expanded...)
	}

	if len(files) == 0 {
		return fmt.Errorf("no YAML files found")
	}

	// Parse all files first to show what will be deleted
	type deleteTarget struct {
		resourceType string // "client" or "clientScope"
		realm        string
		name         string
		file         string
	}
	var targets []deleteTarget

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		var rf config.ResourceFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return fmt.Errorf("failed to parse %s: %w", file, err)
		}

		if rf.Realm == "" {
			return fmt.Errorf("%s: realm is required", file)
		}

		if rf.Client != nil {
			if rf.Client.ClientID == "" {
				return fmt.Errorf("%s: client.clientId is required", file)
			}
			targets = append(targets, deleteTarget{
				resourceType: "client",
				realm:        rf.Realm,
				name:         rf.Client.ClientID,
				file:         file,
			})
		} else if rf.ClientScope != nil {
			if rf.ClientScope.Name == "" {
				return fmt.Errorf("%s: clientScope.name is required", file)
			}
			targets = append(targets, deleteTarget{
				resourceType: "clientScope",
				realm:        rf.Realm,
				name:         rf.ClientScope.Name,
				file:         file,
			})
		} else {
			return fmt.Errorf("%s: file must contain either 'client' or 'clientScope'", file)
		}
	}

	// Confirm deletion
	if !force {
		fmt.Println("The following resources will be deleted:")
		for _, t := range targets {
			fmt.Printf("  - %s %q (realm: %s) [%s]\n", t.resourceType, t.name, t.realm, t.file)
		}
		fmt.Print("\nContinue? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return nil
		}
	}

	// Connect to Keycloak
	ctx := context.Background()
	kc, err := keycloak.New(ctx, url, realm, user, password)
	if err != nil {
		return fmt.Errorf("failed to connect to Keycloak: %w", err)
	}
	fmt.Printf("Connected to %s\n", url)

	// Delete each resource
	for _, t := range targets {
		switch t.resourceType {
		case "client":
			existing, err := kc.GetClient(t.realm, t.name)
			if err != nil {
				return fmt.Errorf("failed to check client %s: %w", t.name, err)
			}
			if existing == nil {
				fmt.Printf("  Client %q not found in realm %q (skipping)\n", t.name, t.realm)
				continue
			}
			if err := kc.DeleteClient(t.realm, *existing.ID); err != nil {
				return fmt.Errorf("failed to delete client %s: %w", t.name, err)
			}
			fmt.Printf("  Deleted client %q from realm %q\n", t.name, t.realm)

		case "clientScope":
			existing, err := kc.GetClientScope(t.realm, t.name)
			if err != nil {
				return fmt.Errorf("failed to check scope %s: %w", t.name, err)
			}
			if existing == nil {
				fmt.Printf("  Client scope %q not found in realm %q (skipping)\n", t.name, t.realm)
				continue
			}
			if err := kc.DeleteClientScope(t.realm, *existing.ID); err != nil {
				return fmt.Errorf("failed to delete scope %s: %w", t.name, err)
			}
			fmt.Printf("  Deleted client scope %q from realm %q\n", t.name, t.realm)
		}
	}

	return nil
}
