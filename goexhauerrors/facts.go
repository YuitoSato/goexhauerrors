package goexhauerrors

import "encoding/gob"

func init() {
	gob.Register(&SentinelErrorFact{})
	gob.Register(&FunctionSentinelsFact{})
}

// SentinelErrorFact marks a variable or type as a sentinel error.
// Attached to *types.Var (for var Err* = errors.New()) or
// *types.TypeName (for custom error types).
type SentinelErrorFact struct {
	Name    string // Variable or type name (e.g., "ErrNotFound", "ValidationError")
	PkgPath string // Package path where sentinel is defined
}

func (*SentinelErrorFact) AFact() {}

func (f *SentinelErrorFact) String() string {
	return f.PkgPath + "." + f.Name
}

// Key returns a unique key for this sentinel.
func (f *SentinelErrorFact) Key() string {
	return f.PkgPath + "." + f.Name
}

// SentinelInfo contains metadata about a sentinel error that a function can return.
type SentinelInfo struct {
	PkgPath string // Package path where sentinel is defined
	Name    string // Variable or type name
	Wrapped bool   // Whether this sentinel might be wrapped with fmt.Errorf %w
}

func (s SentinelInfo) Key() string {
	return s.PkgPath + "." + s.Name
}

// FunctionSentinelsFact stores all sentinel errors a function can return.
// Attached to *types.Func objects.
type FunctionSentinelsFact struct {
	Sentinels []SentinelInfo // Sentinel errors this function can return
}

func (*FunctionSentinelsFact) AFact() {}

func (f *FunctionSentinelsFact) String() string {
	if len(f.Sentinels) == 0 {
		return "[]"
	}
	result := "["
	for i, s := range f.Sentinels {
		if i > 0 {
			result += ", "
		}
		result += s.Key()
	}
	result += "]"
	return result
}

// AddSentinel adds a sentinel to the fact if not already present.
func (f *FunctionSentinelsFact) AddSentinel(info SentinelInfo) {
	key := info.Key()
	for _, existing := range f.Sentinels {
		if existing.Key() == key {
			return
		}
	}
	f.Sentinels = append(f.Sentinels, info)
}

// Merge merges another fact's sentinels into this one.
func (f *FunctionSentinelsFact) Merge(other *FunctionSentinelsFact) {
	for _, s := range other.Sentinels {
		f.AddSentinel(s)
	}
}

// FilterByValidSentinels removes sentinels that are not in the provided set of valid sentinels.
// validSentinels is a map from sentinel key (PkgPath.Name) to true.
func (f *FunctionSentinelsFact) FilterByValidSentinels(validSentinels map[string]bool) {
	var filtered []SentinelInfo
	for _, s := range f.Sentinels {
		if validSentinels[s.Key()] {
			filtered = append(filtered, s)
		}
	}
	f.Sentinels = filtered
}
