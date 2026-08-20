// Package parse turns TypeScript source text into the canonical AST.
//
// [Frontend] is the contract; two implementations exist:
//
//   - [TypeFrontend], here, parses the full type grammar plus interface,
//     type alias, and enum declarations with no dependencies.
//   - [github.com/lilybw/go-solid-compiler/tsx.Frontend] wraps the real
//     TypeScript compiler and handles the entire language including TSX.
//
// Use the tsx frontend by default. Use [TypeFrontend] when the type grammar
// is all you need and the compiler dependency is not worth it.
package parse

import (
	"fmt"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/token"
)

// Frontend converts TypeScript source into the canonical AST.
//
// Implementations return the most complete tree they can alongside any
// errors, so that partial results remain useful.
type Frontend interface {
	// ParseFile parses a whole module.
	ParseFile(name string, src []byte) (*ast.SourceFile, error)

	// ParseType parses a single type expression, the operation a code
	// generator needs when accepting a type as configuration.
	ParseType(src []byte) (ast.Type, error)
}

// Capability reports what a frontend supports.
type Capability uint32

const (
	// CapTypes covers the type grammar.
	CapTypes Capability = 1 << iota
	// CapDeclarations covers interface, type alias, enum, class, and function
	// declarations.
	CapDeclarations
	// CapExpressions covers the full expression and statement grammar.
	CapExpressions
	// CapJSX covers .tsx syntax.
	CapJSX
	// CapComments preserves comments as attached documentation.
	CapComments
	// CapPositions records accurate source positions on every node.
	CapPositions
)

// Capable is implemented by frontends that can describe their own coverage.
type Capable interface {
	Capabilities() Capability
}

// Error is a syntax error with a source location.
type Error struct {
	Pos token.Location
	Msg string
}

func (e Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

// ErrorList is a collection of syntax errors.
type ErrorList []Error

func (l ErrorList) Error() string {
	switch len(l) {
	case 0:
		return "no errors"
	case 1:
		return l[0].Error()
	default:
		return fmt.Sprintf("%s (and %d more errors)", l[0], len(l)-1)
	}
}

// Err returns the list as an error, or nil when empty.
func (l ErrorList) Err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}
