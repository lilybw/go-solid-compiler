package tsx

import (
	"strings"
	"testing"
)

// The parser asserts that a file name is rooted and normalized, and panics
// rather than fixing one up:
//
//	panic: fileName should be normalized and absolute: "attributes.tsx"
//
// That precondition is invisible from the outside, so it is worth pinning
// down: passing a bare name is the obvious thing for a caller to do.
func TestNormalizeFileName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare name is rooted", "Button.tsx", "/Button.tsx"},
		{"relative path is rooted", "components/Button.tsx", "/components/Button.tsx"},
		{"absolute path is preserved", "/src/Button.tsx", "/src/Button.tsx"},
		{"dot segments are removed", "components/../Button.tsx", "/Button.tsx"},
		{"leading dot slash is removed", "./Button.tsx", "/Button.tsx"},
		{"empty name gets a placeholder", "", "/input.tsx"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeFileName(tc.in); got != tc.want {
				t.Errorf("normalizeFileName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseAcceptsRelativeNames is the regression guard for the panic itself:
// the unit test above checks the helper, this checks that Parse actually uses
// it on the path the caller reaches.
func TestParseAcceptsRelativeNames(t *testing.T) {
	for _, name := range []string{"a.tsx", "nested/dir/b.tsx", "./c.tsx"} {
		t.Run(name, func(t *testing.T) {
			file, err := Parse(name, "export const X = 1;\n", TS)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if file == nil {
				t.Fatal("no source file returned")
			}
		})
	}
}

func TestParseTSX(t *testing.T) {
	src := `export const C = () => <div class="card">{title()}</div>;` + "\n"
	file, err := Parse("C.tsx", src, TSX)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Statements == nil || len(file.Statements.Nodes) == 0 {
		t.Fatal("no statements parsed")
	}
	if got := file.Text(); got != src {
		t.Errorf("source text not preserved:\n got %q\nwant %q", got, src)
	}
}

// TestParseReportsSyntaxErrors checks that diagnostics surface, and that they
// name the file the caller passed rather than the rooted form the parser
// required.
func TestParseReportsSyntaxErrors(t *testing.T) {
	_, err := Parse("bad.ts", "export interface A { b: ", TS)
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	if !strings.Contains(err.Error(), "bad.ts") {
		t.Errorf("diagnostic should name the caller's file, got: %v", err)
	}
	if strings.Contains(err.Error(), "/bad.ts") {
		t.Errorf("diagnostic leaked the synthetic root: %v", err)
	}
}

func TestScriptKindOf(t *testing.T) {
	tests := []struct {
		file string
		want ScriptKind
	}{
		{"a.ts", TS},
		{"a.tsx", TSX},
		{"a.d.ts", DTS},
		{"a.js", JS},
		{"a.jsx", JSX},
		{"a.mjs", JS},
		{"noext", TS},
	}
	for _, tc := range tests {
		if got := ScriptKindOf(tc.file); got != tc.want {
			t.Errorf("ScriptKindOf(%q) = %v, want %v", tc.file, got, tc.want)
		}
	}
}

// TestTSXvsTSDisambiguation covers the reason ScriptKind exists at all: in a
// .ts file a leading angle bracket is a type assertion, and in a .tsx file it
// opens an element. The same text has to parse differently.
func TestTSXvsTSDisambiguation(t *testing.T) {
	const src = "const x = <div>hello</div>;\n"

	if _, err := Parse("a.tsx", src, TSX); err != nil {
		t.Errorf("valid TSX rejected: %v", err)
	}
	if _, err := Parse("a.ts", src, TS); err == nil {
		t.Error("JSX in a .ts file should not parse as an element")
	}
}
