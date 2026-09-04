package plugin

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

const (
	// APIVersionV1 标识由框架为旧 Plugin 接口合成的兼容 Manifest。
	APIVersionV1 = "prism-fusion/v1"
	// APIVersionV2 是当前插件 Manifest 版本。
	APIVersionV2 = "prism-fusion/v2"
)

// Kind 描述插件的部署与装配形态。
type Kind string

const (
	KindBackendAddon     Kind = "backend-addon"
	KindStrategyProvider Kind = "strategy-provider"
)

// Dependency 声明一个插件 ID 依赖。
// Version 为后续语义版本求解预留；V2.0 会拒绝非空约束。
type Dependency struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// Manifest 是 Prism Fusion V2 的插件元数据合同。
type Manifest struct {
	APIVersion  string       `json:"apiVersion"`
	ID          string       `json:"id"`
	Version     string       `json:"version"`
	Kind        Kind         `json:"kind"`
	Description string       `json:"description,omitempty"`
	Provides    []string     `json:"provides,omitempty"`
	Requires    []Dependency `json:"requires,omitempty"`
	Optional    []Dependency `json:"optional,omitempty"`
	Conflicts   []string     `json:"conflicts,omitempty"`
	RouteScopes []string     `json:"routeScopes,omitempty"`
}

// ManifestProvider 是 V2 插件可选实现的扩展接口。
// Plugin 本身保持不变，因此 V1 插件无需修改即可继续加载。
type ManifestProvider interface {
	Manifest() Manifest
}

// ActivationProvider allows a plugin to opt out after configuration is loaded.
// The decision is captured when the registry is frozen.
type ActivationProvider interface {
	PluginEnabled() bool
}

var canonicalIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)
var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// ResolveManifest 校验并复制插件 Manifest。V1 插件会获得兼容 Manifest。
func ResolveManifest(p Plugin) (Manifest, error) {
	if isNilPlugin(p) {
		return Manifest{}, ErrNilPlugin
	}

	name := p.Name()
	if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
		return Manifest{}, fmt.Errorf("%w: plugin id must be non-blank without surrounding whitespace", ErrInvalidPlugin)
	}

	provider, isV2 := p.(ManifestProvider)
	if !isV2 {
		scopes, err := normalizeRouteScopes(nil, p.RoutePrefix())
		if err != nil {
			return Manifest{}, fmt.Errorf("%w: plugin %q: %v", ErrInvalidPlugin, name, err)
		}
		return Manifest{
			APIVersion:  APIVersionV1,
			ID:          name,
			Version:     "0.0.0-legacy",
			Kind:        KindBackendAddon,
			Description: p.Description(),
			RouteScopes: scopes,
		}, nil
	}

	manifest := provider.Manifest()
	if err := validateCanonicalID(name); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if manifest.APIVersion != APIVersionV2 {
		return Manifest{}, fmt.Errorf("%w: plugin %q declares unsupported apiVersion %q", ErrInvalidManifest, name, manifest.APIVersion)
	}
	if manifest.ID != name {
		return Manifest{}, fmt.Errorf("%w: plugin name %q does not match manifest id %q", ErrInvalidManifest, name, manifest.ID)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return Manifest{}, fmt.Errorf("%w: plugin %q must declare a version", ErrInvalidManifest, name)
	}
	if !isSemanticVersion(strings.TrimSpace(manifest.Version)) {
		return Manifest{}, fmt.Errorf("%w: plugin %q version %q is not semantic versioning", ErrInvalidManifest, name, manifest.Version)
	}
	if manifest.Kind != KindBackendAddon {
		return Manifest{}, fmt.Errorf("%w: plugin %q kind %q is reserved but not supported in V2.0", ErrInvalidManifest, name, manifest.Kind)
	}

	resolved, err := normalizeManifest(manifest, p)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: plugin %q: %v", ErrInvalidManifest, name, err)
	}
	return resolved, nil
}

func normalizeManifest(manifest Manifest, p Plugin) (Manifest, error) {
	manifest.Version = strings.TrimSpace(manifest.Version)
	if strings.TrimSpace(manifest.Description) == "" {
		manifest.Description = p.Description()
	}

	provides, err := normalizeStrings(manifest.Provides, "capability", false)
	if err != nil {
		return Manifest{}, err
	}
	requires, err := normalizeDependencies(manifest.Requires)
	if err != nil {
		return Manifest{}, err
	}
	optional, err := normalizeDependencies(manifest.Optional)
	if err != nil {
		return Manifest{}, err
	}
	conflicts, err := normalizeStrings(manifest.Conflicts, "conflict id", true)
	if err != nil {
		return Manifest{}, err
	}
	scopes, err := normalizeRouteScopes(manifest.RouteScopes, p.RoutePrefix())
	if err != nil {
		return Manifest{}, err
	}

	requiredIDs := make(map[string]struct{}, len(requires))
	for _, dependency := range requires {
		requiredIDs[dependency.ID] = struct{}{}
	}
	filteredOptional := make([]Dependency, 0, len(optional))
	for _, dependency := range optional {
		if _, required := requiredIDs[dependency.ID]; !required {
			filteredOptional = append(filteredOptional, dependency)
		}
	}

	manifest.Provides = provides
	manifest.Requires = requires
	manifest.Optional = filteredOptional
	manifest.Conflicts = conflicts
	manifest.RouteScopes = scopes
	return manifest, nil
}

func normalizeDependencies(input []Dependency) ([]Dependency, error) {
	byID := make(map[string]Dependency, len(input))
	for _, dependency := range input {
		dependency.ID = strings.TrimSpace(dependency.ID)
		dependency.Version = strings.TrimSpace(dependency.Version)
		if err := validateCanonicalID(dependency.ID); err != nil {
			return nil, fmt.Errorf("invalid dependency: %v", err)
		}
		if dependency.Version != "" {
			return nil, fmt.Errorf("dependency %q uses version constraints, which are not supported in V2.0", dependency.ID)
		}
		byID[dependency.ID] = dependency
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]Dependency, 0, len(ids))
	for _, id := range ids {
		result = append(result, byID[id])
	}
	return result, nil
}

func normalizeStrings(input []string, label string, canonicalIDs bool) ([]string, error) {
	unique := make(map[string]struct{}, len(input))
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%s must not be blank", label)
		}
		if canonicalIDs {
			if err := validateCanonicalID(value); err != nil {
				return nil, fmt.Errorf("invalid %s: %v", label, err)
			}
		} else if strings.ContainsAny(value, " \t\r\n") {
			return nil, fmt.Errorf("%s %q must not contain whitespace", label, value)
		}
		unique[value] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeRouteScopes(scopes []string, fallback string) ([]string, error) {
	if len(scopes) == 0 && fallback != "" {
		scopes = []string{fallback}
	}
	unique := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		normalized, err := NormalizeRouteScope(scope)
		if err != nil {
			return nil, err
		}
		unique[normalized] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for scope := range unique {
		result = append(result, scope)
	}
	sort.Strings(result)
	return result, nil
}

// NormalizeRouteScope 校验并规范化绝对路由作用域。
func NormalizeRouteScope(scope string) (string, error) {
	if scope == "" {
		return "", fmt.Errorf("route scope must not be blank")
	}
	if scope != strings.TrimSpace(scope) {
		return "", fmt.Errorf("route scope %q contains surrounding whitespace", scope)
	}
	if !strings.HasPrefix(scope, "/") {
		return "", fmt.Errorf("route scope %q must be absolute", scope)
	}
	if strings.ContainsAny(scope, "?#") {
		return "", fmt.Errorf("route scope %q must not contain query or fragment", scope)
	}
	if strings.ContainsAny(scope, ":*{}") {
		return "", fmt.Errorf("route scope %q must be a static literal prefix", scope)
	}
	if scope != "/" {
		scope = strings.TrimSuffix(scope, "/")
	}
	if path.Clean(scope) != scope {
		return "", fmt.Errorf("route scope %q is not clean", scope)
	}
	return scope, nil
}

func validateCanonicalID(id string) error {
	if id == "" {
		return fmt.Errorf("plugin id must not be blank")
	}
	if id != strings.TrimSpace(id) || !canonicalIDPattern.MatchString(id) {
		return fmt.Errorf("plugin id %q must match %s", id, canonicalIDPattern.String())
	}
	return nil
}

func isSemanticVersion(version string) bool {
	if !semanticVersionPattern.MatchString(version) {
		return false
	}
	withoutBuild := strings.SplitN(version, "+", 2)[0]
	parts := strings.SplitN(withoutBuild, "-", 2)
	if len(parts) != 2 {
		return true
	}
	for _, identifier := range strings.Split(parts[1], ".") {
		allDigits := true
		for _, char := range identifier {
			if char < '0' || char > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func isNilPlugin(p Plugin) bool {
	if p == nil {
		return true
	}
	value := reflect.ValueOf(p)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.Provides = append([]string(nil), manifest.Provides...)
	manifest.Requires = append([]Dependency(nil), manifest.Requires...)
	manifest.Optional = append([]Dependency(nil), manifest.Optional...)
	manifest.Conflicts = append([]string(nil), manifest.Conflicts...)
	manifest.RouteScopes = append([]string(nil), manifest.RouteScopes...)
	return manifest
}
