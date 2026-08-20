// Package tsx parses TypeScript and TSX and lowers Solid JSX.
//
// Parsing is delegated to the Go port of the TypeScript compiler, so the
// grammar is the real one rather than an approximation. Two entry points:
//
//   - [Parse] returns the compiler's own AST, which is what [TransformSolid]
//     and any other analysis should work on.
//   - [Frontend] implements [parse.Frontend], converting the type-level
//     subset into this library's canonical AST for generation.
//
// This package depends on github.com/lilybw/typescript-go, a fork that
// re-exports the compiler internals. Pin it to an exact version.
package tsx

import (
	"fmt"
	"strings"

	tsast "github.com/lilybw/typescript-go/use-at-your-own-risk/ast"
	tscore "github.com/lilybw/typescript-go/use-at-your-own-risk/core"
	tslocale "github.com/lilybw/typescript-go/use-at-your-own-risk/locale"
	tsparser "github.com/lilybw/typescript-go/use-at-your-own-risk/parser"
	tspath "github.com/lilybw/typescript-go/use-at-your-own-risk/tspath"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/parse"
	"github.com/lilybw/go-solid-compiler/token"
)

// SourceFile is the compiler's source file type.
type SourceFile = tsast.SourceFile

// Node is the compiler's node type.
type Node = tsast.Node

// Kind is the compiler's node kind discriminant.
type Kind = tsast.Kind

// ScriptKind selects the dialect used to parse a file.
type ScriptKind uint8

const (
	// TS parses TypeScript without JSX, where a leading angle bracket is a type
	// assertion rather than an element.
	TS ScriptKind = iota
	// TSX parses TypeScript with JSX.
	TSX
	// DTS parses a declaration file.
	DTS
	// JS parses JavaScript, including JSDoc types.
	JS
	// JSX parses JavaScript with JSX.
	JSX
)

func (k ScriptKind) toCore() tscore.ScriptKind {
	switch k {
	case TSX:
		return tscore.ScriptKindTSX
	case JS:
		return tscore.ScriptKindJS
	case JSX:
		return tscore.ScriptKindJSX
	default:
		// A .d.ts file is still TypeScript; the declaration-ness is carried by
		// the file name, which the parser inspects itself.
		return tscore.ScriptKindTS
	}
}

// ScriptKindOf infers the dialect from a file name, which is what a build
// pipeline usually wants.
func ScriptKindOf(fileName string) ScriptKind {
	switch {
	case strings.HasSuffix(fileName, ".d.ts"):
		return DTS
	case strings.HasSuffix(fileName, ".tsx"):
		return TSX
	case strings.HasSuffix(fileName, ".jsx"):
		return JSX
	case strings.HasSuffix(fileName, ".js"), strings.HasSuffix(fileName, ".mjs"),
		strings.HasSuffix(fileName, ".cjs"):
		return JS
	default:
		return TS
	}
}

// Parse parses source text into the compiler's AST.
//
// Syntax errors are returned as a [parse.ErrorList], but the tree is still
// returned: a partial tree is useful to editors and watch-mode rebuilds.
func Parse(fileName, source string, kind ScriptKind) (file *SourceFile, err error) {
	// The parser asserts its preconditions with panics. Converting them to
	// errors keeps one bad input from killing a long-running build process.
	defer func() {
		if r := recover(); r != nil {
			file = nil
			err = fmt.Errorf("tsx: parsing %s: %v", fileName, r)
		}
	}()

	name := normalizeFileName(fileName)
	opts := tsast.SourceFileParseOptions{
		FileName: name,
		Path:     tspath.ToPath(name, VirtualRoot, true /*useCaseSensitiveFileNames*/),
		ExternalModuleIndicatorOptions: tsast.ExternalModuleIndicatorOptions{
			JSX: kind == TSX || kind == JSX,
		},
	}
	file = tsparser.ParseSourceFile(opts, source, kind.toCore())
	if file == nil {
		return nil, fmt.Errorf("tsx: parser returned no source file for %s", fileName)
	}
	// Report against the caller's own spelling: they recognize "Button.tsx",
	// not the synthetic "/Button.tsx" the parser required.
	return file, diagnosticsError(fileName, file)
}

// VirtualRoot is the directory a relative file name is resolved against.
//
// It is a fixed synthetic root rather than the working directory, so that
// output and diagnostics do not depend on where the build ran. Absolute
// paths are used unchanged.
const VirtualRoot = "/"

// normalizeFileName satisfies the parser's precondition that a file name be
// rooted and normalized.
func normalizeFileName(fileName string) string {
	if fileName == "" {
		return VirtualRoot + "input.tsx"
	}
	if tspath.GetEncodedRootLength(fileName) == 0 {
		return tspath.GetNormalizedAbsolutePath(fileName, VirtualRoot)
	}
	return tspath.NormalizePath(fileName)
}

// ParseFile parses using the dialect inferred from the file name.
func ParseFile(fileName, source string) (*SourceFile, error) {
	return Parse(fileName, source, ScriptKindOf(fileName))
}

// diagnosticsError converts parser diagnostics into a parse.ErrorList.
func diagnosticsError(fileName string, file *SourceFile) error {
	diags := file.Diagnostics()
	if len(diags) == 0 {
		return nil
	}
	var list parse.ErrorList
	for _, d := range diags {
		line, col := lineAndColumn(file, d.Pos())
		list = append(list, parse.Error{
			Pos: token.Location{File: fileName, Line: line, Column: col},
			Msg: d.Localize(tslocale.Default),
		})
	}
	return list
}

// lineAndColumn converts a byte offset to a 1-based line and column.
func lineAndColumn(file *SourceFile, pos int) (int, int) {
	text := file.Text()
	if pos < 0 || pos > len(text) {
		return 1, 1
	}
	line, lineStart := 1, 0
	for i := 0; i < pos; i++ {
		if text[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return line, pos - lineStart + 1
}

// ---------------------------------------------------------------------------
// parse.Frontend implementation
// ---------------------------------------------------------------------------

// Frontend implements [parse.Frontend] on top of the TypeScript compiler.
//
// Conversion covers the type grammar and the declarations that carry types.
// Statements and expressions are preserved verbatim as [ast.RawStmt], which
// keeps the tree printable without duplicating the compiler's expression
// model. To work with expressions structurally, use [Parse] instead.
type Frontend struct{}

var _ parse.Frontend = Frontend{}

// Capabilities reports what this frontend covers.
func (Frontend) Capabilities() parse.Capability {
	return parse.CapTypes | parse.CapDeclarations | parse.CapJSX |
		parse.CapComments | parse.CapPositions
}

// ParseFile parses a module and converts it to the canonical AST.
func (f Frontend) ParseFile(name string, src []byte) (*ast.SourceFile, error) {
	file, err := Parse(name, string(src), ScriptKindOf(name))
	if file == nil {
		return nil, err
	}
	return convertSourceFile(file), err
}

// ParseType parses a single type expression.
func (f Frontend) ParseType(src []byte) (ast.Type, error) {
	const prefix = "type __T = "
	file, err := Parse("__type__.ts", prefix+string(src)+";", TS)
	if err != nil {
		return nil, err
	}
	for _, stmt := range file.Statements.Nodes {
		if stmt.Kind == tsast.KindTypeAliasDeclaration {
			return convertType(stmt.AsTypeAliasDeclaration().Type), nil
		}
	}
	return nil, fmt.Errorf("tsx: could not parse %q as a type", src)
}
