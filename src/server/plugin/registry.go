package plugin

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrNilPlugin         = errors.New("plugin is nil")
	ErrInvalidPlugin     = errors.New("invalid plugin")
	ErrInvalidManifest   = errors.New("invalid plugin manifest")
	ErrDuplicatePlugin   = errors.New("duplicate plugin")
	ErrRegistryFrozen    = errors.New("plugin registry is frozen")
	ErrRegistryFreezing  = errors.New("plugin registry freeze is in progress")
	ErrRegistryResolving = errors.New("plugin registry resolution is in progress")
	ErrMissingDependency = errors.New("missing plugin dependency")
	ErrPluginConflict    = errors.New("plugin conflict")
	ErrDependencyCycle   = errors.New("plugin dependency cycle")
)

// ResolvedPlugin 将插件实现与注册时冻结的 Manifest、优先级快照绑定。
type ResolvedPlugin struct {
	Plugin   Plugin
	Manifest Manifest
	Priority int
}

type registration struct {
	plugin   Plugin
	manifest Manifest
	priority int
}

// Registry 是并发安全、可冻结的插件注册表。
type Registry struct {
	mu            sync.RWMutex
	registrations map[string]registration
	resolving     bool
	freezing      bool
	frozen        bool
	resolved      []ResolvedPlugin
}

// NewRegistry 创建一个空注册表。
func NewRegistry() *Registry {
	return &Registry{registrations: make(map[string]registration)}
}

// Register 校验并注册插件。注册时会复制 Manifest，重复 ID 不会覆盖原插件。
func (r *Registry) Register(p Plugin) error {
	manifest, err := ResolveManifest(p)
	if err != nil {
		return err
	}
	record := registration{
		plugin:   p,
		manifest: cloneManifest(manifest),
		priority: p.Priority(),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen || r.freezing {
		return fmt.Errorf("%w: cannot register %q", ErrRegistryFrozen, manifest.ID)
	}
	if r.resolving {
		return fmt.Errorf("%w: cannot register %q", ErrRegistryResolving, manifest.ID)
	}
	if r.registrations == nil {
		r.registrations = make(map[string]registration)
	}
	if _, exists := r.registrations[manifest.ID]; exists {
		return fmt.Errorf("%w: id %q is already registered", ErrDuplicatePlugin, manifest.ID)
	}
	r.registrations[manifest.ID] = record
	return nil
}

// Freeze 校验依赖图并保存稳定解析结果。成功后不再接受注册。
func (r *Registry) Freeze() (err error) {
	r.mu.Lock()
	if r.frozen {
		r.mu.Unlock()
		return nil
	}
	if r.freezing {
		r.mu.Unlock()
		return ErrRegistryFreezing
	}
	if r.resolving {
		r.mu.Unlock()
		return ErrRegistryResolving
	}
	r.freezing = true
	registrations := cloneRegistrations(r.registrations)
	r.mu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			r.mu.Lock()
			r.freezing = false
			r.mu.Unlock()
			panic(recovered)
		}
	}()

	resolved, err := resolveRegistrations(registrations)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.freezing = false
	if err != nil {
		return err
	}
	r.resolved = cloneResolved(resolved)
	r.frozen = true
	return nil
}

// Resolve 返回稳定的依赖解析结果；未冻结时仅预览，不关闭注册窗口。
func (r *Registry) Resolve() ([]ResolvedPlugin, error) {
	r.mu.Lock()
	if r.freezing {
		r.mu.Unlock()
		return nil, ErrRegistryFreezing
	}
	if r.resolving {
		r.mu.Unlock()
		return nil, ErrRegistryResolving
	}
	if r.frozen {
		resolved := cloneResolved(r.resolved)
		r.mu.Unlock()
		return resolved, nil
	}
	r.resolving = true
	registrations := cloneRegistrations(r.registrations)
	r.mu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			r.mu.Lock()
			r.resolving = false
			r.mu.Unlock()
			panic(recovered)
		}
	}()

	resolved, err := resolveRegistrations(registrations)
	r.mu.Lock()
	r.resolving = false
	r.mu.Unlock()
	return resolved, err
}

// MustResolve 冻结注册表并返回解析结果，失败时 panic。
func (r *Registry) MustResolve() []ResolvedPlugin {
	if err := r.Freeze(); err != nil {
		panic(err)
	}
	resolved, err := r.Resolve()
	if err != nil {
		panic(err)
	}
	return resolved
}

// All 返回注册内容的独立 map 快照。
func (r *Registry) All() map[string]Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugins := make(map[string]Plugin, len(r.registrations))
	for id, record := range r.registrations {
		plugins[id] = record.plugin
	}
	return plugins
}

// Get 根据插件 ID 查询实现。
func (r *Registry) Get(id string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.registrations[id]
	return record.plugin, ok
}

// Names 返回按字典序排列的插件 ID 快照。
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.registrations))
	for name := range r.registrations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Count 返回已注册插件数量。
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.registrations)
}

func resolveRegistrations(registrations map[string]registration) ([]ResolvedPlugin, error) {
	registrations = activeRegistrations(registrations)
	ids := make([]string, 0, len(registrations))
	for id := range registrations {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	indegree := make(map[string]int, len(registrations))
	dependents := make(map[string]map[string]struct{}, len(registrations))
	for _, id := range ids {
		indegree[id] = 0
	}

	for _, id := range ids {
		record := registrations[id]
		for _, conflict := range record.manifest.Conflicts {
			if _, installed := registrations[conflict]; installed {
				return nil, fmt.Errorf("%w: %q conflicts with %q", ErrPluginConflict, id, conflict)
			}
		}
		for _, dependency := range record.manifest.Requires {
			if _, installed := registrations[dependency.ID]; !installed {
				return nil, fmt.Errorf("%w: plugin %q requires %q", ErrMissingDependency, id, dependency.ID)
			}
			addDependencyEdge(dependents, indegree, dependency.ID, id)
		}
		for _, dependency := range record.manifest.Optional {
			if _, installed := registrations[dependency.ID]; installed {
				addDependencyEdge(dependents, indegree, dependency.ID, id)
			}
		}
	}

	ready := make([]string, 0, len(registrations))
	for _, id := range ids {
		if indegree[id] == 0 {
			ready = append(ready, id)
		}
	}
	sortReady(ready, registrations)

	resolved := make([]ResolvedPlugin, 0, len(registrations))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		record := registrations[id]
		resolved = append(resolved, ResolvedPlugin{
			Plugin:   record.plugin,
			Manifest: cloneManifest(record.manifest),
			Priority: record.priority,
		})

		children := make([]string, 0, len(dependents[id]))
		for dependent := range dependents[id] {
			children = append(children, dependent)
		}
		sort.Strings(children)
		for _, dependent := range children {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
		sortReady(ready, registrations)
	}

	if len(resolved) != len(registrations) {
		cycleIDs := make([]string, 0, len(registrations)-len(resolved))
		for _, id := range ids {
			if indegree[id] > 0 {
				cycleIDs = append(cycleIDs, id)
			}
		}
		return nil, fmt.Errorf("%w: unresolved plugins: %s", ErrDependencyCycle, joinIDs(cycleIDs))
	}
	return resolved, nil
}

func activeRegistrations(input map[string]registration) map[string]registration {
	result := make(map[string]registration, len(input))
	for id, record := range input {
		activation, configurable := record.plugin.(ActivationProvider)
		if configurable && !activation.PluginEnabled() {
			continue
		}
		result[id] = record
	}
	return result
}

func addDependencyEdge(dependents map[string]map[string]struct{}, indegree map[string]int, dependency, dependent string) {
	children := dependents[dependency]
	if children == nil {
		children = make(map[string]struct{})
		dependents[dependency] = children
	}
	if _, exists := children[dependent]; exists {
		return
	}
	children[dependent] = struct{}{}
	indegree[dependent]++
}

func sortReady(ready []string, registrations map[string]registration) {
	sort.Slice(ready, func(i, j int) bool {
		left := registrations[ready[i]]
		right := registrations[ready[j]]
		if left.priority != right.priority {
			return left.priority < right.priority
		}
		return ready[i] < ready[j]
	})
}

func cloneRegistrations(input map[string]registration) map[string]registration {
	result := make(map[string]registration, len(input))
	for id, record := range input {
		record.manifest = cloneManifest(record.manifest)
		result[id] = record
	}
	return result
}

func cloneResolved(input []ResolvedPlugin) []ResolvedPlugin {
	result := make([]ResolvedPlugin, len(input))
	for index, entry := range input {
		entry.Manifest = cloneManifest(entry.Manifest)
		result[index] = entry
	}
	return result
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	result := ids[0]
	for _, id := range ids[1:] {
		result += ", " + id
	}
	return result
}
