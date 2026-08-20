package ast

// Members of interfaces, object type literals, and class bodies.

// PropertySignature is a property in an interface or object type.
type PropertySignature struct {
	memberBase
	Name     PropertyName
	Optional bool
	Readonly bool
	Type     Type // may be nil, meaning implicit any
	Docs     *Doc
}

func (*PropertySignature) Kind() Kind { return KindPropertySignature }

// Prop returns a property signature. The printer quotes the name if it is
// not a valid identifier.
func Prop(name string, t Type) *PropertySignature {
	return &PropertySignature{Name: NewIdent(name), Type: t}
}

// MethodSignature is a method in an interface or object type:
// name?<T>(a: A): R.
type MethodSignature struct {
	memberBase
	Name     PropertyName
	Optional bool
	Signature
	Docs *Doc
}

func (*MethodSignature) Kind() Kind { return KindMethodSignature }

// IndexSignature is [key: K]: V.
type IndexSignature struct {
	memberBase
	KeyName  string
	KeyType  Type
	Type     Type
	Readonly bool
	Static   bool
	Docs     *Doc
}

func (*IndexSignature) Kind() Kind { return KindIndexSignature }

// CallSignature is a bare call signature in an object type: <T>(a: A): R.
type CallSignature struct {
	memberBase
	Signature
	Docs *Doc
}

func (*CallSignature) Kind() Kind { return KindCallSignature }

// ConstructSignature is new <T>(a: A): R.
type ConstructSignature struct {
	memberBase
	Signature
	Docs *Doc
}

func (*ConstructSignature) Kind() Kind { return KindConstructSignature }

// GetAccessor is get name(): T.
type GetAccessor struct {
	memberBase
	Name PropertyName
	Mods Modifier
	Signature
	Body *BlockStmt // nil in declarations
	Docs *Doc
}

func (*GetAccessor) Kind() Kind { return KindGetAccessor }

// SetAccessor is set name(v: T).
type SetAccessor struct {
	memberBase
	Name PropertyName
	Mods Modifier
	Signature
	Body *BlockStmt // nil in declarations
	Docs *Doc
}

func (*SetAccessor) Kind() Kind { return KindSetAccessor }

// PropertyDecl is a class property, which unlike a PropertySignature may carry
// modifiers, an initializer, and a definite-assignment assertion.
type PropertyDecl struct {
	memberBase
	Name     PropertyName
	Mods     Modifier
	Optional bool
	Definite bool // name!: T
	Type     Type
	Value    Expr // may be nil
	Docs     *Doc
}

func (*PropertyDecl) Kind() Kind { return KindPropertyDecl }

// MethodDecl is a class method. A nil Body prints as a signature only.
type MethodDecl struct {
	memberBase
	Name     PropertyName
	Mods     Modifier
	Optional bool
	Signature
	Body *BlockStmt // may be nil
	Docs *Doc
}

func (*MethodDecl) Kind() Kind { return KindMethodDecl }
