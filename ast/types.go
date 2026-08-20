package ast

// This file models the TypeScript *type* grammar. It is deliberately complete:
// generating types is the primary job of this library, and a partial type
// language forces consumers into string concatenation, which defeats the point.

// ---------------------------------------------------------------------------
// Keyword types
// ---------------------------------------------------------------------------

// KeywordKind enumerates the intrinsic type keywords.
type KeywordKind uint8

const (
	KwAny KeywordKind = iota
	KwUnknown
	KwNever
	KwVoid
	KwUndefined
	KwNull
	KwString
	KwNumber
	KwBoolean
	KwBigInt
	KwSymbol
	KwObject
)

var keywordText = [...]string{
	KwAny: "any", KwUnknown: "unknown", KwNever: "never", KwVoid: "void",
	KwUndefined: "undefined", KwNull: "null", KwString: "string",
	KwNumber: "number", KwBoolean: "boolean", KwBigInt: "bigint",
	KwSymbol: "symbol", KwObject: "object",
}

func (k KeywordKind) String() string {
	if int(k) < len(keywordText) {
		return keywordText[k]
	}
	return "unknown"
}

// KeywordType is an intrinsic type such as string or never.
type KeywordType struct {
	typeBase
	Keyword KeywordKind
}

func (*KeywordType) Kind() Kind { return KindKeywordType }

// Keyword singletons, safe to share because they carry no position or
// mutable state. Construct a [KeywordType] directly if you need a position.
var (
	Any       Type = &KeywordType{Keyword: KwAny}
	Unknown   Type = &KeywordType{Keyword: KwUnknown}
	Never     Type = &KeywordType{Keyword: KwNever}
	Void      Type = &KeywordType{Keyword: KwVoid}
	Undefined Type = &KeywordType{Keyword: KwUndefined}
	Null      Type = &KeywordType{Keyword: KwNull}
	String    Type = &KeywordType{Keyword: KwString}
	Number    Type = &KeywordType{Keyword: KwNumber}
	Boolean   Type = &KeywordType{Keyword: KwBoolean}
	BigInt    Type = &KeywordType{Keyword: KwBigInt}
	Symbol    Type = &KeywordType{Keyword: KwSymbol}
	Object    Type = &KeywordType{Keyword: KwObject}
)

// ---------------------------------------------------------------------------
// Literal types
// ---------------------------------------------------------------------------

// LiteralKind discriminates the payload of a LiteralType.
type LiteralKind uint8

const (
	LitString LiteralKind = iota
	LitNumber
	LitBoolean
	LitBigInt
)

// LiteralType is a literal in type position. Value holds the source form for
// numbers and the unquoted text for strings.
type LiteralType struct {
	typeBase
	LitKind LiteralKind
	Value   string
	Negated bool // for numeric literals: -1
}

func (*LiteralType) Kind() Kind { return KindLiteralType }

// StringLiteral returns the literal type for a string value.
func StringLiteral(v string) *LiteralType {
	return &LiteralType{LitKind: LitString, Value: v}
}

// NumberLiteral returns the literal type for a numeric value in source form.
func NumberLiteral(v string) *LiteralType {
	return &LiteralType{LitKind: LitNumber, Value: v}
}

// BoolLiteral returns the literal type true or false.
func BoolLiteral(v bool) *LiteralType {
	if v {
		return &LiteralType{LitKind: LitBoolean, Value: "true"}
	}
	return &LiteralType{LitKind: LitBoolean, Value: "false"}
}

// ---------------------------------------------------------------------------
// References
// ---------------------------------------------------------------------------

// TypeRef is a reference to a named type, with optional type arguments:
// User, Array<T>, ns.Thing<A, B>.
type TypeRef struct {
	typeBase
	Name EntityName
	Args []Type
}

func (*TypeRef) Kind() Kind { return KindTypeRef }

// Ref returns a reference to the named type, splitting a dotted name.
func Ref(name string, args ...Type) *TypeRef {
	return &TypeRef{Name: parseEntityName(name), Args: args}
}

// parseEntityName splits a dotted name without invoking the full parser.
func parseEntityName(name string) EntityName {
	start := 0
	var cur EntityName
	for i := 0; i <= len(name); i++ {
		if i == len(name) || name[i] == '.' {
			part := NewIdent(name[start:i])
			if cur == nil {
				cur = part
			} else {
				cur = &QualifiedName{Left: cur, Right: part}
			}
			start = i + 1
		}
	}
	if cur == nil {
		cur = NewIdent(name)
	}
	return cur
}

// ThisType is the this type.
type ThisType struct{ typeBase }

func (*ThisType) Kind() Kind { return KindThisType }

// ---------------------------------------------------------------------------
// Composite types
// ---------------------------------------------------------------------------

// ArrayType is T[]. Use TypeRef("Array", T) for the generic spelling.
type ArrayType struct {
	typeBase
	Elem Type
}

func (*ArrayType) Kind() Kind { return KindArrayType }

// TupleType is [A, B?, ...C[]].
type TupleType struct {
	typeBase
	Elems []Type // may contain *OptionalType, *RestType, *NamedTupleMember
}

func (*TupleType) Kind() Kind { return KindTupleType }

// OptionalType is T? inside a tuple.
type OptionalType struct {
	typeBase
	Elem Type
}

func (*OptionalType) Kind() Kind { return KindOptionalType }

// RestType is ...T inside a tuple or parameter list.
type RestType struct {
	typeBase
	Elem Type
}

func (*RestType) Kind() Kind { return KindRestType }

// NamedTupleMember is a labelled tuple element: [x: number, y?: string].
type NamedTupleMember struct {
	typeBase
	Name     *Ident
	Optional bool
	Rest     bool
	Type     Type
}

func (*NamedTupleMember) Kind() Kind { return KindNamedTupleMember }

// UnionType is A | B | C.
type UnionType struct {
	typeBase
	Types []Type
}

func (*UnionType) Kind() Kind { return KindUnionType }

// IntersectionType is A & B & C.
type IntersectionType struct {
	typeBase
	Types []Type
}

func (*IntersectionType) Kind() Kind { return KindIntersectionType }

// ParenType is an explicit parenthesization. The printer adds parentheses
// where precedence requires them, so this is only needed to preserve
// redundant ones from parsed source.
type ParenType struct {
	typeBase
	Inner Type
}

func (*ParenType) Kind() Kind { return KindParenType }

// ---------------------------------------------------------------------------
// Object and callable types
// ---------------------------------------------------------------------------

// TypeLiteral is an anonymous object type: { a: string; b?: number }.
type TypeLiteral struct {
	typeBase
	Members []Member
}

func (*TypeLiteral) Kind() Kind { return KindTypeLiteral }

// FunctionType is (a: A) => R.
type FunctionType struct {
	typeBase
	Signature
	Abstract bool // unused for function types; present for symmetry
}

func (*FunctionType) Kind() Kind { return KindFunctionType }

// ConstructorType is new (a: A) => R, optionally abstract.
type ConstructorType struct {
	typeBase
	Signature
	Abstract bool
}

func (*ConstructorType) Kind() Kind { return KindConstructorType }

// ---------------------------------------------------------------------------
// Type-level computation
// ---------------------------------------------------------------------------

// IndexedAccessType is T[K].
type IndexedAccessType struct {
	typeBase
	Object Type
	Index  Type
}

func (*IndexedAccessType) Kind() Kind { return KindIndexedAccessType }

// MappedModifier expresses the +/- prefix on readonly and ? in mapped types.
type MappedModifier int8

const (
	MappedUnset  MappedModifier = 0
	MappedAdd    MappedModifier = 1
	MappedRemove MappedModifier = -1
)

// MappedType is { [K in Keys as As]?: T }.
type MappedType struct {
	typeBase
	Param       *TypeParam // Name and Constraint hold K and Keys
	As          Type       // key remapping clause; may be nil
	Type        Type       // may be nil for { [K in Keys] }
	ReadonlyMod MappedModifier
	OptionalMod MappedModifier
}

func (*MappedType) Kind() Kind { return KindMappedType }

// ConditionalType is Check extends Extends ? True : False.
type ConditionalType struct {
	typeBase
	Check   Type
	Extends Type
	True    Type
	False   Type
}

func (*ConditionalType) Kind() Kind { return KindConditionalType }

// InferType is infer T, optionally with an extends constraint.
type InferType struct {
	typeBase
	Param *TypeParam
}

func (*InferType) Kind() Kind { return KindInferType }

// TypeOperatorKind enumerates prefix type operators.
type TypeOperatorKind uint8

const (
	OpKeyOf TypeOperatorKind = iota
	OpUnique
	OpReadonly
)

func (o TypeOperatorKind) String() string {
	switch o {
	case OpUnique:
		return "unique"
	case OpReadonly:
		return "readonly"
	default:
		return "keyof"
	}
}

// TypeOperator is keyof T, unique symbol, or readonly T[].
type TypeOperator struct {
	typeBase
	Op   TypeOperatorKind
	Type Type
}

func (*TypeOperator) Kind() Kind { return KindTypeOperator }

// TypeQuery is typeof x, optionally with type arguments.
type TypeQuery struct {
	typeBase
	Name EntityName
	Args []Type
}

func (*TypeQuery) Kind() Kind { return KindTypeQuery }

// TemplateLiteralType is `a${B}c`, where len(Quasis) == len(Types)+1.
type TemplateLiteralType struct {
	typeBase
	Quasis []string
	Types  []Type
}

func (*TemplateLiteralType) Kind() Kind { return KindTemplateLiteralType }

// ImportType is import("mod").Name<Args>.
type ImportType struct {
	typeBase
	Module     string
	Qualifier  EntityName // may be nil
	Args       []Type
	TypeOf     bool // typeof import("mod")
	Attributes []ImportAttribute
}

func (*ImportType) Kind() Kind { return KindImportType }

// ImportAttribute is a single with { key: "value" } entry.
type ImportAttribute struct {
	Key   string
	Value string
}

// PredicateType is a type predicate in return position.
type PredicateType struct {
	typeBase
	Asserts   bool
	ParamName *Ident
	Type      Type // nil for a bare asserts x
}

func (*PredicateType) Kind() Kind { return KindPredicateType }

// ---------------------------------------------------------------------------
// Escape hatch
// ---------------------------------------------------------------------------

// RawType carries verbatim TypeScript text in type position, for syntax this
// package does not model. It is opaque to traversal.
type RawType struct {
	typeBase
	Text string
}

func (*RawType) Kind() Kind { return KindRawType }

// Raw returns a RawType carrying text.
func Raw(text string) *RawType { return &RawType{Text: text} }

// ---------------------------------------------------------------------------
// Convenience constructors
// ---------------------------------------------------------------------------

// Union returns the union of types, flattening nested unions and removing
// duplicate keywords. One member is returned unwrapped; none becomes never.
func Union(types ...Type) Type {
	flat := make([]Type, 0, len(types))
	var seen map[KeywordKind]bool
	for _, t := range types {
		if t == nil {
			continue
		}
		if u, ok := t.(*UnionType); ok {
			for _, m := range u.Types {
				flat = appendUnique(flat, m, &seen)
			}
			continue
		}
		flat = appendUnique(flat, t, &seen)
	}
	switch len(flat) {
	case 0:
		return Never
	case 1:
		return flat[0]
	default:
		return &UnionType{Types: flat}
	}
}

func appendUnique(dst []Type, t Type, seen *map[KeywordKind]bool) []Type {
	if kw, ok := t.(*KeywordType); ok {
		if *seen == nil {
			*seen = make(map[KeywordKind]bool, 4)
		}
		if (*seen)[kw.Keyword] {
			return dst
		}
		(*seen)[kw.Keyword] = true
	}
	return append(dst, t)
}

// Intersection returns the intersection of types, flattening nested ones.
// One member is returned unwrapped; none becomes unknown.
func Intersection(types ...Type) Type {
	flat := make([]Type, 0, len(types))
	for _, t := range types {
		if t == nil {
			continue
		}
		if i, ok := t.(*IntersectionType); ok {
			flat = append(flat, i.Types...)
			continue
		}
		flat = append(flat, t)
	}
	switch len(flat) {
	case 0:
		return Unknown
	case 1:
		return flat[0]
	default:
		return &IntersectionType{Types: flat}
	}
}

// Array returns elem[].
func Array(elem Type) *ArrayType { return &ArrayType{Elem: elem} }

// Nullable returns t | null.
func Nullable(t Type) Type { return Union(t, Null) }

// Optional returns t | undefined.
func Optional(t Type) Type { return Union(t, Undefined) }

// Record returns Record<k, v>.
func Record(k, v Type) *TypeRef { return Ref("Record", k, v) }

// Promise returns Promise<t>.
func Promise(t Type) *TypeRef { return Ref("Promise", t) }

// Partial returns Partial<t>.
func Partial(t Type) *TypeRef { return Ref("Partial", t) }

// Readonly returns Readonly<t>.
func ReadonlyOf(t Type) *TypeRef { return Ref("Readonly", t) }

// KeyOf returns keyof t.
func KeyOf(t Type) *TypeOperator { return &TypeOperator{Op: OpKeyOf, Type: t} }

// Index returns obj[idx].
func Index(obj, idx Type) *IndexedAccessType {
	return &IndexedAccessType{Object: obj, Index: idx}
}

// StringUnion returns a union of string literal types.
func StringUnion(values ...string) Type {
	ts := make([]Type, len(values))
	for i, v := range values {
		ts[i] = StringLiteral(v)
	}
	return Union(ts...)
}
