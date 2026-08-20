package printer

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lilybw/go-solid-compiler/ast"
)

// Printer renders AST nodes to TypeScript source.
//
// A Printer is single-use and not safe for concurrent use; prefer the
// package-level [Print] and [Fprint].
type Printer struct {
	opts   Options
	buf    []byte
	indent int
	col    int
	pend   bool // an indent is owed before the next write
}

// New returns a Printer configured from [Default] and the supplied options.
func New(opts ...Option) *Printer {
	o := Default()
	for _, fn := range opts {
		fn(&o)
	}
	return &Printer{opts: o}
}

// NewWith returns a Printer using o verbatim.
func NewWith(o Options) *Printer { return &Printer{opts: o} }

// Print renders n and returns the resulting source text.
func Print(n ast.Node, opts ...Option) string {
	p := New(opts...)
	p.node(n)
	return p.finish()
}

// Fprint renders n to w.
func Fprint(w io.Writer, n ast.Node, opts ...Option) error {
	_, err := io.WriteString(w, Print(n, opts...))
	return err
}

// Print renders n using this Printer's configuration and returns the text.
func (p *Printer) Print(n ast.Node) string {
	p.node(n)
	return p.finish()
}

func (p *Printer) finish() string {
	s := string(p.buf)
	if p.opts.FinalNewline && s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

// ---------------------------------------------------------------------------
// Output primitives
// ---------------------------------------------------------------------------

func (p *Printer) w(s string) {
	if s == "" {
		return
	}
	if p.pend {
		ind := p.opts.indentString(p.indent)
		p.buf = append(p.buf, ind...)
		p.col += utf8.RuneCountInString(ind)
		p.pend = false
	}
	p.buf = append(p.buf, s...)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		p.col = utf8.RuneCountInString(s[i+1:])
	} else {
		p.col += utf8.RuneCountInString(s)
	}
}

// nl starts a new line, trimming trailing horizontal whitespace.
func (p *Printer) nl() {
	n := len(p.buf)
	for n > 0 && (p.buf[n-1] == ' ' || p.buf[n-1] == '\t') {
		n--
	}
	p.buf = append(p.buf[:n], '\n')
	p.col = 0
	p.pend = true
}

func (p *Printer) in()  { p.indent++ }
func (p *Printer) out() { p.indent-- }

// sub renders f into a fresh buffer at the current indent, for measuring a
// construct before deciding whether to wrap it.
func (p *Printer) sub(f func(*Printer)) string {
	q := &Printer{opts: p.opts, indent: p.indent}
	f(q)
	return string(q.buf)
}

func (p *Printer) fits(extra int) bool {
	return p.opts.MaxLineWidth <= 0 || p.col+extra <= p.opts.MaxLineWidth
}

// ---------------------------------------------------------------------------
// Lexical helpers
// ---------------------------------------------------------------------------

// isIdent reports whether s may be emitted as a bare identifier.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !(unicode.IsLetter(r) || r == '_' || r == '$') {
				return false
			}
			continue
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' ||
			r == 0x200C || r == 0x200D) {
			return false
		}
	}
	return true
}

// quote renders s as a string literal, escaping as required.
func quote(s string, q byte) string {
	var b strings.Builder
	b.WriteByte(q)
	for _, r := range s {
		switch r {
		case rune(q):
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\v':
			b.WriteString(`\v`)
		case 0x2028:
			b.WriteString(`\u2028`)
		case 0x2029:
			b.WriteString(`\u2029`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte(q)
	return b.String()
}

func (p *Printer) str(s string) { p.w(quote(s, p.opts.quoteChar())) }

// propName emits a property name, quoting it when it is not a bare identifier.
func (p *Printer) propName(n ast.PropertyName) {
	switch x := n.(type) {
	case nil:
	case *ast.Ident:
		if isIdent(x.Text) {
			p.w(x.Text)
		} else {
			p.str(x.Text)
		}
	case *ast.StringLit:
		p.str(x.Value)
	case *ast.NumberLit:
		p.w(x.Text)
	case *ast.ComputedName:
		p.w("[")
		p.expr(x.Expr, precLowest)
		p.w("]")
	default:
		p.node(n)
	}
}

func (p *Printer) entity(n ast.EntityName) {
	switch x := n.(type) {
	case nil:
	case *ast.Ident:
		p.w(x.Text)
	case *ast.QualifiedName:
		p.entity(x.Left)
		p.w(".")
		p.w(x.Right.Text)
	}
}

// ---------------------------------------------------------------------------
// Documentation
// ---------------------------------------------------------------------------

func (p *Printer) docs(d *ast.Doc) {
	if !p.opts.EmitDocs || d.IsEmpty() {
		return
	}
	// A single short line with no tags collapses to one line.
	if len(d.Tags) == 0 && len(d.Text) == 1 && !strings.Contains(d.Text[0], "\n") &&
		p.fits(len(d.Text[0])+7) {
		p.w("/** " + strings.ReplaceAll(d.Text[0], "*/", "*\\/") + " */")
		p.nl()
		return
	}
	p.w("/**")
	p.nl()
	for _, line := range d.Text {
		for _, sub := range strings.Split(line, "\n") {
			p.w(strings.TrimRight(" * "+strings.ReplaceAll(sub, "*/", "*\\/"), " "))
			p.nl()
		}
	}
	for _, t := range d.Tags {
		s := " * @" + t.Name
		if t.Text != "" {
			s += " " + strings.ReplaceAll(t.Text, "*/", "*\\/")
		}
		p.w(s)
		p.nl()
	}
	p.w(" */")
	p.nl()
}

// ---------------------------------------------------------------------------
// Modifiers
// ---------------------------------------------------------------------------

// modifiers emits modifier keywords in canonical order.
func (p *Printer) modifiers(m ast.Modifier) {
	emit := func(bit ast.Modifier, kw string) {
		if m.Has(bit) {
			p.w(kw)
			p.w(" ")
		}
	}
	emit(ast.ModExport, "export")
	emit(ast.ModDefault, "default")
	emit(ast.ModDeclare, "declare")
	emit(ast.ModPublic, "public")
	emit(ast.ModPrivate, "private")
	emit(ast.ModProtected, "protected")
	emit(ast.ModStatic, "static")
	emit(ast.ModOverride, "override")
	emit(ast.ModAbstract, "abstract")
	emit(ast.ModReadonly, "readonly")
	emit(ast.ModAccessor, "accessor")
	emit(ast.ModAsync, "async")
}

// ---------------------------------------------------------------------------
// Type precedence
// ---------------------------------------------------------------------------

type tprec int

const (
	tpLowest tprec = iota // conditional, function, constructor, predicate
	tpUnion
	tpIntersection
	tpOperator // keyof, readonly, unique, infer
	tpPostfix  // T[], T[K]
	tpPrimary
)

func typePrec(t ast.Type) tprec {
	switch t.(type) {
	case *ast.ConditionalType, *ast.FunctionType, *ast.ConstructorType, *ast.PredicateType:
		return tpLowest
	case *ast.UnionType:
		return tpUnion
	case *ast.IntersectionType:
		return tpIntersection
	case *ast.TypeOperator, *ast.InferType:
		return tpOperator
	case *ast.ArrayType, *ast.IndexedAccessType:
		return tpPostfix
	default:
		return tpPrimary
	}
}

// typeAt emits t, parenthesizing when its precedence is looser than min.
func (p *Printer) typeAt(t ast.Type, min tprec) {
	if t == nil {
		return
	}
	if typePrec(t) < min {
		p.w("(")
		p.typeInner(t)
		p.w(")")
		return
	}
	p.typeInner(t)
}

// Type emits a type at the loosest precedence.
func (p *Printer) Type(t ast.Type) { p.typeAt(t, tpLowest) }

func (p *Printer) typeInner(t ast.Type) {
	switch x := t.(type) {
	case *ast.KeywordType:
		p.w(x.Keyword.String())

	case *ast.LiteralType:
		if x.Negated {
			p.w("-")
		}
		switch x.LitKind {
		case ast.LitString:
			p.str(x.Value)
		default:
			p.w(x.Value)
		}

	case *ast.TypeRef:
		p.entity(x.Name)
		p.typeArgs(x.Args)

	case *ast.ThisType:
		p.w("this")

	case *ast.ArrayType:
		p.typeAt(x.Elem, tpPostfix)
		p.w("[]")

	case *ast.TupleType:
		p.w("[")
		for i, e := range x.Elems {
			if i > 0 {
				p.w(", ")
			}
			p.typeAt(e, tpLowest)
		}
		p.w("]")

	case *ast.OptionalType:
		p.typeAt(x.Elem, tpPostfix)
		p.w("?")

	case *ast.RestType:
		p.w("...")
		p.typeAt(x.Elem, tpPostfix)

	case *ast.NamedTupleMember:
		if x.Rest {
			p.w("...")
		}
		p.w(x.Name.Text)
		if x.Optional {
			p.w("?")
		}
		p.w(": ")
		p.typeAt(x.Type, tpLowest)

	case *ast.UnionType:
		p.unionish(x.Types, "|", tpUnion)

	case *ast.IntersectionType:
		p.unionish(x.Types, "&", tpIntersection)

	case *ast.ParenType:
		p.w("(")
		p.typeAt(x.Inner, tpLowest)
		p.w(")")

	case *ast.TypeLiteral:
		p.typeMembersInline(x.Members)

	case *ast.FunctionType:
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		p.w(" => ")
		if x.Return == nil {
			p.w("void")
		} else {
			p.typeAt(x.Return, tpLowest)
		}

	case *ast.ConstructorType:
		if x.Abstract {
			p.w("abstract ")
		}
		p.w("new ")
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		p.w(" => ")
		if x.Return == nil {
			p.w("void")
		} else {
			p.typeAt(x.Return, tpLowest)
		}

	case *ast.IndexedAccessType:
		p.typeAt(x.Object, tpPostfix)
		p.w("[")
		p.typeAt(x.Index, tpLowest)
		p.w("]")

	case *ast.MappedType:
		p.mappedType(x)

	case *ast.ConditionalType:
		p.typeAt(x.Check, tpUnion)
		p.w(" extends ")
		p.typeAt(x.Extends, tpUnion)
		p.w(" ? ")
		p.typeAt(x.True, tpLowest)
		p.w(" : ")
		p.typeAt(x.False, tpLowest)

	case *ast.InferType:
		p.w("infer ")
		p.w(x.Param.Name.Text)
		if x.Param.Constraint != nil {
			p.w(" extends ")
			p.typeAt(x.Param.Constraint, tpUnion)
		}

	case *ast.TypeOperator:
		p.w(x.Op.String())
		p.w(" ")
		p.typeAt(x.Type, tpOperator)

	case *ast.TypeQuery:
		p.w("typeof ")
		p.entity(x.Name)
		p.typeArgs(x.Args)

	case *ast.TemplateLiteralType:
		p.w("`")
		for i, q := range x.Quasis {
			p.w(escapeTemplate(q))
			if i < len(x.Types) {
				p.w("${")
				p.typeAt(x.Types[i], tpLowest)
				p.w("}")
			}
		}
		p.w("`")

	case *ast.ImportType:
		if x.TypeOf {
			p.w("typeof ")
		}
		p.w("import(")
		p.str(x.Module)
		p.importAttrs(x.Attributes)
		p.w(")")
		if x.Qualifier != nil {
			p.w(".")
			p.entity(x.Qualifier)
		}
		p.typeArgs(x.Args)

	case *ast.PredicateType:
		if x.Asserts {
			p.w("asserts ")
		}
		p.w(x.ParamName.Text)
		if x.Type != nil {
			p.w(" is ")
			p.typeAt(x.Type, tpLowest)
		}

	case *ast.RawType:
		p.w(x.Text)

	default:
		p.w(fmt.Sprintf("/* unprintable type %T */ unknown", t))
	}
}

func escapeTemplate(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	return strings.ReplaceAll(s, "${", "\\${")
}

// unionish emits a union or intersection, wrapping when the single-line form
// would exceed the configured width.
func (p *Printer) unionish(types []ast.Type, op string, min tprec) {
	if len(types) == 0 {
		p.w("never")
		return
	}
	parts := make([]string, len(types))
	width := 0
	for i, t := range types {
		parts[i] = p.sub(func(q *Printer) { q.typeAt(t, min) })
		width += utf8.RuneCountInString(parts[i]) + 3
	}
	if len(types) == 1 || p.fits(width) || strings.Contains(strings.Join(parts, ""), "\n") {
		for i, s := range parts {
			if i > 0 {
				p.w(" " + op + " ")
			}
			p.w(s)
		}
		return
	}
	// Leading-delimiter layout, which keeps diffs clean when members change.
	p.in()
	for _, s := range parts {
		p.nl()
		p.w(op + " " + s)
	}
	p.out()
}

func (p *Printer) typeArgs(args []ast.Type) {
	if len(args) == 0 {
		return
	}
	p.w("<")
	for i, a := range args {
		if i > 0 {
			p.w(", ")
		}
		p.typeAt(a, tpLowest)
	}
	p.w(">")
}

func (p *Printer) typeParams(ps []*ast.TypeParam) {
	if len(ps) == 0 {
		return
	}
	p.w("<")
	for i, tp := range ps {
		if i > 0 {
			p.w(", ")
		}
		if tp.Const {
			p.w("const ")
		}
		switch tp.Variance {
		case ast.VarianceIn:
			p.w("in ")
		case ast.VarianceOut:
			p.w("out ")
		case ast.VarianceInOut:
			p.w("in out ")
		}
		p.w(tp.Name.Text)
		if tp.Constraint != nil {
			p.w(" extends ")
			p.typeAt(tp.Constraint, tpLowest)
		}
		if tp.Default != nil {
			p.w(" = ")
			p.typeAt(tp.Default, tpLowest)
		}
	}
	p.w(">")
}

func (p *Printer) params(ps []*ast.Param) {
	p.w("(")
	single := p.sub(func(q *Printer) {
		for i, prm := range ps {
			if i > 0 {
				q.w(", ")
			}
			q.param(prm)
		}
	})
	if p.fits(utf8.RuneCountInString(single)+1) || len(ps) == 0 {
		p.w(single)
		p.w(")")
		return
	}
	p.in()
	for i, prm := range ps {
		p.nl()
		p.param(prm)
		if i < len(ps)-1 || p.opts.TrailingComma {
			p.w(",")
		}
	}
	p.out()
	p.nl()
	p.w(")")
}

func (p *Printer) param(prm *ast.Param) {
	p.modifiers(prm.Mods)
	if prm.Rest {
		p.w("...")
	}
	p.propName(prm.Name)
	if prm.Optional {
		p.w("?")
	}
	if prm.Type != nil {
		p.w(": ")
		p.typeAt(prm.Type, tpLowest)
	}
	if prm.Default != nil {
		p.w(" = ")
		p.expr(prm.Default, precAssign)
	}
}

// mappedType emits a mapped type, keeping it on one line when it fits.
func (p *Printer) mappedType(x *ast.MappedType) {
	single := p.sub(func(q *Printer) {
		q.w("{ ")
		q.mappedBody(x)
		q.w(" }")
	})
	if !strings.Contains(single, "\n") && p.fits(utf8.RuneCountInString(single)) {
		p.w(single)
		return
	}
	p.w("{")
	p.in()
	p.nl()
	p.mappedBody(x)
	p.w(p.opts.memberSepText())
	p.out()
	p.nl()
	p.w("}")
}

func (p *Printer) mappedBody(x *ast.MappedType) {
	switch x.ReadonlyMod {
	case ast.MappedAdd:
		p.w("readonly ")
	case ast.MappedRemove:
		p.w("-readonly ")
	}
	p.w("[")
	p.w(x.Param.Name.Text)
	if x.Param.Constraint != nil {
		p.w(" in ")
		p.typeAt(x.Param.Constraint, tpLowest)
	}
	if x.As != nil {
		p.w(" as ")
		p.typeAt(x.As, tpLowest)
	}
	p.w("]")
	switch x.OptionalMod {
	case ast.MappedAdd:
		p.w("?")
	case ast.MappedRemove:
		p.w("-?")
	}
	if x.Type != nil {
		p.w(": ")
		p.typeAt(x.Type, tpLowest)
	}
}

// typeMembersInline emits an anonymous object type, keeping it on one line
// when short and undocumented. Interface bodies always use the block form.
func (p *Printer) typeMembersInline(ms []ast.Member) {
	if len(ms) == 0 {
		p.w("{}")
		return
	}
	for _, m := range ms {
		if !docsOf(m).IsEmpty() {
			p.typeMembers(ms)
			return
		}
	}
	single := p.sub(func(q *Printer) {
		q.w("{ ")
		for i, m := range ms {
			if i > 0 {
				q.w(" ")
			}
			q.member(m)
			if q.opts.MemberSep == NewlineSeparator && i < len(ms)-1 {
				q.w(";")
			}
		}
		q.w(" }")
	})
	if !strings.Contains(single, "\n") && p.fits(utf8.RuneCountInString(single)) {
		// Trim the separator that would otherwise sit before the brace.
		if sep := p.opts.memberSepText(); sep != "" {
			single = strings.TrimSuffix(single, sep+" }") + " }"
		}
		p.w(single)
		return
	}
	p.typeMembers(ms)
}

// docsOf reports the documentation attached to a member, if any.
func docsOf(m ast.Member) *ast.Doc {
	switch x := m.(type) {
	case *ast.PropertySignature:
		return x.Docs
	case *ast.MethodSignature:
		return x.Docs
	case *ast.IndexSignature:
		return x.Docs
	case *ast.CallSignature:
		return x.Docs
	case *ast.ConstructSignature:
		return x.Docs
	}
	return nil
}

// typeMembers emits a braced member list, collapsing to {} when empty.
func (p *Printer) typeMembers(ms []ast.Member) {
	if len(ms) == 0 {
		p.w("{}")
		return
	}
	p.w("{")
	p.in()
	for _, m := range ms {
		p.nl()
		p.member(m)
	}
	p.out()
	p.nl()
	p.w("}")
}

func (p *Printer) importAttrs(as []ast.ImportAttribute) {
	if len(as) == 0 {
		return
	}
	p.w(", { with: { ")
	for i, a := range as {
		if i > 0 {
			p.w(", ")
		}
		if isIdent(a.Key) {
			p.w(a.Key)
		} else {
			p.str(a.Key)
		}
		p.w(": ")
		p.str(a.Value)
	}
	p.w(" } }")
}
