# keycloak-cli

A stopgap CLI tool for managing Keycloak clients and client scopes via YAML
definitions.

This tool provides a declarative way to manage Keycloak resources until the
Keycloak Operator supports Client CRs (expected in v26.6.0, ~March 2026).

See:

- <https://github.com/keycloak/keycloak/issues/43167> (REST API v2)
- <https://github.com/keycloak/keycloak/issues/43168> (Operator Client CR)

## Quickstart

```sh
mise install                  # toolchain
just                          # task menu
just build                    # binary at bin/keycloak-cli
just test                     # race + coverage
just run -- --help            # run via `go run`
```

## Release

```sh
just release v0.1.0           # tags + pushes; CI runs goreleaser
```

Multi-arch archives land on the Forgejo (or GitHub) release page. Version
metadata (`version`, `commit`, `date`) is embedded via `-ldflags` and surfaced
in the binary's startup output.

## Container

```sh
docker build -t keycloak-cli:dev \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) .
```

Image is distroless + nonroot; entrypoint is `keycloak-cli`.

## Layout

```
cmd/keycloak-cli/    main package
internal/               library code (private to this module)
Dockerfile              multi-stage distroless build
.goreleaser.yml         release config
mise.toml               pinned toolchain
justfile                task runner
```

## Conventions

See `CLAUDE.md` for the full operating notes (Go-specific + homelab universals).

# keycloak-cli

A stopgap CLI tool for managing Keycloak clients and client scopes via YAML
definitions.

This tool provides a declarative way to manage Keycloak resources until the
Keycloak Operator supports Client CRs (expected in v26.6.0, ~March 2026).

See:

- <https://github.com/keycloak/keycloak/issues/43167> (REST API v2)
- <https://github.com/keycloak/keycloak/issues/43168> (Operator Client CR)

## Installation

```bash
cd tools/keycloak-cli
go build -o keycloak-cli .
```

## Configuration

Set credentials via environment variables or flags:

```bash
export KEYCLOAK_URL=https://auth.example.com
export KEYCLOAK_REALM=master        # Admin realm (default: master)
export KEYCLOAK_USER=admin
```

Or use flags:

```bash
./keycloak-cli --url https://auth.example.com --user admin --password xxx apply -f ...
```

## Recommended Architecture: LLDAP Groups

The recommended approach for RBAC across multiple apps is to use **LLDAP
groups** with a single **realm-level `groups` scope**. This mirrors enterprise
patterns (Okta + AD, etc.) and is cleaner than per-app attribute mappers.

### How It Works

```
LLDAP                    Keycloak                  Token                    Apps
──────────────────────────────────────────────────────────────────────────────────
groups:                  groups scope              groups: [                ArgoCD RBAC:
├─ argocd-admins    ──►  (group-membership    ──►   "argocd-admins",   ──►  g, argocd-admins, role:admin
├─ argocd-readonly       mapper)                    "grafana-admins",
├─ grafana-admins                                   "lldap_admin"           Grafana:
├─ grafana-editors                                 ]                        contains(groups, 'grafana-admins')
└─ lldap_admin
```

**Benefits:**

- One scope, one mapper - all LLDAP groups flow through automatically
- No per-app LLDAP attributes needed
- No per-app Keycloak mappers needed
- Apps filter for the groups they care about
- Adding a new app = create LLDAP group + configure app RBAC (no Keycloak
  changes)

### Setup Steps

**1. Create LLDAP groups:**

- `argocd-admins`, `argocd-readonly`
- `grafana-admins`, `grafana-editors`
- Add users to appropriate groups

**2. Create realm-level `groups` scope in Keycloak:**

```yaml
# scopes/groups.yaml
realm: my-realm
clientScope:
  name: groups
  description: Maps LLDAP group membership to token claims
  protocol: openid-connect
  protocolMappers:
    - name: group-membership
      protocolMapper: oidc-group-membership-mapper
      config:
        claim.name: groups
        full.path: "false"
        id.token.claim: "true"
        access.token.claim: "true"
        userinfo.token.claim: "true"
```

```bash
./keycloak-cli apply -f scopes/groups.yaml
```

**3. Assign `groups` scope to clients** (via client YAML - see below)

**4. Configure app RBAC to read groups:**

ArgoCD (`argocd-rbac-cm.yaml`):

```yaml
policy.csv: |
  g, argocd-admins, role:admin
  g, argocd-readonly, role:readonly
scopes: "[groups]"
```

Grafana:

```yaml
role_attribute_path:
  contains(groups[*], 'grafana-admins') && 'Admin' || contains(groups[*],
  'grafana-editors') && 'Editor' || 'Viewer'
```

### Alternative: Per-App Attribute Mappers

If you need fine-grained control (e.g., give someone ArgoCD admin without any
Grafana access), you can use per-app LLDAP attributes with dedicated mappers.
See the `examples/` directory for this pattern. However, the groups approach is
recommended for most setups.

## Commands

### apply

Create or update Keycloak resources from YAML definitions.

```bash
# Apply a single client
./keycloak-cli apply -f clients/argocd.yaml

# Apply a client scope
./keycloak-cli apply -f scopes/argocd-dedicated.yaml

# Apply multiple files
./keycloak-cli apply -f scopes/argocd-dedicated.yaml -f clients/argocd.yaml

# Apply all YAML files in a directory
./keycloak-cli apply -f config/

# Dry run (show what would change, no credentials required)
./keycloak-cli apply -f config/ --dry-run
```

### diff

Show differences between local YAML and current Keycloak state.

```bash
./keycloak-cli diff -f clients/argocd.yaml
./keycloak-cli diff -f config/
```

### delete

Delete resources from Keycloak (with confirmation prompt).

```bash
./keycloak-cli delete -f clients/argocd.yaml
./keycloak-cli delete -f clients/argocd.yaml --force  # Skip confirmation
```

## YAML Schema

### Client

```yaml
realm: my-realm
client:
  clientId: argocd
  name: ArgoCD
  description: ArgoCD GitOps continuous delivery

  # Client type
  publicClient: true # true = no secret (use PKCE), false = confidential

  # OAuth flows
  standardFlowEnabled: true # Authorization Code flow
  directAccessGrantsEnabled: false
  implicitFlowEnabled: false
  serviceAccountsEnabled: false

  # PKCE (for public clients)
  pkceCodeChallengeMethod: S256

  # URLs
  rootUrl: https://argocd.example.com
  redirectUris:
    - https://argocd.example.com/auth/callback
    - http://localhost:8085/auth/callback # CLI
  webOrigins:
    - https://argocd.example.com

  # Scopes (must exist in Keycloak)
  # These are fully managed - scopes not in this list will be REMOVED from the client
  defaultClientScopes:
    - groups # Recommended - LLDAP group membership
    - profile
    - email
  optionalClientScopes:
    - offline_access
```

#### Client Scope Assignment

The `defaultClientScopes` and `optionalClientScopes` fields allow you to
declaratively manage which scopes are assigned to a client. This eliminates
manual UI steps after creating a client.

**Important behavior:**

- When these fields are specified, the CLI will **fully reconcile** the scopes
- Scopes in the list that aren't assigned to the client will be **added**
- Scopes assigned to the client that aren't in the list will be **removed**
- If you don't specify these fields, existing scope assignments are left
  untouched

**Example diff output:**

```
Client: argocd (realm: my-realm)
  ~ defaultClientScopes:
      + groups
      - some-old-scope
  (no other changes)
```

### Client Scope

Client scopes contain protocol mappers that add claims to tokens. Use these to
map LLDAP attributes to token claims.

```yaml
realm: my-realm
clientScope:
  name: argocd-dedicated
  description: ArgoCD role mapping from LLDAP argocd attribute
  protocol: openid-connect

  protocolMappers:
    - name: argocd
      protocolMapper: oidc-usermodel-attribute-mapper
      config:
        user.attribute: argocd # LLDAP attribute name
        claim.name: argocd # Token claim name
        jsonType.label: String
        id.token.claim: "true"
        access.token.claim: "true"
        userinfo.token.claim: "true"
        multivalued: "false"
```

## Workflow

The order matters - create scopes before clients that reference them:

```bash
# 1. Create the scope first
./keycloak-cli apply -f docs/examples/argocd-scope.yaml

# 2. Then create the client that references it
./keycloak-cli apply -f docs/examples/argocd.yaml
```

Or apply a directory (files are processed alphabetically, so name scopes to sort
first):

```bash
./keycloak-cli apply -f docs/examples/
```

## Examples

The `examples/` directory contains:

| File                  | Description                                               |
| --------------------- | --------------------------------------------------------- |
| `groups.yaml`         | **Recommended** - Groups scope for LLDAP group membership |
| `template.yaml`       | Client template with comments                             |
| `scope-template.yaml` | Scope template with comments                              |

## Protocol Mapper Types

Common `protocolMapper` values for OIDC:

| Type                              | Description                         |
| --------------------------------- | ----------------------------------- |
| `oidc-usermodel-attribute-mapper` | Map user attribute to claim         |
| `oidc-usermodel-property-mapper`  | Map user property (email, username) |
| `oidc-hardcoded-claim-mapper`     | Add static claim value              |
| `oidc-audience-mapper`            | Add audience to token               |
| `oidc-group-membership-mapper`    | Add group membership                |

## Project Structure

```
keycloak-cli/
├── go.mod / go.sum
├── cmd/keycloak-cli/
│   ├── main.go
├── cmd/
│   ├── root.go      # CLI setup, env var handling
│   ├── apply.go     # Create/update resources
│   ├── delete.go    # Delete resources
│   └── diff.go      # Show differences
├── internal/
│   ├── config/
│   │   └── types.go # YAML schema types
│   └── keycloak/
│       └── client.go # gocloak wrapper
└── docs/examples/
    └── *.yaml       # Example configurations
```

## Dependencies

- [gocloak](https://github.com/Nerzal/gocloak) - Go client for Keycloak Admin
  API
- [cobra](https://github.com/spf13/cobra) - CLI framework

## License

Apache-2.0
