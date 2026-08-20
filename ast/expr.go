package ast

// Expressions.
//
// This set covers initializers, enum values, default arguments, and emitted
// runtime values. [RawExpr] covers anything not modelled here.

// StringLit is a string literal expression. Value is the decoded value; the
// printer handles quoting and escaping.
type StringLit struct {
	exprBase
	Value string
}

func (*StringLit) Kind() Kind    { return KindStringLit }
func (*StringLit) propertyName() {}

// NumberLit is a numeric literal expression, stored in source form.
type NumberLit struct {
	exprBase
	Text string
}

func (*NumberLit) Kind() Kind    { return KindNumberLit }
func (*NumberLit) propertyName() {}

// BigIntLit is a bigint literal expression, stored in source form including
// the trailing n.
type BigIntLit struct {
	exprBase
	Text string
}

func (*BigIntLit) Kind() Kind { return KindBigIntLit }

// BoolLit is true or false.
type BoolLit struct {
	exprBase
	Value bool
}

func (*BoolLit) Kind() Kind { return KindBoolLit }

// NullLit is null.
type NullLit struct{ exprBase }

func (*NullLit) Kind() Kind { return KindNullLit }

// UndefinedLit is undefined.
type UndefinedLit struct{ exprBase }

func (*UndefinedLit) Kind() Kind { return KindUndefinedLit }

// ArrayLit is [a, b, c].
type ArrayLit struct {
	exprBase
	Elems []Expr
}

func (*ArrayLit) Kind() Kind { return KindArrayLit }

// ObjectProp is one entry of an object literal. Shorthand prints just the
// name; Spread prints ...Value.
type ObjectProp struct {
	Name      PropertyName
	Value     Expr
	Shorthand bool
	Spread    bool
	Docs      *Doc
}

// ObjectLit is { a: 1, ...rest }.
type ObjectLit struct {
	exprBase
	Props []ObjectProp
}

func (*ObjectLit) Kind() Kind { return KindObjectLit }

// CallExpr is fn<T>(a, b), optionally optional-chained.
type CallExpr struct {
	exprBase
	Callee   Expr
	Args     []Expr
	TypeArgs []Type
	Optional bool // fn?.(a)
}

func (*CallExpr) Kind() Kind { return KindCallExpr }

// NewExpr is new C<T>(a).
type NewExpr struct {
	exprBase
	Callee   Expr
	Args     []Expr
	TypeArgs []Type
}

func (*NewExpr) Kind() Kind { return KindNewExpr }

// MemberExpr is obj.prop or obj?.prop.
type MemberExpr struct {
	exprBase
	Object   Expr
	Prop     *Ident
	Optional bool
}

func (*MemberExpr) Kind() Kind { return KindMemberExpr }

// IndexExpr is obj[index] or obj?.[index].
type IndexExpr struct {
	exprBase
	Object   Expr
	Index    Expr
	Optional bool
}

func (*IndexExpr) Kind() Kind { return KindIndexExpr }

// ArrowFunc is (a: A): R => body. Exactly one of Body and Expr is non-nil.
type ArrowFunc struct {
	exprBase
	Signature
	Body  *BlockStmt
	Expr  Expr
	Async bool
}

func (*ArrowFunc) Kind() Kind { return KindArrowFunc }

// AsExpr is x as T, or x as const when Const is set.
type AsExpr struct {
	exprBase
	Expr  Expr
	Type  Type
	Const bool
}

func (*AsExpr) Kind() Kind { return KindAsExpr }

// SatisfiesExpr is x satisfies T.
type SatisfiesExpr struct {
	exprBase
	Expr Expr
	Type Type
}

func (*SatisfiesExpr) Kind() Kind { return KindSatisfiesExpr }

// NonNullExpr is x!.
type NonNullExpr struct {
	exprBase
	Expr Expr
}

func (*NonNullExpr) Kind() Kind { return KindNonNullExpr }

// TemplateLit is a template literal, where len(Quasis) == len(Exprs)+1. A
// non-nil Tag makes it a tagged template.
type TemplateLit struct {
	exprBase
	Tag    Expr
	Quasis []string
	Exprs  []Expr
}

func (*TemplateLit) Kind() Kind { return KindTemplateLit }

// UnaryExpr is a prefix operator application such as !x, -x, typeof x, await x.
type UnaryExpr struct {
	exprBase
	Op   string
	Expr Expr
}

func (*UnaryExpr) Kind() Kind { return KindUnaryExpr }

// BinaryExpr is a binary or assignment operator application. The printer
// parenthesizes operands by precedence.
type BinaryExpr struct {
	exprBase
	Op    string
	Left  Expr
	Right Expr
}

func (*BinaryExpr) Kind() Kind { return KindBinaryExpr }

// CondExpr is cond ? then : else.
type CondExpr struct {
	exprBase
	Cond Expr
	Then Expr
	Else Expr
}

func (*CondExpr) Kind() Kind { return KindCondExpr }

// SpreadExpr is ...x in a call or array literal.
type SpreadExpr struct {
	exprBase
	Expr Expr
}

func (*SpreadExpr) Kind() Kind { return KindSpreadExpr }

// RawExpr carries verbatim TypeScript text in expression position.
type RawExpr struct {
	exprBase
	Text string
}

func (*RawExpr) Kind() Kind { return KindRawExpr }

// ---------------------------------------------------------------------------
// Convenience constructors
// ---------------------------------------------------------------------------

// Str returns a string literal expression.
func Str(v string) *StringLit { return &StringLit{Value: v} }

// Num returns a numeric literal expression from source text.
func Num(text string) *NumberLit { return &NumberLit{Text: text} }

// Bool returns a boolean literal expression.
func Bool(v bool) *BoolLit { return &BoolLit{Value: v} }

// Call returns a call expression.
func Call(callee Expr, args ...Expr) *CallExpr {
	return &CallExpr{Callee: callee, Args: args}
}

// Dot returns obj.prop.
func Dot(obj Expr, prop string) *MemberExpr {
	return &MemberExpr{Object: obj, Prop: NewIdent(prop)}
}
