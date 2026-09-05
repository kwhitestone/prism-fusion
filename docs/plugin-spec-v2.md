# Prism Fusion Plugin Specification V2

Status: backend and frontend V2.0 startup contracts and architecture CI guards implemented. Full backend lifecycle, provider contribution, and remote-application contracts remain staged extensions.

## 1. Goals

V2 turns plugin loading from an implicit `init()` side effect into a validated startup contract while preserving source compatibility with existing backend plugins.

The specification distinguishes four extension forms:

| Kind | Runtime boundary | V2.0 status |
| --- | --- | --- |
| Backend Addon | In-process Go plugin that owns routes, middleware, and models | Implemented |
| Strategy Provider | Replaceable implementation inside a capability domain | Kind constant reserved but rejected by V2.0; contribution registry is next phase |
| Frontend Addon | In-process Vue module in the same Vite bundle | V1-compatible `PluginModule`, V2 manifest, dependency graph and transactional installation |
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

An invalid scope fails closed. A route-free plugin may have no scopes; a plugin registering routes may not. `/` cannot own addon routes because it overlaps reserved core namespaces. Cross-plugin middleware must use the explicit `GlobalMiddlewares` contract.

Every V1/V2 plugin is checked, whether or not it has middleware. Every newly registered route must fall inside its owner's declared scopes. Independent public callback scopes are allowed; attaching scoped middleware to those scopes still protects those callbacks.

Startup rejects any scope containing, contained by, or equal to another active plugin's scope. Nested scopes of the same owner remain valid. `/api/v1/system` and actual core route namespaces (including static assets, documentation and health endpoints) are reserved across HTTP methods. Parameterized core routes reserve their static parent namespace.

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
- retain V1 frontend addons alongside a V2 frontend example;
- keep the host entrypoint composition-only, including moving site-specific footer behavior into `site-info`;
- declare a non-root dashboard scope and configure the shell landing route explicitly;
- pass the same architecture guard as the pinned framework.

The example submodule pointer must be updated only after a V2 framework commit exists. A permanent `replace` to a sibling workspace checkout is not portable and is not an acceptable release setup.

## 9. RBAC permission codes

Permission contributions use exactly three lowercase segments: `domain:resource:action`. Each segment starts with a letter and may contain letters, digits, underscores, or hyphens. The full code is limited to 128 bytes. Wildcards such as `*:*:*` are runtime grants, not permission catalog entries. Invalid contributed seeds fail during registration or before persistence.

## 10. Frontend startup contract

V1 modules remain valid without a manifest; new addons should declare V2:

```ts
const orders: PluginModule = {
  name: "orders",
  manifest: {
    apiVersion: "prism-fusion/v2",
    kind: "frontend-addon",
    id: "orders",
    version: "2.0.0",
    requires: [{ id: "auth" }],
    routeScopes: ["/orders"]
  },
  routes: [{ path: "/orders", name: "Orders", component: OrdersPage }]
};
registerExternalPlugins([orders]);
configurePluginHost({ homePath: "/orders" });
await installPlugins(app, router); // use the framework-exported router
app.use(router);
await router.isReady();
app.mount("#app");
```

Registration copies metadata and is batch-atomic. Duplicate IDs, invalid manifests and late registration fail. `enabled` is a boolean or callback, captured at freeze after configuration loads. Missing/disabled required dependencies, conflicts and cycles fail startup. Required and present optional dependencies precede consumers; ready nodes sort by `priority` (default 100), then ID. Nonempty dependency version constraints are rejected.

Before running hooks, runtime checks all active route paths, names, aliases, nested absolute paths, Vue Router patterns and component names against core and other addons. V2 routes must stay in declared static scopes; V1 routes still undergo collision checks. Cross-owner dynamic patterns with overlapping static prefixes are conservatively rejected, even when custom regular expressions might be disjoint. Use separate namespaces. A layout and its own default child may share their URL.

`install` and `setup` are awaited. Failure rejects startup, removes contributed routes/components, and invokes initialized addons' `destroy` hooks in reverse order. `destroy` must release its own listeners, provider registrations and other effects. Builtin Auth/RBAC use reversible registrations. Arbitrary network effects and addon-owned global state cannot be automatically undone. Cleanup errors are logged; this is cooperative cleanup, not a sandbox or database transaction.

Repeated/concurrent installation in the same host is idempotent. Another host/router is rejected. `uninstallPlugins` is asynchronous and must be awaited; it cancels pending startup, cleans contributions and restores core routes. It does not reopen registration or promise hot plugin replacement. Inspection returns independent metadata snapshots while retaining executable component/function identity.

Menus and the reset baseline are committed only after successful installation. Logout/reset restores this baseline, including plugin routes and the configured landing redirect. Addons must not claim `/` to override the shell; use `configurePluginHost` once before startup. `registerExternalRoutes` now throws: migrate bare routes into a `PluginModule`. The router no longer independently discovers `addons/*/router` files.

Backend `async-routes` is navigation metadata, not executable route ownership. Only validated display fields for already-declared paths are merged; unknown paths and component/name/redirect/authority changes are ignored. Declare business pages in addon routes, not core `views` or database component strings. Menu visibility does not replace backend authorization.

## 11. Architecture enforcement and limits

Run after installing frontend dependencies:

```sh
node scripts/architecture/guard.test.mjs
node scripts/architecture/guard.mjs --root .
node scripts/architecture/guard.mjs --root ../prism-example-site --profile example
pnpm --dir src/web test
```

The shared guard combines TypeScript/Vue and Go AST inspection with an explicit core-source inventory. New runtime files outside addons, unauthorized addon imports, concrete business endpoints/models/stores, and unapproved router mutations fail CI. Existing core files are inspected too. Exceptions are explicit infrastructure contracts, not permission to put arbitrary business behavior in an allowlisted file. Changes to the checker, inventory or exceptions require architectural review.

This establishes enforceable mechanical boundaries for the checked repositories, not a proof that every function is business-free. In-process Go/Vue addons are trusted code; indirect effects, deliberate policy changes and semantic business leakage still need review. Other services must adopt this guard and pass their own consumer tests before claiming the same guarantee.
