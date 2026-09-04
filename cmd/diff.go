package cmd

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/donaldgifford/keycloak-cli/internal/config"
	"github.com/donaldgifford/keycloak-cli/internal/keycloak"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Color definitions for diff output
var (
	colorAdd     = color.New(color.FgGreen)
	colorRemove  = color.New(color.FgRed)
	colorChange  = color.New(color.FgYellow)
	colorHeader  = color.New(color.FgCyan, color.Bold)
	colorNoChange = color.New(color.FgHiBlack)
)

var diffFiles []string

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences between local config and Keycloak",
	Long: `Diff compares local YAML definitions with the current state in Keycloak.

Supports both clients and client scopes.

Examples:
  # Show diff for a single client
  keycloak-cli diff -f clients/argocd.yaml

  # Show diff for a client scope
  keycloak-cli diff -f scopes/argocd-dedicated.yaml

  # Show diff for all resources
  keycloak-cli diff -f config/`,
	RunE: runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
	diffCmd.Flags().StringArrayVarP(&diffFiles, "file", "f", nil, "YAML file or directory (can be specified multiple times)")
	diffCmd.MarkFlagRequired("file")
}

func runDiff(cmd *cobra.Command, args []string) error {
	url, realm, user, password, err := getConfig()
	if err != nil {
		return err
	}

	// Collect all YAML files
	var files []string
	for _, f := range diffFiles {
		expanded, err := expandPath(f)
		if err != nil {
			return fmt.Errorf("failed to expand %s: %w", f, err)
		}
		files = append(files, expanded...)
	}

	if len(files) == 0 {
		return fmt.Errorf("no YAML files found")
	}

	// Connect to Keycloak
	ctx := context.Background()
	kc, err := keycloak.New(ctx, url, realm, user, password)
	if err != nil {
		return fmt.Errorf("failed to connect to Keycloak: %w", err)
	}

	// Process each file
	hasChanges := false
	for _, file := range files {
		changed, err := diffFile(kc, file)
		if err != nil {
			return fmt.Errorf("failed to diff %s: %w", file, err)
		}
		if changed {
			hasChanges = true
		}
	}

	if !hasChanges {
		fmt.Println("\nNo changes detected")
	}

	return nil
}

func diffFile(kc *keycloak.Client, file string) (bool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	var rf config.ResourceFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return false, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if rf.Realm == "" {
		return false, fmt.Errorf("realm is required")
	}

	if rf.Client != nil {
		return diffClient(kc, file, rf.Realm, rf.Client)
	}

	if rf.ClientScope != nil {
		return diffClientScope(kc, file, rf.Realm, rf.ClientScope)
	}

	return false, fmt.Errorf("file must contain either 'client' or 'clientScope'")
}

func diffClient(kc *keycloak.Client, file, realm string, cfg *config.ClientConfig) (bool, error) {
	if cfg.ClientID == "" {
		return false, fmt.Errorf("client.clientId is required")
	}

	// Set default enabled=true if not specified
	if !cfg.Enabled {
		cfg.Enabled = true
	}

	fmt.Println()
	colorHeader.Printf("Client: %s ", cfg.ClientID)
	fmt.Printf("(realm: %s)\n", realm)

	// Get current state
	existing, err := kc.GetClient(realm, cfg.ClientID)
	if err != nil {
		return false, fmt.Errorf("failed to get client: %w", err)
	}

	if existing == nil {
		colorAdd.Println("  + Client does not exist (will be created)")
		return true, nil
	}

	// Get current scopes for comparison
	currentDefaults, currentOptionals, err := kc.GetClientScopes(realm, cfg.ClientID)
	if err != nil {
		return false, fmt.Errorf("failed to get client scopes: %w", err)
	}

	// Compare
	current := keycloak.GocloakToConfig(existing)
	current.DefaultClientScopes = currentDefaults
	current.OptionalClientScopes = currentOptionals
	return compareClientConfigs(*cfg, current), nil
}

func diffClientScope(kc *keycloak.Client, file, realm string, cfg *config.ClientScopeConfig) (bool, error) {
	if cfg.Name == "" {
		return false, fmt.Errorf("clientScope.name is required")
	}

	fmt.Println()
	colorHeader.Printf("ClientScope: %s ", cfg.Name)
	fmt.Printf("(realm: %s)\n", realm)

	// Get current state
	existing, err := kc.GetClientScope(realm, cfg.Name)
	if err != nil {
		return false, fmt.Errorf("failed to get scope: %w", err)
	}

	if existing == nil {
		colorAdd.Println("  + Client scope does not exist (will be created)")
		if len(cfg.ProtocolMappers) > 0 {
			colorAdd.Printf("  + Will add %d protocol mapper(s)\n", len(cfg.ProtocolMappers))
		}
		return true, nil
	}

	// Get mappers
	mappers, err := kc.GetClientScopeMappers(realm, *existing.ID)
	if err != nil {
		return false, fmt.Errorf("failed to get mappers: %w", err)
	}

	current := keycloak.GocloakScopeToConfig(existing, mappers)
	return compareScopeConfigs(*cfg, current), nil
}

func compareClientConfigs(desired, current config.ClientConfig) bool {
	hasChanges := false

	// Compare simple fields
	fields := []struct {
		name    string
		desired interface{}
		current interface{}
	}{
		{"clientId", desired.ClientID, current.ClientID},
		{"name", desired.Name, current.Name},
		{"publicClient", desired.PublicClient, current.PublicClient},
		{"bearerOnly", desired.BearerOnly, current.BearerOnly},
		{"serviceAccountsEnabled", desired.ServiceAccountsEnabled, current.ServiceAccountsEnabled},
		{"standardFlowEnabled", desired.StandardFlowEnabled, current.StandardFlowEnabled},
		{"implicitFlowEnabled", desired.ImplicitFlowEnabled, current.ImplicitFlowEnabled},
		{"directAccessGrantsEnabled", desired.DirectAccessGrantsEnabled, current.DirectAccessGrantsEnabled},
		{"consentRequired", desired.ConsentRequired, current.ConsentRequired},
		{"enabled", desired.Enabled, current.Enabled},
		{"protocol", desired.Protocol, current.Protocol},
		{"rootUrl", desired.RootURL, current.RootURL},
		{"baseUrl", desired.BaseURL, current.BaseURL},
		{"description", desired.Description, current.Description},
		{"pkceCodeChallengeMethod", desired.PKCECodeChallengeMethod, current.PKCECodeChallengeMethod},
	}

	for _, f := range fields {
		if !reflect.DeepEqual(f.desired, f.current) {
			// Skip empty string comparisons for optional fields
			dStr, dOk := f.desired.(string)
			cStr, cOk := f.current.(string)
			if dOk && cOk && dStr == "" && cStr == "" {
				continue
			}
			colorChange.Printf("  ~ %s: ", f.name)
			colorRemove.Printf("%v", f.current)
			fmt.Print(" → ")
			colorAdd.Printf("%v\n", f.desired)
			hasChanges = true
		}
	}

	// Compare slices
	if !stringSliceEqual(desired.RedirectURIs, current.RedirectURIs) {
		colorChange.Println("  ~ redirectUris:")
		added, removed := diffStringSlices(desired.RedirectURIs, current.RedirectURIs)
		for _, s := range added {
			colorAdd.Printf("      + %s\n", s)
		}
		for _, s := range removed {
			colorRemove.Printf("      - %s\n", s)
		}
		hasChanges = true
	}

	if !stringSliceEqual(desired.WebOrigins, current.WebOrigins) {
		colorChange.Println("  ~ webOrigins:")
		added, removed := diffStringSlices(desired.WebOrigins, current.WebOrigins)
		for _, s := range added {
			colorAdd.Printf("      + %s\n", s)
		}
		for _, s := range removed {
			colorRemove.Printf("      - %s\n", s)
		}
		hasChanges = true
	}

	// Compare scopes (only if specified in desired config)
	if len(desired.DefaultClientScopes) > 0 {
		added, removed := diffStringSlices(desired.DefaultClientScopes, current.DefaultClientScopes)
		if len(added) > 0 || len(removed) > 0 {
			colorChange.Println("  ~ defaultClientScopes:")
			for _, s := range added {
				colorAdd.Printf("      + %s\n", s)
			}
			for _, s := range removed {
				colorRemove.Printf("      - %s\n", s)
			}
			hasChanges = true
		}
	}

	if len(desired.OptionalClientScopes) > 0 {
		added, removed := diffStringSlices(desired.OptionalClientScopes, current.OptionalClientScopes)
		if len(added) > 0 || len(removed) > 0 {
			colorChange.Println("  ~ optionalClientScopes:")
			for _, s := range added {
				colorAdd.Printf("      + %s\n", s)
			}
			for _, s := range removed {
				colorRemove.Printf("      - %s\n", s)
			}
			hasChanges = true
		}
	}

	if !hasChanges {
		colorNoChange.Println("  (no changes)")
	}

	return hasChanges
}

// diffStringSlices returns what's in desired but not current (added) and what's in current but not desired (removed)
func diffStringSlices(desired, current []string) (added, removed []string) {
	desiredSet := make(map[string]bool)
	for _, s := range desired {
		desiredSet[s] = true
	}
	currentSet := make(map[string]bool)
	for _, s := range current {
		currentSet[s] = true
	}

	for s := range desiredSet {
		if !currentSet[s] {
			added = append(added, s)
		}
	}
	for s := range currentSet {
		if !desiredSet[s] {
			removed = append(removed, s)
		}
	}
	return added, removed
}

func compareScopeConfigs(desired, current config.ClientScopeConfig) bool {
	hasChanges := false

	// Compare simple fields
	if desired.Name != current.Name {
		colorChange.Printf("  ~ name: ")
		colorRemove.Printf("%s", current.Name)
		fmt.Print(" → ")
		colorAdd.Printf("%s\n", desired.Name)
		hasChanges = true
	}
	if desired.Description != current.Description {
		colorChange.Printf("  ~ description: ")
		colorRemove.Printf("%q", current.Description)
		fmt.Print(" → ")
		colorAdd.Printf("%q\n", desired.Description)
		hasChanges = true
	}
	if desired.Protocol != "" && desired.Protocol != current.Protocol {
		colorChange.Printf("  ~ protocol: ")
		colorRemove.Printf("%s", current.Protocol)
		fmt.Print(" → ")
		colorAdd.Printf("%s\n", desired.Protocol)
		hasChanges = true
	}

	// Compare mappers
	currentMappers := make(map[string]config.ProtocolMapperConfig)
	for _, m := range current.ProtocolMappers {
		currentMappers[m.Name] = m
	}

	desiredMappers := make(map[string]config.ProtocolMapperConfig)
	for _, m := range desired.ProtocolMappers {
		desiredMappers[m.Name] = m
	}

	// Check for new/modified mappers
	for name, dm := range desiredMappers {
		cm, exists := currentMappers[name]
		if !exists {
			colorAdd.Printf("  + mapper %q (will be added)\n", name)
			hasChanges = true
		} else if !reflect.DeepEqual(dm.Config, cm.Config) || dm.ProtocolMapper != cm.ProtocolMapper {
			colorChange.Printf("  ~ mapper %q (will be updated)\n", name)
			hasChanges = true
		}
	}

	// Check for removed mappers
	for name := range currentMappers {
		if _, exists := desiredMappers[name]; !exists {
			colorRemove.Printf("  - mapper %q (will be removed)\n", name)
			hasChanges = true
		}
	}

	if !hasChanges {
		colorNoChange.Println("  (no changes)")
	}

	return hasChanges
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	// Create maps for comparison (order-independent)
	aMap := make(map[string]bool)
	for _, v := range a {
		aMap[v] = true
	}
	for _, v := range b {
		if !aMap[v] {
			return false
		}
	}
	return true
}
