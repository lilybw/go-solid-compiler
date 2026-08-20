package printer

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lilybw/go-solid-compiler/ast"
)

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

// node emits any node, dispatching to the category-specific emitter.
func (p *Printer) node(n ast.Node) {
	switch x := n.(type) {
	case nil:
	case *ast.SourceFile:
		p.file(x)
	case ast.Type:
		p.Type(x)
	case ast.Member:
		p.member(x)
	case ast.Stmt:
		p.stmt(x)
	case ast.Expr:
		p.expr(x, precLowest)
	case ast.PropertyName:
		p.propName(x)
	case *ast.TypeParam:
		p.typeParams([]*ast.TypeParam{x})
	case *ast.Param:
		p.param(x)
	case *ast.Heritage:
		p.heritage(x)
	default:
		p.w(fmt.Sprintf("/* unprintable node %T */", n))
	}
}

// ---------------------------------------------------------------------------
// Source files
// ---------------------------------------------------------------------------

func (p *Printer) file(f *ast.SourceFile) {
	for _, l := range p.opts.Banner {
		p.w(l)
		p.nl()
	}
	if len(p.opts.Banner) > 0 && (len(f.Leading) > 0 || len(f.Stmts) > 0) {
		p.nl()
	}
	for _, l := range f.Leading {
		p.w(l)
		p.nl()
	}
	if len(f.Leading) > 0 && len(f.Stmts) > 0 {
		p.nl()
	}
	for i, s := range f.Stmts {
		if i > 0 && needsBlankLine(f.Stmts[i-1], s) {
			p.nl()
		}
		p.stmt(s)
		p.nl()
	}
}

// needsBlankLine reports whether a blank line belongs between two top-level
// statements.
func needsBlankLine(prev, next ast.Stmt) bool {
	_, pi := prev.(*ast.ImportDecl)
	_, ni := next.(*ast.ImportDecl)
	if pi && ni {
		return false
	}
	// Consecutive re-export clauses (as opposed to exported declarations)
	// read as a block too.
	pe, pok := prev.(*ast.ExportDecl)
	ne, nok := next.(*ast.ExportDecl)
	if pok && nok && pe.Decl == nil && ne.Decl == nil {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Members
// ---------------------------------------------------------------------------

func (p *Printer) member(m ast.Member) {
	sep := p.opts.memberSepText()
	switch x := m.(type) {
	case *ast.PropertySignature:
		p.docs(x.Docs)
		if x.Readonly {
			p.w("readonly ")
		}
		p.propName(x.Name)
		if x.Optional {
			p.w("?")
		}
		if x.Type != nil {
			p.w(": ")
			p.typeAt(x.Type, tpLowest)
		}
		p.w(sep)

	case *ast.MethodSignature:
		p.docs(x.Docs)
		p.propName(x.Name)
		if x.Optional {
			p.w("?")
		}
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		if x.Return != nil {
			p.w(": ")
			p.typeAt(x.Return, tpLowest)
		}
		p.w(sep)

	case *ast.IndexSignature:
		p.docs(x.Docs)
		if x.Static {
			p.w("static ")
		}
		if x.Readonly {
			p.w("readonly ")
		}
		key := x.KeyName
		if key == "" {
			key = "key"
		}
		p.w("[" + key + ": ")
		p.typeAt(x.KeyType, tpLowest)
		p.w("]: ")
		p.typeAt(x.Type, tpLowest)
		p.w(sep)

	case *ast.CallSignature:
		p.docs(x.Docs)
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		if x.Return != nil {
			p.w(": ")
			p.typeAt(x.Return, tpLowest)
		}
		p.w(sep)

	case *ast.ConstructSignature:
		p.docs(x.Docs)
		p.w("new ")
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		if x.Return != nil {
			p.w(": ")
			p.typeAt(x.Return, tpLowest)
		}
		p.w(sep)

	case *ast.GetAccessor:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.w("get ")
		p.propName(x.Name)
		p.params(x.Params)
		if x.Return != nil {
			p.w(": ")
			p.typeAt(x.Return, tpLowest)
		}
		p.bodyOrSemi(x.Body)

	case *ast.SetAccessor:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.w("set ")
		p.propName(x.Name)
		p.params(x.Params)
		p.bodyOrSemi(x.Body)

	case *ast.PropertyDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.propName(x.Name)
		if x.Optional {
			p.w("?")
		} else if x.Definite {
			p.w("!")
		}
		if x.Type != nil {
			p.w(": ")
			p.typeAt(x.Type, tpLowest)
		}
		if x.Value != nil {
			p.w(" = ")
			p.expr(x.Value, precAssign)
		}
		p.w(p.opts.semi())

	case *ast.MethodDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.propName(x.Name)
		if x.Optional {
			p.w("?")
		}
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		if x.Return != nil {
			p.w(": ")
			p.typeAt(x.Return, tpLowest)
		}
		p.bodyOrSemi(x.Body)

	default:
		p.w(fmt.Sprintf("/* unprintable member %T */", m))
	}
}

func (p *Printer) bodyOrSemi(b *ast.BlockStmt) {
	if b == nil {
		p.w(p.opts.semi())
		return
	}
	p.w(" ")
	p.block(b)
}

// ---------------------------------------------------------------------------
// Statements and declarations
// ---------------------------------------------------------------------------

func (p *Printer) stmt(s ast.Stmt) {
	switch x := s.(type) {
	case nil:

	case *ast.InterfaceDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.w("interface ")
		p.w(x.Name.Text)
		p.typeParams(x.TypeParams)
		if len(x.Extends) > 0 {
			p.w(" extends ")
			for i, h := range x.Extends {
				if i > 0 {
					p.w(", ")
				}
				p.heritage(h)
			}
		}
		p.w(" ")
		p.typeMembers(x.Members)

	case *ast.TypeAliasDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.w("type ")
		p.w(x.Name.Text)
		p.typeParams(x.TypeParams)
		p.w(" = ")
		p.typeAt(x.Type, tpLowest)
		p.w(p.opts.semi())

	case *ast.EnumDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods.Without(ast.ModConst))
		if x.Mods.Has(ast.ModConst) {
			p.w("const ")
		}
		p.w("enum ")
		p.w(x.Name.Text)
		p.w(" {")
		p.in()
		for _, m := range x.Members {
			p.nl()
			p.docs(m.Docs)
			p.propName(m.Name)
			if m.Value != nil {
				p.w(" = ")
				p.expr(m.Value, precAssign)
			}
			p.w(",")
		}
		p.out()
		p.nl()
		p.w("}")

	case *ast.ClassDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.w("class")
		if x.Name != nil {
			p.w(" ")
			p.w(x.Name.Text)
		}
		p.typeParams(x.TypeParams)
		if x.Extends != nil {
			p.w(" extends ")
			p.heritage(x.Extends)
		}
		if len(x.Implements) > 0 {
			p.w(" implements ")
			for i, h := range x.Implements {
				if i > 0 {
					p.w(", ")
				}
				p.heritage(h)
			}
		}
		p.w(" {")
		if len(x.Members) > 0 {
			p.in()
			for _, m := range x.Members {
				p.nl()
				p.member(m)
			}
			p.out()
			p.nl()
		}
		p.w("}")

	case *ast.FunctionDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.w("function")
		if x.Generator {
			p.w("*")
		}
		p.w(" ")
		p.w(x.Name.Text)
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		if x.Return != nil {
			p.w(": ")
			p.typeAt(x.Return, tpLowest)
		}
		p.bodyOrSemi(x.Body)

	case *ast.VarDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		p.w(x.VarKind.String())
		p.w(" ")
		for i, b := range x.Bindings {
			if i > 0 {
				p.w(", ")
			}
			p.propName(b.Name)
			if b.Definite {
				p.w("!")
			}
			if b.Type != nil {
				p.w(": ")
				p.typeAt(b.Type, tpLowest)
			}
			if b.Value != nil {
				p.w(" = ")
				p.expr(b.Value, precAssign)
			}
		}
		p.w(p.opts.semi())

	case *ast.ModuleDecl:
		p.docs(x.Docs)
		p.modifiers(x.Mods)
		switch x.ModuleKind {
		case ast.ModuleGlobal:
			p.w("global")
		case ast.ModuleAmbient:
			p.w("module ")
			p.str(x.Name)
		default:
			p.w("namespace ")
			p.w(x.Name)
		}
		p.w(" {")
		if len(x.Body) > 0 {
			p.in()
			for i, s := range x.Body {
				if i > 0 && needsBlankLine(x.Body[i-1], s) {
					p.nl()
				}
				p.nl()
				p.stmt(s)
			}
			p.out()
			p.nl()
		}
		p.w("}")

	case *ast.ImportDecl:
		p.docs(x.Docs)
		p.importDecl(x)

	case *ast.ExportDecl:
		p.docs(x.Docs)
		p.exportDecl(x)

	case *ast.ExportAssign:
		p.docs(x.Docs)
		if x.Default {
			p.w("export default ")
		} else {
			p.w("export = ")
		}
		p.expr(x.Expr, precAssign)
		p.w(p.opts.semi())

	case *ast.BlockStmt:
		p.block(x)

	case *ast.ReturnStmt:
		p.w("return")
		if x.Value != nil {
			p.w(" ")
			p.expr(x.Value, precLowest)
		}
		p.w(p.opts.semi())

	case *ast.ExprStmt:
		p.expr(x.Expr, precLowest)
		p.w(p.opts.semi())

	case *ast.RawStmt:
		lines := strings.Split(x.Text, "\n")
		for i, l := range lines {
			if i > 0 {
				p.nl()
			}
			p.w(l)
		}

	default:
		p.w(fmt.Sprintf("/* unprintable statement %T */", s))
	}
}

func (p *Printer) heritage(h *ast.Heritage) {
	p.entity(h.Name)
	p.typeArgs(h.Args)
}

func (p *Printer) block(b *ast.BlockStmt) {
	p.w("{")
	if len(b.Stmts) > 0 {
		p.in()
		for _, s := range b.Stmts {
			p.nl()
			p.stmt(s)
		}
		p.out()
		p.nl()
	}
	p.w("}")
}

func (p *Printer) importDecl(x *ast.ImportDecl) {
	p.w("import ")
	if x.TypeOnly {
		p.w("type ")
	}
	wrote := false
	if x.Default != "" {
		p.w(x.Default)
		wrote = true
	}
	if x.Namespace != "" {
		if wrote {
			p.w(", ")
		}
		p.w("* as " + x.Namespace)
		wrote = true
	}
	if len(x.Named) > 0 {
		if wrote {
			p.w(", ")
		}
		p.specifiers(x.Named)
		wrote = true
	}
	if wrote {
		p.w(" from ")
	}
	p.str(x.Module)
	p.attrClause(x.Attributes)
	p.w(p.opts.semi())
}

func (p *Printer) exportDecl(x *ast.ExportDecl) {
	if x.Decl != nil {
		// Re-emit the inner declaration with the export bit set, so that
		// callers may wrap a declaration without mutating it.
		p.stmt(withExport(x.Decl))
		return
	}
	p.w("export ")
	if x.TypeOnly {
		p.w("type ")
	}
	switch {
	case x.Star && x.StarAs != "":
		p.w("* as " + x.StarAs)
	case x.Star:
		p.w("*")
	default:
		p.specifiers(x.Named)
	}
	if x.Module != "" {
		p.w(" from ")
		p.str(x.Module)
	}
	p.attrClause(x.Attributes)
	p.w(p.opts.semi())
}

// withExport returns a shallow copy of d with ModExport set, so printing has
// no side effect on the caller's tree.
func withExport(d ast.Decl) ast.Decl {
	switch x := d.(type) {
	case *ast.InterfaceDecl:
		c := *x
		c.Mods = c.Mods.With(ast.ModExport)
		return &c
	case *ast.TypeAliasDecl:
		c := *x
		c.Mods = c.Mods.With(ast.ModExport)
		return &c
	case *ast.EnumDecl:
		c := *x
		c.Mods = c.Mods.With(ast.ModExport)
		return &c
	case *ast.ClassDecl:
		c := *x
		c.Mods = c.Mods.With(ast.ModExport)
		return &c
	case *ast.FunctionDecl:
		c := *x
		c.Mods = c.Mods.With(ast.ModExport)
		return &c
	case *ast.VarDecl:
		c := *x
		c.Mods = c.Mods.With(ast.ModExport)
		return &c
	case *ast.ModuleDecl:
		c := *x
		c.Mods = c.Mods.With(ast.ModExport)
		return &c
	}
	return d
}

func (p *Printer) specifiers(specs []ast.ImportSpec) {
	one := func(q *Printer, s ast.ImportSpec) {
		if s.TypeOnly {
			q.w("type ")
		}
		q.w(s.Name)
		if s.Alias != "" {
			q.w(" as " + s.Alias)
		}
	}
	single := p.sub(func(q *Printer) {
		q.w("{ ")
		for i, s := range specs {
			if i > 0 {
				q.w(", ")
			}
			one(q, s)
		}
		q.w(" }")
	})
	if len(specs) == 0 {
		p.w("{}")
		return
	}
	if p.fits(utf8.RuneCountInString(single)) {
		p.w(single)
		return
	}
	p.w("{")
	p.in()
	for i, s := range specs {
		p.nl()
		one(p, s)
		if i < len(specs)-1 || p.opts.TrailingComma {
			p.w(",")
		}
	}
	p.out()
	p.nl()
	p.w("}")
}

func (p *Printer) attrClause(as []ast.ImportAttribute) {
	if len(as) == 0 {
		return
	}
	p.w(" with { ")
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
	p.w(" }")
}

// ---------------------------------------------------------------------------
// Expression precedence
// ---------------------------------------------------------------------------

type eprec int

const (
	precLowest eprec = iota
	precComma
	precAssign // assignment, arrow, yield
	precCond
	precNullish
	precLogOr
	precLogAnd
	precBitOr
	precBitXor
	precBitAnd
	precEquality
	precRelational // <, >, instanceof, in, as, satisfies
	precShift
	precAdditive
	precMultiplicative
	precExponent
	precUnary
	precPostfixExpr
	precCall
	precPrimaryExpr
)

var binaryPrec = map[string]eprec{
	"??": precNullish, "||": precLogOr, "&&": precLogAnd,
	"|": precBitOr, "^": precBitXor, "&": precBitAnd,
	"==": precEquality, "!=": precEquality, "===": precEquality, "!==": precEquality,
	"<": precRelational, ">": precRelational, "<=": precRelational, ">=": precRelational,
	"instanceof": precRelational, "in": precRelational,
	"<<": precShift, ">>": precShift, ">>>": precShift,
	"+": precAdditive, "-": precAdditive,
	"*": precMultiplicative, "/": precMultiplicative, "%": precMultiplicative,
	"**": precExponent,
}

func isAssignOp(op string) bool {
	switch op {
	case "=", "+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", ">>>=",
		"&=", "|=", "^=", "&&=", "||=", "??=":
		return true
	}
	return false
}

func exprPrec(e ast.Expr) eprec {
	switch x := e.(type) {
	case *ast.ArrowFunc:
		return precAssign
	case *ast.CondExpr:
		return precCond
	case *ast.BinaryExpr:
		if isAssignOp(x.Op) {
			return precAssign
		}
		if p, ok := binaryPrec[x.Op]; ok {
			return p
		}
		return precRelational
	case *ast.AsExpr, *ast.SatisfiesExpr:
		return precRelational
	case *ast.UnaryExpr:
		return precUnary
	case *ast.SpreadExpr:
		return precAssign
	case *ast.CallExpr, *ast.NewExpr, *ast.MemberExpr, *ast.IndexExpr, *ast.NonNullExpr:
		return precCall
	default:
		return precPrimaryExpr
	}
}

// expr emits e, parenthesizing when its precedence is looser than min.
func (p *Printer) expr(e ast.Expr, min eprec) {
	if e == nil {
		return
	}
	if exprPrec(e) < min {
		p.w("(")
		p.exprInner(e)
		p.w(")")
		return
	}
	p.exprInner(e)
}

func (p *Printer) exprInner(e ast.Expr) {
	switch x := e.(type) {
	case *ast.Ident:
		p.w(x.Text)
	case *ast.StringLit:
		p.str(x.Value)
	case *ast.NumberLit:
		p.w(x.Text)
	case *ast.BigIntLit:
		p.w(x.Text)
	case *ast.BoolLit:
		if x.Value {
			p.w("true")
		} else {
			p.w("false")
		}
	case *ast.NullLit:
		p.w("null")
	case *ast.UndefinedLit:
		p.w("undefined")

	case *ast.ArrayLit:
		single := p.sub(func(q *Printer) {
			q.w("[")
			for i, el := range x.Elems {
				if i > 0 {
					q.w(", ")
				}
				q.expr(el, precAssign)
			}
			q.w("]")
		})
		if len(x.Elems) == 0 || (p.fits(utf8.RuneCountInString(single)) && !strings.Contains(single, "\n")) {
			p.w(single)
			return
		}
		p.w("[")
		p.in()
		for i, el := range x.Elems {
			p.nl()
			p.expr(el, precAssign)
			if i < len(x.Elems)-1 || p.opts.TrailingComma {
				p.w(",")
			}
		}
		p.out()
		p.nl()
		p.w("]")

	case *ast.ObjectLit:
		p.objectLit(x)

	case *ast.CallExpr:
		p.expr(x.Callee, precCall)
		if x.Optional {
			p.w("?.")
		}
		p.typeArgs(x.TypeArgs)
		p.args(x.Args)

	case *ast.NewExpr:
		p.w("new ")
		p.expr(x.Callee, precCall)
		p.typeArgs(x.TypeArgs)
		p.args(x.Args)

	case *ast.MemberExpr:
		p.expr(x.Object, precCall)
		if x.Optional {
			p.w("?.")
		} else {
			p.w(".")
		}
		p.w(x.Prop.Text)

	case *ast.IndexExpr:
		p.expr(x.Object, precCall)
		if x.Optional {
			p.w("?.")
		}
		p.w("[")
		p.expr(x.Index, precLowest)
		p.w("]")

	case *ast.ArrowFunc:
		if x.Async {
			p.w("async ")
		}
		p.typeParams(x.TypeParams)
		p.params(x.Params)
		if x.Return != nil {
			p.w(": ")
			p.typeAt(x.Return, tpLowest)
		}
		p.w(" => ")
		switch {
		case x.Body != nil:
			p.block(x.Body)
		case x.Expr != nil:
			// An object literal body needs parentheses to avoid being parsed
			// as a block.
			if _, ok := x.Expr.(*ast.ObjectLit); ok {
				p.w("(")
				p.expr(x.Expr, precAssign)
				p.w(")")
			} else {
				p.expr(x.Expr, precAssign)
			}
		default:
			p.w("{}")
		}

	case *ast.AsExpr:
		p.expr(x.Expr, precRelational)
		p.w(" as ")
		if x.Const {
			p.w("const")
		} else {
			p.typeAt(x.Type, tpLowest)
		}

	case *ast.SatisfiesExpr:
		p.expr(x.Expr, precRelational)
		p.w(" satisfies ")
		p.typeAt(x.Type, tpLowest)

	case *ast.NonNullExpr:
		p.expr(x.Expr, precCall)
		p.w("!")

	case *ast.TemplateLit:
		if x.Tag != nil {
			p.expr(x.Tag, precCall)
		}
		p.w("`")
		for i, q := range x.Quasis {
			p.w(escapeTemplate(q))
			if i < len(x.Exprs) {
				p.w("${")
				p.expr(x.Exprs[i], precLowest)
				p.w("}")
			}
		}
		p.w("`")

	case *ast.UnaryExpr:
		p.w(x.Op)
		// Word operators and repeated sigils need a separating space.
		if isWordOp(x.Op) || needsUnarySpace(x.Op, x.Expr) {
			p.w(" ")
		}
		p.expr(x.Expr, precUnary)

	case *ast.BinaryExpr:
		prec := exprPrec(x)
		if isAssignOp(x.Op) {
			// Assignment is right-associative.
			p.expr(x.Left, precPostfixExpr)
			p.w(" " + x.Op + " ")
			p.expr(x.Right, precAssign)
			return
		}
		right := prec + 1
		if x.Op == "**" {
			right = prec // right-associative
			p.expr(x.Left, prec+1)
			p.w(" " + x.Op + " ")
			p.expr(x.Right, right)
			return
		}
		p.expr(x.Left, prec)
		p.w(" " + x.Op + " ")
		p.expr(x.Right, right)

	case *ast.CondExpr:
		p.expr(x.Cond, precCond+1)
		p.w(" ? ")
		p.expr(x.Then, precAssign)
		p.w(" : ")
		p.expr(x.Else, precAssign)

	case *ast.SpreadExpr:
		p.w("...")
		p.expr(x.Expr, precAssign)

	case *ast.RawExpr:
		p.w(x.Text)

	default:
		p.w(fmt.Sprintf("/* unprintable expression %T */", e))
	}
}

func isWordOp(op string) bool {
	switch op {
	case "typeof", "void", "delete", "await", "yield":
		return true
	}
	return false
}

// needsUnarySpace prevents -(-x) collapsing into the decrement token --x.
func needsUnarySpace(op string, inner ast.Expr) bool {
	u, ok := inner.(*ast.UnaryExpr)
	if !ok {
		return false
	}
	return (op == "-" && strings.HasPrefix(u.Op, "-")) ||
		(op == "+" && strings.HasPrefix(u.Op, "+"))
}

func (p *Printer) args(args []ast.Expr) {
	single := p.sub(func(q *Printer) {
		q.w("(")
		for i, a := range args {
			if i > 0 {
				q.w(", ")
			}
			q.expr(a, precAssign)
		}
		q.w(")")
	})
	if len(args) == 0 || (p.fits(utf8.RuneCountInString(single)) && !strings.Contains(single, "\n")) {
		p.w(single)
		return
	}
	p.w("(")
	p.in()
	for i, a := range args {
		p.nl()
		p.expr(a, precAssign)
		if i < len(args)-1 || p.opts.TrailingComma {
			p.w(",")
		}
	}
	p.out()
	p.nl()
	p.w(")")
}

func (p *Printer) objectLit(x *ast.ObjectLit) {
	if len(x.Props) == 0 {
		p.w("{}")
		return
	}
	one := func(q *Printer, pr ast.ObjectProp) {
		if pr.Spread {
			q.w("...")
			q.expr(pr.Value, precAssign)
			return
		}
		q.propName(pr.Name)
		if pr.Shorthand {
			return
		}
		q.w(": ")
		q.expr(pr.Value, precAssign)
	}
	hasDocs := false
	for _, pr := range x.Props {
		if !pr.Docs.IsEmpty() {
			hasDocs = true
			break
		}
	}
	if !hasDocs {
		single := p.sub(func(q *Printer) {
			q.w("{ ")
			for i, pr := range x.Props {
				if i > 0 {
					q.w(", ")
				}
				one(q, pr)
			}
			q.w(" }")
		})
		if p.fits(utf8.RuneCountInString(single)) && !strings.Contains(single, "\n") {
			p.w(single)
			return
		}
	}
	p.w("{")
	p.in()
	for i, pr := range x.Props {
		p.nl()
		p.docs(pr.Docs)
		one(p, pr)
		if i < len(x.Props)-1 || p.opts.TrailingComma {
			p.w(",")
		}
	}
	p.out()
	p.nl()
	p.w("}")
}
