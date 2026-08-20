package ast

// ---------------------------------------------------------------------------
// Declarations
// ---------------------------------------------------------------------------

// Heritage is one entry of an extends or implements clause.
type Heritage struct {
	Base
	Name EntityName
	Args []Type
}

func (*Heritage) Kind() Kind { return KindHeritage }

// Extends returns a heritage clause entry for the named type.
func Extends(name string, args ...Type) *Heritage {
	return &Heritage{Name: parseEntityName(name), Args: args}
}

// InterfaceDecl is an interface declaration.
type InterfaceDecl struct {
	declBase
	Name       *Ident
	TypeParams []*TypeParam
	Extends    []*Heritage
	Members    []Member
	Mods       Modifier
	Docs       *Doc
}

func (*InterfaceDecl) Kind() Kind { return KindInterfaceDecl }

// TypeAliasDecl is a type alias declaration.
type TypeAliasDecl struct {
	declBase
	Name       *Ident
	TypeParams []*TypeParam
	Type       Type
	Mods       Modifier
	Docs       *Doc
}

func (*TypeAliasDecl) Kind() Kind { return KindTypeAliasDecl }

// EnumMember is one member of an enum.
type EnumMember struct {
	Base
	Name  PropertyName
	Value Expr // may be nil for auto-numbering
	Docs  *Doc
}

func (*EnumMember) Kind() Kind { return KindEnumMember }

// EnumDecl is an enum declaration. Set ModConst for a const enum.
type EnumDecl struct {
	declBase
	Name    *Ident
	Members []*EnumMember
	Mods    Modifier
	Docs    *Doc
}

func (*EnumDecl) Kind() Kind { return KindEnumDecl }

// ClassDecl is a class declaration.
type ClassDecl struct {
	declBase
	Name       *Ident // may be nil for a default-exported anonymous class
	TypeParams []*TypeParam
	Extends    *Heritage
	Implements []*Heritage
	Members    []Member
	Mods       Modifier
	Docs       *Doc
}

func (*ClassDecl) Kind() Kind { return KindClassDecl }

// FunctionDecl is a function declaration. A nil Body prints as an ambient
// signature, as required in a .d.ts.
type FunctionDecl struct {
	declBase
	Name *Ident
	Signature
	Body      *BlockStmt // may be nil
	Mods      Modifier
	Generator bool
	Docs      *Doc
}

func (*FunctionDecl) Kind() Kind { return KindFunctionDecl }

// VarKind is the binding form of a variable declaration.
type VarKind uint8

const (
	VarConst VarKind = iota
	VarLet
	VarVar
	VarUsing
	VarAwaitUsing
)

func (v VarKind) String() string {
	switch v {
	case VarLet:
		return "let"
	case VarVar:
		return "var"
	case VarUsing:
		return "using"
	case VarAwaitUsing:
		return "await using"
	default:
		return "const"
	}
}

// Binding is a single name in a variable declaration.
type Binding struct {
	Base
	Name     PropertyName
	Type     Type
	Value    Expr
	Definite bool
}

func (*Binding) Kind() Kind { return KindBinding }

// VarDecl is a variable declaration statement, possibly with several bindings.
type VarDecl struct {
	declBase
	VarKind  VarKind
	Bindings []*Binding
	Mods     Modifier
	Docs     *Doc
}

func (*VarDecl) Kind() Kind { return KindVarDecl }

// ModuleKind discriminates the flavours of module and namespace declaration.
type ModuleKind uint8

const (
	ModuleNamespace ModuleKind = iota // namespace X { }
	ModuleAmbient                     // declare module "x" { }
	ModuleGlobal                      // declare global { }
)

// ModuleDecl is a namespace, ambient module, or global augmentation block.
type ModuleDecl struct {
	declBase
	ModuleKind ModuleKind
	Name       string // dotted for namespaces, module specifier for ambient
	Body       []Stmt
	Mods       Modifier
	Docs       *Doc
}

func (*ModuleDecl) Kind() Kind { return KindModuleDecl }

// ---------------------------------------------------------------------------
// Imports and exports
// ---------------------------------------------------------------------------

// ImportSpec is one named binding in an import or export clause. Name is the
// name in the other module; Alias, when set, is the local name.
type ImportSpec struct {
	Name     string
	Alias    string
	TypeOnly bool
}

// ImportDecl is an import declaration. With only Module set it prints as a
// side-effect import.
type ImportDecl struct {
	declBase
	Default    string // default binding, if any
	Namespace  string // * as NS binding, if any
	Named      []ImportSpec
	Module     string
	TypeOnly   bool
	Attributes []ImportAttribute
	Docs       *Doc
}

func (*ImportDecl) Kind() Kind { return KindImportDecl }

// ExportDecl is an export clause or re-export. A non-nil Decl exports that
// declaration; otherwise it is a clause, optionally re-exporting Module.
type ExportDecl struct {
	declBase
	Decl       Decl // export <decl>
	Named      []ImportSpec
	Star       bool   // export * from "m"
	StarAs     string // export * as ns from "m"
	Module     string
	TypeOnly   bool
	Attributes []ImportAttribute
	Docs       *Doc
}

func (*ExportDecl) Kind() Kind { return KindExportDecl }

// ExportAssign is export = X or export default X.
type ExportAssign struct {
	declBase
	Expr    Expr
	Default bool // true for export default, false for export =
	Docs    *Doc
}

func (*ExportAssign) Kind() Kind { return KindExportAssign }

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

// BlockStmt is a braced statement list.
type BlockStmt struct {
	stmtBase
	Stmts []Stmt
}

func (*BlockStmt) Kind() Kind { return KindBlockStmt }

// ReturnStmt is a return statement.
type ReturnStmt struct {
	stmtBase
	Value Expr // may be nil
}

func (*ReturnStmt) Kind() Kind { return KindReturnStmt }

// ExprStmt is an expression statement.
type ExprStmt struct {
	stmtBase
	Expr Expr
}

func (*ExprStmt) Kind() Kind { return KindExprStmt }

// RawStmt carries verbatim TypeScript text in statement position.
type RawStmt struct {
	stmtBase
	Text string
}

func (*RawStmt) Kind() Kind { return KindRawStmt }

// ---------------------------------------------------------------------------
// Source files
// ---------------------------------------------------------------------------

// ScriptKind describes how a source file should be parsed and printed.
type ScriptKind uint8

const (
	ScriptTS ScriptKind = iota
	ScriptTSX
	ScriptDTS
	ScriptJS
	ScriptJSX
)

// SourceFile is a whole module. Leading holds file-level comment lines,
// emitted before everything else.
type SourceFile struct {
	Base
	Name       string
	ScriptKind ScriptKind
	Leading    []string
	Stmts      []Stmt
}

func (*SourceFile) Kind() Kind { return KindSourceFile }

// Add appends statements to the file and returns it, allowing chaining.
func (f *SourceFile) Add(stmts ...Stmt) *SourceFile {
	f.Stmts = append(f.Stmts, stmts...)
	return f
}
