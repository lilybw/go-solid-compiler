package solid

import (
	"fmt"
	"sort"
	"strings"
)

// Options configures the transform. The zero value is valid.
type Options struct {
	// ModuleName is the import source for runtime helpers.
	// Defaults to "solid-js/web".
	ModuleName string

	// Delegate routes supported events through a single document-level
	// listener. Defaults to true; set DisableDelegation to turn it off.
	DisableDelegation bool

	// Prefix is prepended to generated runtime helper identifiers.
	// Defaults to "_$", matching solid's own output.
	Prefix string
}

func (o *Options) withDefaults() {
	if o.ModuleName == "" {
		o.ModuleName = "solid-js/web"
	}
	if o.Prefix == "" {
		o.Prefix = "_$"
	}
}

// Module accumulates the state shared across every JSX expression in one
// source file: hoisted templates, the runtime helpers used, and the events
// needing delegation.
//
// Compile one expression at a time, then take the Prelude and Postlude.
// A Module is not safe for concurrent use.
type Module struct {
	opts Options

	templates []template
	helpers   map[string]bool
	delegated map[string]bool

	elemUID int
	tmplUID int
	memoUID int
}

type template struct {
	name string
	html string
}

// NewModule returns a Module ready to compile the JSX of one source file.
func NewModule(opts Options) *Module {
	opts.withDefaults()
	return &Module{
		opts:      opts,
		helpers:   map[string]bool{},
		delegated: map[string]bool{},
	}
}

// helper records a use of a runtime helper and returns the local name to call.
func (m *Module) helper(name string) string {
	m.helpers[name] = true
	return m.opts.Prefix + name
}

func (m *Module) nextElem() string {
	m.elemUID++
	if m.elemUID == 1 {
		return "_el$"
	}
	return fmt.Sprintf("_el$%d", m.elemUID)
}

func (m *Module) nextMemo() string {
	m.memoUID++
	if m.memoUID == 1 {
		return "_c$"
	}
	return fmt.Sprintf("_c$%d", m.memoUID)
}

func (m *Module) nextTemplate(html string) string {
	m.tmplUID++
	name := "_tmpl$"
	if m.tmplUID > 1 {
		name = fmt.Sprintf("_tmpl$%d", m.tmplUID)
	}
	m.templates = append(m.templates, template{name: name, html: html})
	return name
}

// Compile lowers one JSX node into a JavaScript expression, suitable for
// substituting in place of the original JSX.
func (m *Module) Compile(n Node) string {
	switch x := n.(type) {
	case *Element:
		return m.compileElement(x)
	case *Component:
		return m.compileComponent(x)
	case *Fragment:
		return m.compileFragment(x)
	case *Text:
		return quoteJS(x.Value)
	case *Expr:
		return x.Code
	}
	return "null"
}

// ---------------------------------------------------------------------------
// Host elements
// ---------------------------------------------------------------------------

// tmplNode is one node of the static template skeleton. Dynamic children do
// not appear here, but they record which static sibling they precede.
type tmplNode struct {
	el       *Element
	parent   *tmplNode
	index    int // position among the parent's static children
	varName  string
	needsVar bool // this node itself is referenced by a statement
	needed   bool // this node or a descendant needs a variable
	ord      int  // document-order rank, assigned with the variable
	children []*tmplNode
	isText   bool
}

// elementParts lowers a host element, returning either a standalone
// expression for a fully static tree or the statement body for a dynamic one.
// Exactly one is non-empty.
func (m *Module) elementParts(el *Element) (static, body string) {
	c := &elemCompiler{mod: m}
	root := c.walk(el, nil, 0)

	// Only nodes on a path to a referenced node need to be materialized.
	markNeeded(root)
	c.assignVars(root)

	tmplName := m.nextTemplate(elideTrailingCloseTags(c.html.String()))

	// A fully static tree is just a clone.
	if len(c.stmts) == 0 && !root.needsVar {
		return tmplName + "()", ""
	}

	var b strings.Builder
	// var rather than const, matching the reference output. The bindings are
	// never reassigned either way; this exists so the generated text compares
	// equal.
	b.WriteString("  var " + root.varName + " = " + tmplName + "()")
	for _, d := range c.decls {
		b.WriteString(",\n    " + d)
	}
	b.WriteString(";\n")
	for _, s := range c.stmts {
		b.WriteString("  " + s + ";\n")
	}
	b.WriteString("  return " + root.varName + ";\n")
	return "", b.String()
}

// compileElement lowers a host element into an expression.
func (m *Module) compileElement(el *Element) string {
	static, body := m.elementParts(el)
	if body == "" {
		return static
	}
	return "(() => {\n" + body + "})()"
}

// compileElementInline lowers a host element into statements suitable for a
// getter body, without the immediately-invoked wrapper.
func (m *Module) compileElementInline(el *Element) string {
	static, body := m.elementParts(el)
	if body == "" {
		return "return " + static + ";"
	}
	return strings.TrimSpace(body)
}

// elemCompiler holds the per-element compilation state.
type elemCompiler struct {
	mod   *Module
	html  strings.Builder
	decls []string
	stmts []string
	ord   int

	// pending records work that can only be emitted once variables have been
	// assigned, since a statement may reference a node discovered later.
	pending []pendingStmt
}

// pendingStmt is a statement whose text depends on variables assigned after
// the tree walk completes.
type pendingStmt struct {
	node   *tmplNode
	marker *tmplNode // insertion marker, may be nil
	render func(self string, marker string) string
}

// walk emits static HTML for el and records the dynamic work it implies.
func (c *elemCompiler) walk(el *Element, parent *tmplNode, index int) *tmplNode {
	node := &tmplNode{el: el, parent: parent, index: index}

	tag := el.Tag
	c.html.WriteString("<" + tag)

	// A spread makes every attribute dynamic: the spread may set or clear any
	// of them, and the result depends on their relative order, so none can be
	// baked into the template.
	if hasSpread(el.Attrs) {
		c.html.WriteString(">")
		if !IsVoidElement(tag) {
			c.walkChildren(el.Children, node)
			c.html.WriteString("</" + tag + ">")
		}
		c.spreadAttrs(el, node)
		return node
	}

	// Static attributes are baked into the template; dynamic ones become
	// statements. staticAttr writes as a side effect, so its decision is
	// recorded rather than recomputed below.
	isStatic := make([]bool, len(el.Attrs))
	for i, a := range el.Attrs {
		isStatic[i] = c.staticAttr(a)
		if !isStatic[i] {
			node.needsVar = true
		}
	}
	c.html.WriteString(">")

	if !IsVoidElement(tag) {
		c.walkChildren(el.Children, node)
		c.html.WriteString("</" + tag + ">")
	}

	// Dynamic attribute statements are queued after children so that variable
	// numbering follows document order, matching solid's own output.
	for i, a := range el.Attrs {
		if isStatic[i] {
			continue
		}
		c.dynamicAttr(a, node)
	}
	return node
}

// hasSpread reports whether any attribute is a {...props} spread.
func hasSpread(attrs []Attr) bool {
	for _, a := range attrs {
		if a.Namespace == "" && a.Name == "..." {
			return true
		}
	}
	return false
}

// spreadAttrs emits a single spread call carrying every attribute. The
// runtime signature is spread(element, props, isSVG, hasChildren).
func (c *elemCompiler) spreadAttrs(el *Element, node *tmplNode) {
	m := c.mod
	var parts []string
	var props []string

	flushProps := func() {
		if len(props) == 0 {
			return
		}
		parts = append(parts, "{\n    "+strings.Join(props, ",\n    ")+"\n  }")
		props = nil
	}

	for _, a := range el.Attrs {
		if a.Namespace == "" && a.Name == "..." {
			// Order matters, so a spread closes the object literal being
			// accumulated before it rather than merging into it.
			flushProps()
			parts = append(parts, lazyValue(a.Value))
			continue
		}
		name := a.Name
		if a.Namespace != "" {
			name = a.Namespace + ":" + a.Name
		}
		key := quoteJS(name)
		switch {
		case a.Kind == AttrBare:
			props = append(props, key+": true")
		case a.Kind == AttrString:
			props = append(props, key+": "+quoteJS(a.Value))
		case !a.IsDynamic():
			props = append(props, key+": "+a.Value)
		default:
			props = append(props, "get "+key+"() { return "+a.Value+"; }")
		}
	}
	flushProps()

	spread := m.helper("spread")
	arg := "{}"
	switch len(parts) {
	case 0:
	case 1:
		arg = parts[0]
	default:
		arg = m.helper("mergeProps") + "(" + strings.Join(parts, ", ") + ")"
	}
	if len(parts) == 1 {
		// mergeProps normalizes a lone spread into a props object too.
		arg = m.helper("mergeProps") + "(" + parts[0] + ")"
	}

	hasChildren := len(filterChildren(el.Children)) > 0
	c.queue(node, nil, func(self, _ string) string {
		return fmt.Sprintf("%s(%s, %s, false, %t)", spread, self, arg, hasChildren)
	})
}

// staticAttr writes an attribute into the template when it can be, reporting
// whether it did.
func (c *elemCompiler) staticAttr(a Attr) bool {
	if a.Namespace != "" {
		return false
	}
	switch a.Name {
	case "ref", "classList", "style":
		return false
	}
	if propertyAttrs[a.Name] {
		// Properties always need assignment, because the parser will not
		// reflect them back once the user interacts with the element.
		return false
	}
	switch a.Kind {
	case AttrBare:
		c.html.WriteString(" " + attrHTMLName(a.Name))
		return true
	case AttrString:
		c.writeStaticAttr(attrHTMLName(a.Name), a.Value)
		return true
	case AttrExpr:
		// A literal expression is static in every sense that matters.
		if !a.IsDynamic() {
			if lit, ok := literalString(a.Value); ok {
				c.writeStaticAttr(attrHTMLName(a.Name), lit)
				return true
			}
		}
	}
	return false
}

// writeStaticAttr emits an attribute into the template, quoting only when HTML
// requires it.
func (c *elemCompiler) writeStaticAttr(name, value string) {
	c.html.WriteString(" " + name)
	if value == "" {
		return
	}
	if attrNeedsQuotes(value) {
		c.html.WriteString(`="` + escapeHTMLAttr(value) + `"`)
		return
	}
	c.html.WriteString("=" + escapeHTMLText(value))
}

// literalString extracts the content of a quoted string literal.
func literalString(src string) (string, bool) {
	s := strings.TrimSpace(src)
	if len(s) >= 2 {
		q := s[0]
		if (q == '"' || q == '\'') && s[len(s)-1] == q {
			return s[1 : len(s)-1], true
		}
	}
	return "", false
}

// dynamicAttr queues the statement binding one dynamic attribute.
func (c *elemCompiler) dynamicAttr(a Attr, node *tmplNode) {
	m := c.mod
	value := a.Value
	if a.Kind == AttrString {
		value = quoteJS(a.Value)
	} else if a.Kind == AttrBare {
		value = "true"
	}

	switch a.Namespace {
	case "on":
		// on:Click preserves case and never delegates.
		h := m.helper("addEventListener")
		c.queue(node, nil, func(self, _ string) string {
			return fmt.Sprintf("%s(%s, %s, %s)", h, self, quoteJS(a.Name), value)
		})
		return
	case "oncapture":
		c.queue(node, nil, func(self, _ string) string {
			return fmt.Sprintf("%s.addEventListener(%s, %s, true)", self, quoteJS(a.Name), value)
		})
		return
	case "use":
		h := m.helper("use")
		c.queue(node, nil, func(self, _ string) string {
			return fmt.Sprintf("%s(%s, %s, () => (%s))", h, a.Name, self, value)
		})
		return
	case "prop":
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(), fmt.Sprintf("%s.%s = %s", self, a.Name, value))
		})
		return
	case "attr":
		h := m.helper("setAttribute")
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(),
				fmt.Sprintf("%s(%s, %s, %s)", h, self, quoteJS(a.Name), value))
		})
		return
	case "bool":
		h := m.helper("setBoolAttribute")
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(),
				fmt.Sprintf("%s(%s, %s, %s)", h, self, quoteJS(a.Name), value))
		})
		return
	case "style":
		h := m.helper("setStyleProperty")
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(),
				fmt.Sprintf("%s(%s, %s, %s)", h, self, quoteJS(a.Name), value))
		})
		return
	case "class":
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(),
				fmt.Sprintf("%s.classList.toggle(%s, %s)", self, quoteJS(a.Name), value))
		})
		return
	}

	// Unnamespaced attributes.
	switch {
	case a.Name == "ref":
		// A ref is either a callback to invoke or a variable to assign, and
		// which one is only knowable at runtime, so the emitted code branches
		// on it exactly as the reference implementation does.
		h := m.helper("use")
		tmp := "_ref$"
		c.queue(node, nil, func(self, _ string) string {
			return fmt.Sprintf("var %s = %s; typeof %s === \"function\" ? %s(%s, %s) : %s = %s",
				tmp, value, tmp, h, tmp, self, value, self)
		})

	case strings.HasPrefix(a.Name, "on") && len(a.Name) > 2:
		event := strings.ToLower(a.Name[2:])
		h := m.helper("addEventListener")
		if IsDelegatedEvent(event) && !m.opts.DisableDelegation {
			m.delegated[event] = true
			// The trailing true tells the runtime this event is delegated.
			c.queue(node, nil, func(self, _ string) string {
				return fmt.Sprintf("%s(%s, %s, %s, true)", h, self, quoteJS(event), value)
			})
			return
		}
		c.queue(node, nil, func(self, _ string) string {
			return fmt.Sprintf("%s(%s, %s, %s)", h, self, quoteJS(event), value)
		})

	case a.Name == "class" || a.Name == "className":
		h := m.helper("className")
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(), fmt.Sprintf("%s(%s, %s)", h, self, value))
		})

	case a.Name == "classList":
		h := m.helper("classList")
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(), fmt.Sprintf("%s(%s, %s)", h, self, value))
		})

	case a.Name == "style":
		h := m.helper("style")
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(), fmt.Sprintf("%s(%s, %s)", h, self, value))
		})

	case propertyAttrs[a.Name]:
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(), fmt.Sprintf("%s.%s = %s", self, a.Name, value))
		})

	default:
		h := m.helper("setAttribute")
		c.queue(node, nil, func(self, _ string) string {
			return c.maybeEffect(a.IsDynamic(),
				fmt.Sprintf("%s(%s, %s, %s)", h, self, quoteJS(attrHTMLName(a.Name)), value))
		})
	}
}

// maybeEffect wraps a statement so it re-runs when its dependencies change.
func (c *elemCompiler) maybeEffect(dynamic bool, stmt string) string {
	if !dynamic {
		return stmt
	}
	return fmt.Sprintf("%s(() => %s)", c.mod.helper("effect"), stmt)
}

func (c *elemCompiler) queue(node, marker *tmplNode, render func(self, marker string) string) {
	node.needsVar = true
	if marker != nil {
		marker.needsVar = true
	}
	c.pending = append(c.pending, pendingStmt{node: node, marker: marker, render: render})
}

// walkChildren emits static children into the template and queues inserts for
// the dynamic ones.
func (c *elemCompiler) walkChildren(children []Node, parent *tmplNode) {
	// Normalize first so that index bookkeeping matches what ends up in the
	// template: dropped whitespace must not occupy a slot.
	kept := make([]Node, 0, len(children))
	for _, ch := range children {
		if t, ok := ch.(*Text); ok {
			v := NormalizeJSXText(t.Value)
			if v == "" {
				continue
			}
			kept = append(kept, &Text{Value: v})
			continue
		}
		kept = append(kept, ch)
	}

	// Build the static skeleton, recording for each dynamic child which static
	// node follows it. That node becomes the insertion marker.
	staticIdx := 0
	type deferredInsert struct {
		code       string
		markerSlot int // index into parent.children, or -1 for "append"
		dynamic    bool
	}
	var inserts []deferredInsert

	for _, ch := range kept {
		switch x := ch.(type) {
		case *Text:
			c.html.WriteString(escapeHTMLText(x.Value))
			tn := &tmplNode{parent: parent, index: staticIdx, isText: true}
			parent.children = append(parent.children, tn)
			staticIdx++

		case *Element:
			child := c.walk(x, parent, staticIdx)
			parent.children = append(parent.children, child)
			staticIdx++

		case *Expr:
			if !x.IsDynamic() {
				// A literal child can be baked into the template.
				if lit, ok := literalString(x.Code); ok {
					c.html.WriteString(escapeHTMLText(lit))
					tn := &tmplNode{parent: parent, index: staticIdx, isText: true}
					parent.children = append(parent.children, tn)
					staticIdx++
					continue
				}
			}
			inserts = append(inserts, deferredInsert{
				code: x.Code, markerSlot: staticIdx, dynamic: x.IsDynamic(),
			})

		case *Component:
			// createComponent defers its own work, so wrapping it in an arrow
			// would add a redundant layer the reference output does not have.
			inserts = append(inserts, deferredInsert{
				code: c.mod.compileComponent(x), markerSlot: staticIdx, dynamic: false,
			})

		case *Fragment:
			inserts = append(inserts, deferredInsert{
				code: c.mod.compileFragment(x), markerSlot: staticIdx, dynamic: true,
			})
		}
	}

	// Queue the inserts now that the static slots are known.
	for _, ins := range inserts {
		var marker *tmplNode
		if ins.markerSlot < len(parent.children) {
			marker = parent.children[ins.markerSlot]
		}
		code := ins.code
		accessor := code
		if ins.dynamic {
			// A conditional gets a memoized test so that only a genuine change
			// of branch rebuilds the DOM.
			if wrapped, ok := c.mod.wrapConditional(code); ok {
				accessor = wrapped
			} else {
				// insert takes an accessor so it can re-run; a bare expression
				// would be evaluated once at construction time.
				accessor = "() => " + code
			}
		}
		// insert without a marker takes ownership of the parent's entire
		// content, so it is only safe when this really is the sole child.
		// Counting static children alone would miss dynamic siblings, and two
		// unmarked inserts on one parent would have the second erase the
		// first.
		onlyChild := len(kept) == 1
		h := c.mod.helper("insert")
		c.queue(parent, marker, func(self, mark string) string {
			if onlyChild {
				return fmt.Sprintf("%s(%s, %s)", h, self, accessor)
			}
			if mark == "" {
				mark = "null"
			}
			return fmt.Sprintf("%s(%s, %s, %s)", h, self, accessor, mark)
		})
	}
}

// ---------------------------------------------------------------------------
// Variable assignment and DOM navigation
// ---------------------------------------------------------------------------

// markNeeded marks every node that must be materialized: those referenced by
// a statement, and their ancestors.
func markNeeded(n *tmplNode) bool {
	need := n.needsVar
	for _, ch := range n.children {
		if markNeeded(ch) {
			need = true
		}
	}
	n.needed = need
	return need
}

// assignVars walks the template in document order, giving each needed node a
// variable and a declaration that navigates to it from an already-declared
// node using firstChild and nextSibling.
func (c *elemCompiler) assignVars(root *tmplNode) {
	if !root.needed {
		root.needed = true
	}
	root.varName = c.mod.nextElem()
	c.ord++
	root.ord = c.ord
	c.assignChildren(root)
	c.flushPending()
}

func (c *elemCompiler) assignChildren(parent *tmplNode) {
	for i, ch := range parent.children {
		if !ch.needed {
			continue
		}
		ch.varName = c.mod.nextElem()
		c.ord++
		ch.ord = c.ord
		c.decls = append(c.decls, ch.varName+" = "+navigate(parent, i))
		c.assignChildren(ch)
	}
}

// navigate builds the expression reaching the child at index i of parent.
func navigate(parent *tmplNode, i int) string {
	// Prefer chaining from the nearest already-declared preceding sibling,
	// which keeps the chains short.
	for j := i - 1; j >= 0; j-- {
		if sib := parent.children[j]; sib.varName != "" {
			return sib.varName + strings.Repeat(".nextSibling", i-j)
		}
	}
	return parent.varName + ".firstChild" + strings.Repeat(".nextSibling", i)
}

// flushPending renders the queued statements, ordered by the document
// position of the element they act on.
func (c *elemCompiler) flushPending() {
	sort.SliceStable(c.pending, func(i, j int) bool {
		return c.pending[i].node.ord < c.pending[j].node.ord
	})
	for _, p := range c.pending {
		marker := ""
		if p.marker != nil {
			marker = p.marker.varName
		}
		c.stmts = append(c.stmts, p.render(p.node.varName, marker))
	}
}

// ---------------------------------------------------------------------------
// Conditionals
// ---------------------------------------------------------------------------

// splitConditional recognizes a ternary or logical operator at the top level
// of an expression, returning the condition and the remainder.
func splitConditional(code string) (cond, rest, op string, ok bool) {
	depth := 0
	var inStr byte
	for i := 0; i < len(code); i++ {
		c := code[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '?':
			// Skip optional chaining and nullish coalescing.
			if depth == 0 && i+1 < len(code) && code[i+1] != '.' && code[i+1] != '?' {
				return strings.TrimSpace(code[:i]), strings.TrimSpace(code[i+1:]), "?", true
			}
			if depth == 0 && i+1 < len(code) && code[i+1] == '?' {
				i++
			}
		case '&':
			if depth == 0 && i+1 < len(code) && code[i+1] == '&' {
				return strings.TrimSpace(code[:i]), strings.TrimSpace(code[i+2:]), "&&", true
			}
		case '|':
			if depth == 0 && i+1 < len(code) && code[i+1] == '|' {
				return strings.TrimSpace(code[:i]), strings.TrimSpace(code[i+2:]), "||", true
			}
		}
	}
	return "", "", "", false
}

// wrapConditional memoizes the test of a conditional child, so the branch is
// rebuilt only when the answer changes rather than whenever the condition
// expression yields a new value.
//
// It reports false if the expression is not a conditional.
func (m *Module) wrapConditional(code string) (string, bool) {
	cond, rest, op, ok := splitConditional(code)
	if !ok || cond == "" || rest == "" {
		return "", false
	}
	name := m.nextMemo()
	memo := m.helper("memo")

	var body string
	switch op {
	case "?":
		branches := splitTernaryBranches(rest)
		if branches == nil {
			return "", false
		}
		body = fmt.Sprintf("%s() ? %s : %s", name, branches[0], branches[1])
	default:
		body = fmt.Sprintf("%s() %s %s", name, op, rest)
	}
	test := cond
	if condNeedsParens(cond) {
		test = "(" + cond + ")"
	}
	return fmt.Sprintf("(() => {\n    var %s = %s(() => !!%s);\n    return () => %s;\n  })()",
		name, memo, test, body), true
}

// condNeedsParens reports whether a memo test needs parentheses under the
// double-negation coercion.
func condNeedsParens(cond string) bool {
	depth := 0
	var inStr byte
	for i := 0; i < len(cond); i++ {
		c := cond[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ' ', '+', '-', '*', '/', '%', '<', '>', '=', '!', '&', '|', ',', '^':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// splitTernaryBranches divides the two arms of a ternary at its top-level
// colon.
func splitTernaryBranches(rest string) []string {
	depth, ternary := 0, 0
	var inStr byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '?':
			if depth == 0 {
				ternary++
			}
		case ':':
			if depth == 0 {
				if ternary == 0 {
					return []string{
						strings.TrimSpace(rest[:i]),
						strings.TrimSpace(rest[i+1:]),
					}
				}
				ternary--
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Components and fragments
// ---------------------------------------------------------------------------

// solidBuiltins are the components solid-js/web provides optimized versions
// of, which are imported from the runtime rather than referenced by name.
var solidBuiltins = map[string]bool{
	"For": true, "Show": true, "Index": true, "Switch": true, "Match": true,
	"ErrorBoundary": true, "Suspense": true, "SuspenseList": true,
	"Portal": true, "Dynamic": true,
}

// compileComponent lowers a component element into a createComponent call.
//
// Dynamic props become getters, which preserves reactivity across the
// component boundary.
func (m *Module) compileComponent(comp *Component) string {
	create := m.helper("createComponent")
	tag := comp.Tag
	if solidBuiltins[tag] {
		tag = m.helper(tag)
	}

	var spreads []string
	var props []string

	for _, a := range comp.Attrs {
		if a.Namespace == "" && a.Name == "..." {
			spreads = append(spreads, lazyValue(a.Value))
			continue
		}
		name := a.Name
		if a.Namespace != "" {
			name = a.Namespace + ":" + a.Name
		}
		key := name
		if !isPlainIdent(name) {
			key = quoteJS(name)
		}
		switch {
		case a.Kind == AttrBare:
			props = append(props, key+": true")
		case a.Kind == AttrString:
			props = append(props, key+": "+quoteJS(a.Value))
		case !a.IsDynamic():
			props = append(props, key+": "+a.Value)
		default:
			props = append(props, "get "+key+"() { return "+a.Value+"; }")
		}
	}

	if kids := m.componentChildren(comp.Children); kids != "" {
		props = append(props, kids)
	}

	obj := "{}"
	if len(props) > 0 {
		obj = "{\n    " + strings.Join(props, ",\n    ") + "\n  }"
	}

	if len(spreads) > 0 {
		merge := m.helper("mergeProps")
		args := append(append([]string{}, spreads...), obj)
		return fmt.Sprintf("%s(%s, %s(%s))", create, tag, merge, strings.Join(args, ", "))
	}
	return fmt.Sprintf("%s(%s, %s)", create, tag, obj)
}

// componentChildren renders the children prop, or "" when there are none.
func (m *Module) componentChildren(children []Node) string {
	kept := filterChildren(children)
	if len(kept) == 0 {
		return ""
	}
	// A single static child needs no getter: it can never change. Text is the
	// obvious case; a function expression is the important one, because
	// control-flow components such as For take their children as a callback.
	if len(kept) == 1 {
		if t, ok := kept[0].(*Text); ok {
			return "children: " + quoteJS(t.Value)
		}
		if e, ok := kept[0].(*Expr); ok && !e.IsDynamic() {
			return "children: " + e.Code
		}
	}
	if len(kept) == 1 {
		// A getter is already a function; an element child can be built
		// directly in its body rather than inside a nested IIFE.
		if el, ok := kept[0].(*Element); ok {
			return "get children() { " + m.compileElementInline(el) + " }"
		}
		return "get children() { return " + m.Compile(kept[0]) + "; }"
	}
	parts := make([]string, len(kept))
	for i, ch := range kept {
		parts[i] = m.Compile(ch)
	}
	return "get children() { return [" + strings.Join(parts, ", ") + "]; }"
}

// compileFragment lowers <>...</> into an array.
func (m *Module) compileFragment(f *Fragment) string {
	kept := filterChildren(f.Children)
	switch len(kept) {
	case 0:
		return "[]"
	case 1:
		return m.Compile(kept[0])
	}
	parts := make([]string, len(kept))
	for i, ch := range kept {
		parts[i] = m.Compile(ch)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// filterChildren drops whitespace-only text, normalizing what remains.
func filterChildren(children []Node) []Node {
	out := make([]Node, 0, len(children))
	for _, ch := range children {
		if t, ok := ch.(*Text); ok {
			v := NormalizeJSXText(t.Value)
			if strings.TrimSpace(v) == "" {
				continue
			}
			out = append(out, &Text{Value: v})
			continue
		}
		out = append(out, ch)
	}
	return out
}

func isPlainIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '_' || r == '$' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Module assembly
// ---------------------------------------------------------------------------

// Prelude returns the statements that must appear at the top of the module:
// the runtime import and the hoisted template declarations.
func (m *Module) Prelude() string {
	if len(m.helpers) == 0 && len(m.templates) == 0 {
		return ""
	}

	// template and delegateEvents are emitted by Prelude and Postlude rather
	// than by Compile, so they are registered here — otherwise the generated
	// module would reference helpers it never imported.
	if len(m.templates) > 0 {
		m.helpers["template"] = true
	}
	if len(m.delegated) > 0 {
		m.helpers["delegateEvents"] = true
	}

	var b strings.Builder

	if len(m.helpers) > 0 {
		names := make([]string, 0, len(m.helpers))
		for h := range m.helpers {
			names = append(names, h)
		}
		sort.Strings(names)
		specs := make([]string, len(names))
		for i, h := range names {
			specs[i] = h + " as " + m.opts.Prefix + h
		}
		b.WriteString("import { " + strings.Join(specs, ", ") + " } from " +
			quoteJS(m.opts.ModuleName) + ";\n")
	}

	for _, t := range m.templates {
		b.WriteString("const " + t.name + " = /*#__PURE__*/" +
			m.opts.Prefix + "template(" + quoteTemplate(t.html) + ");\n")
	}
	return b.String()
}

// Postlude returns the statements that must appear at the end of the module:
// the event delegation registration, if any.
func (m *Module) Postlude() string {
	if len(m.delegated) == 0 {
		return ""
	}
	names := make([]string, 0, len(m.delegated))
	for e := range m.delegated {
		names = append(names, quoteJS(e))
	}
	sort.Strings(names)
	return m.opts.Prefix + "delegateEvents([" + strings.Join(names, ", ") + "]);\n"
}

// Helpers returns the runtime helpers used so far, sorted. The template helper
// is included whenever any template was hoisted.
func (m *Module) Helpers() []string {
	if len(m.templates) > 0 {
		m.helpers["template"] = true
	}
	out := make([]string, 0, len(m.helpers))
	for h := range m.helpers {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}
