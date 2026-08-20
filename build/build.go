// Package build is a fluent construction layer over the ast package.
//
//	iface := build.Interface("User").
//	    Doc("A user of the system.").
//	    Extends("Entity").
//	    Prop("id", ast.String).
//	    Prop("nickname", ast.String, build.Optional).
//	    Build()
//
// Every builder returns plain AST nodes, so builders and literal nodes mix
// freely. Per-member settings are option values rather than chained calls,
// which keeps the chain flat and lets consumers define their own.
package build

import "github.com/lilybw/go-solid-compiler/ast"

// ---------------------------------------------------------------------------
// Member options
// ---------------------------------------------------------------------------

// PropOption customizes a property signature.
type PropOption func(*ast.PropertySignature)

// Optional marks a property optional.
var Optional PropOption = func(p *ast.PropertySignature) { p.Optional = true }

// Readonly marks a property readonly.
var Readonly PropOption = func(p *ast.PropertySignature) { p.Readonly = true }

// PropDoc attaches a JSDoc comment to a property.
func PropDoc(lines ...string) PropOption {
	return func(p *ast.PropertySignature) { p.Docs = ast.Comment(lines...) }
}

// PropTag attaches a JSDoc block tag to a property.
func PropTag(name, text string) PropOption {
	return func(p *ast.PropertySignature) {
		if p.Docs == nil {
			p.Docs = &ast.Doc{}
		}
		p.Docs.Tags = append(p.Docs.Tags, ast.DocTag{Name: name, Text: text})
	}
}

// Deprecated marks a property deprecated.
func Deprecated(reason string) PropOption { return PropTag("deprecated", reason) }

// TypeParamOption customizes a type parameter.
type TypeParamOption func(*ast.TypeParam)

// Constraint sets a type parameter's extends clause.
func Constraint(t ast.Type) TypeParamOption {
	return func(p *ast.TypeParam) { p.Constraint = t }
}

// Default sets a type parameter's default.
func Default(t ast.Type) TypeParamOption {
	return func(p *ast.TypeParam) { p.Default = t }
}

// Const marks a const type parameter.
var Const TypeParamOption = func(p *ast.TypeParam) { p.Const = true }

// In, Out, and InOut set explicit variance annotations.
var (
	In    TypeParamOption = func(p *ast.TypeParam) { p.Variance = ast.VarianceIn }
	Out   TypeParamOption = func(p *ast.TypeParam) { p.Variance = ast.VarianceOut }
	InOut TypeParamOption = func(p *ast.TypeParam) { p.Variance = ast.VarianceInOut }
)

// ParamOption customizes a function parameter.
type ParamOption func(*ast.Param)

// OptionalParam marks a parameter optional.
var OptionalParam ParamOption = func(p *ast.Param) { p.Optional = true }

// Rest marks a parameter as rest.
var Rest ParamOption = func(p *ast.Param) { p.Rest = true }

// DefaultValue sets a parameter's default expression.
func DefaultValue(e ast.Expr) ParamOption { return func(p *ast.Param) { p.Default = e } }

func newTypeParam(name string, opts ...TypeParamOption) *ast.TypeParam {
	tp := &ast.TypeParam{Name: ast.NewIdent(name)}
	for _, o := range opts {
		o(tp)
	}
	return tp
}

func newParam(name string, t ast.Type, opts ...ParamOption) *ast.Param {
	p := &ast.Param{Name: ast.NewIdent(name), Type: t}
	for _, o := range opts {
		o(p)
	}
	return p
}

func newProp(name string, t ast.Type, opts ...PropOption) *ast.PropertySignature {
	p := &ast.PropertySignature{Name: ast.NewIdent(name), Type: t}
	for _, o := range opts {
		o(p)
	}
	return p
}

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// InterfaceBuilder builds an interface declaration.
type InterfaceBuilder struct{ d *ast.InterfaceDecl }

// Interface starts an interface declaration.
func Interface(name string) *InterfaceBuilder {
	return &InterfaceBuilder{d: &ast.InterfaceDecl{Name: ast.NewIdent(name)}}
}

// Export marks the interface exported.
func (b *InterfaceBuilder) Export() *InterfaceBuilder {
	b.d.Mods = b.d.Mods.With(ast.ModExport)
	return b
}

// Declare marks the interface ambient.
func (b *InterfaceBuilder) Declare() *InterfaceBuilder {
	b.d.Mods = b.d.Mods.With(ast.ModDeclare)
	return b
}

// Doc attaches a JSDoc comment.
func (b *InterfaceBuilder) Doc(lines ...string) *InterfaceBuilder {
	b.d.Docs = ast.Comment(lines...)
	return b
}

// Tag appends a JSDoc block tag.
func (b *InterfaceBuilder) Tag(name, text string) *InterfaceBuilder {
	if b.d.Docs == nil {
		b.d.Docs = &ast.Doc{}
	}
	b.d.Docs.Tags = append(b.d.Docs.Tags, ast.DocTag{Name: name, Text: text})
	return b
}

// TypeParam appends a generic type parameter.
func (b *InterfaceBuilder) TypeParam(name string, opts ...TypeParamOption) *InterfaceBuilder {
	b.d.TypeParams = append(b.d.TypeParams, newTypeParam(name, opts...))
	return b
}

// Extends appends an extends clause entry.
func (b *InterfaceBuilder) Extends(name string, args ...ast.Type) *InterfaceBuilder {
	b.d.Extends = append(b.d.Extends, ast.Extends(name, args...))
	return b
}

// Prop appends a property signature.
func (b *InterfaceBuilder) Prop(name string, t ast.Type, opts ...PropOption) *InterfaceBuilder {
	b.d.Members = append(b.d.Members, newProp(name, t, opts...))
	return b
}

// Index appends an index signature.
func (b *InterfaceBuilder) Index(keyName string, keyType, valType ast.Type) *InterfaceBuilder {
	b.d.Members = append(b.d.Members, &ast.IndexSignature{
		KeyName: keyName, KeyType: keyType, Type: valType,
	})
	return b
}

// Method appends a method signature, configured through a signature builder.
func (b *InterfaceBuilder) Method(name string, fn func(*SigBuilder)) *InterfaceBuilder {
	sb := &SigBuilder{}
	if fn != nil {
		fn(sb)
	}
	b.d.Members = append(b.d.Members, &ast.MethodSignature{
		Name: ast.NewIdent(name), Signature: sb.sig, Docs: sb.docs,
	})
	return b
}

// Call appends a bare call signature.
func (b *InterfaceBuilder) Call(fn func(*SigBuilder)) *InterfaceBuilder {
	sb := &SigBuilder{}
	if fn != nil {
		fn(sb)
	}
	b.d.Members = append(b.d.Members, &ast.CallSignature{Signature: sb.sig, Docs: sb.docs})
	return b
}

// Members appends arbitrary members, the escape hatch to hand-built nodes.
func (b *InterfaceBuilder) Members(ms ...ast.Member) *InterfaceBuilder {
	b.d.Members = append(b.d.Members, ms...)
	return b
}

// Clone returns an independent copy, so a partially built interface can
// serve as a template.
func (b *InterfaceBuilder) Clone() *InterfaceBuilder {
	c := *b.d
	c.TypeParams = append([]*ast.TypeParam(nil), b.d.TypeParams...)
	c.Extends = append([]*ast.Heritage(nil), b.d.Extends...)
	c.Members = append([]ast.Member(nil), b.d.Members...)
	return &InterfaceBuilder{d: &c}
}

// Build returns the declaration.
func (b *InterfaceBuilder) Build() *ast.InterfaceDecl { return b.d }

// ---------------------------------------------------------------------------
// Signatures
// ---------------------------------------------------------------------------

// SigBuilder builds the callable part of a function, method, or constructor.
type SigBuilder struct {
	sig  ast.Signature
	docs *ast.Doc
}

// TypeParam appends a generic type parameter.
func (s *SigBuilder) TypeParam(name string, opts ...TypeParamOption) *SigBuilder {
	s.sig.TypeParams = append(s.sig.TypeParams, newTypeParam(name, opts...))
	return s
}

// Param appends a parameter.
func (s *SigBuilder) Param(name string, t ast.Type, opts ...ParamOption) *SigBuilder {
	s.sig.Params = append(s.sig.Params, newParam(name, t, opts...))
	return s
}

// Returns sets the return type.
func (s *SigBuilder) Returns(t ast.Type) *SigBuilder { s.sig.Return = t; return s }

// Doc attaches a JSDoc comment.
func (s *SigBuilder) Doc(lines ...string) *SigBuilder {
	s.docs = ast.Comment(lines...)
	return s
}

// Signature returns the built signature.
func (s *SigBuilder) Signature() ast.Signature { return s.sig }

// Fn returns a function type built from a signature builder.
func Fn(fn func(*SigBuilder)) *ast.FunctionType {
	sb := &SigBuilder{}
	if fn != nil {
		fn(sb)
	}
	return &ast.FunctionType{Signature: sb.sig}
}

// ---------------------------------------------------------------------------
// Type aliases
// ---------------------------------------------------------------------------

// AliasBuilder builds a type alias declaration.
type AliasBuilder struct{ d *ast.TypeAliasDecl }

// Alias starts a type alias declaration.
func Alias(name string, t ast.Type) *AliasBuilder {
	return &AliasBuilder{d: &ast.TypeAliasDecl{Name: ast.NewIdent(name), Type: t}}
}

// Export marks the alias exported.
func (b *AliasBuilder) Export() *AliasBuilder {
	b.d.Mods = b.d.Mods.With(ast.ModExport)
	return b
}

// Doc attaches a JSDoc comment.
func (b *AliasBuilder) Doc(lines ...string) *AliasBuilder {
	b.d.Docs = ast.Comment(lines...)
	return b
}

// TypeParam appends a generic type parameter.
func (b *AliasBuilder) TypeParam(name string, opts ...TypeParamOption) *AliasBuilder {
	b.d.TypeParams = append(b.d.TypeParams, newTypeParam(name, opts...))
	return b
}

// Build returns the declaration.
func (b *AliasBuilder) Build() *ast.TypeAliasDecl { return b.d }

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

// FuncBuilder builds a function declaration.
type FuncBuilder struct {
	d *ast.FunctionDecl
}

// Func starts a function declaration. With no body it prints as an ambient
// signature, as required in a .d.ts.
func Func(name string) *FuncBuilder {
	return &FuncBuilder{d: &ast.FunctionDecl{Name: ast.NewIdent(name)}}
}

// Export marks the function exported.
func (b *FuncBuilder) Export() *FuncBuilder {
	b.d.Mods = b.d.Mods.With(ast.ModExport)
	return b
}

// Declare marks the function ambient.
func (b *FuncBuilder) Declare() *FuncBuilder {
	b.d.Mods = b.d.Mods.With(ast.ModDeclare)
	return b
}

// Async marks the function async.
func (b *FuncBuilder) Async() *FuncBuilder {
	b.d.Mods = b.d.Mods.With(ast.ModAsync)
	return b
}

// Doc attaches a JSDoc comment.
func (b *FuncBuilder) Doc(lines ...string) *FuncBuilder {
	b.d.Docs = ast.Comment(lines...)
	return b
}

// TypeParam appends a generic type parameter.
func (b *FuncBuilder) TypeParam(name string, opts ...TypeParamOption) *FuncBuilder {
	b.d.TypeParams = append(b.d.TypeParams, newTypeParam(name, opts...))
	return b
}

// Param appends a parameter.
func (b *FuncBuilder) Param(name string, t ast.Type, opts ...ParamOption) *FuncBuilder {
	b.d.Params = append(b.d.Params, newParam(name, t, opts...))
	return b
}

// Returns sets the return type.
func (b *FuncBuilder) Returns(t ast.Type) *FuncBuilder { b.d.Return = t; return b }

// Body sets the function body.
func (b *FuncBuilder) Body(stmts ...ast.Stmt) *FuncBuilder {
	b.d.Body = &ast.BlockStmt{Stmts: stmts}
	return b
}

// Build returns the declaration.
func (b *FuncBuilder) Build() *ast.FunctionDecl { return b.d }

// ---------------------------------------------------------------------------
// Source files
// ---------------------------------------------------------------------------

// FileBuilder assembles a source file.
type FileBuilder struct{ f *ast.SourceFile }

// File starts a source file.
func File(name string) *FileBuilder {
	return &FileBuilder{f: &ast.SourceFile{Name: name, ScriptKind: ast.ScriptTS}}
}

// TSX marks the file as containing JSX syntax.
func (b *FileBuilder) TSX() *FileBuilder { b.f.ScriptKind = ast.ScriptTSX; return b }

// Dts marks the file as a declaration file.
func (b *FileBuilder) Dts() *FileBuilder { b.f.ScriptKind = ast.ScriptDTS; return b }

// Leading adds file-level comment lines.
func (b *FileBuilder) Leading(lines ...string) *FileBuilder {
	b.f.Leading = append(b.f.Leading, lines...)
	return b
}

// Named returns a named import or export specifier.
func Named(name string) ast.ImportSpec { return ast.ImportSpec{Name: name} }

// Aliased returns a named specifier with a local alias.
func Aliased(name, alias string) ast.ImportSpec {
	return ast.ImportSpec{Name: name, Alias: alias}
}

// Import appends a named import.
func (b *FileBuilder) Import(module string, specs ...ast.ImportSpec) *FileBuilder {
	b.f.Stmts = append(b.f.Stmts, &ast.ImportDecl{Module: module, Named: specs})
	return b
}

// ImportType appends a type-only import, which erases completely at build time.
func (b *FileBuilder) ImportType(module string, specs ...ast.ImportSpec) *FileBuilder {
	b.f.Stmts = append(b.f.Stmts, &ast.ImportDecl{Module: module, Named: specs, TypeOnly: true})
	return b
}

// ImportDefault appends a default import.
func (b *FileBuilder) ImportDefault(module, name string) *FileBuilder {
	b.f.Stmts = append(b.f.Stmts, &ast.ImportDecl{Module: module, Default: name})
	return b
}

// ImportNamespace appends a namespace import.
func (b *FileBuilder) ImportNamespace(module, name string) *FileBuilder {
	b.f.Stmts = append(b.f.Stmts, &ast.ImportDecl{Module: module, Namespace: name})
	return b
}

// Add appends statements.
func (b *FileBuilder) Add(stmts ...ast.Stmt) *FileBuilder {
	b.f.Stmts = append(b.f.Stmts, stmts...)
	return b
}

// AddDecls appends declarations.
func (b *FileBuilder) AddDecls(ds ...ast.Decl) *FileBuilder {
	for _, d := range ds {
		b.f.Stmts = append(b.f.Stmts, d)
	}
	return b
}

// ExportDefault appends an export default statement.
func (b *FileBuilder) ExportDefault(e ast.Expr) *FileBuilder {
	b.f.Stmts = append(b.f.Stmts, &ast.ExportAssign{Expr: e, Default: true})
	return b
}

// Build returns the source file.
func (b *FileBuilder) Build() *ast.SourceFile { return b.f }
