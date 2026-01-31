package facts

import (
	"testing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ei(pkg, name string) ErrorInfo {
	return ErrorInfo{PkgPath: pkg, Name: name}
}

func eiw(pkg, name string) ErrorInfo {
	return ErrorInfo{PkgPath: pkg, Name: name, Wrapped: true}
}

// ---------------------------------------------------------------------------
// ErrorFact
// ---------------------------------------------------------------------------

func TestErrorFact_String(t *testing.T) {
	tests := []struct {
		name string
		fact ErrorFact
		want string
	}{
		{"normal", ErrorFact{Name: "ErrNotFound", PkgPath: "pkg/errors"}, "pkg/errors.ErrNotFound"},
		{"empty fields", ErrorFact{}, "."},
		{"empty name", ErrorFact{PkgPath: "pkg"}, "pkg."},
		{"empty pkg", ErrorFact{Name: "Err"}, ".Err"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fact.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorFact_Key(t *testing.T) {
	f := &ErrorFact{Name: "ErrTimeout", PkgPath: "net/http"}
	if got := f.Key(); got != "net/http.ErrTimeout" {
		t.Errorf("Key() = %q, want %q", got, "net/http.ErrTimeout")
	}
}

func TestErrorFact_AFact(t *testing.T) {
	// Just ensure it doesn't panic; it's a marker method.
	f := &ErrorFact{}
	f.AFact()
}

// ---------------------------------------------------------------------------
// ErrorInfo
// ---------------------------------------------------------------------------

func TestErrorInfo_Key(t *testing.T) {
	tests := []struct {
		name string
		info ErrorInfo
		want string
	}{
		{"normal", ei("pkg/a", "ErrA"), "pkg/a.ErrA"},
		{"wrapped ignored in key", eiw("pkg/b", "ErrB"), "pkg/b.ErrB"},
		{"empty", ErrorInfo{}, "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.Key(); got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FunctionErrorsFact – AddError
// ---------------------------------------------------------------------------

func TestFunctionErrorsFact_AddError(t *testing.T) {
	t.Run("normal add", func(t *testing.T) {
		f := &FunctionErrorsFact{}
		f.AddError(ei("p", "A"))
		f.AddError(ei("p", "B"))
		if len(f.Errors) != 2 {
			t.Fatalf("expected 2 errors, got %d", len(f.Errors))
		}
	})

	t.Run("duplicate prevention", func(t *testing.T) {
		f := &FunctionErrorsFact{}
		f.AddError(ei("p", "A"))
		f.AddError(ei("p", "A"))
		if len(f.Errors) != 1 {
			t.Fatalf("expected 1 error after duplicate add, got %d", len(f.Errors))
		}
	})

	t.Run("same key different wrapped", func(t *testing.T) {
		f := &FunctionErrorsFact{}
		f.AddError(ei("p", "A"))
		f.AddError(eiw("p", "A")) // same key
		if len(f.Errors) != 1 {
			t.Fatalf("expected 1 error (key dedup), got %d", len(f.Errors))
		}
	})

	t.Run("empty fact", func(t *testing.T) {
		f := &FunctionErrorsFact{}
		if len(f.Errors) != 0 {
			t.Fatalf("expected 0 errors, got %d", len(f.Errors))
		}
		f.AddError(ei("p", "X"))
		if len(f.Errors) != 1 {
			t.Fatalf("expected 1 error, got %d", len(f.Errors))
		}
	})
}

// ---------------------------------------------------------------------------
// FunctionErrorsFact – Merge
// ---------------------------------------------------------------------------

func TestFunctionErrorsFact_Merge(t *testing.T) {
	t.Run("merge into empty", func(t *testing.T) {
		f := &FunctionErrorsFact{}
		other := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "A"), ei("p", "B")}}
		f.Merge(other)
		if len(f.Errors) != 2 {
			t.Fatalf("expected 2, got %d", len(f.Errors))
		}
	})

	t.Run("merge non-overlapping", func(t *testing.T) {
		f := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "A")}}
		other := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "B")}}
		f.Merge(other)
		if len(f.Errors) != 2 {
			t.Fatalf("expected 2, got %d", len(f.Errors))
		}
	})

	t.Run("merge overlapping", func(t *testing.T) {
		f := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "A"), ei("p", "B")}}
		other := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "B"), ei("p", "C")}}
		f.Merge(other)
		if len(f.Errors) != 3 {
			t.Fatalf("expected 3, got %d", len(f.Errors))
		}
	})
}

// ---------------------------------------------------------------------------
// FunctionErrorsFact – FilterByValidErrors
// ---------------------------------------------------------------------------

func TestFunctionErrorsFact_FilterByValidErrors(t *testing.T) {
	t.Run("filter some", func(t *testing.T) {
		f := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "A"), ei("p", "B"), ei("p", "C")}}
		f.FilterByValidErrors(map[string]bool{"p.A": true, "p.C": true})
		if len(f.Errors) != 2 {
			t.Fatalf("expected 2, got %d", len(f.Errors))
		}
	})

	t.Run("filter all", func(t *testing.T) {
		f := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "A")}}
		f.FilterByValidErrors(map[string]bool{})
		if len(f.Errors) != 0 {
			t.Fatalf("expected 0, got %d", len(f.Errors))
		}
	})

	t.Run("filter none", func(t *testing.T) {
		f := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "A"), ei("p", "B")}}
		f.FilterByValidErrors(map[string]bool{"p.A": true, "p.B": true})
		if len(f.Errors) != 2 {
			t.Fatalf("expected 2, got %d", len(f.Errors))
		}
	})

	t.Run("empty valid set", func(t *testing.T) {
		f := &FunctionErrorsFact{Errors: []ErrorInfo{ei("p", "A")}}
		f.FilterByValidErrors(map[string]bool{})
		if len(f.Errors) != 0 {
			t.Fatalf("expected 0, got %d", len(f.Errors))
		}
	})
}

// ---------------------------------------------------------------------------
// FunctionErrorsFact – String
// ---------------------------------------------------------------------------

func TestFunctionErrorsFact_String(t *testing.T) {
	tests := []struct {
		name   string
		errors []ErrorInfo
		want   string
	}{
		{"empty", nil, "[]"},
		{"single", []ErrorInfo{ei("p", "A")}, "[p.A]"},
		{"multiple", []ErrorInfo{ei("p", "A"), ei("q", "B")}, "[p.A, q.B]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FunctionErrorsFact{Errors: tt.errors}
			if got := f.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParameterFlowFact – AddFlow
// ---------------------------------------------------------------------------

func TestParameterFlowFact_AddFlow(t *testing.T) {
	t.Run("new flow", func(t *testing.T) {
		f := &ParameterFlowFact{}
		f.AddFlow(ParameterFlowInfo{ParamIndex: 0, Wrapped: false})
		if len(f.Flows) != 1 {
			t.Fatalf("expected 1 flow, got %d", len(f.Flows))
		}
	})

	t.Run("duplicate same param does not add", func(t *testing.T) {
		f := &ParameterFlowFact{}
		f.AddFlow(ParameterFlowInfo{ParamIndex: 0, Wrapped: false})
		f.AddFlow(ParameterFlowInfo{ParamIndex: 0, Wrapped: false})
		if len(f.Flows) != 1 {
			t.Fatalf("expected 1, got %d", len(f.Flows))
		}
	})

	// NOTE: The upgrade logic in AddFlow iterates by value, so the mutation
	// of existing.Wrapped doesn't actually persist. This test documents the
	// current behaviour.
	t.Run("upgrade wrapped false to true (current behaviour)", func(t *testing.T) {
		f := &ParameterFlowFact{}
		f.AddFlow(ParameterFlowInfo{ParamIndex: 0, Wrapped: false})
		f.AddFlow(ParameterFlowInfo{ParamIndex: 0, Wrapped: true})
		// Because range iterates by value, the upgrade is not persisted.
		if f.Flows[0].Wrapped != false {
			t.Fatalf("expected Wrapped=false (current behaviour), got true")
		}
	})

	t.Run("keep wrapped true", func(t *testing.T) {
		f := &ParameterFlowFact{}
		f.AddFlow(ParameterFlowInfo{ParamIndex: 0, Wrapped: true})
		f.AddFlow(ParameterFlowInfo{ParamIndex: 0, Wrapped: false})
		// Already true, stays true (no downgrade path).
		if f.Flows[0].Wrapped != true {
			t.Fatalf("expected Wrapped=true, got false")
		}
	})
}

// ---------------------------------------------------------------------------
// ParameterFlowFact – HasFlowForParam
// ---------------------------------------------------------------------------

func TestParameterFlowFact_HasFlowForParam(t *testing.T) {
	f := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 1}}}

	if !f.HasFlowForParam(1) {
		t.Error("expected true for param 1")
	}
	if f.HasFlowForParam(0) {
		t.Error("expected false for param 0")
	}
}

// ---------------------------------------------------------------------------
// ParameterFlowFact – Merge
// ---------------------------------------------------------------------------

func TestParameterFlowFact_Merge(t *testing.T) {
	t.Run("non-overlapping", func(t *testing.T) {
		f := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}}}
		other := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 1}}}
		f.Merge(other)
		if len(f.Flows) != 2 {
			t.Fatalf("expected 2 flows, got %d", len(f.Flows))
		}
	})

	t.Run("overlapping", func(t *testing.T) {
		f := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}}}
		other := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}}}
		f.Merge(other)
		if len(f.Flows) != 1 {
			t.Fatalf("expected 1 flow, got %d", len(f.Flows))
		}
	})
}

// ---------------------------------------------------------------------------
// ParameterFlowFact – String
// ---------------------------------------------------------------------------

func TestParameterFlowFact_String(t *testing.T) {
	tests := []struct {
		name  string
		flows []ParameterFlowInfo
		want  string
	}{
		{"empty", nil, "[]"},
		{"single unwrapped", []ParameterFlowInfo{{ParamIndex: 2}}, "[2]"},
		{"single wrapped", []ParameterFlowInfo{{ParamIndex: 1, Wrapped: true}}, "[wrapped:1]"},
		{"multiple", []ParameterFlowInfo{{ParamIndex: 0}, {ParamIndex: 3, Wrapped: true}}, "[0, wrapped:3]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &ParameterFlowFact{Flows: tt.flows}
			if got := f.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// InterfaceMethodFact – AddError / AddErrors / String
// ---------------------------------------------------------------------------

func TestInterfaceMethodFact_AddError(t *testing.T) {
	t.Run("normal add", func(t *testing.T) {
		f := &InterfaceMethodFact{}
		f.AddError(ei("p", "A"))
		if len(f.Errors) != 1 {
			t.Fatalf("expected 1, got %d", len(f.Errors))
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		f := &InterfaceMethodFact{}
		f.AddError(ei("p", "A"))
		f.AddError(ei("p", "A"))
		if len(f.Errors) != 1 {
			t.Fatalf("expected 1, got %d", len(f.Errors))
		}
	})
}

func TestInterfaceMethodFact_AddErrors(t *testing.T) {
	t.Run("multiple", func(t *testing.T) {
		f := &InterfaceMethodFact{}
		f.AddErrors([]ErrorInfo{ei("p", "A"), ei("p", "B")})
		if len(f.Errors) != 2 {
			t.Fatalf("expected 2, got %d", len(f.Errors))
		}
	})

	t.Run("with duplicates", func(t *testing.T) {
		f := &InterfaceMethodFact{}
		f.AddErrors([]ErrorInfo{ei("p", "A"), ei("p", "A"), ei("p", "B")})
		if len(f.Errors) != 2 {
			t.Fatalf("expected 2, got %d", len(f.Errors))
		}
	})
}

func TestInterfaceMethodFact_String(t *testing.T) {
	tests := []struct {
		name   string
		errors []ErrorInfo
		want   string
	}{
		{"empty", nil, "[]"},
		{"single", []ErrorInfo{ei("p", "A")}, "[p.A]"},
		{"multiple", []ErrorInfo{ei("p", "A"), ei("q", "B")}, "[p.A, q.B]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &InterfaceMethodFact{Errors: tt.errors}
			if got := f.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInterfaceMethodFact_AFact(t *testing.T) {
	f := &InterfaceMethodFact{}
	f.AFact()
}

// ---------------------------------------------------------------------------
// FunctionParamCallFlowFact – AddCallFlow / Merge / String
// ---------------------------------------------------------------------------

func TestFunctionParamCallFlowFact_AddCallFlow(t *testing.T) {
	t.Run("new", func(t *testing.T) {
		f := &FunctionParamCallFlowFact{}
		f.AddCallFlow(FunctionParamCallFlowInfo{ParamIndex: 0})
		if len(f.CallFlows) != 1 {
			t.Fatalf("expected 1, got %d", len(f.CallFlows))
		}
	})

	t.Run("duplicate does not add", func(t *testing.T) {
		f := &FunctionParamCallFlowFact{}
		f.AddCallFlow(FunctionParamCallFlowInfo{ParamIndex: 0})
		f.AddCallFlow(FunctionParamCallFlowInfo{ParamIndex: 0})
		if len(f.CallFlows) != 1 {
			t.Fatalf("expected 1, got %d", len(f.CallFlows))
		}
	})

	t.Run("upgrade wrapped", func(t *testing.T) {
		f := &FunctionParamCallFlowFact{}
		f.AddCallFlow(FunctionParamCallFlowInfo{ParamIndex: 0, Wrapped: false})
		f.AddCallFlow(FunctionParamCallFlowInfo{ParamIndex: 0, Wrapped: true})
		// Unlike ParameterFlowFact, this uses index-based mutation so it persists.
		if !f.CallFlows[0].Wrapped {
			t.Error("expected Wrapped=true after upgrade")
		}
	})

	t.Run("no downgrade", func(t *testing.T) {
		f := &FunctionParamCallFlowFact{}
		f.AddCallFlow(FunctionParamCallFlowInfo{ParamIndex: 0, Wrapped: true})
		f.AddCallFlow(FunctionParamCallFlowInfo{ParamIndex: 0, Wrapped: false})
		if !f.CallFlows[0].Wrapped {
			t.Error("expected Wrapped=true, should not downgrade")
		}
	})
}

func TestFunctionParamCallFlowFact_Merge(t *testing.T) {
	t.Run("non-overlapping", func(t *testing.T) {
		f := &FunctionParamCallFlowFact{CallFlows: []FunctionParamCallFlowInfo{{ParamIndex: 0}}}
		other := &FunctionParamCallFlowFact{CallFlows: []FunctionParamCallFlowInfo{{ParamIndex: 1}}}
		f.Merge(other)
		if len(f.CallFlows) != 2 {
			t.Fatalf("expected 2, got %d", len(f.CallFlows))
		}
	})

	t.Run("overlapping", func(t *testing.T) {
		f := &FunctionParamCallFlowFact{CallFlows: []FunctionParamCallFlowInfo{{ParamIndex: 0}}}
		other := &FunctionParamCallFlowFact{CallFlows: []FunctionParamCallFlowInfo{{ParamIndex: 0, Wrapped: true}}}
		f.Merge(other)
		if len(f.CallFlows) != 1 {
			t.Fatalf("expected 1, got %d", len(f.CallFlows))
		}
		if !f.CallFlows[0].Wrapped {
			t.Error("expected merge to upgrade Wrapped to true")
		}
	})
}

func TestFunctionParamCallFlowFact_String(t *testing.T) {
	tests := []struct {
		name  string
		flows []FunctionParamCallFlowInfo
		want  string
	}{
		{"empty", nil, "[]"},
		{"single", []FunctionParamCallFlowInfo{{ParamIndex: 0}}, "[call:0]"},
		{"wrapped", []FunctionParamCallFlowInfo{{ParamIndex: 1, Wrapped: true}}, "[wrapped:call:1]"},
		{"multiple", []FunctionParamCallFlowInfo{{ParamIndex: 0}, {ParamIndex: 2, Wrapped: true}}, "[call:0, wrapped:call:2]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FunctionParamCallFlowFact{CallFlows: tt.flows}
			if got := f.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFunctionParamCallFlowFact_AFact(t *testing.T) {
	f := &FunctionParamCallFlowFact{}
	f.AFact()
}

// ---------------------------------------------------------------------------
// ParameterCheckedErrorsFact – AddCheck / GetCheckedErrors / String
// ---------------------------------------------------------------------------

func TestParameterCheckedErrorsFact_AddCheck(t *testing.T) {
	t.Run("single param", func(t *testing.T) {
		f := &ParameterCheckedErrorsFact{}
		f.AddCheck(0, ei("p", "A"))
		if len(f.Checks) != 1 {
			t.Fatalf("expected 1 check, got %d", len(f.Checks))
		}
		if len(f.Checks[0].CheckedErrors) != 1 {
			t.Fatalf("expected 1 checked error, got %d", len(f.Checks[0].CheckedErrors))
		}
	})

	t.Run("multiple params", func(t *testing.T) {
		f := &ParameterCheckedErrorsFact{}
		f.AddCheck(0, ei("p", "A"))
		f.AddCheck(1, ei("p", "B"))
		if len(f.Checks) != 2 {
			t.Fatalf("expected 2 checks, got %d", len(f.Checks))
		}
	})

	t.Run("multiple errors same param", func(t *testing.T) {
		f := &ParameterCheckedErrorsFact{}
		f.AddCheck(0, ei("p", "A"))
		f.AddCheck(0, ei("p", "B"))
		if len(f.Checks) != 1 {
			t.Fatalf("expected 1 check entry, got %d", len(f.Checks))
		}
		if len(f.Checks[0].CheckedErrors) != 2 {
			t.Fatalf("expected 2 checked errors, got %d", len(f.Checks[0].CheckedErrors))
		}
	})

	t.Run("duplicate error on same param", func(t *testing.T) {
		f := &ParameterCheckedErrorsFact{}
		f.AddCheck(0, ei("p", "A"))
		f.AddCheck(0, ei("p", "A"))
		if len(f.Checks[0].CheckedErrors) != 1 {
			t.Fatalf("expected 1 (deduped), got %d", len(f.Checks[0].CheckedErrors))
		}
	})
}

func TestParameterCheckedErrorsFact_GetCheckedErrors(t *testing.T) {
	f := &ParameterCheckedErrorsFact{}
	f.AddCheck(0, ei("p", "A"))
	f.AddCheck(0, ei("p", "B"))
	f.AddCheck(1, ei("p", "C"))

	t.Run("existing param", func(t *testing.T) {
		errs := f.GetCheckedErrors(0)
		if len(errs) != 2 {
			t.Fatalf("expected 2, got %d", len(errs))
		}
	})

	t.Run("non-existing param", func(t *testing.T) {
		errs := f.GetCheckedErrors(99)
		if errs != nil {
			t.Fatalf("expected nil, got %v", errs)
		}
	})
}

func TestParameterCheckedErrorsFact_String(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *ParameterCheckedErrorsFact
		want   string
	}{
		{
			"empty",
			func() *ParameterCheckedErrorsFact { return &ParameterCheckedErrorsFact{} },
			"[]",
		},
		{
			"single param single error",
			func() *ParameterCheckedErrorsFact {
				f := &ParameterCheckedErrorsFact{}
				f.AddCheck(0, ei("p", "A"))
				return f
			},
			"[param0:[p.A]]",
		},
		{
			"single param multiple errors",
			func() *ParameterCheckedErrorsFact {
				f := &ParameterCheckedErrorsFact{}
				f.AddCheck(1, ei("p", "A"))
				f.AddCheck(1, ei("q", "B"))
				return f
			},
			"[param1:[p.A,q.B]]",
		},
		{
			"multiple params",
			func() *ParameterCheckedErrorsFact {
				f := &ParameterCheckedErrorsFact{}
				f.AddCheck(0, ei("p", "A"))
				f.AddCheck(2, ei("q", "B"))
				return f
			},
			"[param0:[p.A], param2:[q.B]]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.setup()
			if got := f.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParameterCheckedErrorsFact_AFact(t *testing.T) {
	f := &ParameterCheckedErrorsFact{}
	f.AFact()
}

// ---------------------------------------------------------------------------
// IntersectParameterFlowFacts
// ---------------------------------------------------------------------------

func TestIntersectParameterFlowFacts(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		if got := IntersectParameterFlowFacts(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		if got := IntersectParameterFlowFacts([]*ParameterFlowFact{}); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("single fact", func(t *testing.T) {
		f := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}}}
		got := IntersectParameterFlowFacts([]*ParameterFlowFact{f})
		if got == nil || len(got.Flows) != 1 {
			t.Errorf("expected 1 flow, got %v", got)
		}
	})

	t.Run("multiple facts with intersection", func(t *testing.T) {
		f1 := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}, {ParamIndex: 1}}}
		f2 := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 1}, {ParamIndex: 2}}}
		got := IntersectParameterFlowFacts([]*ParameterFlowFact{f1, f2})
		if got == nil || len(got.Flows) != 1 {
			t.Fatalf("expected 1 flow, got %v", got)
		}
		if got.Flows[0].ParamIndex != 1 {
			t.Errorf("expected ParamIndex=1, got %d", got.Flows[0].ParamIndex)
		}
	})

	t.Run("no intersection", func(t *testing.T) {
		f1 := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}}}
		f2 := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 1}}}
		got := IntersectParameterFlowFacts([]*ParameterFlowFact{f1, f2})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("nil entry makes result nil", func(t *testing.T) {
		f1 := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}}}
		got := IntersectParameterFlowFacts([]*ParameterFlowFact{f1, nil})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty flows entry makes result nil", func(t *testing.T) {
		f1 := &ParameterFlowFact{Flows: []ParameterFlowInfo{{ParamIndex: 0}}}
		f2 := &ParameterFlowFact{}
		got := IntersectParameterFlowFacts([]*ParameterFlowFact{f1, f2})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// IntersectParameterCheckedErrorsFacts
// ---------------------------------------------------------------------------

func TestIntersectParameterCheckedErrorsFacts(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		if got := IntersectParameterCheckedErrorsFacts(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		if got := IntersectParameterCheckedErrorsFacts([]*ParameterCheckedErrorsFact{}); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("single fact", func(t *testing.T) {
		f := &ParameterCheckedErrorsFact{}
		f.AddCheck(0, ei("p", "A"))
		got := IntersectParameterCheckedErrorsFacts([]*ParameterCheckedErrorsFact{f})
		if got == nil || len(got.Checks) != 1 {
			t.Fatalf("expected 1 check, got %v", got)
		}
	})

	t.Run("multiple facts with intersection", func(t *testing.T) {
		f1 := &ParameterCheckedErrorsFact{}
		f1.AddCheck(0, ei("p", "A"))
		f1.AddCheck(0, ei("p", "B"))

		f2 := &ParameterCheckedErrorsFact{}
		f2.AddCheck(0, ei("p", "A"))
		f2.AddCheck(0, ei("p", "C"))

		got := IntersectParameterCheckedErrorsFacts([]*ParameterCheckedErrorsFact{f1, f2})
		if got == nil {
			t.Fatal("expected non-nil")
		}
		errs := got.GetCheckedErrors(0)
		if len(errs) != 1 {
			t.Fatalf("expected 1 intersected error, got %d", len(errs))
		}
		if errs[0].Key() != "p.A" {
			t.Errorf("expected p.A, got %s", errs[0].Key())
		}
	})

	t.Run("no intersection", func(t *testing.T) {
		f1 := &ParameterCheckedErrorsFact{}
		f1.AddCheck(0, ei("p", "A"))

		f2 := &ParameterCheckedErrorsFact{}
		f2.AddCheck(0, ei("p", "B"))

		got := IntersectParameterCheckedErrorsFacts([]*ParameterCheckedErrorsFact{f1, f2})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("nil entry", func(t *testing.T) {
		f1 := &ParameterCheckedErrorsFact{}
		f1.AddCheck(0, ei("p", "A"))
		got := IntersectParameterCheckedErrorsFacts([]*ParameterCheckedErrorsFact{f1, nil})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty checks entry", func(t *testing.T) {
		f1 := &ParameterCheckedErrorsFact{}
		f1.AddCheck(0, ei("p", "A"))
		f2 := &ParameterCheckedErrorsFact{}
		got := IntersectParameterCheckedErrorsFacts([]*ParameterCheckedErrorsFact{f1, f2})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("different param indices no overlap", func(t *testing.T) {
		f1 := &ParameterCheckedErrorsFact{}
		f1.AddCheck(0, ei("p", "A"))

		f2 := &ParameterCheckedErrorsFact{}
		f2.AddCheck(1, ei("p", "A"))

		got := IntersectParameterCheckedErrorsFacts([]*ParameterCheckedErrorsFact{f1, f2})
		// f1 checks param 0 with A, f2 doesn't check param 0 at all -> intersection empty
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// ContainsErrorInfo
// ---------------------------------------------------------------------------

func TestContainsErrorInfo(t *testing.T) {
	infos := []ErrorInfo{ei("p", "A"), ei("q", "B")}

	t.Run("found", func(t *testing.T) {
		if !ContainsErrorInfo(infos, ei("p", "A")) {
			t.Error("expected true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		if ContainsErrorInfo(infos, ei("p", "C")) {
			t.Error("expected false")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		if ContainsErrorInfo(nil, ei("p", "A")) {
			t.Error("expected false for nil slice")
		}
	})

	t.Run("match by key ignoring wrapped", func(t *testing.T) {
		if !ContainsErrorInfo(infos, eiw("p", "A")) {
			t.Error("expected true: key match should suffice")
		}
	})
}
