package harness_test

import (
	"strings"
	"testing"

	"github.com/lilybw/go-solid-compiler/harness"
)

// The harness is the thing that decides whether the compiler is correct, so it
// needs its own tests: a comparator that reports false agreement is worse than
// no comparator, because it converts an unknown into a wrong answer.

func TestExtractTemplates(t *testing.T) {
	src := "const _tmpl$ = /*#__PURE__*/_$template(`<div>a</div>`);\n" +
		"const _tmpl$2 = /*#__PURE__*/_$template(`<span></span>`);\n" +
		"const x = 1;\n"
	out := harness.Extract(src)

	if len(out.Templates) != 2 {
		t.Fatalf("got %d templates: %#v", len(out.Templates), out.Templates)
	}
	if out.Templates[0] != "<div>a</div>" || out.Templates[1] != "<span></span>" {
		t.Errorf("wrong templates: %#v", out.Templates)
	}
	if !strings.Contains(out.Body, "const x=1") {
		t.Errorf("body should retain other code, got %q", out.Body)
	}
	if strings.Contains(out.Body, "_$template") {
		t.Errorf("template declarations should be removed, got %q", out.Body)
	}
}

func TestExtractTemplateWithEscapes(t *testing.T) {
	// Escaping conventions differ between compilers, so extraction unescapes
	// before comparing.
	src := "const _tmpl$ = _$template(`<div>a\\`b \\${c} $5</div>`);"
	out := harness.Extract(src)
	if len(out.Templates) != 1 {
		t.Fatalf("got %#v", out.Templates)
	}
	if want := "<div>a`b ${c} $5</div>"; out.Templates[0] != want {
		t.Errorf("got %q, want %q", out.Templates[0], want)
	}
}

func TestExtractHelpers(t *testing.T) {
	src := `import { insert as _$insert, template as _$template } from "solid-js/web";
const a = 1;`
	out := harness.Extract(src)

	if strings.Join(out.Helpers, ",") != "insert,template" {
		t.Errorf("got %#v", out.Helpers)
	}
	if strings.Contains(out.Body, "import") {
		t.Errorf("import should be removed, got %q", out.Body)
	}
}

func TestExtractHelpersIgnoresUnrelatedImports(t *testing.T) {
	src := `import { createSignal } from "solid-js";
import foo from "./other";
const a = 1;`
	out := harness.Extract(src)
	// solid-js imports are helper imports; unrelated modules stay in the body.
	if len(out.Helpers) != 1 || out.Helpers[0] != "createSignal" {
		t.Errorf("got %#v", out.Helpers)
	}
	if !strings.Contains(out.Body, "./other") {
		t.Errorf("unrelated import should remain in body, got %q", out.Body)
	}
}

func TestExtractDelegatedEvents(t *testing.T) {
	out := harness.Extract(`_$delegateEvents(["click", "input"]);`)
	if strings.Join(out.Delegated, ",") != "click,input" {
		t.Errorf("got %#v", out.Delegated)
	}
	if strings.TrimSpace(out.Body) != "" {
		t.Errorf("body should be empty, got %q", out.Body)
	}
}

func TestNormalizeAlphaRenamesGeneratedVariables(t *testing.T) {
	// Different variable numbering must not register as a difference.
	a := harness.NormalizeBody("const _el$ = t(), _el$2 = _el$.firstChild; f(_el$2);")
	b := harness.NormalizeBody("const _el$7 = t(), _el$9 = _el$7.firstChild; f(_el$9);")
	if a != b {
		t.Errorf("renaming should normalize:\n a=%s\n b=%s", a, b)
	}
}

func TestNormalizeDistinguishesDifferentStructure(t *testing.T) {
	// A genuinely different navigation path must survive canonicalization.
	// The variable has to be referenced: an unused binding addresses no node,
	// so erasing it loses nothing.
	a := harness.NormalizeBody("const _el$2 = _el$.firstChild; f(_el$2);")
	b := harness.NormalizeBody("const _el$2 = _el$.firstChild.nextSibling; f(_el$2);")
	if a == b {
		t.Errorf("different navigation should not normalize alike:\n a=%s\n b=%s", a, b)
	}
}

// TestNormalizeAcceptsDifferentBindingStrategies locks in the behaviour that
// made canonicalization necessary: the two compilers disagree about which
// intermediate nodes deserve a variable, while addressing the same node.
func TestNormalizeAcceptsDifferentBindingStrategies(t *testing.T) {
	// babel binds each node it walks past.
	viaSteps := harness.NormalizeBody(
		"const _el$ = t(), _el$2 = _el$.firstChild, _el$3 = _el$2.nextSibling; f(_el$3);")
	// this compiler chains straight to the node it needs.
	viaChain := harness.NormalizeBody(
		"const _el$ = t(), _el$2 = _el$.firstChild.nextSibling; f(_el$2);")

	if viaSteps != viaChain {
		t.Errorf("equivalent addressing should normalize alike:\n steps=%s\n chain=%s",
			viaSteps, viaChain)
	}
}

// TestNormalizeCatchesOffByOneNavigation is the counterpart: canonicalization
// must not be so aggressive that a wrong path looks right.
func TestNormalizeCatchesOffByOneNavigation(t *testing.T) {
	right := harness.NormalizeBody(
		"const _el$ = t(), _el$2 = _el$.firstChild, _el$3 = _el$2.nextSibling; f(_el$3);")
	wrong := harness.NormalizeBody(
		"const _el$ = t(), _el$2 = _el$.firstChild, _el$3 = _el$2.nextSibling.nextSibling; f(_el$3);")

	if right == wrong {
		t.Error("an extra nextSibling must remain visible")
	}
}

func TestNormalizeCollapsesArrowCallToIdentifier(t *testing.T) {
	// babel passes the identifier; this compiler passes an equivalent arrow.
	a := harness.NormalizeBody("_$insert(_el$, () => title());")
	b := harness.NormalizeBody("_$insert(_el$, title);")
	if a != b {
		t.Errorf("known-equivalent forms should normalize:\n a=%s\n b=%s", a, b)
	}
}

func TestNormalizeDoesNotCollapseArrowWithArguments(t *testing.T) {
	// Only a zero-argument call is equivalent; anything else is a real
	// difference and must survive normalization.
	a := harness.NormalizeBody("_$insert(_el$, () => title(x));")
	b := harness.NormalizeBody("_$insert(_el$, title);")
	if a == b {
		t.Error("a call with arguments must not collapse")
	}
}

func TestNormalizeStripsCommentsAndWhitespace(t *testing.T) {
	a := harness.NormalizeBody("const x = /*#__PURE__*/ f( 1 , 2 ); // trailing")
	b := harness.NormalizeBody("const x=f(1,2);")
	if a != b {
		t.Errorf("\n a=%s\n b=%s", a, b)
	}
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

const refModule = `import { insert as _$insert, template as _$template, delegateEvents as _$delegateEvents } from "solid-js/web";
const _tmpl$ = /*#__PURE__*/_$template(` + "`<div><h1></h1></div>`" + `);
const C = () => {
  const _el$ = _tmpl$(), _el$2 = _el$.firstChild;
  _$insert(_el$2, title);
  _el$.$$click = h;
  return _el$;
};
_$delegateEvents(["click"]);`

func TestCompareAcceptsEquivalentOutput(t *testing.T) {
	// Same semantics, different spelling throughout: variable numbering,
	// import order, and the arrow-versus-identifier accessor.
	ours := `import { template as _$template, delegateEvents as _$delegateEvents, insert as _$insert } from "solid-js/web";
const _tmpl$ = /*#__PURE__*/_$template(` + "`<div><h1></h1></div>`" + `);
const C = () => {
  const _el$4 = _tmpl$(),
    _el$5 = _el$4.firstChild;
  _$insert(_el$5, () => title());
  _el$4.$$click = h;
  return _el$4;
};
_$delegateEvents(["click"]);`

	if rep := harness.Compare("t", refModule, ours); !rep.OK() {
		t.Errorf("equivalent output reported as different:\n%s", rep)
	}
}

func TestCompareDetectsTemplateDifference(t *testing.T) {
	ours := strings.Replace(refModule, "<h1></h1>", "<h2></h2>", 1)
	rep := harness.Compare("t", refModule, ours)
	if rep.OK() {
		t.Fatal("template difference not detected")
	}
	if rep.Mismatches[0].Kind != "template" {
		t.Errorf("wrong category: %s", rep.Mismatches[0].Kind)
	}
}

func TestCompareDetectsMissingHelper(t *testing.T) {
	// The exact bug that shipped earlier: a helper used but never imported.
	ours := strings.Replace(refModule, ", template as _$template", "", 1)
	rep := harness.Compare("t", refModule, ours)
	if rep.OK() {
		t.Fatal("missing helper not detected")
	}
	found := false
	for _, m := range rep.Mismatches {
		if m.Kind == "helpers" && strings.Contains(m.Detail, "template") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing helper not reported:\n%s", rep)
	}
}

func TestCompareDetectsMissingDelegatedEvent(t *testing.T) {
	ours := strings.Replace(refModule, `_$delegateEvents(["click"]);`, "", 1)
	rep := harness.Compare("t", refModule, ours)
	if rep.OK() {
		t.Fatal("missing delegation not detected")
	}
}

func TestCompareDetectsMissingEffectWrapper(t *testing.T) {
	// The silent failure this whole harness exists to catch: the DOM is built
	// correctly but never updates.
	ref := `const C = () => { _$effect(() => _$className(_el$, cls())); };`
	ours := `const C = () => { _$className(_el$, cls()); };`
	if rep := harness.Compare("t", ref, ours); rep.OK() {
		t.Error("a dropped effect wrapper must be detected")
	}
}

func TestCompareDetectsWrongInsertionMarker(t *testing.T) {
	ref := `const C = () => { _$insert(_el$, x, _el$2); };`
	ours := `const C = () => { _$insert(_el$, x, null); };`
	if rep := harness.Compare("t", ref, ours); rep.OK() {
		t.Error("a wrong insertion marker must be detected")
	}
}

func TestReportStringIsReadable(t *testing.T) {
	rep := harness.Compare("fixture.tsx", refModule,
		strings.Replace(refModule, "<h1></h1>", "<h2></h2>", 1))
	s := rep.String()
	if !strings.Contains(s, "fixture.tsx") || !strings.Contains(s, "[template]") {
		t.Errorf("report is not informative:\n%s", s)
	}
	if !strings.Contains(s, "babel:") || !strings.Contains(s, "ours:") {
		t.Errorf("report should show both sides:\n%s", s)
	}
}
