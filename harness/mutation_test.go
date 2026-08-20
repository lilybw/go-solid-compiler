package harness_test

import (
	"strings"
	"testing"

	"github.com/lilybw/go-solid-compiler/harness"
)

// Mutation testing for the comparator.
//
// The harness is what decides whether the compiler is correct, so "the harness
// passes" is only meaningful if the harness would have failed on a real bug.
// These tests inject each class of defect the transform can plausibly have and
// assert it is caught — and, just as importantly, assert that output which is
// merely spelled differently is not.
//
// A comparator that reports agreement too readily is worse than no comparator,
// because it turns an open question into a false answer.

const referenceModule = `import { insert as _$insert, template as _$template, effect as _$effect, className as _$className, delegateEvents as _$delegateEvents } from "solid-js/web";
const _tmpl$ = /*#__PURE__*/_$template(` + "`<div><span>a</span><b></b></div>`" + `);
const C = () => {
  const _el$ = _tmpl$(), _el$2 = _el$.firstChild, _el$3 = _el$2.nextSibling;
  _$insert(_el$3, title);
  _$effect(() => _$className(_el$, cls()));
  _el$.$$click = h;
  return _el$;
};
_$delegateEvents(["click"]);`

func TestMutationsAreDetected(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(string) string
		wantKind string
	}{
		{
			// The signature silent failure: the DOM builds correctly and then
			// never updates, because nothing re-runs when state changes.
			name:     "dropped effect wrapper",
			mutate:   sub("_$effect(() => _$className(_el$, cls()))", "_$className(_el$, cls())"),
			wantKind: "body",
		},
		{
			// A disagreement about what counts as static structure.
			name:     "wrong template structure",
			mutate:   sub("<b></b>", "<i></i>"),
			wantKind: "template",
		},
		{
			// Content lands on the wrong node, or in the wrong position.
			name:     "off-by-one navigation",
			mutate:   sub("_el$3 = _el$2.nextSibling", "_el$3 = _el$2.nextSibling.nextSibling"),
			wantKind: "body",
		},
		{
			name:     "insert on the wrong element",
			mutate:   sub("_$insert(_el$3, title)", "_$insert(_el$2, title)"),
			wantKind: "body",
		},
		{
			// A module that throws on load. This bug shipped once already.
			name:     "helper used but not imported",
			mutate:   sub("template as _$template, ", ""),
			wantKind: "helpers",
		},
		{
			// Handlers attach but never fire, because nothing listens.
			name:     "lost event delegation",
			mutate:   sub(`_$delegateEvents(["click"]);`, ""),
			wantKind: "events",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := tc.mutate(referenceModule)
			if mutated == referenceModule {
				t.Fatal("mutation did not change the input; the test is not testing anything")
			}
			rep := harness.Compare(tc.name, referenceModule, mutated)
			if rep.OK() {
				t.Fatalf("mutation went undetected:\n%s", mutated)
			}
			var kinds []string
			for _, m := range rep.Mismatches {
				kinds = append(kinds, m.Kind)
			}
			found := false
			for _, k := range kinds {
				if k == tc.wantKind {
					found = true
				}
			}
			if !found {
				t.Errorf("reported %v, expected a %q mismatch", kinds, tc.wantKind)
			}
		})
	}
}

func TestEquivalentSpellingsAreAccepted(t *testing.T) {
	// Everything that differs here is spelling: import order, generated
	// variable numbering, which intermediate nodes get bound, and accessor
	// form. None of it changes behaviour, and flagging any of it would make
	// the harness too noisy to be read.
	equivalent := `import { template as _$template, effect as _$effect, insert as _$insert, className as _$className, delegateEvents as _$delegateEvents } from "solid-js/web";
const _tmpl$ = /*#__PURE__*/_$template(` + "`<div><span>a</span><b></b></div>`" + `);
const C = () => {
  const _el$9 = _tmpl$(),
    _el$8 = _el$9.firstChild.nextSibling;
  _$insert(_el$8, () => title());
  _$effect(() => _$className(_el$9, cls()));
  _el$9.$$click = h;
  return _el$9;
};
_$delegateEvents(["click"]);`

	if rep := harness.Compare("equivalent", referenceModule, equivalent); !rep.OK() {
		t.Errorf("equivalent output was reported as different:\n%s", rep)
	}
}

func TestIdenticalInputAgrees(t *testing.T) {
	if rep := harness.Compare("identical", referenceModule, referenceModule); !rep.OK() {
		t.Errorf("a module disagreed with itself:\n%s", rep)
	}
}

// sub returns a mutation replacing every occurrence of old with new.
func sub(old, new string) func(string) string {
	return func(s string) string { return strings.ReplaceAll(s, old, new) }
}
