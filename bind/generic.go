//go:build go1.27

package bind

import (
	"reflect"

	"github.com/lilybw/go-solid-compiler/ast"
)

// Generic methods, available on Go 1.27 and later.
//
// The build constraint above means this file is skipped by older toolchains,
// where the equivalent free functions are the whole API. Each method here
// delegates to one.
//
// Interface methods may not declare type parameters, so [Mapper],
// [FieldRule], and [Namer] remain non-generic.

// Of projects the Go type argument into a TypeScript type.
func (b *Binder) Of[T any]() ast.Type { return Of[T](b) }

// Declare projects T for its declarations alone, returning the binder so
// several types can be registered in one chain.
func (b *Binder) Declare[T any]() *Binder {
	Of[T](b)
	return b
}

// Map registers a mapper claiming exactly the Go type T.
//
//	b.Map[time.Time](ast.Ref("Date"))
func (b *Binder) Map[T any](t ast.Type) *Binder {
	b.opts.Mappers = append(b.opts.Mappers, MapExact[T](t))
	return b
}

// MapFunc registers a mapper for T whose result is computed per projection.
func (b *Binder) MapFunc[T any](fn func(*Context) ast.Type) *Binder {
	want := reflect.TypeFor[T]()
	b.opts.Mappers = append(b.opts.Mappers, MapperFunc(
		func(c *Context, rt reflect.Type) (ast.Type, bool) {
			if rt == want {
				return fn(c), true
			}
			return nil, false
		}))
	return b
}

// Enum registers a named union alias for T over the supplied values.
func (b *Binder) Enum[T comparable](values ...T) ast.Type { return Enum(b, values...) }

// Ref returns a reference to T's TypeScript name without projecting it,
// reporting false if T has not been bound.
func (b *Binder) Ref[T any]() (ast.Type, bool) {
	name, ok := b.Lookup(reflect.TypeFor[T]())
	if !ok {
		return nil, false
	}
	return ast.Ref(name), true
}
