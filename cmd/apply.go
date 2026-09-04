package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/donaldgifford/keycloak-cli/internal/config"
	"github.com/donaldgifford/keycloak-cli/internal/keycloak"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	applyFiles []string
	dryRun     bool
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply configurations to Keycloak",
	Long: `Apply creates or updates Keycloak resources from YAML definitions.

Supports both clients and client scopes. The YAML file should contain either
a 'client' or 'clientScope' key (not both).

Examples:
  # Apply a single client
  keycloak-cli apply -f clients/argocd.yaml

  # Apply a client scope
  keycloak-cli apply -f scopes/argocd-dedicated.yaml

  # Apply multiple files
  keycloak-cli apply -f clients/argocd.yaml -f scopes/argocd-dedicated.yaml

  # Apply all YAML files in a directory
  keycloak-cli apply -f config/

  # Dry run (show what would change)
  keycloak-cli apply -f config/ --dry-run`,
	RunE: runApply,
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().StringArrayVarP(&applyFiles, "file", "f", nil, "YAML file or directory (can be specified multiple times)")
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would change without making changes")
	applyCmd.MarkFlagRequired("file")
}

func runApply(cmd *cobra.Command, args []string) error {
	// Collect all YAML files first
	var files []string
	for _, f := range applyFiles {
		expanded, err := expandPath(f)
		if err != nil {
			return fmt.Errorf("failed to expand %s: %w", f, err)
		}
		files = append(files, expanded...)
	}

	if len(files) == 0 {
		return fmt.Errorf("no YAML files found")
	}

	// Connect to Keycloak (skip for dry-run)
	var kc *keycloak.Client
	if dryRun {
		fmt.Println("DRY RUN - no changes will be made")
	} else {
		url, realm, user, password, err := getConfig()
		if err != nil {
			return err
		}
		ctx := context.Background()
		kc, err = keycloak.New(ctx, url, realm, user, password)
		if err != nil {
			return fmt.Errorf("failed to connect to Keycloak: %w", err)
		}
		fmt.Printf("Connected to %s\n", url)
	}

	// Process each file
	for _, file := range files {
		if err := applyFile(kc, file); err != nil {
			return fmt.Errorf("failed to apply %s: %w", file, err)
		}
	}

	return nil
}

func applyFile(kc *keycloak.Client, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var rf config.ResourceFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	if rf.Realm == "" {
		return fmt.Errorf("realm is required")
	}

	// Determine resource type and apply
	if rf.Client != nil && rf.ClientScope != nil {
		return fmt.Errorf("file cannot contain both 'client' and 'clientScope'")
	}

	if rf.Client != nil {
		return applyClient(kc, file, rf.Realm, rf.Client)
	}

	if rf.ClientScope != nil {
		return applyClientScope(kc, file, rf.Realm, rf.ClientScope)
	}

	return fmt.Errorf("file must contain either 'client' or 'clientScope'")
}

func applyClient(kc *keycloak.Client, file, realm string, cfg *config.ClientConfig) error {
	if cfg.ClientID == "" {
		return fmt.Errorf("client.clientId is required")
	}

	// Set default enabled=true if not specified
	if !cfg.Enabled {
		cfg.Enabled = true
	}

	fmt.Printf("\nClient: %s (realm: %s, file: %s)\n", cfg.ClientID, realm, file)

	if dryRun {
		fmt.Printf("  Would create/update client %q in realm %q\n", cfg.ClientID, realm)
		return nil
	}

	// Check if client exists
	existing, err := kc.GetClient(realm, cfg.ClientID)
	if err != nil {
		return fmt.Errorf("failed to check existing client: %w", err)
	}

	if existing == nil {
		// Create new client
		id, err := kc.CreateClient(realm, *cfg)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		fmt.Printf("  Created client %q (id: %s)\n", cfg.ClientID, id)
	} else {
		// Update existing client
		if err := kc.UpdateClient(realm, existing, *cfg); err != nil {
			return fmt.Errorf("failed to update client: %w", err)
		}
		fmt.Printf("  Updated client %q\n", cfg.ClientID)
	}

	return nil
}

func applyClientScope(kc *keycloak.Client, file, realm string, cfg *config.ClientScopeConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("clientScope.name is required")
	}

	fmt.Printf("\nClientScope: %s (realm: %s, file: %s)\n", cfg.Name, realm, file)

	if dryRun {
		fmt.Printf("  Would create/update client scope %q in realm %q\n", cfg.Name, realm)
		if len(cfg.ProtocolMappers) > 0 {
			fmt.Printf("  With %d protocol mapper(s):\n", len(cfg.ProtocolMappers))
			for _, pm := range cfg.ProtocolMappers {
				fmt.Printf("    - %s (%s)\n", pm.Name, pm.ProtocolMapper)
			}
		}
		return nil
	}

	// Check if scope exists
	existing, err := kc.GetClientScope(realm, cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to check existing scope: %w", err)
	}

	if existing == nil {
		// Create new scope
		id, err := kc.CreateClientScope(realm, *cfg)
		if err != nil {
			return fmt.Errorf("failed to create scope: %w", err)
		}
		fmt.Printf("  Created client scope %q (id: %s)\n", cfg.Name, id)
		if len(cfg.ProtocolMappers) > 0 {
			fmt.Printf("  Added %d protocol mapper(s)\n", len(cfg.ProtocolMappers))
		}
	} else {
		// Update existing scope
		if err := kc.UpdateClientScope(realm, existing, *cfg); err != nil {
			return fmt.Errorf("failed to update scope: %w", err)
		}
		fmt.Printf("  Updated client scope %q\n", cfg.Name)
	}

	return nil
}

func expandPath(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	// Directory - find all YAML files
	var files []string
	err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(p)
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, p)
		}
		return nil
	})

	return files, err
}
