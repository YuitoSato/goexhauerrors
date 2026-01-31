package detector_test

import (
	"go/types"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/YuitoSato/goexhauerrors/goexhauerrors/detector"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/facts"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

func testdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestDetectLocalErrors_Sentinel(t *testing.T) {
	var result *detector.LocalErrors

	testAnalyzer := &analysis.Analyzer{
		Name:     "test_sentinel",
		Doc:      "test sentinel detection",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (interface{}, error) {
			result = detector.DetectLocalErrors(pass)

			// Collect detected var names
			varNames := make(map[string]bool)
			for v := range result.Vars {
				varNames[v.Name()] = true
			}

			// ErrNotFound: errors.New → detected
			if !varNames["ErrNotFound"] {
				t.Error("expected ErrNotFound to be detected as sentinel var")
			}

			// ErrTimeout: fmt.Errorf without %w → detected
			if !varNames["ErrTimeout"] {
				t.Error("expected ErrTimeout to be detected as sentinel var")
			}

			// errPrivate: errors.New but unexported → detected in Vars
			if !varNames["errPrivate"] {
				t.Error("expected errPrivate to be detected as sentinel var")
			}

			// someErr: errors.New → detected (it is a sentinel init)
			if !varNames["someErr"] {
				t.Error("expected someErr to be detected as sentinel var")
			}

			// NotAnError: string type → NOT detected
			if varNames["NotAnError"] {
				t.Error("NotAnError should not be detected as sentinel var")
			}

			// ErrWrapped: fmt.Errorf with %w → NOT detected
			if varNames["ErrWrapped"] {
				t.Error("ErrWrapped should not be detected (uses %w)")
			}

			// Verify facts: exported vars should have facts, unexported should not
			for v := range result.Vars {
				var fact facts.ErrorFact
				hasFact := pass.ImportObjectFact(v, &fact)
				if v.Exported() && !hasFact {
					t.Errorf("expected ErrorFact to be exported for %s", v.Name())
				}
				if !v.Exported() && hasFact {
					t.Errorf("did not expect ErrorFact for unexported %s", v.Name())
				}
			}

			return nil, nil
		},
	}

	analysistest.Run(t, testdataDir(), testAnalyzer, "sentinel")
}

func TestDetectLocalErrors_CustomType(t *testing.T) {
	var result *detector.LocalErrors

	testAnalyzer := &analysis.Analyzer{
		Name:     "test_customtype",
		Doc:      "test custom error type detection",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (interface{}, error) {
			result = detector.DetectLocalErrors(pass)

			// Collect detected type names
			typeNames := make(map[string]bool)
			for tn := range result.Types {
				typeNames[tn.Name()] = true
			}

			// ValidationError: value receiver Error() → detected
			if !typeNames["ValidationError"] {
				t.Error("expected ValidationError to be detected as custom error type")
			}

			// privateError: implements error but unexported → detected in Types
			if !typeNames["privateError"] {
				t.Error("expected privateError to be detected as custom error type")
			}

			// NotError: no Error() method → NOT detected
			if typeNames["NotError"] {
				t.Error("NotError should not be detected as custom error type")
			}

			// PtrError: pointer receiver Error() → detected
			if typeNames["PtrError"] {
				// good
			} else {
				t.Error("expected PtrError to be detected as custom error type")
			}

			// Verify facts: exported types should have facts, unexported should not
			for tn := range result.Types {
				var fact facts.ErrorFact
				hasFact := pass.ImportObjectFact(tn, &fact)
				if tn.Exported() && !hasFact {
					t.Errorf("expected ErrorFact to be exported for %s", tn.Name())
				}
				if !tn.Exported() && hasFact {
					t.Errorf("did not expect ErrorFact for unexported %s", tn.Name())
				}
			}

			return nil, nil
		},
	}

	analysistest.Run(t, testdataDir(), testAnalyzer, "customtype")
}

func TestDetectLocalErrors_Empty(t *testing.T) {
	testAnalyzer := &analysis.Analyzer{
		Name:     "test_empty",
		Doc:      "test empty package",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (interface{}, error) {
			result := detector.DetectLocalErrors(pass)

			if len(result.Vars) != 0 {
				t.Errorf("expected no vars, got %d", len(result.Vars))
			}
			if len(result.Types) != 0 {
				t.Errorf("expected no types, got %d", len(result.Types))
			}

			return nil, nil
		},
	}

	analysistest.Run(t, testdataDir(), testAnalyzer, "empty")
}

func TestDetectLocalErrors_SentinelFactContent(t *testing.T) {
	testAnalyzer := &analysis.Analyzer{
		Name:     "test_sentinel_facts",
		Doc:      "test sentinel fact content",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (interface{}, error) {
			detector.DetectLocalErrors(pass)

			// Find ErrNotFound and verify fact content
			scope := pass.Pkg.Scope()
			obj := scope.Lookup("ErrNotFound")
			if obj == nil {
				t.Fatal("ErrNotFound not found in scope")
			}

			var fact facts.ErrorFact
			if !pass.ImportObjectFact(obj, &fact) {
				t.Fatal("no ErrorFact for ErrNotFound")
			}
			if fact.Name != "ErrNotFound" {
				t.Errorf("expected fact.Name=ErrNotFound, got %s", fact.Name)
			}
			if fact.PkgPath != "sentinel" {
				t.Errorf("expected fact.PkgPath=sentinel, got %s", fact.PkgPath)
			}

			return nil, nil
		},
	}

	analysistest.Run(t, testdataDir(), testAnalyzer, "sentinel")
}

func TestDetectLocalErrors_CustomTypeFactContent(t *testing.T) {
	testAnalyzer := &analysis.Analyzer{
		Name:     "test_customtype_facts",
		Doc:      "test custom type fact content",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (interface{}, error) {
			detector.DetectLocalErrors(pass)

			scope := pass.Pkg.Scope()

			// Verify ValidationError fact
			obj := scope.Lookup("ValidationError")
			if obj == nil {
				t.Fatal("ValidationError not found in scope")
			}
			var fact facts.ErrorFact
			if !pass.ImportObjectFact(obj, &fact) {
				t.Fatal("no ErrorFact for ValidationError")
			}
			if fact.Name != "ValidationError" {
				t.Errorf("expected fact.Name=ValidationError, got %s", fact.Name)
			}
			if fact.PkgPath != "customtype" {
				t.Errorf("expected fact.PkgPath=customtype, got %s", fact.PkgPath)
			}

			// Verify PtrError fact
			obj = scope.Lookup("PtrError")
			if obj == nil {
				t.Fatal("PtrError not found in scope")
			}
			var fact2 facts.ErrorFact
			if !pass.ImportObjectFact(obj, &fact2) {
				t.Fatal("no ErrorFact for PtrError")
			}
			if fact2.Name != "PtrError" {
				t.Errorf("expected fact.Name=PtrError, got %s", fact2.Name)
			}

			// Verify privateError has NO fact
			obj = scope.Lookup("privateError")
			if obj == nil {
				t.Fatal("privateError not found in scope")
			}
			var fact3 facts.ErrorFact
			if pass.ImportObjectFact(obj, &fact3) {
				t.Error("privateError should not have an exported ErrorFact")
			}

			return nil, nil
		},
	}

	analysistest.Run(t, testdataDir(), testAnalyzer, "customtype")
}

func TestDetectLocalErrors_SentinelVarCount(t *testing.T) {
	testAnalyzer := &analysis.Analyzer{
		Name:     "test_sentinel_count",
		Doc:      "test sentinel var count",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (interface{}, error) {
			result := detector.DetectLocalErrors(pass)

			// Expected sentinels: ErrNotFound, ErrTimeout, errPrivate, someErr
			// NOT: NotAnError (string), ErrWrapped (%w)
			expectedCount := 4
			if len(result.Vars) != expectedCount {
				names := make([]string, 0, len(result.Vars))
				for v := range result.Vars {
					names = append(names, v.Name())
				}
				sort.Strings(names)
				t.Errorf("expected %d sentinel vars, got %d: %v", expectedCount, len(result.Vars), names)
			}

			return nil, nil
		},
	}

	analysistest.Run(t, testdataDir(), testAnalyzer, "sentinel")
}

func TestDetectLocalErrors_CustomTypeCount(t *testing.T) {
	testAnalyzer := &analysis.Analyzer{
		Name:     "test_customtype_count",
		Doc:      "test custom type count",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (interface{}, error) {
			result := detector.DetectLocalErrors(pass)

			// Expected: ValidationError, privateError, PtrError
			// NOT: NotError
			expectedCount := 3
			if len(result.Types) != expectedCount {
				names := make([]string, 0, len(result.Types))
				for tn := range result.Types {
					names = append(names, tn.Name())
				}
				sort.Strings(names)
				t.Errorf("expected %d custom error types, got %d: %v", expectedCount, len(result.Types), names)
			}

			// Verify no types are detected as vars
			if len(result.Vars) != 0 {
				t.Errorf("expected no sentinel vars in customtype package, got %d", len(result.Vars))
			}

			return nil, nil
		},
	}

	analysistest.Run(t, testdataDir(), testAnalyzer, "customtype")
}

// Ensure types.Var and types.TypeName are used (avoid import errors).
var _ *types.Var
var _ *types.TypeName
