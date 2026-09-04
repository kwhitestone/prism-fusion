# Prism Fusion Plugin Specification V2

Status: V2.0 foundation implemented; lifecycle, provider contribution, and remote-application contracts remain staged extensions.

## 1. Goals

V2 turns plugin loading from an implicit `init()` side effect into a validated startup contract while preserving source compatibility with existing backend plugins.

The specification distinguishes four extension forms:

| Kind | Runtime boundary | V2.0 status |
| --- | --- | --- |
| Backend Addon | In-process Go plugin that owns routes, middleware, and models | Implemented |
| Strategy Provider | Replaceable implementation inside a capability domain | Kind constant reserved but rejected by V2.0; contribution registry is next phase |
| Frontend Addon | In-process Vue module in the same Vite bundle | Existing `PluginModule`; V2 dependency validation is next phase |
| Remote Application | Independently deployed iframe/application | `RemoteAppManifest` is next phase |

These forms must not share one registry merely because they are all called “plugins”. Each registry has different identity, trust, lifecycle, and deployment semantics.

## 2. Compatibility

The existing Go `Plugin` interface is unchanged. A V1 plugin remains valid and receives a synthesized compatibility manifest:

```text
apiVersion: prism-fusion/v1
version: 0.0.0-legacy
kind: backend-addon
id, description, routeScopes: derived from the Plugin methods
```

A V2 plugin additionally implements `ManifestProvider`:

```go
type ManifestProvider interface {
    Manifest() Manifest
}
```

Do not add `Manifest()` to `BasePlugin`. Doing so would make every embedded V1 plugin look like V2 without an explicit migration.

## 3. Backend manifest

```go
type Manifest struct {
    APIVersion  string
    ID          string
    Version     string
    Kind        Kind
    Description string
    Provides    []string
    Requires    []Dependency
    Optional    []Dependency
    Conflicts   []string
    RouteScopes []string
}
```

Rules:

- `apiVersion` must be `prism-fusion/v2` for a V2 plugin.
- V2 `id` must equal `Plugin.Name()` and match `[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?`. V1 IDs retain their legacy syntax compatibility.
- `version` is required and must use Semantic Versioning.
- V2.0 accepts only `backend-addon`; `strategy-provider` remains reserved until its independent registry exists.
- `requires` and `optional` reference plugin IDs, not capability names.
- `provides` contains capability metadata. V2.0 records it but does not select providers from it.
- dependency version constraints are reserved but not solved in V2.0. A non-empty constraint is rejected so the registry cannot imply compatibility it has not verified.
- `routeScopes` are absolute, clean, static literal URL prefixes without query strings, fragments, or route parameters. A route such as `/tenants/:tenant/items` must use the static parent scope `/tenants`.

Example:

```go
func (p *AuditPlugin) Manifest() plugin.Manifest {
    return plugin.Manifest{
        APIVersion: plugin.APIVersionV2,
        ID:         "audit",
        Version:    "2.0.0",
        Kind:       plugin.KindBackendAddon,
        Provides:   []string{"compliance.audit-log"},
        Requires: []plugin.Dependency{
            {ID: "auth"},
        },
        Optional: []plugin.Dependency{
            {ID: "rbac"},
        },
        RouteScopes: []string{"/api/v1/addons/audit"},
    }
}
```

## 4. Registry and startup

Registration remains compatible:

```go
func init() {
    plugin.Register(&AuditPlugin{})
}
```

`Register` now fails fast by panicking on an invalid plugin or duplicate ID. Code that needs an error can use `TryRegister`; isolated tests or hosts can use `NewRegistry`.

The registration window closes when the registry is frozen. `initialize.InitTables`, `initialize.Routers`, and the compatibility `Sorted` path freeze before consuming plugins. Registering after freeze returns `ErrRegistryFrozen`.

A plugin may implement `ActivationProvider.PluginEnabled() bool`. The plugin-specific method name avoids accidentally treating a legacy business method named `Enabled` as a framework activation contract. Hosts must load configuration before freezing and treat activation inputs as immutable afterward. Inactive plugins remain installed and visible through registry inspection, but are excluded from dependency resolution, migrations, middleware, and routes. An inactive required dependency is treated as missing. The registry captures the decision at freeze and does not reevaluate it; plugin methods must not independently switch behavior after that boundary.

Resolution follows these rules:

1. Reject missing required dependencies.
2. Ignore missing optional dependencies.
3. Reject conflicts among active plugins.
4. Reject dependency cycles.
5. Place required and installed optional dependencies before dependents.
6. Among currently available nodes, sort by `Priority()` and then manifest ID.

`All`, `Names`, and resolved manifests return independent snapshots. Callers cannot mutate registry state through returned maps or slices.

`PluginEnabled()` may inspect `Get`, `Count`, or `Names`. Nested or concurrent `Resolve`/`Freeze` calls are rejected with a registry-state error so activation callbacks cannot recurse or run concurrently.

## 5. Route scope

Scoped middleware matches path-segment boundaries. Scope `/api/v1/addons/auth` matches itself and `/api/v1/addons/auth/session`, but not `/api/v1/addons/authz` or `/api/v1/addons/auth-other`.

An empty or invalid scope fails closed. `/` is the only explicit global scope; cross-plugin middleware should normally use `GlobalMiddlewares` instead.

If a plugin has scoped middleware, startup fails when its scope list is empty or when any route it registers falls outside the declared scopes. Public endpoints should be separated into a plugin without that scoped middleware until an explicit per-route policy contract is introduced.

Startup also rejects a scoped-middleware plugin whose scope contains, is contained by, or equals another active plugin's route scope. This prevents one plugin's middleware from silently governing a sibling plugin. Nested scopes owned by the same plugin remain valid.

V1 uses `RoutePrefix()`. V2 may declare multiple `RouteScopes`; when omitted, `RoutePrefix()` remains the compatibility fallback.

## 6. Migration hooks

Plugins may implement the exported optional `BeforeMigrator` and `AfterMigrator` interfaces. Both hooks run even when a plugin returns no AutoMigrate models, allowing data-only migrations. Migration execution follows the same frozen dependency order as route setup.

General `Validate`, `Initialize`, `Start`, `Ready`, and `Stop` phases are intentionally not declared without orchestration. They will be added together with startup rollback, readiness aggregation, and reverse-order shutdown semantics.

## 7. Provider rule

Duplicate plugin IDs never mean provider replacement and are always rejected. `strategy-provider` manifests are rejected in V2.0. A future provider contribution registry will use a compound identity such as `domain + providerName`, explicit selection, and capability-specific validation. Until that exists, configuration such as `auth.provider` is application behavior, not a framework-level replacement guarantee.

The built-in RBAC addon uses the narrower, domain-specific `RegisterAuthorizationResolver` bridge so Auth does not import RBAC or scan menu data on every authenticated request. It still requires the concrete built-in `auth` plugin because its current control plane depends on the built-in user schema, JWT actor, and refresh-session revocation. Consequently, a non-builtin Auth provider cannot be combined with builtin RBAC in V2.0; the registry fails that combination during startup. This bridge is not the generic strategy-provider registry and does not change the V2.0 kind rules.

## 8. Example-site contract

`prism-example-site` is the compatibility consumer for this specification:

- retain at least one V1 backend plugin;
- migrate at least one backend plugin to `ManifestProvider`;
- prove a required dependency is ordered before its consumer;
- prove scoped middleware cannot leak to a sibling addon;
- build and test against an initialized, pinned Prism Fusion submodule;
- run backend tests/build, frontend typecheck/build, and a `/health` smoke test in CI.

The example submodule pointer must be updated only after a V2 framework commit exists. A permanent `replace` to a sibling workspace checkout is not portable and is not an acceptable release setup.

## 9. RBAC permission codes

Permission contributions use exactly three lowercase segments: `domain:resource:action`. Each segment starts with a letter and may contain letters, digits, underscores, or hyphens. The full code is limited to 128 bytes. Wildcards such as `*:*:*` are runtime grants, not permission catalog entries. Invalid contributed seeds fail during registration or before persistence.
