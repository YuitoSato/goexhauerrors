package goexhauerrors

import "encoding/gob"

func init() {
	gob.Register(&ErrorFact{})
	gob.Register(&FunctionErrorsFact{})
	gob.Register(&ParameterFlowFact{})
	gob.Register(&InterfaceMethodFact{})
	gob.Register(&FunctionParamCallFlowFact{})
	gob.Register(&ParameterCheckedErrorsFact{})
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

// HasFlowForParam checks if any parameter flow exists for the given parameter index.
func (f *ParameterFlowFact) HasFlowForParam(paramIndex int) bool {
	for _, flow := range f.Flows {
		if flow.ParamIndex == paramIndex {
			return true
		}
	}
	return false
}

// Merge merges another fact's flows into this one.
func (f *ParameterFlowFact) Merge(other *ParameterFlowFact) {
	for _, flow := range other.Flows {
		f.AddFlow(flow)
	}
}

// InterfaceMethodFact stores all errors that implementations of an interface method can return.
// Attached to *types.Func objects representing interface methods.
type InterfaceMethodFact struct {
	Errors []ErrorInfo // Union of errors from all implementations
}

func (*InterfaceMethodFact) AFact() {}

func (f *InterfaceMethodFact) String() string {
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
func (f *InterfaceMethodFact) AddError(info ErrorInfo) {
	key := info.Key()
	for _, existing := range f.Errors {
		if existing.Key() == key {
			return
		}
	}
	f.Errors = append(f.Errors, info)
}

// AddErrors adds multiple errors to the fact.
func (f *InterfaceMethodFact) AddErrors(infos []ErrorInfo) {
	for _, info := range infos {
		f.AddError(info)
	}
}

// FunctionParamCallFlowInfo describes how a function parameter's call result
// flows to the return value.
type FunctionParamCallFlowInfo struct {
	ParamIndex int  // Index of the function-typed parameter (0-based)
	Wrapped    bool // Whether the result is wrapped (e.g., via fmt.Errorf %w)
}

// FunctionParamCallFlowFact tracks parameters that are functions whose
// call results flow to the return value.
// Example: func RunInTx(fn func() error) error { return fn() }
// -> FunctionParamCallFlowFact{CallFlows: [{ParamIndex: 0}]}
// Attached to *types.Func objects.
type FunctionParamCallFlowFact struct {
	CallFlows []FunctionParamCallFlowInfo
}

func (*FunctionParamCallFlowFact) AFact() {}

func (f *FunctionParamCallFlowFact) String() string {
	if len(f.CallFlows) == 0 {
		return "[]"
	}
	result := "["
	for i, flow := range f.CallFlows {
		if i > 0 {
			result += ", "
		}
		if flow.Wrapped {
			result += "wrapped:"
		}
		result += "call:"
		result += string(rune('0' + flow.ParamIndex))
	}
	result += "]"
	return result
}

// AddCallFlow adds a function parameter call flow to the fact if not already present.
func (f *FunctionParamCallFlowFact) AddCallFlow(flow FunctionParamCallFlowInfo) {
	for i, existing := range f.CallFlows {
		if existing.ParamIndex == flow.ParamIndex {
			// If already exists, upgrade to wrapped if needed
			if flow.Wrapped && !existing.Wrapped {
				f.CallFlows[i].Wrapped = true
			}
			return
		}
	}
	f.CallFlows = append(f.CallFlows, flow)
}

// Merge merges another fact's flows into this one.
func (f *FunctionParamCallFlowFact) Merge(other *FunctionParamCallFlowFact) {
	for _, flow := range other.CallFlows {
		f.AddCallFlow(flow)
	}
}

// ParameterCheckedErrorInfo describes which errors are checked with errors.Is/As
// on a specific error parameter inside a function body.
type ParameterCheckedErrorInfo struct {
	ParamIndex    int         // Index of the error parameter (0-based, excluding receiver)
	CheckedErrors []ErrorInfo // Errors checked with errors.Is/As on this parameter
}

// ParameterCheckedErrorsFact stores which errors are checked with errors.Is/As
// on each error parameter inside a function body.
// Attached to *types.Func objects.
type ParameterCheckedErrorsFact struct {
	Checks []ParameterCheckedErrorInfo
}

func (*ParameterCheckedErrorsFact) AFact() {}

func (f *ParameterCheckedErrorsFact) String() string {
	if len(f.Checks) == 0 {
		return "[]"
	}
	result := "["
	for i, check := range f.Checks {
		if i > 0 {
			result += ", "
		}
		result += "param" + string(rune('0'+check.ParamIndex)) + ":["
		for j, err := range check.CheckedErrors {
			if j > 0 {
				result += ","
			}
			result += err.Key()
		}
		result += "]"
	}
	result += "]"
	return result
}

// AddCheck adds a checked error for a parameter.
func (f *ParameterCheckedErrorsFact) AddCheck(paramIndex int, errInfo ErrorInfo) {
	key := errInfo.Key()
	for i, check := range f.Checks {
		if check.ParamIndex == paramIndex {
			// Check if already present
			for _, existing := range check.CheckedErrors {
				if existing.Key() == key {
					return
				}
			}
			f.Checks[i].CheckedErrors = append(f.Checks[i].CheckedErrors, errInfo)
			return
		}
	}
	f.Checks = append(f.Checks, ParameterCheckedErrorInfo{
		ParamIndex:    paramIndex,
		CheckedErrors: []ErrorInfo{errInfo},
	})
}

// GetCheckedErrors returns the checked errors for a given parameter index.
func (f *ParameterCheckedErrorsFact) GetCheckedErrors(paramIndex int) []ErrorInfo {
	for _, check := range f.Checks {
		if check.ParamIndex == paramIndex {
			return check.CheckedErrors
		}
	}
	return nil
}

// intersectParameterFlowFacts computes the intersection of ParameterFlowFact across implementations.
// A parameter flow is kept only if it exists in ALL non-nil facts.
// If any fact is nil (implementation has no flow), the intersection is empty.
func intersectParameterFlowFacts(facts []*ParameterFlowFact) *ParameterFlowFact {
	if len(facts) == 0 {
		return nil
	}

	// If any implementation has no ParameterFlowFact, intersection is empty
	for _, f := range facts {
		if f == nil || len(f.Flows) == 0 {
			return nil
		}
	}

	// Start with the first fact's flows, keep only those present in all others
	result := &ParameterFlowFact{}
	for _, flow := range facts[0].Flows {
		presentInAll := true
		for _, other := range facts[1:] {
			if !other.HasFlowForParam(flow.ParamIndex) {
				presentInAll = false
				break
			}
		}
		if presentInAll {
			result.AddFlow(flow)
		}
	}

	if len(result.Flows) == 0 {
		return nil
	}
	return result
}

// intersectParameterCheckedErrorsFacts computes the intersection of ParameterCheckedErrorsFact across implementations.
// An error check is kept only if it exists in ALL non-nil facts.
// If any fact is nil, the intersection is empty.
func intersectParameterCheckedErrorsFacts(facts []*ParameterCheckedErrorsFact) *ParameterCheckedErrorsFact {
	if len(facts) == 0 {
		return nil
	}

	// If any implementation has no ParameterCheckedErrorsFact, intersection is empty
	for _, f := range facts {
		if f == nil || len(f.Checks) == 0 {
			return nil
		}
	}

	// For each param index in the first fact, keep only errors checked in ALL facts
	result := &ParameterCheckedErrorsFact{}
	for _, check := range facts[0].Checks {
		for _, errInfo := range check.CheckedErrors {
			checkedInAll := true
			for _, other := range facts[1:] {
				otherChecked := other.GetCheckedErrors(check.ParamIndex)
				if !containsErrorInfo(otherChecked, errInfo) {
					checkedInAll = false
					break
				}
			}
			if checkedInAll {
				result.AddCheck(check.ParamIndex, errInfo)
			}
		}
	}

	if len(result.Checks) == 0 {
		return nil
	}
	return result
}

// containsErrorInfo checks if a slice of ErrorInfo contains the given error.
func containsErrorInfo(infos []ErrorInfo, target ErrorInfo) bool {
	key := target.Key()
	for _, info := range infos {
		if info.Key() == key {
			return true
		}
	}
	return false
}
