package tsx

import (
	"fmt"
	"strings"

	tsast "github.com/Zzzen/typescript-go/use-at-your-own-risk/ast"

	"github.com/lilybw/go-solid-compiler/solid"
)

// Conversion from the TypeScript compiler's JSX nodes to the solid package's
// intermediate representation.

// transformer carries the module that accumulates templates and helpers, and
// the file whose text expressions are sliced from.
type transformer struct {
	mod  *solid.Module
	file *tsast.SourceFile
}

// ToJSX converts a compiler JSX node into the solid IR. Prefer
// [TransformSolid] for whole files.
func ToJSX(n *tsast.Node, mod *solid.Module) (solid.Node, bool) {
	t := &transformer{mod: mod, file: tsast.GetSourceFileOfNode(n)}
	return t.toJSX(n)
}

func (t *transformer) toJSX(n *tsast.Node) (solid.Node, bool) {
	if n == nil {
		return nil, false
	}
	switch n.Kind {
	case tsast.KindJsxElement:
		return t.jsxElement(n.AsJsxElement()), true
	case tsast.KindJsxSelfClosingElement:
		return t.jsxSelfClosing(n.AsJsxSelfClosingElement()), true
	case tsast.KindJsxFragment:
		return t.jsxFragment(n.AsJsxFragment()), true
	}
	return nil, false
}

// exprText returns the source of an expression with any JSX inside it
// compiled and spliced in place, so that constructs such as
// items.map(i => <li/>) are handled.
func (t *transformer) exprText(n *tsast.Node) string {
	if n == nil {
		return ""
	}
	src := t.file.Text()
	start, end := n.Pos(), n.End()
	if start < 0 || end > len(src) || start > end {
		return ""
	}

	var spans []jsxSpan
	var walk func(*tsast.Node)
	walk = func(x *tsast.Node) {
		if x == nil {
			return
		}
		if outer, inner, ok := parenSpan(x); ok {
			if node, ok := t.toJSX(inner); ok {
				spans = append(spans, jsxSpan{
					start: outer.Pos(), end: outer.End(), code: t.mod.Compile(node),
				})
				return
			}
		}
		if node, ok := t.toJSX(x); ok {
			// Compile and stop: the children were handled by Compile, and
			// descending would splice inner spans twice.
			spans = append(spans, jsxSpan{start: x.Pos(), end: x.End(), code: t.mod.Compile(node)})
			return
		}
		x.ForEachChild(func(c *tsast.Node) bool {
			walk(c)
			return false
		})
	}
	walk(n)

	if len(spans) == 0 {
		return strings.TrimSpace(src[start:end])
	}
	var b strings.Builder
	prev := start
	for _, sp := range spans {
		if sp.start < prev {
			continue
		}
		b.WriteString(src[prev:sp.start])
		b.WriteString(sp.code)
		prev = sp.end
	}
	b.WriteString(src[prev:end])
	return strings.TrimSpace(b.String())
}

func (t *transformer) jsxElement(el *tsast.JsxElement) solid.Node {
	opening := el.OpeningElement.AsJsxOpeningElement()
	tag := tagName(opening.TagName)
	attrs := t.jsxAttributes(opening.Attributes)
	children := t.jsxChildren(el.Children)

	if solid.IsComponentTag(tag) {
		return &solid.Component{Tag: tag, Attrs: attrs, Children: children}
	}
	return &solid.Element{Tag: tag, Attrs: attrs, Children: children}
}

func (t *transformer) jsxSelfClosing(el *tsast.JsxSelfClosingElement) solid.Node {
	tag := tagName(el.TagName)
	attrs := t.jsxAttributes(el.Attributes)
	if solid.IsComponentTag(tag) {
		return &solid.Component{Tag: tag, Attrs: attrs}
	}
	return &solid.Element{Tag: tag, Attrs: attrs}
}

func (t *transformer) jsxFragment(f *tsast.JsxFragment) solid.Node {
	return &solid.Fragment{Children: t.jsxChildren(f.Children)}
}

// tagName renders a JSX tag: an identifier, a dotted name, or a namespaced
// name.
func tagName(n *tsast.Node) string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case tsast.KindPropertyAccessExpression:
		p := n.AsPropertyAccessExpression()
		return tagName(p.Expression) + "." + identText(p.Name())
	case tsast.KindJsxNamespacedName:
		q := n.AsJsxNamespacedName()
		return identText(q.Namespace) + ":" + identText(q.Name())
	case tsast.KindIdentifier, tsast.KindPrivateIdentifier:
		return n.Text()
	}
	// Node.Text panics on any kind outside its switch, so an unfamiliar tag
	// falls back to its source text rather than taking down the build.
	return nodeText(n)
}

func (t *transformer) jsxAttributes(n *tsast.Node) []solid.Attr {
	if n == nil {
		return nil
	}
	attrs := n.AsJsxAttributes()
	if attrs == nil || attrs.Properties == nil {
		return nil
	}
	out := make([]solid.Attr, 0, len(attrs.Properties.Nodes))
	for _, p := range attrs.Properties.Nodes {
		switch p.Kind {
		case tsast.KindJsxSpreadAttribute:
			out = append(out, solid.Attr{
				Name:  "...",
				Kind:  solid.AttrExpr,
				Value: t.exprText(p.AsJsxSpreadAttribute().Expression),
			})
		case tsast.KindJsxAttribute:
			out = append(out, t.jsxAttribute(p.AsJsxAttribute()))
		}
	}
	return out
}

func (t *transformer) jsxAttribute(a *tsast.JsxAttribute) solid.Attr {
	attr := solid.Attr{}

	// A namespaced attribute is on:click, use:directive, and so on.
	name := a.Name()
	if name != nil && name.Kind == tsast.KindJsxNamespacedName {
		q := name.AsJsxNamespacedName()
		attr.Namespace = identText(q.Namespace)
		attr.Name = identText(q.Name())
	} else {
		attr.Name = identText(name)
	}

	init := a.Initializer
	switch {
	case init == nil:
		attr.Kind = solid.AttrBare

	case init.Kind == tsast.KindStringLiteral:
		attr.Kind = solid.AttrString
		attr.Value = init.Text()

	case init.Kind == tsast.KindJsxText:
		attr.Kind = solid.AttrString
		attr.Value = init.AsJsxText().Text

	case init.Kind == tsast.KindJsxExpression:
		attr.Kind = solid.AttrExpr
		// An attribute value may itself contain JSX, as in
		// fallback={<span>loading</span>}.
		attr.Value = t.exprText(init.AsJsxExpression().Expression)

	default:
		attr.Kind = solid.AttrExpr
		attr.Value = t.exprText(init)
	}
	return attr
}

func (t *transformer) jsxChildren(list *tsast.NodeList) []solid.Node {
	if list == nil {
		return nil
	}
	out := make([]solid.Node, 0, len(list.Nodes))
	for _, ch := range list.Nodes {
		switch ch.Kind {
		case tsast.KindJsxText:
			// Node.Text does not handle JsxText - it panics on any kind
			// outside its switch - so read the concrete node instead.
			out = append(out, &solid.Text{Value: ch.AsJsxText().Text})

		case tsast.KindJsxExpression:
			e := ch.AsJsxExpression()
			if e.Expression == nil {
				// An empty {} or a comment-only expression contributes nothing.
				continue
			}
			if e.DotDotDotToken != nil {
				out = append(out, &solid.Spread{Code: t.exprText(e.Expression)})
				continue
			}
			// Direct JSX becomes a child structurally, so it shares the
			// parent's template. Anything else keeps its source, with any JSX
			// buried inside it compiled first.
			if inner, ok := t.toJSX(e.Expression); ok {
				out = append(out, inner)
				continue
			}
			out = append(out, &solid.Expr{Code: t.exprText(e.Expression)})

		default:
			if inner, ok := t.toJSX(ch); ok {
				out = append(out, inner)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Whole-file transformation
// ---------------------------------------------------------------------------

// parenSpan reports the parentheses wrapping a JSX expression, so they can be
// replaced along with it. They exist only to allow a multi-line arrow body.
func parenSpan(n *tsast.Node) (*tsast.Node, *tsast.Node, bool) {
	if n.Kind != tsast.KindParenthesizedExpression {
		return nil, nil, false
	}
	inner := n.AsParenthesizedExpression().Expression
	if inner == nil {
		return nil, nil, false
	}
	switch inner.Kind {
	case tsast.KindJsxElement, tsast.KindJsxSelfClosingElement, tsast.KindJsxFragment:
		return n, inner, true
	}
	return nil, nil, false
}

// jsxSpan records a JSX expression's byte range and its replacement.
type jsxSpan struct {
	start, end int
	code       string
}

// TransformSolid lowers every JSX expression in a parsed file and returns the
// resulting TypeScript source.
//
// Type annotations are left in place: only JSX is lowered. Pass the result to
// esbuild with the TS loader to strip types and bundle.
func TransformSolid(file *tsast.SourceFile, opts solid.Options) (out string, err error) {
	// The compiler panics on node kinds its helpers do not cover; converting
	// that to an error keeps one malformed file from killing a build process.
	defer func() {
		if r := recover(); r != nil {
			out = ""
			err = fmt.Errorf("tsx: transforming %s: %v", file.FileName(), r)
		}
	}()

	mod := solid.NewModule(opts)
	t := &transformer{mod: mod, file: file}
	src := file.Text()

	var spans []jsxSpan
	var walk func(n *tsast.Node)
	walk = func(n *tsast.Node) {
		if n == nil {
			return
		}
		if outer, inner, ok := parenSpan(n); ok {
			if node, ok := t.toJSX(inner); ok {
				spans = append(spans, jsxSpan{
					start: outer.Pos(), end: outer.End(), code: mod.Compile(node),
				})
				return
			}
		}
		if node, ok := t.toJSX(n); ok {
			spans = append(spans, jsxSpan{
				start: n.Pos(), end: n.End(), code: mod.Compile(node),
			})
			// Do not descend: Compile handled the children, including any JSX
			// nested inside expressions.
			return
		}
		n.ForEachChild(func(child *tsast.Node) bool {
			walk(child)
			return false
		})
	}
	walk(file.AsNode())

	if len(spans) == 0 {
		return src, nil
	}

	var b strings.Builder
	b.WriteString(mod.Prelude())
	prev := 0
	for _, s := range spans {
		if s.start < prev {
			continue // defensive: overlapping spans should not occur
		}
		b.WriteString(src[prev:s.start])
		b.WriteString(s.code)
		prev = s.end
	}
	b.WriteString(src[prev:])
	b.WriteString(mod.Postlude())
	return b.String(), nil
}
