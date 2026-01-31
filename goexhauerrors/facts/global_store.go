package facts

import "sync"

// globalInterfaceMethodStore is a process-wide store for InterfaceMethodFact.
// It allows cross-package interface method error tracking even when the caller
// does not import the implementation package (DI pattern).
var globalInterfaceMethodStore = &GlobalInterfaceMethodStore{
	store: make(map[string]*InterfaceMethodFact),
}

// GlobalInterfaceMethodStore stores InterfaceMethodFact across packages.
// It is keyed by "pkgPath.TypeName.MethodName" to identify interface methods.
type GlobalInterfaceMethodStore struct {
	mu    sync.RWMutex
	store map[string]*InterfaceMethodFact
}

// MergeInterfaceMethodFact merges errors into the global store for the given key.
// Uses union semantics: multiple implementation packages may contribute errors
// to the same interface method.
func MergeInterfaceMethodFact(key string, fact *InterfaceMethodFact) {
	globalInterfaceMethodStore.mu.Lock()
	defer globalInterfaceMethodStore.mu.Unlock()

	existing, ok := globalInterfaceMethodStore.store[key]
	if !ok {
		// Deep copy to avoid sharing pointers
		copied := &InterfaceMethodFact{}
		copied.AddErrors(fact.Errors)
		globalInterfaceMethodStore.store[key] = copied
		return
	}
	existing.AddErrors(fact.Errors)
}

// LoadInterfaceMethodFact loads the InterfaceMethodFact for the given key.
func LoadInterfaceMethodFact(key string) (*InterfaceMethodFact, bool) {
	globalInterfaceMethodStore.mu.RLock()
	defer globalInterfaceMethodStore.mu.RUnlock()

	fact, ok := globalInterfaceMethodStore.store[key]
	return fact, ok
}

// DeferredFunctionCheck stores the context needed to re-analyze a function
// whose checker couldn't resolve an interface method via the global store.
type DeferredFunctionCheck struct {
	// ReAnalyze re-runs the checker for this function.
	// Returns true if it still has unresolved global store misses.
	ReAnalyze func() bool
}

var deferredChecks struct {
	mu    sync.Mutex
	items []*DeferredFunctionCheck
}

// AddDeferredFunctionCheck registers a function for deferred re-analysis.
func AddDeferredFunctionCheck(check *DeferredFunctionCheck) {
	deferredChecks.mu.Lock()
	defer deferredChecks.mu.Unlock()
	deferredChecks.items = append(deferredChecks.items, check)
}

// ProcessDeferredFunctionChecks re-analyzes all deferred functions.
// Called at the end of each package's run() to process functions from
// packages that were analyzed before their implementation packages.
// If a re-analysis still has unresolved misses, the check is re-added
// so that a later package's run() can try again.
func ProcessDeferredFunctionChecks() {
	deferredChecks.mu.Lock()
	items := deferredChecks.items
	deferredChecks.items = nil
	deferredChecks.mu.Unlock()

	var stillMissing []*DeferredFunctionCheck
	for _, check := range items {
		if check.ReAnalyze() {
			stillMissing = append(stillMissing, check)
		}
	}

	if len(stillMissing) > 0 {
		deferredChecks.mu.Lock()
		deferredChecks.items = append(deferredChecks.items, stillMissing...)
		deferredChecks.mu.Unlock()
	}
}

// ResetGlobalStore clears the global store and deferred checks. Used in tests.
func ResetGlobalStore() {
	globalInterfaceMethodStore.mu.Lock()
	defer globalInterfaceMethodStore.mu.Unlock()
	globalInterfaceMethodStore.store = make(map[string]*InterfaceMethodFact)

	deferredChecks.mu.Lock()
	defer deferredChecks.mu.Unlock()
	deferredChecks.items = nil
}

// InterfaceMethodKey builds a key for the global store from package path, type name, and method name.
func InterfaceMethodKey(pkgPath, typeName, methodName string) string {
	return pkgPath + "." + typeName + "." + methodName
}
