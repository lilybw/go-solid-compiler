// Package ast defines a source-independent representation of TypeScript
// syntax.
//
// Nodes are concrete structs behind sealed interfaces ([Node], [Type],
// [Expr], [Stmt], [Decl], [Member]), so a type switch over a category stays
// exhaustive. Every node also reports a [Kind] for cheap discrimination.
//
// All fields are exported and every node is usable as a zero-valued
// composite literal:
//
//	&ast.UnionType{Types: []ast.Type{ast.String, ast.Null}}
//
// Positions are optional. [RawType], [RawExpr], and [RawStmt] carry verbatim
// text for syntax this package does not model; they print faithfully but are
// opaque to traversal.
package ast

import "github.com/lilybw/go-solid-compiler/token"

// Node is the interface implemented by every AST node. It is sealed.
type Node interface {
	// Span reports the source range of the node, or token.NoSpan if the node
	// was synthesized.
	Span() token.Span
	// Kind reports the node's discriminant.
	Kind() Kind

	node()
}

// Base is embedded by every node and supplies position storage. The Loc
// field is promoted, so a parser can set it without knowing the node type.
type Base struct {
	Loc token.Span
}

func (b *Base) Span() token.Span            { return b.Loc }
func (b *Base) SetSpan(s token.Span)        { b.Loc = s }
func (b *Base) setPos(start, end token.Pos) { b.Loc = token.Span{Start: start, End: end} }
func (*Base) node()                         {}

// Positioner is implemented by every node, allowing position rewriting
// without a type switch.
type Positioner interface {
	Node
	SetSpan(token.Span)
}

// ---------------------------------------------------------------------------
// Sealed category interfaces
// ---------------------------------------------------------------------------

// Type is the interface for type-level syntax.
type Type interface {
	Node
	typeNode()
}

// Expr is the interface for value-level expressions.
type Expr interface {
	Node
	exprNode()
}

// Stmt is the interface for statements.
type Stmt interface {
	Node
	stmtNode()
}

// Decl is a statement that introduces a name. Every Decl is a Stmt.
type Decl interface {
	Stmt
	declNode()
}

// Member is a member of an interface body, object type literal, or class body.
type Member interface {
	Node
	memberNode()
}

// PropertyName is the name position of a member: an identifier, a string or
// numeric literal, or a computed name.
type PropertyName interface {
	Node
	propertyName()
}

// Category bases, each embedding Base so that Loc stays promoted.
type (
	typeBase   struct{ Base }
	exprBase   struct{ Base }
	stmtBase   struct{ Base }
	declBase   struct{ stmtBase }
	memberBase struct{ Base }
)

func (typeBase) typeNode()     {}
func (exprBase) exprNode()     {}
func (stmtBase) stmtNode()     {}
func (declBase) declNode()     {}
func (memberBase) memberNode() {}

// ---------------------------------------------------------------------------
// Kinds
// ---------------------------------------------------------------------------

// Kind discriminates node types without a type switch.
type Kind uint16

const (
	KindInvalid Kind = iota

	// Types
	KindKeywordType
	KindLiteralType
	KindTypeRef
	KindArrayType
	KindTupleType
	KindUnionType
	KindIntersectionType
	KindFunctionType
	KindConstructorType
	KindTypeLiteral
	KindIndexedAccessType
	KindMappedType
	KindConditionalType
	KindInferType
	KindTypeOperator
	KindTypeQuery
	KindTemplateLiteralType
	KindParenType
	KindImportType
	KindPredicateType
	KindThisType
	KindRestType
	KindOptionalType
	KindNamedTupleMember
	KindRawType

	// Members
	KindPropertySignature
	KindMethodSignature
	KindIndexSignature
	KindCallSignature
	KindConstructSignature
	KindGetAccessor
	KindSetAccessor
	KindPropertyDecl
	KindMethodDecl

	// Names
	KindIdent
	KindQualifiedName
	KindComputedName

	// Expressions
	KindStringLit
	KindNumberLit
	KindBigIntLit
	KindBoolLit
	KindNullLit
	KindUndefinedLit
	KindArrayLit
	KindObjectLit
	KindCallExpr
	KindNewExpr
	KindMemberExpr
	KindIndexExpr
	KindArrowFunc
	KindAsExpr
	KindSatisfiesExpr
	KindNonNullExpr
	KindTemplateLit
	KindUnaryExpr
	KindBinaryExpr
	KindCondExpr
	KindSpreadExpr
	KindRawExpr

	// Declarations & statements
	KindInterfaceDecl
	KindTypeAliasDecl
	KindEnumDecl
	KindClassDecl
	KindFunctionDecl
	KindVarDecl
	KindModuleDecl
	KindImportDecl
	KindExportDecl
	KindExportAssign
	KindReturnStmt
	KindExprStmt
	KindBlockStmt
	KindRawStmt

	// Structural
	KindSourceFile
	KindTypeParam
	KindParam
	KindEnumMember
	KindHeritage
	KindBinding
)

//go:generate stringer -type=Kind

// ---------------------------------------------------------------------------
// Modifiers
// ---------------------------------------------------------------------------

// Modifier is a bit set of declaration modifiers. Combine with |.
type Modifier uint32

const (
	ModExport Modifier = 1 << iota
	ModDeclare
	ModDefault
	ModAbstract
	ModStatic
	ModReadonly
	ModAsync
	ModPublic
	ModPrivate
	ModProtected
	ModConst
	ModOverride
	ModAccessor
)

// Has reports whether every bit in m is set.
func (mo Modifier) Has(m Modifier) bool { return mo&m == m }

// With returns mo with the bits in m set.
func (mo Modifier) With(m Modifier) Modifier { return mo | m }

// Without returns mo with the bits in m cleared.
func (mo Modifier) Without(m Modifier) Modifier { return mo &^ m }

// ---------------------------------------------------------------------------
// Documentation
// ---------------------------------------------------------------------------

// Doc is a JSDoc comment. Text holds the prose lines and Tags the block
// tags; an empty Doc prints nothing.
type Doc struct {
	Text []string
	Tags []DocTag
}

// DocTag is a JSDoc block tag, for example {Name: "deprecated", Text: "use X"}.
type DocTag struct {
	Name string
	Text string
}

// IsEmpty reports whether the Doc would print nothing.
func (d *Doc) IsEmpty() bool {
	if d == nil {
		return true
	}
	for _, l := range d.Text {
		if l != "" {
			return false
		}
	}
	return len(d.Tags) == 0
}

// Comment returns a Doc with the given prose lines.
func Comment(lines ...string) *Doc { return &Doc{Text: lines} }

// ---------------------------------------------------------------------------
// Names
// ---------------------------------------------------------------------------

// Ident is a bare identifier. In a property-name position the printer quotes
// Text automatically when it is not a valid identifier.
type Ident struct {
	Base
	Text string
}

func (*Ident) Kind() Kind    { return KindIdent }
func (*Ident) propertyName() {}
func (*Ident) exprNode()     {}
func (*Ident) entityName()   {}

// NewIdent returns an identifier node for text.
func NewIdent(text string) *Ident { return &Ident{Text: text} }

// EntityName is a dotted name usable in type position: A, A.B, A.B.C.
type EntityName interface {
	Node
	entityName()
}

// QualifiedName is a dotted name, Left.Right.
type QualifiedName struct {
	Base
	Left  EntityName
	Right *Ident
}

func (*QualifiedName) Kind() Kind  { return KindQualifiedName }
func (*QualifiedName) entityName() {}

// ComputedName is a computed property name, [expr].
type ComputedName struct {
	Base
	Expr Expr
}

func (*ComputedName) Kind() Kind    { return KindComputedName }
func (*ComputedName) propertyName() {}

// ---------------------------------------------------------------------------
// Type parameters and parameters
// ---------------------------------------------------------------------------

// TypeParam is a generic type parameter declaration.
type TypeParam struct {
	Base
	Name       *Ident
	Constraint Type // may be nil
	Default    Type // may be nil
	Variance   Variance
	Const      bool
	Docs       *Doc
}

func (*TypeParam) Kind() Kind { return KindTypeParam }

// Variance is an explicit variance annotation on a type parameter.
type Variance uint8

const (
	VarianceNone Variance = iota
	VarianceIn
	VarianceOut
	VarianceInOut
)

// Param is a function, method, or constructor parameter.
type Param struct {
	Base
	Name     PropertyName // usually *Ident; may be a binding pattern rendered via RawExpr
	Type     Type         // may be nil
	Optional bool
	Rest     bool
	Default  Expr     // may be nil
	Mods     Modifier // parameter properties on constructors
	Docs     *Doc
}

func (*Param) Kind() Kind { return KindParam }

// Signature is the shared shape of anything callable: type parameters,
// parameters, and an optional return type. It is embedded rather than
// interface-implemented so that callers can construct it directly.
type Signature struct {
	TypeParams []*TypeParam
	Params     []*Param
	Return     Type // may be nil
}
