package tsx

import (
	"strings"

	tsast "github.com/lilybw/typescript-go/use-at-your-own-risk/ast"

	"github.com/lilybw/go-solid-compiler/ast"
)

// nodeText returns the original source text of a node.
func nodeText(n *tsast.Node) string {
	if n == nil {
		return ""
	}
	file := tsast.GetSourceFileOfNode(n)
	if file == nil {
		return ""
	}
	text := file.Text()
	start, end := n.Pos(), n.End()
	if start < 0 || end > len(text) || start > end {
		return ""
	}
	return strings.TrimSpace(text[start:end])
}

// docsOf extracts JSDoc attached to a declaration.
func docsOf(n *tsast.Node) *ast.Doc {
	if n == nil {
		return nil
	}
	file := tsast.GetSourceFileOfNode(n)
	if file == nil {
		return nil
	}
	text := file.Text()
	// Scan backwards from the node start over whitespace to find a preceding
	// block comment.
	i := n.Pos()
	if i > len(text) {
		return nil
	}
	for i > 0 && (text[i-1] == ' ' || text[i-1] == '\t' || text[i-1] == '\n' || text[i-1] == '\r') {
		i--
	}
	if i < 4 || text[i-2:i] != "*/" {
		return nil
	}
	start := strings.LastIndex(text[:i], "/**")
	if start < 0 {
		return nil
	}
	return parseDocComment(text[start+3 : i-2])
}

// parseDocComment turns the interior of a JSDoc block into a Doc.
func parseDocComment(body string) *ast.Doc {
	d := &ast.Doc{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@") {
			name, rest, _ := strings.Cut(strings.TrimPrefix(line, "@"), " ")
			d.Tags = append(d.Tags, ast.DocTag{Name: name, Text: strings.TrimSpace(rest)})
			continue
		}
		if line == "" && len(d.Text) == 0 {
			continue
		}
		d.Text = append(d.Text, line)
	}
	for len(d.Text) > 0 && d.Text[len(d.Text)-1] == "" {
		d.Text = d.Text[:len(d.Text)-1]
	}
	if d.IsEmpty() {
		return nil
	}
	return d
}

// modifiers converts compiler modifier flags to canonical modifier bits.
func modifiers(n *tsast.Node) ast.Modifier {
	f := n.ModifierFlags()
	var m ast.Modifier
	set := func(flag tsast.ModifierFlags, bit ast.Modifier) {
		if f&flag != 0 {
			m = m.With(bit)
		}
	}
	set(tsast.ModifierFlagsExport, ast.ModExport)
	set(tsast.ModifierFlagsAmbient, ast.ModDeclare)
	set(tsast.ModifierFlagsDefault, ast.ModDefault)
	set(tsast.ModifierFlagsAbstract, ast.ModAbstract)
	set(tsast.ModifierFlagsStatic, ast.ModStatic)
	set(tsast.ModifierFlagsReadonly, ast.ModReadonly)
	set(tsast.ModifierFlagsAsync, ast.ModAsync)
	set(tsast.ModifierFlagsPublic, ast.ModPublic)
	set(tsast.ModifierFlagsPrivate, ast.ModPrivate)
	set(tsast.ModifierFlagsProtected, ast.ModProtected)
	set(tsast.ModifierFlagsOverride, ast.ModOverride)
	return m
}

// ---------------------------------------------------------------------------
// Source files and declarations
// ---------------------------------------------------------------------------

// ConvertSourceFile converts a compiler source file to the canonical AST.
// Constructs outside the type grammar are preserved verbatim.
func ConvertSourceFile(file *tsast.SourceFile) *ast.SourceFile { return convertSourceFile(file) }

func convertSourceFile(file *tsast.SourceFile) *ast.SourceFile {
	out := &ast.SourceFile{Name: file.FileName()}
	switch {
	case file.IsDeclarationFile:
		out.ScriptKind = ast.ScriptDTS
	case strings.HasSuffix(file.FileName(), ".tsx"):
		out.ScriptKind = ast.ScriptTSX
	}
	if file.Statements == nil {
		return out
	}
	for _, stmt := range file.Statements.Nodes {
		if s := convertStmt(stmt); s != nil {
			out.Stmts = append(out.Stmts, s)
		}
	}
	return out
}

func convertStmt(n *tsast.Node) ast.Stmt {
	switch n.Kind {

	case tsast.KindInterfaceDeclaration:
		x := n.AsInterfaceDeclaration()
		d := &ast.InterfaceDecl{
			Name:       ast.NewIdent(identText(x.Name())),
			TypeParams: convertTypeParams(x.TypeParameters),
			Members:    convertMembers(x.Members),
			Mods:       modifiers(n),
			Docs:       docsOf(n),
		}
		if x.HeritageClauses != nil {
			for _, hc := range x.HeritageClauses.Nodes {
				for _, t := range hc.AsHeritageClause().Types.Nodes {
					e := t.AsExpressionWithTypeArguments()
					d.Extends = append(d.Extends, &ast.Heritage{
						Name: convertEntityName(e.Expression),
						Args: convertTypeList(e.TypeArguments),
					})
				}
			}
		}
		return at(d, n)

	case tsast.KindTypeAliasDeclaration:
		x := n.AsTypeAliasDeclaration()
		return at(&ast.TypeAliasDecl{
			Name:       ast.NewIdent(identText(x.Name())),
			TypeParams: convertTypeParams(x.TypeParameters),
			Type:       convertType(x.Type),
			Mods:       modifiers(n),
			Docs:       docsOf(n),
		}, n)

	case tsast.KindEnumDeclaration:
		x := n.AsEnumDeclaration()
		d := &ast.EnumDecl{
			Name: ast.NewIdent(identText(x.Name())),
			Mods: modifiers(n),
			Docs: docsOf(n),
		}
		if n.ModifierFlags()&tsast.ModifierFlagsConst != 0 {
			d.Mods = d.Mods.With(ast.ModConst)
		}
		if x.Members != nil {
			for _, m := range x.Members.Nodes {
				em := m.AsEnumMember()
				member := &ast.EnumMember{
					Name: convertPropertyName(em.Name()),
					Docs: docsOf(m),
				}
				if em.Initializer != nil {
					member.Value = convertLiteralExpr(em.Initializer)
				}
				d.Members = append(d.Members, member)
			}
		}
		return at(d, n)

	case tsast.KindImportDeclaration:
		return convertImportDecl(n)

	case tsast.KindExportDeclaration:
		return convertExportDecl(n)
	}

	// Statements, expressions, classes, and functions with bodies are kept as
	// text. See the comment at the top of convert.go for why this is the
	// design rather than a gap.
	return at(&ast.RawStmt{Text: nodeText(n)}, n)
}

// convertLiteralExpr converts the expressions that appear in type-carrying
// positions; anything else is preserved as text.
func convertLiteralExpr(n *tsast.Node) ast.Expr {
	switch n.Kind {
	case tsast.KindStringLiteral:
		return &ast.StringLit{Value: n.Text()}
	case tsast.KindNumericLiteral:
		return &ast.NumberLit{Text: n.Text()}
	case tsast.KindTrueKeyword:
		return ast.Bool(true)
	case tsast.KindFalseKeyword:
		return ast.Bool(false)
	case tsast.KindNullKeyword:
		return &ast.NullLit{}
	}
	return &ast.RawExpr{Text: nodeText(n)}
}

func convertImportDecl(n *tsast.Node) ast.Stmt {
	x := n.AsImportDeclaration()
	d := &ast.ImportDecl{Docs: docsOf(n)}
	if x.ModuleSpecifier != nil {
		d.Module = x.ModuleSpecifier.Text()
	}
	if x.ImportClause != nil {
		c := x.ImportClause.AsImportClause()
		// TypeScript replaced the ImportClause.IsTypeOnly flag with a phase
		// modifier when deferred imports (import defer) were added, so
		// type-only-ness is now a keyword comparison rather than a bool.
		d.TypeOnly = c.PhaseModifier == tsast.KindTypeKeyword
		if c.Name() != nil {
			d.Default = identText(c.Name())
		}
		if c.NamedBindings != nil {
			switch c.NamedBindings.Kind {
			case tsast.KindNamespaceImport:
				d.Namespace = identText(c.NamedBindings.AsNamespaceImport().Name())
			case tsast.KindNamedImports:
				for _, e := range c.NamedBindings.AsNamedImports().Elements.Nodes {
					s := e.AsImportSpecifier()
					spec := ast.ImportSpec{
						Name:     identText(s.Name()),
						TypeOnly: s.IsTypeOnly,
					}
					if s.PropertyName != nil {
						spec.Name = identText(s.PropertyName)
						spec.Alias = identText(s.Name())
					}
					d.Named = append(d.Named, spec)
				}
			}
		}
	}
	return at(d, n)
}

func convertExportDecl(n *tsast.Node) ast.Stmt {
	x := n.AsExportDeclaration()
	d := &ast.ExportDecl{TypeOnly: x.IsTypeOnly, Docs: docsOf(n)}
	if x.ModuleSpecifier != nil {
		d.Module = x.ModuleSpecifier.Text()
	}
	if x.ExportClause != nil {
		switch x.ExportClause.Kind {
		case tsast.KindNamespaceExport:
			d.Star = true
			d.StarAs = identText(x.ExportClause.AsNamespaceExport().Name())
		case tsast.KindNamedExports:
			for _, e := range x.ExportClause.AsNamedExports().Elements.Nodes {
				s := e.AsExportSpecifier()
				spec := ast.ImportSpec{
					Name:     identText(s.Name()),
					TypeOnly: s.IsTypeOnly,
				}
				if s.PropertyName != nil {
					spec.Name = identText(s.PropertyName)
					spec.Alias = identText(s.Name())
				}
				d.Named = append(d.Named, spec)
			}
		}
	} else {
		d.Star = true
	}
	return at(d, n)
}
