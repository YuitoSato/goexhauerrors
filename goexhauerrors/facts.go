package goexhauerrors

import "encoding/gob"

func init() {
	gob.Register(&ErrorFact{})
	gob.Register(&FunctionErrorsFact{})
	gob.Register(&ParameterFlowFact{})
}

// ErrorFact marks a variable or type as an error.
// Attached to *types.Var (for var Err* = errors.New()) or
// *types.TypeName (for custom error types).
type ErrorFact struct {
	Name    string // Variable or type name (e.g., "ErrNotFound", "ValidationError")
	PkgPath string // Package path where error is defined
}

func (*ErrorFact) AFact() {}

func (f *ErrorFact) String() string {
	return f.PkgPath + "." + f.Name
}

// Key returns a unique key for this error.
func (f *ErrorFact) Key() string {
	return f.PkgPath + "." + f.Name
}

// ErrorInfo contains metadata about an error that a function can return.
type ErrorInfo struct {
	PkgPath string // Package path where error is defined
	Name    string // Variable or type name
	Wrapped bool   // Whether this error might be wrapped with fmt.Errorf %w
}

func (s ErrorInfo) Key() string {
	return s.PkgPath + "." + s.Name
}

// FunctionErrorsFact stores all errors a function can return.
// Attached to *types.Func objects.
type FunctionErrorsFact struct {
	Errors []ErrorInfo // Errors this function can return
}

func (*FunctionErrorsFact) AFact() {}

func (f *FunctionErrorsFact) String() string {
	if len(f.Errors) == 0 {
		return "[]"
	}
	result := "["
	for i, s := range f.Errors {
		if i > 0 {
			result += ", "
		}
		result += s.Key()
	}
	result += "]"
	return result
}

// AddError adds an error to the fact if not already present.
func (f *FunctionErrorsFact) AddError(info ErrorInfo) {
	key := info.Key()
	for _, existing := range f.Errors {
		if existing.Key() == key {
			return
		}
	}
	f.Errors = append(f.Errors, info)
}

// Merge merges another fact's errors into this one.
func (f *FunctionErrorsFact) Merge(other *FunctionErrorsFact) {
	for _, s := range other.Errors {
		f.AddError(s)
	}
}

// FilterByValidErrors removes errors that are not in the provided set of valid errors.
// validErrors is a map from error key (PkgPath.Name) to true.
func (f *FunctionErrorsFact) FilterByValidErrors(validErrors map[string]bool) {
	var filtered []ErrorInfo
	for _, s := range f.Errors {
		if validErrors[s.Key()] {
			filtered = append(filtered, s)
		}
	}
	f.Errors = filtered
}

// ParameterFlowInfo describes how a function parameter flows to return values.
type ParameterFlowInfo struct {
	ParamIndex int  // Index of the parameter (0-based, excluding receiver for methods)
	Wrapped    bool // Whether the parameter is wrapped (e.g., via fmt.Errorf %w)
}

// ParameterFlowFact stores information about parameters that flow to return values.
// Attached to *types.Func objects for functions where error parameters are propagated.
type ParameterFlowFact struct {
	Flows []ParameterFlowInfo // Parameters that flow to error returns
}

func (*ParameterFlowFact) AFact() {}

func (f *ParameterFlowFact) String() string {
	if len(f.Flows) == 0 {
		return "[]"
	}
	result := "["
	for i, flow := range f.Flows {
		if i > 0 {
			result += ", "
		}
		if flow.Wrapped {
			result += "wrapped:"
		}
		result += string(rune('0' + flow.ParamIndex))
	}
	result += "]"
	return result
}

// AddFlow adds a parameter flow to the fact if not already present.
func (f *ParameterFlowFact) AddFlow(flow ParameterFlowInfo) {
	for _, existing := range f.Flows {
		if existing.ParamIndex == flow.ParamIndex {
			// If already exists, upgrade to wrapped if needed
			if flow.Wrapped && !existing.Wrapped {
				existing.Wrapped = true
			}
			return
		}
	}
	f.Flows = append(f.Flows, flow)
}

// Merge merges another fact's flows into this one.
func (f *ParameterFlowFact) Merge(other *ParameterFlowFact) {
	for _, flow := range other.Flows {
		f.AddFlow(flow)
	}
}
