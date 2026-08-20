package parse

import (
	"fmt"
	"strings"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/token"
)

// TypeFrontend parses the TypeScript type grammar plus interface, type
// alias, enum, import, and export declarations.
//
// It does not parse the expression and statement grammar; anything it does
// not recognize is preserved verbatim as an [ast.RawStmt] and reported.
type TypeFrontend struct{}

// Capabilities reports what this frontend covers.
func (TypeFrontend) Capabilities() Capability {
	return CapTypes | CapDeclarations | CapComments | CapPositions
}

// ParseFile parses a module.
func (TypeFrontend) ParseFile(name string, src []byte) (*ast.SourceFile, error) {
	p := newParser(name, src)
	f := p.parseFile(name)
	return f, p.errs.Err()
}

// ParseType parses a single type expression.
func (TypeFrontend) ParseType(src []byte) (ast.Type, error) {
	p := newParser("<type>", src)
	// parseReturnType is a superset of parseType: it also accepts predicate
	// forms such as `x is string`, which callers reasonably expect to parse.
	t := p.parseReturnType()
	if !p.atEOF() {
		p.errorf(p.cur().Span.Start, "unexpected %q after type", p.cur().Text)
	}
	return t, p.errs.Err()
}

// Default is the frontend used by the package-level helpers.
var Default Frontend = TypeFrontend{}

// Type parses a single type expression.
func Type(src string) (ast.Type, error) { return Default.ParseType([]byte(src)) }

// MustType parses a type expression and panics on error. It is intended for
// package-level variables and tests, where a malformed literal is a bug.
func MustType(src string) ast.Type {
	t, err := Type(src)
	if err != nil {
		panic(fmt.Sprintf("parse.MustType(%q): %v", src, err))
	}
	return t
}

// File parses a module with the default frontend.
func File(name string, src []byte) (*ast.SourceFile, error) {
	return Default.ParseFile(name, src)
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

type parser struct {
	toks []Token
	pos  int
	file *token.File
	errs ErrorList
}

func newParser(name string, src []byte) *parser {
	p := &parser{}
	s := NewScanner(name, src, &p.errs)
	p.file = s.File()
	for {
		t := s.Next()
		p.toks = append(p.toks, t)
		if t.Kind == EOF {
			break
		}
	}
	return p
}

func (p *parser) cur() Token { return p.toks[p.pos] }

func (p *parser) peek(n int) Token {
	if p.pos+n < len(p.toks) {
		return p.toks[p.pos+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) next() Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) atEOF() bool { return p.cur().Kind == EOF }

// at reports whether the current token has the given spelling, matching both
// punctuation and identifiers so contextual keywords work.
func (p *parser) at(text string) bool {
	t := p.cur()
	return (t.Kind == Punct || t.Kind == Ident) && t.Text == text
}

func (p *parser) atIdent() bool { return p.cur().Kind == Ident }

func (p *parser) accept(text string) bool {
	if p.at(text) {
		p.next()
		return true
	}
	return false
}

func (p *parser) expect(text string) Token {
	if p.at(text) {
		return p.next()
	}
	p.errorf(p.cur().Span.Start, "expected %q, found %q", text, p.cur().Text)
	return p.cur()
}

func (p *parser) errorf(pos token.Pos, format string, args ...any) {
	// Suppress cascades: one error per source position is enough.
	loc := p.file.Position(pos)
	if n := len(p.errs); n > 0 && p.errs[n-1].Pos == loc {
		return
	}
	p.errs = append(p.errs, Error{Pos: loc, Msg: fmt.Sprintf(format, args...)})
}

func (p *parser) span(start token.Pos) token.Span {
	end := start
	if p.pos > 0 {
		end = p.toks[p.pos-1].Span.End
	}
	return token.Span{Start: start, End: end}
}

// expectGT consumes a single '>', splitting a compound token such as '>>' so
// that nested type arguments close correctly.
func (p *parser) expectGT() {
	t := p.cur()
	if t.Kind == Punct && strings.HasPrefix(t.Text, ">") {
		if t.Text == ">" {
			p.next()
			return
		}
		p.toks[p.pos].Text = t.Text[1:]
		p.toks[p.pos].Span.Start = t.Span.Start + 1
		return
	}
	p.errorf(t.Span.Start, "expected '>', found %q", t.Text)
}

// ---------------------------------------------------------------------------
// Type grammar
// ---------------------------------------------------------------------------

var keywordTypes = map[string]ast.KeywordKind{
	"any": ast.KwAny, "unknown": ast.KwUnknown, "never": ast.KwNever,
	"void": ast.KwVoid, "undefined": ast.KwUndefined, "string": ast.KwString,
	"number": ast.KwNumber, "boolean": ast.KwBoolean, "bigint": ast.KwBigInt,
	"symbol": ast.KwSymbol, "object": ast.KwObject,
}

// parseType parses a full type, including conditional types.
func (p *parser) parseType() ast.Type { return p.parseTypeInner(true) }

// parseTypeNoCond parses a type without consuming a trailing extends clause
// as a conditional.
func (p *parser) parseTypeNoCond() ast.Type { return p.parseTypeInner(false) }

func (p *parser) parseTypeInner(allowCond bool) ast.Type {
	start := p.cur().Span.Start

	if t, ok := p.tryFunctionOrConstructorType(); ok {
		return t
	}

	t := p.parseUnion()

	if allowCond && p.at("extends") {
		p.next()
		ext := p.parseTypeInner(false)
		p.expect("?")
		yes := p.parseType()
		p.expect(":")
		no := p.parseType()
		c := &ast.ConditionalType{Check: t, Extends: ext, True: yes, False: no}
		c.Loc = p.span(start)
		return c
	}
	return t
}

// tryFunctionOrConstructorType recognizes the arrow forms.
func (p *parser) tryFunctionOrConstructorType() (ast.Type, bool) {
	start := p.cur().Span.Start

	abstract := false
	save := p.pos
	if p.at("abstract") && p.peek(1).Text == "new" {
		abstract = true
		p.next()
	}
	if p.at("new") {
		p.next()
		sig := p.parseSignature("=>")
		c := &ast.ConstructorType{Signature: sig, Abstract: abstract}
		c.Loc = p.span(start)
		return c, true
	}
	p.pos = save

	if p.at("<") || (p.at("(") && p.parenClosedBy("=>")) {
		sig := p.parseSignature("=>")
		f := &ast.FunctionType{Signature: sig}
		f.Loc = p.span(start)
		return f, true
	}
	return nil, false
}

// parenClosedBy reports whether the bracket group at the current token is
// followed by a token with the given spelling.
func (p *parser) parenClosedBy(text string) bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		t := p.toks[i]
		if t.Kind != Punct {
			continue
		}
		switch t.Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
			if depth == 0 {
				return i+1 < len(p.toks) && p.toks[i+1].Text == text
			}
		}
	}
	return false
}

// parseSignature parses type parameters, parameters, and a return type
// introduced by ret.
func (p *parser) parseSignature(ret string) ast.Signature {
	var sig ast.Signature
	sig.TypeParams = p.parseTypeParams()
	sig.Params = p.parseParams()
	if p.accept(ret) {
		sig.Return = p.parseReturnType()
	}
	return sig
}

// parseReturnType handles type predicates, which may appear only here.
func (p *parser) parseReturnType() ast.Type {
	start := p.cur().Span.Start
	if p.at("asserts") && p.peek(1).Kind == Ident && p.peek(1).Text != "is" {
		p.next()
		name := ast.NewIdent(p.next().Text)
		pred := &ast.PredicateType{Asserts: true, ParamName: name}
		if p.accept("is") {
			pred.Type = p.parseType()
		}
		pred.Loc = p.span(start)
		return pred
	}
	if p.cur().Kind == Ident && p.peek(1).Text == "is" && p.peek(1).Kind == Ident {
		name := ast.NewIdent(p.next().Text)
		p.next() // is
		pred := &ast.PredicateType{ParamName: name, Type: p.parseType()}
		pred.Loc = p.span(start)
		return pred
	}
	return p.parseType()
}

func (p *parser) parseTypeParams() []*ast.TypeParam {
	if !p.at("<") {
		return nil
	}
	p.next()
	var out []*ast.TypeParam
	for !p.at(">") && !p.atEOF() {
		start := p.cur().Span.Start
		tp := &ast.TypeParam{}
		if p.at("const") && p.peek(1).Kind == Ident {
			p.next()
			tp.Const = true
		}
		switch {
		case p.at("in") && p.peek(1).Text == "out":
			p.next()
			p.next()
			tp.Variance = ast.VarianceInOut
		case p.at("in") && p.peek(1).Kind == Ident:
			p.next()
			tp.Variance = ast.VarianceIn
		case p.at("out") && p.peek(1).Kind == Ident:
			p.next()
			tp.Variance = ast.VarianceOut
		}
		if !p.atIdent() {
			p.errorf(p.cur().Span.Start, "expected type parameter name, found %q", p.cur().Text)
			break
		}
		tp.Name = ast.NewIdent(p.next().Text)
		if p.accept("extends") {
			tp.Constraint = p.parseTypeNoCond()
		}
		if p.accept("=") {
			tp.Default = p.parseType()
		}
		tp.Loc = p.span(start)
		out = append(out, tp)
		if !p.accept(",") {
			break
		}
	}
	p.expectGT()
	return out
}

func (p *parser) parseTypeArgs() []ast.Type {
	if !p.at("<") {
		return nil
	}
	p.next()
	var out []ast.Type
	for !p.at(">") && !p.atEOF() {
		out = append(out, p.parseType())
		if !p.accept(",") {
			break
		}
	}
	p.expectGT()
	return out
}

func (p *parser) parseParams() []*ast.Param {
	p.expect("(")
	var out []*ast.Param
	for !p.at(")") && !p.atEOF() {
		start := p.cur().Span.Start
		prm := &ast.Param{}
		if p.accept("...") {
			prm.Rest = true
		}
		if p.cur().Kind == Ident || p.cur().Kind == Str {
			prm.Name = ast.NewIdent(p.next().Text)
		} else if p.at("{") || p.at("[") {
			// Destructuring patterns carry no useful name for a type surface;
			// preserve their text so the signature still prints.
			prm.Name = ast.NewIdent(p.captureBracketed())
		} else {
			p.errorf(p.cur().Span.Start, "expected parameter name, found %q", p.cur().Text)
			break
		}
		if p.accept("?") {
			prm.Optional = true
		}
		if p.accept(":") {
			prm.Type = p.parseType()
		}
		prm.Loc = p.span(start)
		out = append(out, prm)
		if !p.accept(",") {
			break
		}
	}
	p.expect(")")
	return out
}

// captureBracketed consumes a balanced bracket group and returns its raw text.
func (p *parser) captureBracketed() string {
	var b strings.Builder
	depth := 0
	for !p.atEOF() {
		t := p.cur()
		if t.Kind == Punct {
			switch t.Text {
			case "{", "[", "(":
				depth++
			case "}", "]", ")":
				depth--
			}
		}
		b.WriteString(t.Raw)
		p.next()
		if depth == 0 {
			break
		}
	}
	return b.String()
}

func (p *parser) parseUnion() ast.Type {
	start := p.cur().Span.Start
	p.accept("|") // leading delimiter
	first := p.parseIntersection()
	if !p.at("|") {
		return first
	}
	types := []ast.Type{first}
	for p.accept("|") {
		types = append(types, p.parseIntersection())
	}
	u := &ast.UnionType{Types: types}
	u.Loc = p.span(start)
	return u
}

func (p *parser) parseIntersection() ast.Type {
	start := p.cur().Span.Start
	p.accept("&")
	first := p.parseTypeOperator()
	if !p.at("&") {
		return first
	}
	types := []ast.Type{first}
	for p.accept("&") {
		types = append(types, p.parseTypeOperator())
	}
	i := &ast.IntersectionType{Types: types}
	i.Loc = p.span(start)
	return i
}

func (p *parser) parseTypeOperator() ast.Type {
	start := p.cur().Span.Start
	switch {
	case p.at("keyof"):
		p.next()
		t := &ast.TypeOperator{Op: ast.OpKeyOf, Type: p.parseTypeOperator()}
		t.Loc = p.span(start)
		return t
	case p.at("unique"):
		p.next()
		t := &ast.TypeOperator{Op: ast.OpUnique, Type: p.parseTypeOperator()}
		t.Loc = p.span(start)
		return t
	case p.at("readonly"):
		p.next()
		t := &ast.TypeOperator{Op: ast.OpReadonly, Type: p.parseTypeOperator()}
		t.Loc = p.span(start)
		return t
	case p.at("infer"):
		p.next()
		tp := &ast.TypeParam{}
		if p.atIdent() {
			tp.Name = ast.NewIdent(p.next().Text)
		} else {
			p.errorf(p.cur().Span.Start, "expected name after 'infer'")
			tp.Name = ast.NewIdent("_")
		}
		// An 'extends' after 'infer X' is always the constraint, never the
		// start of a conditional: the enclosing conditional has already
		// consumed its own 'extends'. Parsing the constraint without
		// conditionals leaves any following '?' to that enclosing type.
		if p.accept("extends") {
			tp.Constraint = p.parseTypeNoCond()
		}
		t := &ast.InferType{Param: tp}
		t.Loc = p.span(start)
		return t
	}
	return p.parsePostfix()
}

func (p *parser) parsePostfix() ast.Type {
	start := p.cur().Span.Start
	t := p.parsePrimary()
	for {
		// A line break before '[' ends the type, matching ASI behaviour.
		if !p.at("[") || p.cur().NewlineBefore {
			return t
		}
		p.next()
		if p.accept("]") {
			a := &ast.ArrayType{Elem: t}
			a.Loc = p.span(start)
			t = a
			continue
		}
		idx := p.parseType()
		p.expect("]")
		ia := &ast.IndexedAccessType{Object: t, Index: idx}
		ia.Loc = p.span(start)
		t = ia
	}
}

func (p *parser) parsePrimary() ast.Type {
	start := p.cur().Span.Start
	t := p.cur()

	switch t.Kind {
	case Str:
		p.next()
		lit := ast.StringLiteral(t.Text)
		lit.Loc = p.span(start)
		return lit
	case Num:
		p.next()
		lit := ast.NumberLiteral(t.Raw)
		lit.Loc = p.span(start)
		return lit
	case BigIntTok:
		p.next()
		lit := &ast.LiteralType{LitKind: ast.LitBigInt, Value: t.Raw}
		lit.Loc = p.span(start)
		return lit
	case NoSubTmpl:
		p.next()
		tl := &ast.TemplateLiteralType{Quasis: []string{t.Text}}
		tl.Loc = p.span(start)
		return tl
	case TmplHead:
		return p.parseTemplateLiteralType()
	}

	if t.Kind == Punct {
		switch t.Text {
		case "(":
			p.next()
			inner := p.parseType()
			p.expect(")")
			pt := &ast.ParenType{Inner: inner}
			pt.Loc = p.span(start)
			return pt
		case "{":
			return p.parseObjectOrMapped()
		case "[":
			return p.parseTuple()
		case "-":
			// Negative numeric literal type.
			p.next()
			if p.cur().Kind == Num || p.cur().Kind == BigIntTok {
				n := p.next()
				kind := ast.LitNumber
				if n.Kind == BigIntTok {
					kind = ast.LitBigInt
				}
				lit := &ast.LiteralType{LitKind: kind, Value: n.Raw, Negated: true}
				lit.Loc = p.span(start)
				return lit
			}
			p.errorf(start, "expected numeric literal after '-'")
			return ast.Never
		}
	}

	if t.Kind == Ident {
		switch t.Text {
		case "true", "false":
			p.next()
			lit := ast.BoolLiteral(t.Text == "true")
			lit.Loc = p.span(start)
			return lit
		case "null":
			p.next()
			return ast.Null
		case "this":
			p.next()
			th := &ast.ThisType{}
			th.Loc = p.span(start)
			return th
		case "typeof":
			p.next()
			if p.at("import") {
				it := p.parseImportType()
				if imp, ok := it.(*ast.ImportType); ok {
					imp.TypeOf = true
				}
				return it
			}
			q := &ast.TypeQuery{Name: p.parseEntityName()}
			q.Args = p.parseTypeArgs()
			q.Loc = p.span(start)
			return q
		case "import":
			return p.parseImportType()
		}
		if kw, ok := keywordTypes[t.Text]; ok {
			// A keyword is only intrinsic when not followed by '.', which
			// would make it a namespace qualifier.
			if p.peek(1).Text != "." {
				p.next()
				k := &ast.KeywordType{Keyword: kw}
				k.Loc = p.span(start)
				return k
			}
		}
		ref := &ast.TypeRef{Name: p.parseEntityName()}
		ref.Args = p.parseTypeArgs()
		ref.Loc = p.span(start)
		return ref
	}

	p.errorf(start, "unexpected %q in type", t.Text)
	p.next()
	return ast.Unknown
}

func (p *parser) parseEntityName() ast.EntityName {
	if !p.atIdent() {
		p.errorf(p.cur().Span.Start, "expected identifier, found %q", p.cur().Text)
		return ast.NewIdent("unknown")
	}
	var name ast.EntityName = ast.NewIdent(p.next().Text)
	for p.at(".") && p.peek(1).Kind == Ident {
		p.next()
		name = &ast.QualifiedName{Left: name, Right: ast.NewIdent(p.next().Text)}
	}
	return name
}

func (p *parser) parseImportType() ast.Type {
	start := p.cur().Span.Start
	p.expect("import")
	p.expect("(")
	mod := ""
	if p.cur().Kind == Str {
		mod = p.next().Text
	} else {
		p.errorf(p.cur().Span.Start, "expected module specifier string")
	}
	var attrs []ast.ImportAttribute
	if p.accept(",") {
		attrs = p.parseImportAttrObject()
	}
	p.expect(")")
	it := &ast.ImportType{Module: mod, Attributes: attrs}
	if p.accept(".") {
		it.Qualifier = p.parseEntityName()
	}
	it.Args = p.parseTypeArgs()
	it.Loc = p.span(start)
	return it
}

// parseImportAttrObject reads the { with: { k: "v" } } form.
func (p *parser) parseImportAttrObject() []ast.ImportAttribute {
	var out []ast.ImportAttribute
	if !p.accept("{") {
		return nil
	}
	for !p.at("}") && !p.atEOF() {
		if p.at("with") || p.at("assert") {
			p.next()
			p.accept(":")
			out = append(out, p.parseAttrEntries()...)
		} else {
			p.next()
		}
		if !p.accept(",") {
			break
		}
	}
	p.expect("}")
	return out
}

func (p *parser) parseAttrEntries() []ast.ImportAttribute {
	var out []ast.ImportAttribute
	if !p.accept("{") {
		return nil
	}
	for !p.at("}") && !p.atEOF() {
		key := p.next().Text
		p.expect(":")
		val := ""
		if p.cur().Kind == Str {
			val = p.next().Text
		}
		out = append(out, ast.ImportAttribute{Key: key, Value: val})
		if !p.accept(",") {
			break
		}
	}
	p.expect("}")
	return out
}

func (p *parser) parseTemplateLiteralType() ast.Type {
	start := p.cur().Span.Start
	tl := &ast.TemplateLiteralType{}
	head := p.next() // TmplHead
	tl.Quasis = append(tl.Quasis, head.Text)
	for {
		tl.Types = append(tl.Types, p.parseType())
		t := p.cur()
		switch t.Kind {
		case TmplMiddle:
			p.next()
			tl.Quasis = append(tl.Quasis, t.Text)
		case TmplTail:
			p.next()
			tl.Quasis = append(tl.Quasis, t.Text)
			tl.Loc = p.span(start)
			return tl
		default:
			p.errorf(t.Span.Start, "unterminated template literal type")
			tl.Quasis = append(tl.Quasis, "")
			tl.Loc = p.span(start)
			return tl
		}
	}
}

func (p *parser) parseTuple() ast.Type {
	start := p.cur().Span.Start
	p.expect("[")
	tt := &ast.TupleType{}
	for !p.at("]") && !p.atEOF() {
		elemStart := p.cur().Span.Start
		rest := p.accept("...")

		// A labelled element is `name:` or `name?:`.
		if p.cur().Kind == Ident &&
			(p.peek(1).Text == ":" || (p.peek(1).Text == "?" && p.peek(2).Text == ":")) {
			name := ast.NewIdent(p.next().Text)
			opt := p.accept("?")
			p.expect(":")
			m := &ast.NamedTupleMember{Name: name, Optional: opt, Rest: rest, Type: p.parseType()}
			m.Loc = p.span(elemStart)
			tt.Elems = append(tt.Elems, m)
		} else {
			t := p.parseType()
			if p.accept("?") {
				o := &ast.OptionalType{Elem: t}
				o.Loc = p.span(elemStart)
				t = o
			}
			if rest {
				r := &ast.RestType{Elem: t}
				r.Loc = p.span(elemStart)
				t = r
			}
			tt.Elems = append(tt.Elems, t)
		}
		if !p.accept(",") {
			break
		}
	}
	p.expect("]")
	tt.Loc = p.span(start)
	return tt
}

// isMappedType reports whether the object type starting at '{' is a mapped
// type, by looking for `[ Ident in` past any modifier prefix.
func (p *parser) isMappedType() bool {
	i := p.pos + 1
	for i < len(p.toks) {
		switch p.toks[i].Text {
		case "+", "-", "readonly":
			i++
			continue
		}
		break
	}
	return i+2 < len(p.toks) &&
		p.toks[i].Text == "[" &&
		p.toks[i+1].Kind == Ident &&
		p.toks[i+2].Text == "in"
}

func (p *parser) parseObjectOrMapped() ast.Type {
	if p.isMappedType() {
		return p.parseMapped()
	}
	start := p.cur().Span.Start
	p.expect("{")
	lit := &ast.TypeLiteral{}
	for !p.at("}") && !p.atEOF() {
		m := p.parseMember()
		if m != nil {
			lit.Members = append(lit.Members, m)
		}
		if !p.accept(";") && !p.accept(",") {
			if !p.at("}") {
				// Members may also be newline-separated; only complain when
				// nothing separates them at all.
				if !p.cur().NewlineBefore {
					p.errorf(p.cur().Span.Start, "expected ';' between members, found %q", p.cur().Text)
					p.next()
				}
			}
		}
	}
	p.expect("}")
	lit.Loc = p.span(start)
	return lit
}

func (p *parser) parseMapped() ast.Type {
	start := p.cur().Span.Start
	p.expect("{")
	m := &ast.MappedType{}
	switch {
	case p.at("+") && p.peek(1).Text == "readonly":
		p.next()
		p.next()
		m.ReadonlyMod = ast.MappedAdd
	case p.at("-") && p.peek(1).Text == "readonly":
		p.next()
		p.next()
		m.ReadonlyMod = ast.MappedRemove
	case p.at("readonly"):
		p.next()
		m.ReadonlyMod = ast.MappedAdd
	}
	p.expect("[")
	tp := &ast.TypeParam{Name: ast.NewIdent(p.next().Text)}
	p.expect("in")
	tp.Constraint = p.parseType()
	m.Param = tp
	if p.accept("as") {
		m.As = p.parseType()
	}
	p.expect("]")
	switch {
	case p.at("+") && p.peek(1).Text == "?":
		p.next()
		p.next()
		m.OptionalMod = ast.MappedAdd
	case p.at("-") && p.peek(1).Text == "?":
		p.next()
		p.next()
		m.OptionalMod = ast.MappedRemove
	case p.at("?"):
		p.next()
		m.OptionalMod = ast.MappedAdd
	}
	if p.accept(":") {
		m.Type = p.parseType()
	}
	p.accept(";")
	p.accept(",")
	p.expect("}")
	m.Loc = p.span(start)
	return m
}

// ---------------------------------------------------------------------------
// Members
// ---------------------------------------------------------------------------

func (p *parser) parseMember() ast.Member {
	start := p.cur().Span.Start
	docs := docFromToken(p.cur())

	// Construct signature.
	if p.at("new") && (p.peek(1).Text == "(" || p.peek(1).Text == "<") {
		p.next()
		sig := p.parseSignature(":")
		m := &ast.ConstructSignature{Signature: sig, Docs: docs}
		m.Loc = p.span(start)
		return m
	}

	// Call signature.
	if p.at("(") || p.at("<") {
		sig := p.parseSignature(":")
		m := &ast.CallSignature{Signature: sig, Docs: docs}
		m.Loc = p.span(start)
		return m
	}

	readonly := false
	if p.at("readonly") && p.peek(1).Text != ":" && p.peek(1).Text != "?" &&
		p.peek(1).Text != "(" && p.peek(1).Text != ";" && p.peek(1).Text != "}" {
		p.next()
		readonly = true
	}

	// Index signature: '[' Ident ':' Type ']'.
	if p.at("[") && p.peek(1).Kind == Ident && p.peek(2).Text == ":" {
		p.next()
		keyName := p.next().Text
		p.expect(":")
		keyType := p.parseType()
		p.expect("]")
		p.expect(":")
		m := &ast.IndexSignature{
			KeyName: keyName, KeyType: keyType, Type: p.parseType(),
			Readonly: readonly, Docs: docs,
		}
		m.Loc = p.span(start)
		return m
	}

	// Accessors.
	if (p.at("get") || p.at("set")) && p.peek(1).Kind != Punct {
		isGet := p.at("get")
		p.next()
		name := p.parsePropertyName()
		sig := p.parseSignature(":")
		if isGet {
			m := &ast.GetAccessor{Name: name, Signature: sig, Docs: docs}
			m.Loc = p.span(start)
			return m
		}
		m := &ast.SetAccessor{Name: name, Signature: sig, Docs: docs}
		m.Loc = p.span(start)
		return m
	}

	name := p.parsePropertyName()
	optional := p.accept("?")

	// Method.
	if p.at("(") || p.at("<") {
		sig := p.parseSignature(":")
		m := &ast.MethodSignature{Name: name, Optional: optional, Signature: sig, Docs: docs}
		m.Loc = p.span(start)
		return m
	}

	prop := &ast.PropertySignature{
		Name: name, Optional: optional, Readonly: readonly, Docs: docs,
	}
	if p.accept(":") {
		prop.Type = p.parseType()
	}
	prop.Loc = p.span(start)
	return prop
}

func (p *parser) parsePropertyName() ast.PropertyName {
	t := p.cur()
	switch t.Kind {
	case Ident:
		p.next()
		return ast.NewIdent(t.Text)
	case Str:
		p.next()
		return &ast.StringLit{Value: t.Text}
	case Num:
		p.next()
		return &ast.NumberLit{Text: t.Raw}
	}
	if t.Text == "[" {
		p.next()
		// Computed names hold expressions, which this frontend does not parse;
		// preserve the text so the member still prints correctly.
		var b strings.Builder
		depth := 1
		for !p.atEOF() && depth > 0 {
			tk := p.next()
			if tk.Text == "[" {
				depth++
			} else if tk.Text == "]" {
				depth--
				if depth == 0 {
					break
				}
			}
			b.WriteString(tk.Raw)
		}
		return &ast.ComputedName{Expr: &ast.RawExpr{Text: b.String()}}
	}
	p.errorf(t.Span.Start, "expected property name, found %q", t.Text)
	p.next()
	return ast.NewIdent("unknown")
}

// docFromToken converts a token's leading JSDoc blocks into a Doc.
func docFromToken(t Token) *ast.Doc {
	if len(t.Docs) == 0 {
		return nil
	}
	d := &ast.Doc{}
	for _, block := range t.Docs {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "*")
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "@") {
				rest := strings.TrimPrefix(line, "@")
				name, text, _ := strings.Cut(rest, " ")
				d.Tags = append(d.Tags, ast.DocTag{Name: name, Text: strings.TrimSpace(text)})
				continue
			}
			if line == "" && len(d.Text) == 0 {
				continue
			}
			d.Text = append(d.Text, line)
		}
	}
	for len(d.Text) > 0 && d.Text[len(d.Text)-1] == "" {
		d.Text = d.Text[:len(d.Text)-1]
	}
	if d.IsEmpty() {
		return nil
	}
	return d
}

// ---------------------------------------------------------------------------
// Files and declarations
// ---------------------------------------------------------------------------

func (p *parser) parseFile(name string) *ast.SourceFile {
	f := &ast.SourceFile{Name: name, ScriptKind: ast.ScriptTS}
	if strings.HasSuffix(name, ".d.ts") {
		f.ScriptKind = ast.ScriptDTS
	} else if strings.HasSuffix(name, ".tsx") {
		f.ScriptKind = ast.ScriptTSX
	}
	for !p.atEOF() {
		before := p.pos
		s := p.parseStmt()
		if s != nil {
			f.Stmts = append(f.Stmts, s)
		}
		if p.pos == before {
			p.next() // guarantee progress
		}
	}
	return f
}

func (p *parser) parseStmt() ast.Stmt {
	start := p.cur().Span.Start
	docs := docFromToken(p.cur())
	mods := ast.Modifier(0)

	save := p.pos
	if p.at("export") {
		p.next()
		if p.at("default") || p.at("{") || p.at("*") || p.at("=") {
			p.pos = save
			return p.parseExport()
		}
		mods = mods.With(ast.ModExport)
	}
	if p.at("declare") {
		p.next()
		mods = mods.With(ast.ModDeclare)
	}

	switch {
	case p.at("import"):
		if mods != 0 {
			p.pos = save
		}
		return p.parseImport()

	case p.at("interface"):
		p.next()
		d := &ast.InterfaceDecl{Mods: mods, Docs: docs}
		d.Name = ast.NewIdent(p.expectIdent("interface name"))
		d.TypeParams = p.parseTypeParams()
		if p.accept("extends") {
			for {
				h := &ast.Heritage{Name: p.parseEntityName(), Args: p.parseTypeArgs()}
				d.Extends = append(d.Extends, h)
				if !p.accept(",") {
					break
				}
			}
		}
		if lit, ok := p.parseObjectOrMapped().(*ast.TypeLiteral); ok {
			d.Members = lit.Members
		}
		d.Loc = p.span(start)
		return d

	case p.at("type"):
		p.next()
		d := &ast.TypeAliasDecl{Mods: mods, Docs: docs}
		d.Name = ast.NewIdent(p.expectIdent("type alias name"))
		d.TypeParams = p.parseTypeParams()
		p.expect("=")
		d.Type = p.parseType()
		p.accept(";")
		d.Loc = p.span(start)
		return d

	case p.at("enum") || (p.at("const") && p.peek(1).Text == "enum"):
		if p.accept("const") {
			mods = mods.With(ast.ModConst)
		}
		p.expect("enum")
		d := &ast.EnumDecl{Mods: mods, Docs: docs}
		d.Name = ast.NewIdent(p.expectIdent("enum name"))
		p.expect("{")
		for !p.at("}") && !p.atEOF() {
			mDocs := docFromToken(p.cur())
			em := &ast.EnumMember{Name: p.parsePropertyName(), Docs: mDocs}
			if p.accept("=") {
				em.Value = p.parseEnumValue()
			}
			d.Members = append(d.Members, em)
			if !p.accept(",") {
				break
			}
		}
		p.expect("}")
		d.Loc = p.span(start)
		return d

	case p.at("namespace") || p.at("module"):
		isModule := p.at("module")
		p.next()
		d := &ast.ModuleDecl{Mods: mods, Docs: docs, ModuleKind: ast.ModuleNamespace}
		if isModule && p.cur().Kind == Str {
			d.ModuleKind = ast.ModuleAmbient
			d.Name = p.next().Text
		} else {
			var parts []string
			parts = append(parts, p.expectIdent("namespace name"))
			for p.accept(".") {
				parts = append(parts, p.expectIdent("namespace name"))
			}
			d.Name = strings.Join(parts, ".")
		}
		p.expect("{")
		for !p.at("}") && !p.atEOF() {
			before := p.pos
			if s := p.parseStmt(); s != nil {
				d.Body = append(d.Body, s)
			}
			if p.pos == before {
				p.next()
			}
		}
		p.expect("}")
		d.Loc = p.span(start)
		return d

	case p.at("global") && p.peek(1).Text == "{":
		p.next()
		d := &ast.ModuleDecl{Mods: mods, Docs: docs, ModuleKind: ast.ModuleGlobal}
		p.expect("{")
		for !p.at("}") && !p.atEOF() {
			before := p.pos
			if s := p.parseStmt(); s != nil {
				d.Body = append(d.Body, s)
			}
			if p.pos == before {
				p.next()
			}
		}
		p.expect("}")
		d.Loc = p.span(start)
		return d
	}

	// Anything else is preserved verbatim rather than dropped, so a partially
	// understood file still round-trips.
	p.pos = save
	return p.captureRawStmt()
}

func (p *parser) parseEnumValue() ast.Expr {
	t := p.cur()
	switch t.Kind {
	case Str:
		p.next()
		return &ast.StringLit{Value: t.Text}
	case Num:
		p.next()
		return &ast.NumberLit{Text: t.Raw}
	}
	if t.Text == "-" && p.peek(1).Kind == Num {
		p.next()
		n := p.next()
		return &ast.NumberLit{Text: "-" + n.Raw}
	}
	// Computed enum values are expressions; keep their text.
	var b strings.Builder
	for !p.atEOF() && !p.at(",") && !p.at("}") {
		b.WriteString(p.next().Raw)
	}
	return &ast.RawExpr{Text: strings.TrimSpace(b.String())}
}

func (p *parser) expectIdent(what string) string {
	if p.atIdent() {
		return p.next().Text
	}
	p.errorf(p.cur().Span.Start, "expected %s, found %q", what, p.cur().Text)
	return "unknown"
}

func (p *parser) parseImport() ast.Stmt {
	start := p.cur().Span.Start
	docs := docFromToken(p.cur())
	p.expect("import")
	d := &ast.ImportDecl{Docs: docs}

	if p.at("type") && p.peek(1).Text != "from" && p.peek(1).Text != "," {
		p.next()
		d.TypeOnly = true
	}
	switch {
	case p.cur().Kind == Str: // side-effect import
		d.Module = p.next().Text
		p.accept(";")
		d.Loc = p.span(start)
		return d
	case p.at("*"):
		p.next()
		p.expect("as")
		d.Namespace = p.expectIdent("namespace binding")
	case p.at("{"):
		d.Named = p.parseImportSpecs()
	default:
		d.Default = p.expectIdent("default binding")
		if p.accept(",") {
			if p.at("*") {
				p.next()
				p.expect("as")
				d.Namespace = p.expectIdent("namespace binding")
			} else {
				d.Named = p.parseImportSpecs()
			}
		}
	}
	p.expect("from")
	if p.cur().Kind == Str {
		d.Module = p.next().Text
	} else {
		p.errorf(p.cur().Span.Start, "expected module specifier")
	}
	if p.at("with") || p.at("assert") {
		p.next()
		d.Attributes = p.parseAttrEntries()
	}
	p.accept(";")
	d.Loc = p.span(start)
	return d
}

func (p *parser) parseImportSpecs() []ast.ImportSpec {
	p.expect("{")
	var out []ast.ImportSpec
	for !p.at("}") && !p.atEOF() {
		var sp ast.ImportSpec
		if p.at("type") && p.peek(1).Kind == Ident && p.peek(1).Text != "as" {
			p.next()
			sp.TypeOnly = true
		}
		if p.cur().Kind == Str {
			sp.Name = p.next().Text
		} else {
			sp.Name = p.expectIdent("import specifier")
		}
		if p.accept("as") {
			sp.Alias = p.expectIdent("import alias")
		}
		out = append(out, sp)
		if !p.accept(",") {
			break
		}
	}
	p.expect("}")
	return out
}

func (p *parser) parseExport() ast.Stmt {
	start := p.cur().Span.Start
	docs := docFromToken(p.cur())
	p.expect("export")

	if p.accept("default") {
		var b strings.Builder
		for !p.atEOF() && !p.at(";") {
			b.WriteString(p.next().Raw)
			if !p.at(";") && !p.atEOF() {
				b.WriteString(" ")
			}
		}
		p.accept(";")
		d := &ast.ExportAssign{Expr: &ast.RawExpr{Text: strings.TrimSpace(b.String())}, Default: true, Docs: docs}
		d.Loc = p.span(start)
		return d
	}
	if p.accept("=") {
		name := p.expectIdent("export target")
		p.accept(";")
		d := &ast.ExportAssign{Expr: ast.NewIdent(name), Docs: docs}
		d.Loc = p.span(start)
		return d
	}

	d := &ast.ExportDecl{Docs: docs}
	if p.at("type") && (p.peek(1).Text == "{" || p.peek(1).Text == "*") {
		p.next()
		d.TypeOnly = true
	}
	switch {
	case p.at("*"):
		p.next()
		d.Star = true
		if p.accept("as") {
			d.StarAs = p.expectIdent("export alias")
		}
	default:
		d.Named = p.parseImportSpecs()
	}
	if p.accept("from") {
		if p.cur().Kind == Str {
			d.Module = p.next().Text
		} else {
			p.errorf(p.cur().Span.Start, "expected module specifier")
		}
	}
	p.accept(";")
	d.Loc = p.span(start)
	return d
}

// captureRawStmt consumes an unrecognized construct, preserving its text.
func (p *parser) captureRawStmt() ast.Stmt {
	start := p.cur().Span.Start
	p.errorf(start, "unsupported construct %q; preserved verbatim (this frontend parses types and declarations only)", p.cur().Text)

	depth := 0
	var b strings.Builder
	for !p.atEOF() {
		t := p.cur()
		if t.Kind == Punct {
			switch t.Text {
			case "{", "(", "[":
				depth++
			case "}", ")", "]":
				if depth == 0 {
					goto done
				}
				depth--
			case ";":
				if depth == 0 {
					b.WriteString(";")
					p.next()
					goto done
				}
			}
		}
		if b.Len() > 0 && t.NewlineBefore && depth == 0 {
			goto done
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(t.Raw)
		p.next()
	}
done:
	s := &ast.RawStmt{Text: strings.TrimSpace(b.String())}
	s.Loc = p.span(start)
	return s
}
