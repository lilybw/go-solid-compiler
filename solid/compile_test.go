package solid_test

import (
	"strings"
	"testing"

	"github.com/lilybw/go-solid-compiler/solid"
)

// el is a terse constructor for host elements in tests.
func el(tag string, attrs []solid.Attr, children ...solid.Node) *solid.Element {
	return &solid.Element{Tag: tag, Attrs: attrs, Children: children}
}

func comp(tag string, attrs []solid.Attr, children ...solid.Node) *solid.Component {
	return &solid.Component{Tag: tag, Attrs: attrs, Children: children}
}

func txt(s string) *solid.Text { return &solid.Text{Value: s} }
func ex(s string) *solid.Expr  { return &solid.Expr{Code: s} }

func str(name, v string) solid.Attr {
	return solid.Attr{Name: name, Kind: solid.AttrString, Value: v}
}
func dyn(name, code string) solid.Attr {
	return solid.Attr{Name: name, Kind: solid.AttrExpr, Value: code}
}

// compileOne compiles a single node and returns the prelude and expression.
func compileOne(t *testing.T, n solid.Node) (prelude, expr string) {
	t.Helper()
	m := solid.NewModule(solid.Options{})
	expr = m.Compile(n)
	return m.Prelude(), expr
}

func mustContain(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Static templates
// ---------------------------------------------------------------------------

func TestFullyStaticElementIsJustAClone(t *testing.T) {
	pre, expr := compileOne(t, el("div", []solid.Attr{str("class", "card")}, txt("hello")))

	if expr != "_tmpl$()" {
		t.Errorf("a static tree should compile to a bare clone, got:\n%s", expr)
	}
	mustContain(t, pre, "`<div class=card>hello`")
}

func TestVoidElementHasNoClosingTag(t *testing.T) {
	pre, _ := compileOne(t, el("div", nil, el("br", nil), el("img", []solid.Attr{str("src", "a.png")})))
	mustContain(t, pre, "<br>", "<img src=a.png>")
	if strings.Contains(pre, "</br>") || strings.Contains(pre, "</img>") {
		t.Errorf("void elements must not be closed:\n%s", pre)
	}
}

func TestAttributeNameMapping(t *testing.T) {
	pre, _ := compileOne(t, el("label", []solid.Attr{
		str("className", "x"), str("htmlFor", "y"),
	}))
	mustContain(t, pre, "class=x", "for=y")
}

func TestHTMLEscapingInTemplate(t *testing.T) {
	pre, _ := compileOne(t, el("div", []solid.Attr{str("title", `a"b&c`)}, txt("<script>&")))
	mustContain(t, pre, `title="a&quot;b&amp;c"`, "&lt;script&gt;&amp;")
}

func TestBareAttributeBecomesBooleanAttribute(t *testing.T) {
	pre, expr := compileOne(t, el("input", []solid.Attr{
		{Name: "disabled", Kind: solid.AttrBare},
	}))
	mustContain(t, pre, "<input disabled>")
	if expr != "_tmpl$()" {
		t.Errorf("bare attribute should stay static, got %s", expr)
	}
}

func TestLiteralExpressionAttributeStaysStatic(t *testing.T) {
	pre, expr := compileOne(t, el("div", []solid.Attr{dyn("title", `"fixed"`)}))
	mustContain(t, pre, "title=fixed")
	if expr != "_tmpl$()" {
		t.Errorf("literal attribute should not produce an effect, got:\n%s", expr)
	}
}

// ---------------------------------------------------------------------------
// Dynamic children and DOM navigation
// ---------------------------------------------------------------------------

func TestSingleDynamicChildUsesInsertWithoutMarker(t *testing.T) {
	pre, expr := compileOne(t, el("div", nil, ex("count()")))

	mustContain(t, pre, "`<div>`")
	mustContain(t, expr, "_$insert(_el$, () => count())")
	if strings.Contains(expr, "null") {
		t.Errorf("an only child needs no marker:\n%s", expr)
	}
}

func TestDynamicChildAfterTextInsertsWithNullMarker(t *testing.T) {
	pre, expr := compileOne(t, el("p", nil, txt("Hello "), ex("name()")))

	mustContain(t, pre, "`<p>Hello `")
	// The text occupies a slot, so the insert must be told to append.
	mustContain(t, expr, "_$insert(_el$, () => name(), null)")
}

func TestDynamicChildBetweenStaticsUsesFollowingSiblingAsMarker(t *testing.T) {
	_, expr := compileOne(t, el("div", nil,
		el("span", nil, txt("a")),
		ex("mid()"),
		el("b", nil, txt("z")),
	))
	// The marker is the element that follows the hole, so insertion lands in
	// the right place rather than at the end. The <span> before the hole needs
	// no variable of its own, so the marker is reached by navigating past it.
	mustContain(t, expr, "_$insert(_el$, () => mid(), _el$2)")
	mustContain(t, expr, "_el$2 = _el$.firstChild.nextSibling")
}

func TestNavigationUsesFirstChildAndNextSibling(t *testing.T) {
	_, expr := compileOne(t, el("div", nil,
		el("h1", nil, ex("title()")),
		el("p", nil, ex("body()")),
	))
	mustContain(t, expr,
		"_el$2 = _el$.firstChild",
		"_el$3 = _el$2.nextSibling",
		"_$insert(_el$2, () => title())",
		"_$insert(_el$3, () => body())",
	)
}

func TestUnreferencedNodesGetNoVariable(t *testing.T) {
	// Only the node holding the dynamic child needs materializing; the two
	// static siblings before it should not be declared.
	_, expr := compileOne(t, el("div", nil,
		el("span", nil, txt("a")),
		el("span", nil, txt("b")),
		el("span", nil, ex("x()")),
	))
	if strings.Count(expr, "_el$") == 0 {
		t.Fatalf("expected some variables:\n%s", expr)
	}
	// Three static spans would mean four variables; skipping the unreferenced
	// ones should mean two.
	if n := countDecls(expr); n > 2 {
		t.Errorf("declared %d variables, expected at most 2:\n%s", n, expr)
	}
	mustContain(t, expr, ".firstChild.nextSibling.nextSibling")
}

func countDecls(expr string) int {
	n := 0
	for _, line := range strings.Split(expr, "\n") {
		if strings.Contains(line, "_el$") && strings.Contains(line, "=") &&
			!strings.Contains(line, "_$") {
			n++
		}
	}
	return n
}

func TestNestedElementsNavigateCorrectly(t *testing.T) {
	pre, expr := compileOne(t, el("div", nil,
		el("section", nil,
			el("h1", nil, ex("t()")),
		),
	))
	mustContain(t, pre, "`<div><section><h1>`")
	mustContain(t, expr,
		"_el$2 = _el$.firstChild",
		"_el$3 = _el$2.firstChild",
		"_$insert(_el$3, () => t())",
	)
}

func TestStaticLiteralChildIsBakedIntoTemplate(t *testing.T) {
	pre, expr := compileOne(t, el("div", nil, ex(`"literal"`)))
	mustContain(t, pre, "<div>literal`")
	if expr != "_tmpl$()" {
		t.Errorf("literal child should not need insert, got:\n%s", expr)
	}
}

// ---------------------------------------------------------------------------
// Dynamic attributes
// ---------------------------------------------------------------------------

func TestDynamicClassWrapsInEffect(t *testing.T) {
	_, expr := compileOne(t, el("div", []solid.Attr{dyn("class", "cls()")}))
	mustContain(t, expr, "_$effect(() => _$className(_el$, cls()))")
}

func TestDynamicAttributeUsesSetAttribute(t *testing.T) {
	_, expr := compileOne(t, el("a", []solid.Attr{dyn("href", "url()")}))
	mustContain(t, expr, `_$effect(() => _$setAttribute(_el$, "href", url()))`)
}

func TestPropertyAttributesAreAssignedNotSet(t *testing.T) {
	// value must be a property: the attribute stops reflecting once the user
	// types into the field.
	_, expr := compileOne(t, el("input", []solid.Attr{dyn("value", "v()")}))
	mustContain(t, expr, "_$effect(() => _el$.value = v())")
	if strings.Contains(expr, "setAttribute") {
		t.Errorf("value should not go through setAttribute:\n%s", expr)
	}
}

func TestStyleAndClassListUseHelpers(t *testing.T) {
	_, expr := compileOne(t, el("div", []solid.Attr{
		dyn("style", "s()"), dyn("classList", "c()"),
	}))
	mustContain(t, expr, "_$style(_el$, s())", "_$classList(_el$, c())")
}

func TestNamespacedStyleAndClassAttributes(t *testing.T) {
	_, expr := compileOne(t, el("div", []solid.Attr{
		{Namespace: "style", Name: "color", Kind: solid.AttrExpr, Value: "c()"},
		{Namespace: "class", Name: "active", Kind: solid.AttrExpr, Value: "isOn()"},
	}))
	mustContain(t, expr,
		`_$setStyleProperty(_el$, "color", c())`,
		`_el$.classList.toggle("active", isOn())`,
	)
}

func TestRefBinding(t *testing.T) {
	_, expr := compileOne(t, el("div", []solid.Attr{dyn("ref", "myRef")}))
	mustContain(t, expr, `typeof _ref$ === "function" ? _$use(_ref$, _el$) : myRef = _el$`)
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

func TestDelegatedEventUsesPropertyAndRegisters(t *testing.T) {
	m := solid.NewModule(solid.Options{})
	expr := m.Compile(el("button", []solid.Attr{dyn("onClick", "handle")}))

	mustContain(t, expr, `_$addEventListener(_el$, "click", handle, true)`)
	mustContain(t, m.Postlude(), `_$delegateEvents(["click"])`)
}

func TestNonDelegatedEventUsesAddEventListener(t *testing.T) {
	m := solid.NewModule(solid.Options{})
	expr := m.Compile(el("div", []solid.Attr{dyn("onScroll", "onScroll")}))

	mustContain(t, expr, `_$addEventListener(_el$, "scroll", onScroll)`)
	if m.Postlude() != "" {
		t.Errorf("scroll is not delegated, got postlude: %s", m.Postlude())
	}
}

func TestNamespacedEventPreservesCaseAndDoesNotDelegate(t *testing.T) {
	m := solid.NewModule(solid.Options{})
	expr := m.Compile(el("div", []solid.Attr{
		{Namespace: "on", Name: "CustomEvent", Kind: solid.AttrExpr, Value: "h"},
	}))
	mustContain(t, expr, `_$addEventListener(_el$, "CustomEvent", h)`)
	if m.Postlude() != "" {
		t.Error("on: events must never be delegated")
	}
}

func TestCaptureEvent(t *testing.T) {
	_, expr := compileOne(t, el("div", []solid.Attr{
		{Namespace: "oncapture", Name: "click", Kind: solid.AttrExpr, Value: "h"},
	}))
	mustContain(t, expr, `_el$.addEventListener("click", h, true)`)
}

func TestDelegationCanBeDisabled(t *testing.T) {
	m := solid.NewModule(solid.Options{DisableDelegation: true})
	expr := m.Compile(el("button", []solid.Attr{dyn("onClick", "h")}))
	mustContain(t, expr, `_$addEventListener(_el$, "click", h)`)
	if m.Postlude() != "" {
		t.Error("delegation disabled but events were registered")
	}
}

// ---------------------------------------------------------------------------
// Conditionals
// ---------------------------------------------------------------------------

func TestTernaryIsMemoized(t *testing.T) {
	// Memoizing the coerced test means a condition returning a fresh truthy
	// value each time does not tear down and rebuild the branch.
	_, expr := compileOne(t, el("div", nil, ex("cond() ? a() : b()")))
	mustContain(t, expr,
		"var _c$ = _$memo(() => !!cond())",
		"return () => _c$() ? a() : b();",
	)
}

func TestLogicalAndIsMemoized(t *testing.T) {
	_, expr := compileOne(t, el("div", nil, ex("flag() && x()")))
	mustContain(t, expr, "_$memo(() => !!flag())", "_c$() && x()")
}

func TestComplexConditionIsParenthesized(t *testing.T) {
	_, expr := compileOne(t, el("div", nil, ex("a() > 2 ? x() : y()")))
	mustContain(t, expr, "!!(a() > 2)")
}

func TestSimpleConditionIsNotParenthesized(t *testing.T) {
	_, expr := compileOne(t, el("div", nil, ex("props.ready ? x() : y()")))
	mustContain(t, expr, "!!props.ready")
}

func TestNestedTernarySplitsAtTopLevel(t *testing.T) {
	_, expr := compileOne(t, el("div", nil, ex("a() ? b() : c() ? d() : e()")))
	mustContain(t, expr, "_c$() ? b() : c() ? d() : e()")
}

func TestOptionalChainingIsNotMistakenForTernary(t *testing.T) {
	// a?.b is a property access, not a conditional.
	_, expr := compileOne(t, el("div", nil, ex("a?.b")))
	if strings.Contains(expr, "_$memo") {
		t.Errorf("optional chaining should not be memoized:\n%s", expr)
	}
}

func TestColonInsideCallIsNotATernarySplit(t *testing.T) {
	_, expr := compileOne(t, el("div", nil, ex("c() ? f({a: 1}) : g()")))
	mustContain(t, expr, "_c$() ? f({a: 1}) : g()")
}

// ---------------------------------------------------------------------------
// Insertion markers
// ---------------------------------------------------------------------------

func TestSoleDynamicChildOmitsMarker(t *testing.T) {
	_, expr := compileOne(t, el("div", nil, ex("only()")))
	mustContain(t, expr, "_$insert(_el$, () => only())")
	if strings.Contains(expr, ", null)") {
		t.Errorf("a sole child needs no marker:\n%s", expr)
	}
}

func TestTwoDynamicChildrenBothGetMarkers(t *testing.T) {
	// insert without a marker owns the parent's whole content, so two
	// unmarked inserts would have the second erase the first.
	_, expr := compileOne(t, el("div", nil, ex("a()"), ex("b()")))
	if n := strings.Count(expr, ", null)"); n != 2 {
		t.Errorf("expected both inserts to be marked, got %d:\n%s", n, expr)
	}
}

// ---------------------------------------------------------------------------
// Components
// ---------------------------------------------------------------------------

func TestComponentDynamicPropsBecomeGetters(t *testing.T) {
	_, expr := compileOne(t, comp("Card", []solid.Attr{
		dyn("count", "count()"), str("title", "Hi"),
	}))
	// The getter is what preserves reactivity across the boundary.
	mustContain(t, expr,
		"_$createComponent(Card, {",
		"get count() { return count(); }",
		`title: "Hi"`,
	)
}

func TestComponentChildrenBecomeGetter(t *testing.T) {
	// The element is built directly in the getter body: a getter is already a
	// function, so an inner IIFE would add a closure for nothing.
	_, expr := compileOne(t, comp("Wrap", nil, el("div", nil, ex("x()"))))
	mustContain(t, expr, "get children() { var _el$ = _tmpl$()")
	if strings.Contains(expr, "get children() { return (() =>") {
		t.Errorf("children should be inlined, not wrapped in an IIFE:\n%s", expr)
	}
}

func TestFunctionChildIsNotWrappedInAGetter(t *testing.T) {
	// Control-flow components take their children as a callback. A function is
	// a value, so re-reading it through a getter would be pointless.
	_, expr := compileOne(t, comp("For", nil, ex("(item) => item.name")))
	mustContain(t, expr, "children: (item) => item.name")
	if strings.Contains(expr, "get children") {
		t.Errorf("a function child needs no getter:\n%s", expr)
	}
}

func TestSolidBuiltinsAreImported(t *testing.T) {
	// For and Show have web-specialized implementations; using them removes
	// any dependence on the component file importing them itself.
	m := solid.NewModule(solid.Options{})
	expr := m.Compile(comp("For", []solid.Attr{dyn("each", "items()")}))
	mustContain(t, expr, "_$createComponent(_$For,")
	mustContain(t, m.Prelude(), "For as _$For")
}

func TestNonBuiltinComponentIsNotRewritten(t *testing.T) {
	m := solid.NewModule(solid.Options{})
	expr := m.Compile(comp("Card", nil))
	mustContain(t, expr, "_$createComponent(Card,")
	if strings.Contains(m.Prelude(), "Card as") {
		t.Errorf("a user component must not be imported from the runtime:\n%s", m.Prelude())
	}
}

func TestIdentifierPropIsNotAGetter(t *testing.T) {
	// A binding cannot itself change, so re-reading it achieves nothing.
	_, expr := compileOne(t, comp("Btn", []solid.Attr{dyn("onClick", "go")}))
	mustContain(t, expr, "onClick: go")
	if strings.Contains(expr, "get onClick") {
		t.Errorf("an identifier prop needs no getter:\n%s", expr)
	}
}

func TestPropertyAccessPropStaysAGetter(t *testing.T) {
	// props.x is reactive even though it looks like a simple reference.
	_, expr := compileOne(t, comp("Btn", []solid.Attr{dyn("label", "props.label")}))
	mustContain(t, expr, "get label() { return props.label; }")
}

func TestSpreadCallIsPassedLazily(t *testing.T) {
	// Evaluating the call here would snapshot the props and drop reactivity.
	_, expr := compileOne(t, el("div", []solid.Attr{spread("inputProps()")}))
	mustContain(t, expr, "_$mergeProps(inputProps)")
	if strings.Contains(expr, "inputProps()") {
		t.Errorf("spread argument was evaluated eagerly:\n%s", expr)
	}
}

func TestComponentStaticTextChildIsPlainValue(t *testing.T) {
	_, expr := compileOne(t, comp("Wrap", nil, txt("hello")))
	mustContain(t, expr, `children: "hello"`)
	if strings.Contains(expr, "get children") {
		t.Errorf("static text needs no getter:\n%s", expr)
	}
}

func TestComponentMultipleChildrenBecomeArray(t *testing.T) {
	_, expr := compileOne(t, comp("Wrap", nil, el("a", nil), el("b", nil)))
	mustContain(t, expr, "get children() { return [")
}

func TestComponentSpreadUsesMergeProps(t *testing.T) {
	_, expr := compileOne(t, comp("Card", []solid.Attr{
		{Name: "...", Kind: solid.AttrExpr, Value: "rest"},
		dyn("id", "id()"),
	}))
	mustContain(t, expr, "_$mergeProps(rest, {")
}

func TestNestedComponentInsideElementIsInserted(t *testing.T) {
	_, expr := compileOne(t, el("div", nil, comp("Child", nil)))
	mustContain(t, expr, "_$insert(_el$, _$createComponent(Child, {}))")
}

func TestDottedComponentTag(t *testing.T) {
	_, expr := compileOne(t, comp("UI.Button", nil))
	mustContain(t, expr, "_$createComponent(UI.Button, {})")
}

// ---------------------------------------------------------------------------
// Spread on host elements
// ---------------------------------------------------------------------------

func spread(code string) solid.Attr {
	return solid.Attr{Name: "...", Kind: solid.AttrExpr, Value: code}
}

func TestHostSpreadUsesSpreadHelper(t *testing.T) {
	// A spread has to go through the runtime: it may set or clear any
	// attribute, so nothing can be decided at compile time.
	pre, expr := compileOne(t, el("div", []solid.Attr{spread("props")}))
	mustContain(t, expr, "_$spread(_el$, _$mergeProps(props), false, false)")
	mustContain(t, pre, "`<div>`")
}

func TestHostSpreadKeepsAttributeOrder(t *testing.T) {
	// Attributes after a spread must override it, so they cannot be hoisted
	// into the template and must stay on the far side of the spread argument.
	_, expr := compileOne(t, el("div", []solid.Attr{
		spread("props"), str("id", "fixed"),
	}))
	mustContain(t, expr, `_$spread(_el$, _$mergeProps(props, {`, `"id": "fixed"`)
}

func TestHostSpreadSuppressesStaticAttributesInTemplate(t *testing.T) {
	pre, _ := compileOne(t, el("div", []solid.Attr{spread("p"), str("id", "x")}))
	if strings.Contains(pre, "id=") {
		t.Errorf("a spread makes every attribute dynamic:\n%s", pre)
	}
}

func TestHostSpreadReportsChildren(t *testing.T) {
	// The final argument tells the runtime whether the template already
	// populated children, so it can skip reconciliation.
	_, withKids := compileOne(t, el("div", []solid.Attr{spread("p")}, txt("hi")))
	mustContain(t, withKids, "false, true)")

	_, without := compileOne(t, el("div", []solid.Attr{spread("p")}))
	mustContain(t, without, "false, false)")
}

func TestSpreadDynamicPropsBecomeGetters(t *testing.T) {
	_, expr := compileOne(t, el("div", []solid.Attr{
		spread("p"), dyn("title", "t()"),
	}))
	mustContain(t, expr, `get "title"() { return t(); }`)
}

// ---------------------------------------------------------------------------
// Statement ordering
// ---------------------------------------------------------------------------

func TestStatementsFollowDocumentOrder(t *testing.T) {
	// A parent's bindings come before its children's, regardless of the order
	// the tree walk queued them.
	_, expr := compileOne(t, el("div", []solid.Attr{dyn("class", "outer()")},
		el("span", []solid.Attr{dyn("class", "inner()")}),
	))
	outer := strings.Index(expr, "outer()")
	inner := strings.Index(expr, "inner()")
	if outer < 0 || inner < 0 {
		t.Fatalf("both bindings should be present:\n%s", expr)
	}
	if outer > inner {
		t.Errorf("parent binding should precede child binding:\n%s", expr)
	}
}

// ---------------------------------------------------------------------------
// Fragments
// ---------------------------------------------------------------------------

func TestFragmentBecomesArray(t *testing.T) {
	_, expr := compileOne(t, &solid.Fragment{Children: []solid.Node{
		el("a", nil), el("b", nil),
	}})
	if !strings.HasPrefix(expr, "[") {
		t.Errorf("fragment should be an array, got:\n%s", expr)
	}
}

func TestSingleChildFragmentUnwraps(t *testing.T) {
	_, expr := compileOne(t, &solid.Fragment{Children: []solid.Node{el("a", nil)}})
	if strings.HasPrefix(expr, "[") {
		t.Errorf("a one-child fragment needs no array:\n%s", expr)
	}
}

func TestEmptyFragment(t *testing.T) {
	_, expr := compileOne(t, &solid.Fragment{})
	if expr != "[]" {
		t.Errorf("got %s", expr)
	}
}

// ---------------------------------------------------------------------------
// Module assembly
// ---------------------------------------------------------------------------

func TestPreludeImportsOnlyUsedHelpers(t *testing.T) {
	m := solid.NewModule(solid.Options{})
	m.Compile(el("div", nil, ex("x()")))
	pre := m.Prelude()

	mustContain(t, pre, `from "solid-js/web"`, "insert as _$insert")
	if strings.Contains(pre, "mergeProps") {
		t.Errorf("unused helper was imported:\n%s", pre)
	}
}

func TestTemplatesAreHoistedAndNumbered(t *testing.T) {
	m := solid.NewModule(solid.Options{})
	m.Compile(el("div", nil, txt("a")))
	m.Compile(el("span", nil, txt("b")))
	pre := m.Prelude()

	mustContain(t, pre,
		"const _tmpl$ = /*#__PURE__*/_$template(`<div>a`)",
		"const _tmpl$2 = /*#__PURE__*/_$template(`<span>b`)",
	)
}

// TestEveryUsedHelperIsImported guards the invariant that generated output is
// self-contained: any _$name it references must appear in the import clause,
// or the module throws at load time.
func TestEveryUsedHelperIsImported(t *testing.T) {
	m := solid.NewModule(solid.Options{})
	m.Compile(el("button", []solid.Attr{dyn("onClick", "h"), dyn("class", "c()")},
		txt("x"), ex("y()"), comp("Child", nil)))

	body := m.Prelude() + m.Postlude()
	imported := map[string]bool{}
	if i := strings.Index(body, "import { "); i >= 0 {
		clause := body[i+len("import { "):]
		clause = clause[:strings.Index(clause, " }")]
		for _, spec := range strings.Split(clause, ", ") {
			if j := strings.Index(spec, " as "); j >= 0 {
				imported[strings.TrimSpace(spec[j+4:])] = true
			}
		}
	}
	for _, used := range referencedHelpers(body) {
		if !imported[used] {
			t.Errorf("helper %s is used but not imported\n%s", used, body)
		}
	}
}

// referencedHelpers finds every _$name token in generated source.
func referencedHelpers(src string) []string {
	var out []string
	for i := 0; i+2 < len(src); i++ {
		if src[i] != '_' || src[i+1] != '$' {
			continue
		}
		j := i + 2
		for j < len(src) && (src[j] == '_' ||
			(src[j] >= 'a' && src[j] <= 'z') ||
			(src[j] >= 'A' && src[j] <= 'Z') ||
			(src[j] >= '0' && src[j] <= '9')) {
			j++
		}
		name := src[i:j]
		// Skip the declaration side of "x as _$x" and template variables.
		if strings.HasPrefix(name, "_$") && len(name) > 2 {
			out = append(out, name)
		}
		i = j - 1
	}
	return out
}

func TestCustomModuleName(t *testing.T) {
	m := solid.NewModule(solid.Options{ModuleName: "solid-js/web/dist/server.js"})
	m.Compile(el("div", nil, ex("x()")))
	mustContain(t, m.Prelude(), `from "solid-js/web/dist/server.js"`)
}

func TestTemplateEscaping(t *testing.T) {
	// A backtick and an interpolation opener must be escaped; a lone dollar
	// sign must not be, so that output matches babel's byte for byte.
	pre, _ := compileOne(t, el("div", nil, txt("a`b${c} $5")))
	mustContain(t, pre, "\\`", "\\${c}", " $5")
}

// ---------------------------------------------------------------------------
// JSX text normalization
// ---------------------------------------------------------------------------

func TestNormalizeJSXText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"  spaced  ", "  spaced  "},
		{"\n  indented\n", "indented"},
		{"\n  a\n  b\n", "a b"},
		{"\n   \n", ""},
		{"Hello ", "Hello "},
		{"\n  Hello\n  World\n  ", "Hello World"},
	}
	for _, tc := range tests {
		if got := solid.NormalizeJSXText(tc.in); got != tc.want {
			t.Errorf("NormalizeJSXText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWhitespaceOnlyChildrenAreDropped(t *testing.T) {
	pre, _ := compileOne(t, el("div", nil,
		txt("\n  "), el("span", nil), txt("\n  "), el("b", nil), txt("\n"),
	))
	mustContain(t, pre, "`<div><span></span><b>`")
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

func TestTagClassification(t *testing.T) {
	tests := []struct {
		tag  string
		comp bool
	}{
		{"div", false}, {"span", false}, {"my-element", false},
		{"Card", true}, {"UI.Button", true}, {"_Private", true},
	}
	for _, tc := range tests {
		if got := solid.IsComponentTag(tc.tag); got != tc.comp {
			t.Errorf("IsComponentTag(%q) = %v, want %v", tc.tag, got, tc.comp)
		}
	}
}

func TestDelegatedEventSet(t *testing.T) {
	for _, e := range []string{"click", "input", "keydown", "pointerup"} {
		if !solid.IsDelegatedEvent(e) {
			t.Errorf("%s should be delegated", e)
		}
	}
	for _, e := range []string{"scroll", "load", "focus", "mouseenter"} {
		if solid.IsDelegatedEvent(e) {
			t.Errorf("%s should not be delegated", e)
		}
	}
}
