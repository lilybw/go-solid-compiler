// Package differential runs the differential harness over the fixture corpus.
//
// It lives in its own package because it is the only test that depends on the
// TypeScript compiler frontend. Keeping it separate means `go test ./harness`
// — which exercises the comparison logic itself — runs with no external
// dependency at all.
//
// # How this is meant to be used
//
// Two jobs, doing different work:
//
//	go test ./harness/...
//	    Compiles the corpus with this compiler and compares against the
//	    committed goldens. Needs no Node. This is the check that runs on every
//	    commit and gates merges.
//
//	npm --prefix harness/babel ci && npm --prefix harness/babel run record
//	    Re-derives the goldens from babel-preset-solid. Run this when adding a
//	    fixture, and on a schedule in CI with a `git diff --exit-code` after,
//	    so that an upstream change to the reference implementation surfaces as
//	    a failing build rather than as a silently stale definition of correct.
//
// The distinction matters: the first job asks "did we regress against what
// babel did", the second asks "did babel change".
package differential

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lilybw/go-solid-compiler/harness"
	"github.com/lilybw/go-solid-compiler/solid"
	"github.com/lilybw/go-solid-compiler/tsx"
)

const (
	corpusDir   = "../testdata/corpus"
	expectedDir = "../testdata/expected"
)

// knownDivergences records output that differs from the reference for reasons
// that are understood, accepted, and tracked.
//
// A gate nobody can pass is a gate everybody learns to ignore, so an
// unimplemented optimization must not leave the suite permanently red. But
// silently tolerating everything would defeat the harness, so this is narrow
// in two directions: only the listed mismatch kinds are excused for the listed
// fixtures, and a fixture that stops diverging fails too. The second half is
// what keeps the list from rotting - implement the optimization and the test
// tells you to delete the entry.
var knownDivergences = map[string]map[string]string{
	"attributes.tsx": {
		"body": "batched attribute effects: the reference folds every dynamic " +
			"attribute on a template into one effect with previous-value " +
			"change detection; this compiler emits one effect per attribute, " +
			"which is correct but does more work",
		"helpers": "the reference expands a literal classList object into " +
			"individual classList.toggle calls and so never imports the helper",
	},
	"namespaces.tsx": {
		"body": "batched attribute effects, as above",
	},
}

// TestDifferential compiles every fixture and compares against babel's output.
func TestDifferential(t *testing.T) {
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatalf("reading corpus: %v", err)
	}

	var ran int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".tsx") {
			continue
		}
		ran++

		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(corpusDir, name))
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}

			goldenPath := filepath.Join(expectedDir,
				strings.TrimSuffix(name, ".tsx")+".js")
			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Skipf("no golden output at %s; run the babel recorder", goldenPath)
			}

			file, perr := tsx.Parse(name, string(src), tsx.TSX)
			if perr != nil {
				t.Fatalf("parsing fixture: %v", perr)
			}
			actual, terr := tsx.TransformSolid(file, solid.Options{})
			if terr != nil {
				t.Fatalf("transform: %v", terr)
			}

			rep := harness.Compare(name, string(golden), actual)
			known := knownDivergences[name]

			var unexpected []harness.Mismatch
			seen := map[string]bool{}
			for _, m := range rep.Mismatches {
				if reason, ok := known[m.Kind]; ok {
					seen[m.Kind] = true
					t.Logf("known divergence [%s]: %s", m.Kind, reason)
					continue
				}
				unexpected = append(unexpected, m)
			}

			if len(unexpected) > 0 {
				var b strings.Builder
				fmt.Fprintf(&b, "%s: %d unexpected mismatch(es)\n", name, len(unexpected))
				for _, m := range unexpected {
					fmt.Fprintf(&b, "  [%s] %s\n    babel: %s\n    ours:  %s\n",
						m.Kind, m.Detail, m.Reference, m.Actual)
				}
				t.Errorf("%s\n--- our full output ---\n%s", b.String(), actual)
			}

			// A divergence that has been fixed should be removed from the list,
			// or it will quietly excuse a future regression.
			for kind := range known {
				if !seen[kind] {
					t.Errorf("%s no longer diverges on [%s]; "+
						"remove it from knownDivergences", name, kind)
				}
			}
		})
	}

	if ran == 0 {
		t.Fatal("no fixtures found; the corpus should not be empty")
	}
}

// TestGoldensExistForEveryFixture keeps the corpus and the goldens in step.
//
// Without this, adding a fixture and forgetting to record it would show up as
// a skipped subtest, which is easy to miss in a passing build.
func TestGoldensExistForEveryFixture(t *testing.T) {
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatalf("reading corpus: %v", err)
	}
	var missing []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tsx") {
			continue
		}
		golden := filepath.Join(expectedDir,
			strings.TrimSuffix(e.Name(), ".tsx")+".js")
		if _, err := os.Stat(golden); err != nil {
			missing = append(missing, e.Name())
		}
	}
	if len(missing) > 0 {
		t.Errorf("fixtures without recorded output: %s\n"+
			"run: npm --prefix harness/babel ci && npm --prefix harness/babel run record",
			strings.Join(missing, ", "))
	}
}

// TestOutputIsDeterministic guards against map iteration leaking into
// generated code, which would make the goldens flap and destroy the harness's
// usefulness.
func TestOutputIsDeterministic(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(corpusDir, "attributes.tsx"))
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	var first string
	for i := 0; i < 25; i++ {
		file, perr := tsx.Parse("attributes.tsx", string(src), tsx.TSX)
		if perr != nil {
			t.Fatalf("parse: %v", perr)
		}
		out, terr := tsx.TransformSolid(file, solid.Options{})
		if terr != nil {
			t.Fatalf("transform: %v", terr)
		}
		if i == 0 {
			first = out
			continue
		}
		if out != first {
			t.Fatalf("output is not deterministic across runs:\n--- a ---\n%s\n--- b ---\n%s",
				first, out)
		}
	}
}
