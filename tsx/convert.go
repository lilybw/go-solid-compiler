package tsx

import (
	tsast "github.com/lilybw/typescript-go/use-at-your-own-risk/ast"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/token"
)

// Conversion from the compiler's AST to the canonical AST.
//
// Only the type grammar and the declarations that carry types are converted;
// statements and expressions become [ast.RawStmt] and [ast.RawExpr] holding
// their source text.

// span converts a compiler text range to a canonical span.
func span(n *tsast.Node) token.Span {
	if n == nil {
		return token.NoSpan
	}
	return token.Span{Start: token.Pos(n.Pos() + 1), End: token.Pos(n.End() + 1)}
}

// at attaches a converted node's source position and returns it.
func at[T ast.Positioner](node T, src *tsast.Node) T {
	node.SetSpan(span(src))
	return node
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ConvertType converts a compiler type node to a canonical type. Unrecognized
// nodes become [ast.RawType] carrying their source text.
func ConvertType(n *tsast.Node) ast.Type { return convertType(n) }

func convertType(n *tsast.Node) ast.Type {
	if n == nil {
		return nil
	}
	switch n.Kind {

	// --- intrinsic keywords ------------------------------------------------
	case tsast.KindAnyKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwAny}, n)
	case tsast.KindUnknownKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwUnknown}, n)
	case tsast.KindNeverKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwNever}, n)
	case tsast.KindVoidKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwVoid}, n)
	case tsast.KindUndefinedKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwUndefined}, n)
	case tsast.KindStringKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwString}, n)
	case tsast.KindNumberKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwNumber}, n)
	case tsast.KindBooleanKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwBoolean}, n)
	case tsast.KindBigIntKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwBigInt}, n)
	case tsast.KindSymbolKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwSymbol}, n)
	case tsast.KindObjectKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwObject}, n)
	case tsast.KindThisType:
		return at(&ast.ThisType{}, n)

	// --- literals ----------------------------------------------------------
	case tsast.KindLiteralType:
		return convertLiteralType(n)

	// --- references --------------------------------------------------------
	case tsast.KindTypeReference:
		x := n.AsTypeReferenceNode()
		return at(&ast.TypeRef{
			Name: convertEntityName(x.TypeName),
			Args: convertTypeList(x.TypeArguments),
		}, n)

	case tsast.KindTypeQuery:
		x := n.AsTypeQueryNode()
		return at(&ast.TypeQuery{
			Name: convertEntityName(x.ExprName),
			Args: convertTypeList(x.TypeArguments),
		}, n)

	// --- composites --------------------------------------------------------
	case tsast.KindArrayType:
		return at(&ast.ArrayType{Elem: convertType(n.AsArrayTypeNode().ElementType)}, n)

	case tsast.KindTupleType:
		return at(&ast.TupleType{Elems: convertTypeList(n.AsTupleTypeNode().Elements)}, n)

	case tsast.KindOptionalType:
		return at(&ast.OptionalType{Elem: convertType(n.AsOptionalTypeNode().Type)}, n)

	case tsast.KindRestType:
		return at(&ast.RestType{Elem: convertType(n.AsRestTypeNode().Type)}, n)

	case tsast.KindNamedTupleMember:
		x := n.AsNamedTupleMember()
		return at(&ast.NamedTupleMember{
			Name:     ast.NewIdent(identText(x.Name())),
			Optional: x.QuestionToken != nil,
			Rest:     x.DotDotDotToken != nil,
			Type:     convertType(x.Type),
		}, n)

	case tsast.KindUnionType:
		return at(&ast.UnionType{Types: convertTypeList(n.AsUnionTypeNode().Types)}, n)

	case tsast.KindIntersectionType:
		return at(&ast.IntersectionType{Types: convertTypeList(n.AsIntersectionTypeNode().Types)}, n)

	case tsast.KindParenthesizedType:
		return at(&ast.ParenType{Inner: convertType(n.AsParenthesizedTypeNode().Type)}, n)

	case tsast.KindTypeLiteral:
		return at(&ast.TypeLiteral{Members: convertMembers(n.AsTypeLiteralNode().Members)}, n)

	// --- callables ---------------------------------------------------------
	case tsast.KindFunctionType:
		x := n.AsFunctionTypeNode()
		return at(&ast.FunctionType{Signature: ast.Signature{
			TypeParams: convertTypeParams(x.TypeParameters),
			Params:     convertParams(x.Parameters),
			Return:     convertType(x.Type),
		}}, n)

	case tsast.KindConstructorType:
		x := n.AsConstructorTypeNode()
		return at(&ast.ConstructorType{
			Signature: ast.Signature{
				TypeParams: convertTypeParams(x.TypeParameters),
				Params:     convertParams(x.Parameters),
				Return:     convertType(x.Type),
			},
			Abstract: x.ModifierFlags()&tsast.ModifierFlagsAbstract != 0,
		}, n)

	// --- type-level computation --------------------------------------------
	case tsast.KindIndexedAccessType:
		x := n.AsIndexedAccessTypeNode()
		return at(&ast.IndexedAccessType{
			Object: convertType(x.ObjectType),
			Index:  convertType(x.IndexType),
		}, n)

	case tsast.KindConditionalType:
		x := n.AsConditionalTypeNode()
		return at(&ast.ConditionalType{
			Check:   convertType(x.CheckType),
			Extends: convertType(x.ExtendsType),
			True:    convertType(x.TrueType),
			False:   convertType(x.FalseType),
		}, n)

	case tsast.KindInferType:
		return at(&ast.InferType{
			Param: convertTypeParam(n.AsInferTypeNode().TypeParameter),
		}, n)

	case tsast.KindMappedType:
		return convertMappedType(n)

	case tsast.KindTypeOperator:
		x := n.AsTypeOperatorNode()
		op := ast.OpKeyOf
		switch x.Operator {
		case tsast.KindUniqueKeyword:
			op = ast.OpUnique
		case tsast.KindReadonlyKeyword:
			op = ast.OpReadonly
		}
		return at(&ast.TypeOperator{Op: op, Type: convertType(x.Type)}, n)

	case tsast.KindTemplateLiteralType:
		return convertTemplateLiteralType(n)

	case tsast.KindImportType:
		return convertImportType(n)

	case tsast.KindTypePredicate:
		x := n.AsTypePredicateNode()
		return at(&ast.PredicateType{
			Asserts:   x.AssertsModifier != nil,
			ParamName: ast.NewIdent(identText(x.ParameterName)),
			Type:      convertType(x.Type),
		}, n)
	}

	// Anything else — including JSX in type position, which cannot occur, and
	// JSDoc type nodes, which can — is preserved as text.
	return at(&ast.RawType{Text: nodeText(n)}, n)
}

func convertLiteralType(n *tsast.Node) ast.Type {
	lit := n.AsLiteralTypeNode().Literal
	if lit == nil {
		return at(&ast.RawType{Text: nodeText(n)}, n)
	}
	switch lit.Kind {
	case tsast.KindStringLiteral:
		return at(ast.StringLiteral(lit.Text()), n)
	case tsast.KindNumericLiteral:
		return at(ast.NumberLiteral(lit.Text()), n)
	case tsast.KindBigIntLiteral:
		return at(&ast.LiteralType{LitKind: ast.LitBigInt, Value: lit.Text()}, n)
	case tsast.KindTrueKeyword:
		return at(ast.BoolLiteral(true), n)
	case tsast.KindFalseKeyword:
		return at(ast.BoolLiteral(false), n)
	case tsast.KindNullKeyword:
		return at(&ast.KeywordType{Keyword: ast.KwNull}, n)
	case tsast.KindPrefixUnaryExpression:
		// A negative numeric literal type, -1.
		u := lit.AsPrefixUnaryExpression()
		if u.Operand != nil {
			kind := ast.LitNumber
			if u.Operand.Kind == tsast.KindBigIntLiteral {
				kind = ast.LitBigInt
			}
			return at(&ast.LiteralType{
				LitKind: kind, Value: u.Operand.Text(), Negated: true,
			}, n)
		}
	}
	return at(&ast.RawType{Text: nodeText(n)}, n)
}

func convertMappedType(n *tsast.Node) ast.Type {
	x := n.AsMappedTypeNode()
	m := &ast.MappedType{
		Param: convertTypeParam(x.TypeParameter),
		As:    convertType(x.NameType),
		Type:  convertType(x.Type),
	}
	if x.ReadonlyToken != nil {
		m.ReadonlyMod = ast.MappedAdd
		if x.ReadonlyToken.Kind == tsast.KindMinusToken {
			m.ReadonlyMod = ast.MappedRemove
		}
	}
	if x.QuestionToken != nil {
		m.OptionalMod = ast.MappedAdd
		if x.QuestionToken.Kind == tsast.KindMinusToken {
			m.OptionalMod = ast.MappedRemove
		}
	}
	return at(m, n)
}

func convertTemplateLiteralType(n *tsast.Node) ast.Type {
	x := n.AsTemplateLiteralTypeNode()
	tl := &ast.TemplateLiteralType{}
	if x.Head != nil {
		tl.Quasis = append(tl.Quasis, x.Head.Text())
	}
	if x.TemplateSpans != nil {
		for _, sp := range x.TemplateSpans.Nodes {
			s := sp.AsTemplateLiteralTypeSpan()
			tl.Types = append(tl.Types, convertType(s.Type))
			if s.Literal != nil {
				tl.Quasis = append(tl.Quasis, s.Literal.Text())
			}
		}
	}
	// The printer relies on len(Quasis) == len(Types)+1.
	for len(tl.Quasis) < len(tl.Types)+1 {
		tl.Quasis = append(tl.Quasis, "")
	}
	return at(tl, n)
}

func convertImportType(n *tsast.Node) ast.Type {
	x := n.AsImportTypeNode()
	it := &ast.ImportType{
		TypeOf: x.IsTypeOf,
		Args:   convertTypeList(x.TypeArguments),
	}
	// The module specifier is a literal type wrapping a string literal.
	if x.Argument != nil && x.Argument.Kind == tsast.KindLiteralType {
		if lit := x.Argument.AsLiteralTypeNode().Literal; lit != nil {
			it.Module = lit.Text()
		}
	}
	if x.Qualifier != nil {
		it.Qualifier = convertEntityName(x.Qualifier)
	}
	return at(it, n)
}

// ---------------------------------------------------------------------------
// Names, parameters, members
// ---------------------------------------------------------------------------

func convertEntityName(n *tsast.Node) ast.EntityName {
	if n == nil {
		return nil
	}
	if n.Kind == tsast.KindQualifiedName {
		q := n.AsQualifiedName()
		return &ast.QualifiedName{
			Left:  convertEntityName(q.Left),
			Right: ast.NewIdent(identText(q.Right)),
		}
	}
	return ast.NewIdent(identText(n))
}

// identText reads a name node's text, tolerating nil and unexpected kinds.
func identText(n *tsast.Node) string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case tsast.KindIdentifier,
		tsast.KindPrivateIdentifier,
		tsast.KindStringLiteral,
		tsast.KindNumericLiteral,
		tsast.KindBigIntLiteral,
		tsast.KindJsxNamespacedName:
		return n.Text()
	case tsast.KindJsxText:
		return n.AsJsxText().Text
	}
	return nodeText(n)
}

func convertTypeList(l *tsast.NodeList) []ast.Type {
	if l == nil {
		return nil
	}
	out := make([]ast.Type, 0, len(l.Nodes))
	for _, n := range l.Nodes {
		if t := convertType(n); t != nil {
			out = append(out, t)
		}
	}
	return out
}

func convertTypeParams(l *tsast.NodeList) []*ast.TypeParam {
	if l == nil {
		return nil
	}
	out := make([]*ast.TypeParam, 0, len(l.Nodes))
	for _, n := range l.Nodes {
		if p := convertTypeParam(n); p != nil {
			out = append(out, p)
		}
	}
	return out
}

func convertTypeParam(n *tsast.Node) *ast.TypeParam {
	if n == nil {
		return nil
	}
	x := n.AsTypeParameterDeclaration()
	p := &ast.TypeParam{
		Name:       ast.NewIdent(identText(x.Name())),
		Constraint: convertType(x.Constraint),
		Default:    convertType(x.DefaultType),
	}
	flags := n.ModifierFlags()
	switch {
	case flags&tsast.ModifierFlagsIn != 0 && flags&tsast.ModifierFlagsOut != 0:
		p.Variance = ast.VarianceInOut
	case flags&tsast.ModifierFlagsIn != 0:
		p.Variance = ast.VarianceIn
	case flags&tsast.ModifierFlagsOut != 0:
		p.Variance = ast.VarianceOut
	}
	p.Const = flags&tsast.ModifierFlagsConst != 0
	return at(p, n)
}

func convertParams(l *tsast.NodeList) []*ast.Param {
	if l == nil {
		return nil
	}
	out := make([]*ast.Param, 0, len(l.Nodes))
	for _, n := range l.Nodes {
		x := n.AsParameterDeclaration()
		p := &ast.Param{
			Name:     convertPropertyName(x.Name()),
			Type:     convertType(x.Type),
			Optional: x.QuestionToken != nil,
			Rest:     x.DotDotDotToken != nil,
		}
		out = append(out, at(p, n))
	}
	return out
}

func convertPropertyName(n *tsast.Node) ast.PropertyName {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case tsast.KindStringLiteral:
		return &ast.StringLit{Value: n.Text()}
	case tsast.KindNumericLiteral:
		return &ast.NumberLit{Text: n.Text()}
	case tsast.KindComputedPropertyName:
		return &ast.ComputedName{
			Expr: &ast.RawExpr{Text: nodeText(n.AsComputedPropertyName().Expression)},
		}
	default:
		return ast.NewIdent(identText(n))
	}
}

func convertMembers(l *tsast.NodeList) []ast.Member {
	if l == nil {
		return nil
	}
	out := make([]ast.Member, 0, len(l.Nodes))
	for _, n := range l.Nodes {
		if m := convertMember(n); m != nil {
			out = append(out, m)
		}
	}
	return out
}

func convertMember(n *tsast.Node) ast.Member {
	switch n.Kind {
	case tsast.KindPropertySignature:
		x := n.AsPropertySignatureDeclaration()
		return at(&ast.PropertySignature{
			Name:     convertPropertyName(x.Name()),
			Type:     convertType(x.Type),
			Optional: x.PostfixToken != nil,
			Readonly: n.ModifierFlags()&tsast.ModifierFlagsReadonly != 0,
			Docs:     docsOf(n),
		}, n)

	case tsast.KindMethodSignature:
		x := n.AsMethodSignatureDeclaration()
		return at(&ast.MethodSignature{
			Name:     convertPropertyName(x.Name()),
			Optional: x.PostfixToken != nil,
			Signature: ast.Signature{
				TypeParams: convertTypeParams(x.TypeParameters),
				Params:     convertParams(x.Parameters),
				Return:     convertType(x.Type),
			},
			Docs: docsOf(n),
		}, n)

	case tsast.KindIndexSignature:
		x := n.AsIndexSignatureDeclaration()
		sig := &ast.IndexSignature{
			Type:     convertType(x.Type),
			Readonly: n.ModifierFlags()&tsast.ModifierFlagsReadonly != 0,
			Docs:     docsOf(n),
		}
		if x.Parameters != nil && len(x.Parameters.Nodes) > 0 {
			p := x.Parameters.Nodes[0].AsParameterDeclaration()
			sig.KeyName = identText(p.Name())
			sig.KeyType = convertType(p.Type)
		}
		return at(sig, n)

	case tsast.KindCallSignature:
		x := n.AsCallSignatureDeclaration()
		return at(&ast.CallSignature{Signature: ast.Signature{
			TypeParams: convertTypeParams(x.TypeParameters),
			Params:     convertParams(x.Parameters),
			Return:     convertType(x.Type),
		}, Docs: docsOf(n)}, n)

	case tsast.KindConstructSignature:
		x := n.AsConstructSignatureDeclaration()
		return at(&ast.ConstructSignature{Signature: ast.Signature{
			TypeParams: convertTypeParams(x.TypeParameters),
			Params:     convertParams(x.Parameters),
			Return:     convertType(x.Type),
		}, Docs: docsOf(n)}, n)
	}
	return nil
}
