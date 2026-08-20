//go:build go1.27

package build

import (
	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/bind"
)

// Generic methods, available on Go 1.27 and later, letting a Go type name a
// member's type inside a builder chain:
//
//	build.Interface("Post").PropOf[time.Time](b, "createdAt")
//
// The binder stays an explicit argument so it is clear which one accumulates
// the resulting declarations.

// PropOf appends a property whose type is projected from the Go type T.
func (b *InterfaceBuilder) PropOf[T any](bd *bind.Binder, name string, opts ...PropOption) *InterfaceBuilder {
	return b.Prop(name, bd.Of[T](), opts...)
}

// ExtendsOf appends an extends clause entry referencing the Go type T. It is
// a no-op if T does not project to a named type.
func (b *InterfaceBuilder) ExtendsOf[T any](bd *bind.Binder) *InterfaceBuilder {
	if ref, ok := bd.Of[T]().(*ast.TypeRef); ok {
		b.d.Extends = append(b.d.Extends, &ast.Heritage{Name: ref.Name, Args: ref.Args})
	}
	return b
}

// ParamOf appends a parameter whose type is projected from the Go type T.
func (s *SigBuilder) ParamOf[T any](bd *bind.Binder, name string, opts ...ParamOption) *SigBuilder {
	return s.Param(name, bd.Of[T](), opts...)
}

// ReturnsOf sets the return type from the Go type T.
func (s *SigBuilder) ReturnsOf[T any](bd *bind.Binder) *SigBuilder {
	return s.Returns(bd.Of[T]())
}

// ParamOf appends a parameter whose type is projected from the Go type T.
func (b *FuncBuilder) ParamOf[T any](bd *bind.Binder, name string, opts ...ParamOption) *FuncBuilder {
	return b.Param(name, bd.Of[T](), opts...)
}

// ReturnsOf sets the return type from the Go type T.
func (b *FuncBuilder) ReturnsOf[T any](bd *bind.Binder) *FuncBuilder {
	return b.Returns(bd.Of[T]())
}

// AliasOf returns an exported type alias binding name to the projection of T.
func AliasOf[T any](bd *bind.Binder, name string) *AliasBuilder {
	return Alias(name, bd.Of[T]())
}

// DeclareOf projects T and appends every declaration it implied to the file.
//
//	build.File("models.ts").DeclareOf[User](b).DeclareOf[Invoice](b).Build()
func (b *FileBuilder) DeclareOf[T any](bd *bind.Binder) *FileBuilder {
	before := len(bd.Declarations())
	bd.Of[T]()
	return b.AddDecls(bd.Declarations()[before:]...)
}
