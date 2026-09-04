package keycloak

import (
	"context"
	"fmt"

	"github.com/Nerzal/gocloak/v13"
	"github.com/donaldgifford/keycloak-cli/internal/config"
)

// Client wraps gocloak with convenience methods
type Client struct {
	gocloak *gocloak.GoCloak
	token   *gocloak.JWT
	ctx     context.Context
}

// New creates a new Keycloak client
func New(ctx context.Context, url, realm, user, password string) (*Client, error) {
	gc := gocloak.NewClient(url)

	token, err := gc.LoginAdmin(ctx, user, password, realm)
	if err != nil {
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	return &Client{
		gocloak: gc,
		token:   token,
		ctx:     ctx,
	}, nil
}

// GetClient retrieves a client by clientID
func (c *Client) GetClient(realm, clientID string) (*gocloak.Client, error) {
	clients, err := c.gocloak.GetClients(c.ctx, c.token.AccessToken, realm, gocloak.GetClientsParams{
		ClientID: gocloak.StringP(clientID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}

	if len(clients) == 0 {
		return nil, nil // Not found
	}

	return clients[0], nil
}

// CreateClient creates a new client
func (c *Client) CreateClient(realm string, cfg config.ClientConfig) (string, error) {
	client := configToGocloak(cfg)

	id, err := c.gocloak.CreateClient(c.ctx, c.token.AccessToken, realm, client)
	if err != nil {
		return "", fmt.Errorf("failed to create client: %w", err)
	}

	// Handle scopes after creation
	if err := c.updateClientScopes(realm, id, cfg); err != nil {
		return id, fmt.Errorf("client created but failed to set scopes: %w", err)
	}

	return id, nil
}

// UpdateClient updates an existing client
func (c *Client) UpdateClient(realm string, existing *gocloak.Client, cfg config.ClientConfig) error {
	client := configToGocloak(cfg)
	client.ID = existing.ID // Preserve internal ID

	if err := c.gocloak.UpdateClient(c.ctx, c.token.AccessToken, realm, client); err != nil {
		return fmt.Errorf("failed to update client: %w", err)
	}

	// Handle scopes after update
	if err := c.updateClientScopes(realm, *existing.ID, cfg); err != nil {
		return fmt.Errorf("client updated but failed to set scopes: %w", err)
	}

	return nil
}

// DeleteClient deletes a client by its internal ID
func (c *Client) DeleteClient(realm, id string) error {
	if err := c.gocloak.DeleteClient(c.ctx, c.token.AccessToken, realm, id); err != nil {
		return fmt.Errorf("failed to delete client: %w", err)
	}
	return nil
}

// updateClientScopes sets the default and optional client scopes
func (c *Client) updateClientScopes(realm, clientID string, cfg config.ClientConfig) error {
	// Skip if no scopes specified in config
	if len(cfg.DefaultClientScopes) == 0 && len(cfg.OptionalClientScopes) == 0 {
		return nil
	}

	// Get all available client scopes
	scopes, err := c.gocloak.GetClientScopes(c.ctx, c.token.AccessToken, realm)
	if err != nil {
		return fmt.Errorf("failed to get client scopes: %w", err)
	}

	// Build name -> ID and ID -> name maps
	scopeNameToID := make(map[string]string)
	scopeIDToName := make(map[string]string)
	for _, s := range scopes {
		if s.Name != nil && s.ID != nil {
			scopeNameToID[*s.Name] = *s.ID
			scopeIDToName[*s.ID] = *s.Name
		}
	}

	// Build desired scope sets
	desiredDefaults := make(map[string]bool)
	for _, name := range cfg.DefaultClientScopes {
		desiredDefaults[name] = true
	}
	desiredOptionals := make(map[string]bool)
	for _, name := range cfg.OptionalClientScopes {
		desiredOptionals[name] = true
	}

	// --- Handle Default Scopes ---
	currentDefaults, err := c.gocloak.GetClientsDefaultScopes(c.ctx, c.token.AccessToken, realm, clientID)
	if err != nil {
		return fmt.Errorf("failed to get current default scopes: %w", err)
	}

	currentDefaultNames := make(map[string]bool)
	for _, s := range currentDefaults {
		if s.ID != nil {
			if name, ok := scopeIDToName[*s.ID]; ok {
				currentDefaultNames[name] = true
			}
		}
	}

	// Add missing default scopes
	for scopeName := range desiredDefaults {
		scopeID, ok := scopeNameToID[scopeName]
		if !ok {
			return fmt.Errorf("client scope %q not found", scopeName)
		}
		if !currentDefaultNames[scopeName] {
			if err := c.gocloak.AddDefaultScopeToClient(c.ctx, c.token.AccessToken, realm, clientID, scopeID); err != nil {
				return fmt.Errorf("failed to add default scope %s: %w", scopeName, err)
			}
		}
	}

	// Remove default scopes that shouldn't be there
	for scopeName := range currentDefaultNames {
		if !desiredDefaults[scopeName] {
			scopeID := scopeNameToID[scopeName]
			if err := c.gocloak.RemoveDefaultScopeFromClient(c.ctx, c.token.AccessToken, realm, clientID, scopeID); err != nil {
				return fmt.Errorf("failed to remove default scope %s: %w", scopeName, err)
			}
		}
	}

	// --- Handle Optional Scopes ---
	currentOptionals, err := c.gocloak.GetClientsOptionalScopes(c.ctx, c.token.AccessToken, realm, clientID)
	if err != nil {
		return fmt.Errorf("failed to get current optional scopes: %w", err)
	}

	currentOptionalNames := make(map[string]bool)
	for _, s := range currentOptionals {
		if s.ID != nil {
			if name, ok := scopeIDToName[*s.ID]; ok {
				currentOptionalNames[name] = true
			}
		}
	}

	// Add missing optional scopes
	for scopeName := range desiredOptionals {
		scopeID, ok := scopeNameToID[scopeName]
		if !ok {
			return fmt.Errorf("client scope %q not found", scopeName)
		}
		if !currentOptionalNames[scopeName] {
			if err := c.gocloak.AddOptionalScopeToClient(c.ctx, c.token.AccessToken, realm, clientID, scopeID); err != nil {
				return fmt.Errorf("failed to add optional scope %s: %w", scopeName, err)
			}
		}
	}

	// Remove optional scopes that shouldn't be there
	for scopeName := range currentOptionalNames {
		if !desiredOptionals[scopeName] {
			scopeID := scopeNameToID[scopeName]
			if err := c.gocloak.RemoveOptionalScopeFromClient(c.ctx, c.token.AccessToken, realm, clientID, scopeID); err != nil {
				return fmt.Errorf("failed to remove optional scope %s: %w", scopeName, err)
			}
		}
	}

	return nil
}

// GetClientScopes retrieves the current default and optional scopes for a client
func (c *Client) GetClientScopes(realm, clientID string) (defaultScopes, optionalScopes []string, err error) {
	// Get the client first to get its internal ID
	client, err := c.GetClient(realm, clientID)
	if err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, nil, fmt.Errorf("client %s not found", clientID)
	}

	// Get default scopes
	defaults, err := c.gocloak.GetClientsDefaultScopes(c.ctx, c.token.AccessToken, realm, *client.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get default scopes: %w", err)
	}
	for _, s := range defaults {
		if s.Name != nil {
			defaultScopes = append(defaultScopes, *s.Name)
		}
	}

	// Get optional scopes
	optionals, err := c.gocloak.GetClientsOptionalScopes(c.ctx, c.token.AccessToken, realm, *client.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get optional scopes: %w", err)
	}
	for _, s := range optionals {
		if s.Name != nil {
			optionalScopes = append(optionalScopes, *s.Name)
		}
	}

	return defaultScopes, optionalScopes, nil
}

// configToGocloak converts our config to gocloak.Client
func configToGocloak(cfg config.ClientConfig) gocloak.Client {
	client := gocloak.Client{
		ClientID:                  gocloak.StringP(cfg.ClientID),
		Name:                      gocloak.StringP(cfg.Name),
		Enabled:                   gocloak.BoolP(cfg.Enabled),
		PublicClient:              gocloak.BoolP(cfg.PublicClient),
		BearerOnly:                gocloak.BoolP(cfg.BearerOnly),
		ServiceAccountsEnabled:    gocloak.BoolP(cfg.ServiceAccountsEnabled),
		StandardFlowEnabled:       gocloak.BoolP(cfg.StandardFlowEnabled),
		ImplicitFlowEnabled:       gocloak.BoolP(cfg.ImplicitFlowEnabled),
		DirectAccessGrantsEnabled: gocloak.BoolP(cfg.DirectAccessGrantsEnabled),
		ConsentRequired:           gocloak.BoolP(cfg.ConsentRequired),
		Description:               gocloak.StringP(cfg.Description),
		RootURL:                   gocloak.StringP(cfg.RootURL),
		BaseURL:                   gocloak.StringP(cfg.BaseURL),
		RedirectURIs:              &cfg.RedirectURIs,
		WebOrigins:                &cfg.WebOrigins,
	}

	if cfg.Protocol != "" {
		client.Protocol = gocloak.StringP(cfg.Protocol)
	} else {
		client.Protocol = gocloak.StringP("openid-connect")
	}

	// Handle PKCE
	if cfg.PKCECodeChallengeMethod != "" {
		if client.Attributes == nil {
			attrs := make(map[string]string)
			client.Attributes = &attrs
		}
		(*client.Attributes)["pkce.code.challenge.method"] = cfg.PKCECodeChallengeMethod
	}

	// Merge additional attributes
	if len(cfg.Attributes) > 0 {
		if client.Attributes == nil {
			attrs := make(map[string]string)
			client.Attributes = &attrs
		}
		for k, v := range cfg.Attributes {
			(*client.Attributes)[k] = v
		}
	}

	return client
}

// GocloakToConfig converts gocloak.Client to our config (for diff)
func GocloakToConfig(gc *gocloak.Client) config.ClientConfig {
	cfg := config.ClientConfig{
		Enabled: true, // Default
	}

	if gc.ClientID != nil {
		cfg.ClientID = *gc.ClientID
	}
	if gc.Name != nil {
		cfg.Name = *gc.Name
	}
	if gc.Enabled != nil {
		cfg.Enabled = *gc.Enabled
	}
	if gc.PublicClient != nil {
		cfg.PublicClient = *gc.PublicClient
	}
	if gc.BearerOnly != nil {
		cfg.BearerOnly = *gc.BearerOnly
	}
	if gc.ServiceAccountsEnabled != nil {
		cfg.ServiceAccountsEnabled = *gc.ServiceAccountsEnabled
	}
	if gc.StandardFlowEnabled != nil {
		cfg.StandardFlowEnabled = *gc.StandardFlowEnabled
	}
	if gc.ImplicitFlowEnabled != nil {
		cfg.ImplicitFlowEnabled = *gc.ImplicitFlowEnabled
	}
	if gc.DirectAccessGrantsEnabled != nil {
		cfg.DirectAccessGrantsEnabled = *gc.DirectAccessGrantsEnabled
	}
	if gc.ConsentRequired != nil {
		cfg.ConsentRequired = *gc.ConsentRequired
	}
	if gc.Description != nil {
		cfg.Description = *gc.Description
	}
	if gc.Protocol != nil {
		cfg.Protocol = *gc.Protocol
	}
	if gc.RootURL != nil {
		cfg.RootURL = *gc.RootURL
	}
	if gc.BaseURL != nil {
		cfg.BaseURL = *gc.BaseURL
	}
	if gc.RedirectURIs != nil {
		cfg.RedirectURIs = *gc.RedirectURIs
	}
	if gc.WebOrigins != nil {
		cfg.WebOrigins = *gc.WebOrigins
	}
	if gc.Attributes != nil {
		if pkce, ok := (*gc.Attributes)["pkce.code.challenge.method"]; ok {
			cfg.PKCECodeChallengeMethod = pkce
		}
		cfg.Attributes = *gc.Attributes
		delete(cfg.Attributes, "pkce.code.challenge.method") // Don't duplicate
	}

	return cfg
}

// =============================================================================
// Client Scope Methods
// =============================================================================

// GetClientScope retrieves a client scope by name
func (c *Client) GetClientScope(realm, name string) (*gocloak.ClientScope, error) {
	scopes, err := c.gocloak.GetClientScopes(c.ctx, c.token.AccessToken, realm)
	if err != nil {
		return nil, fmt.Errorf("failed to get client scopes: %w", err)
	}

	for _, s := range scopes {
		if s.Name != nil && *s.Name == name {
			return s, nil
		}
	}

	return nil, nil // Not found
}

// CreateClientScope creates a new client scope with its protocol mappers
func (c *Client) CreateClientScope(realm string, cfg config.ClientScopeConfig) (string, error) {
	scope := scopeConfigToGocloak(cfg)

	id, err := c.gocloak.CreateClientScope(c.ctx, c.token.AccessToken, realm, scope)
	if err != nil {
		return "", fmt.Errorf("failed to create client scope: %w", err)
	}

	// Add protocol mappers
	for _, pm := range cfg.ProtocolMappers {
		mapper := mapperConfigToGocloak(pm, cfg.Protocol)
		_, err := c.gocloak.CreateClientScopeProtocolMapper(c.ctx, c.token.AccessToken, realm, id, mapper)
		if err != nil {
			return id, fmt.Errorf("scope created but failed to add mapper %s: %w", pm.Name, err)
		}
	}

	return id, nil
}

// UpdateClientScope updates an existing client scope and its protocol mappers
func (c *Client) UpdateClientScope(realm string, existing *gocloak.ClientScope, cfg config.ClientScopeConfig) error {
	scope := scopeConfigToGocloak(cfg)
	scope.ID = existing.ID

	if err := c.gocloak.UpdateClientScope(c.ctx, c.token.AccessToken, realm, scope); err != nil {
		return fmt.Errorf("failed to update client scope: %w", err)
	}

	// Get existing mappers
	existingMappers, err := c.gocloak.GetClientScopeProtocolMappers(c.ctx, c.token.AccessToken, realm, *existing.ID)
	if err != nil {
		return fmt.Errorf("failed to get existing mappers: %w", err)
	}

	// Build map of existing mappers by name
	existingByName := make(map[string]*gocloak.ProtocolMappers)
	for _, m := range existingMappers {
		if m.Name != nil {
			existingByName[*m.Name] = m
		}
	}

	// Update or create mappers
	desiredNames := make(map[string]bool)
	for _, pm := range cfg.ProtocolMappers {
		desiredNames[pm.Name] = true
		mapper := mapperConfigToGocloak(pm, cfg.Protocol)

		if em, exists := existingByName[pm.Name]; exists {
			// Update existing mapper
			mapper.ID = em.ID
			if err := c.gocloak.UpdateClientScopeProtocolMapper(c.ctx, c.token.AccessToken, realm, *existing.ID, mapper); err != nil {
				return fmt.Errorf("failed to update mapper %s: %w", pm.Name, err)
			}
		} else {
			// Create new mapper
			_, err := c.gocloak.CreateClientScopeProtocolMapper(c.ctx, c.token.AccessToken, realm, *existing.ID, mapper)
			if err != nil {
				return fmt.Errorf("failed to create mapper %s: %w", pm.Name, err)
			}
		}
	}

	// Delete mappers that are no longer in config
	for name, em := range existingByName {
		if !desiredNames[name] {
			if err := c.gocloak.DeleteClientScopeProtocolMapper(c.ctx, c.token.AccessToken, realm, *existing.ID, *em.ID); err != nil {
				return fmt.Errorf("failed to delete mapper %s: %w", name, err)
			}
		}
	}

	return nil
}

// DeleteClientScope deletes a client scope by its internal ID
func (c *Client) DeleteClientScope(realm, id string) error {
	if err := c.gocloak.DeleteClientScope(c.ctx, c.token.AccessToken, realm, id); err != nil {
		return fmt.Errorf("failed to delete client scope: %w", err)
	}
	return nil
}

// scopeConfigToGocloak converts our config to gocloak.ClientScope
func scopeConfigToGocloak(cfg config.ClientScopeConfig) gocloak.ClientScope {
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = "openid-connect"
	}

	return gocloak.ClientScope{
		Name:        gocloak.StringP(cfg.Name),
		Description: gocloak.StringP(cfg.Description),
		Protocol:    gocloak.StringP(protocol),
	}
}

// mapperConfigToGocloak converts our config to gocloak.ProtocolMappers
func mapperConfigToGocloak(cfg config.ProtocolMapperConfig, defaultProtocol string) gocloak.ProtocolMappers {
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = defaultProtocol
	}
	if protocol == "" {
		protocol = "openid-connect"
	}

	// Convert our config map to gocloak.ProtocolMappersConfig
	pmConfig := &gocloak.ProtocolMappersConfig{}
	if v, ok := cfg.Config["user.attribute"]; ok {
		pmConfig.UserAttribute = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["claim.name"]; ok {
		pmConfig.ClaimName = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["claim.value"]; ok {
		pmConfig.ClaimValue = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["jsonType.label"]; ok {
		pmConfig.JSONTypeLabel = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["id.token.claim"]; ok {
		pmConfig.IDTokenClaim = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["access.token.claim"]; ok {
		pmConfig.AccessTokenClaim = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["userinfo.token.claim"]; ok {
		pmConfig.UserinfoTokenClaim = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["multivalued"]; ok {
		pmConfig.Multivalued = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["included.client.audience"]; ok {
		pmConfig.IncludedClientAudience = gocloak.StringP(v)
	}
	if v, ok := cfg.Config["full.path"]; ok {
		pmConfig.FullPath = gocloak.StringP(v)
	}

	return gocloak.ProtocolMappers{
		Name:                  gocloak.StringP(cfg.Name),
		Protocol:              gocloak.StringP(protocol),
		ProtocolMapper:        gocloak.StringP(cfg.ProtocolMapper),
		ConsentRequired:       gocloak.BoolP(cfg.ConsentRequired),
		ProtocolMappersConfig: pmConfig,
	}
}

// GocloakScopeToConfig converts gocloak.ClientScope to our config (for diff)
func GocloakScopeToConfig(gc *gocloak.ClientScope, mappers []*gocloak.ProtocolMappers) config.ClientScopeConfig {
	cfg := config.ClientScopeConfig{}

	if gc.Name != nil {
		cfg.Name = *gc.Name
	}
	if gc.Description != nil {
		cfg.Description = *gc.Description
	}
	if gc.Protocol != nil {
		cfg.Protocol = *gc.Protocol
	}

	for _, m := range mappers {
		pm := config.ProtocolMapperConfig{
			Config: make(map[string]string),
		}
		if m.Name != nil {
			pm.Name = *m.Name
		}
		if m.Protocol != nil {
			pm.Protocol = *m.Protocol
		}
		if m.ProtocolMapper != nil {
			pm.ProtocolMapper = *m.ProtocolMapper
		}
		if m.ConsentRequired != nil {
			pm.ConsentRequired = *m.ConsentRequired
		}
		// Convert ProtocolMappersConfig back to map
		if m.ProtocolMappersConfig != nil {
			pmc := m.ProtocolMappersConfig
			if pmc.UserAttribute != nil {
				pm.Config["user.attribute"] = *pmc.UserAttribute
			}
			if pmc.ClaimName != nil {
				pm.Config["claim.name"] = *pmc.ClaimName
			}
			if pmc.ClaimValue != nil {
				pm.Config["claim.value"] = *pmc.ClaimValue
			}
			if pmc.JSONTypeLabel != nil {
				pm.Config["jsonType.label"] = *pmc.JSONTypeLabel
			}
			if pmc.IDTokenClaim != nil {
				pm.Config["id.token.claim"] = *pmc.IDTokenClaim
			}
			if pmc.AccessTokenClaim != nil {
				pm.Config["access.token.claim"] = *pmc.AccessTokenClaim
			}
			if pmc.UserinfoTokenClaim != nil {
				pm.Config["userinfo.token.claim"] = *pmc.UserinfoTokenClaim
			}
			if pmc.Multivalued != nil {
				pm.Config["multivalued"] = *pmc.Multivalued
			}
			if pmc.IncludedClientAudience != nil {
				pm.Config["included.client.audience"] = *pmc.IncludedClientAudience
			}
			if pmc.FullPath != nil {
				pm.Config["full.path"] = *pmc.FullPath
			}
		}
		cfg.ProtocolMappers = append(cfg.ProtocolMappers, pm)
	}

	return cfg
}

// GetClientScopeMappers retrieves all protocol mappers for a client scope
func (c *Client) GetClientScopeMappers(realm, scopeID string) ([]*gocloak.ProtocolMappers, error) {
	mappers, err := c.gocloak.GetClientScopeProtocolMappers(c.ctx, c.token.AccessToken, realm, scopeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol mappers: %w", err)
	}
	return mappers, nil
}
