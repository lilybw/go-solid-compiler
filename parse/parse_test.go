package parse_test

import (
	"strings"
	"testing"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/parse"
	"github.com/lilybw/go-solid-compiler/printer"
)

func render(n ast.Node) string {
	return strings.TrimSpace(printer.Print(n, printer.WithMaxLineWidth(0)))
}

// TestTypeRoundTrip asserts that parsing then printing is the identity on
// normalized input. Round-tripping is the strongest cheap correctness check
// available for a parser: it exercises the scanner, the grammar, and the
// emitter's precedence rules simultaneously.
func TestTypeRoundTrip(t *testing.T) {
	srcs := []string{
		`string`,
		`string[]`,
		`string[][]`,
		`(string | null)[]`,
		`Array<Array<Map<string, number>>>`,
		`Record<string, () => void>`,
		`(a: string, b?: number, ...rest: unknown[]) => Promise<void>`,
		`new (x: number) => Foo`,
		`abstract new (x: number) => Foo`,
		`<T>(x: T) => T`,
		`{ a: string; b?: number }`,
		`{ readonly a: string }`,
		`{ (): void; new (): Foo; readonly [k: string]: number }`,
		`{ [K in keyof T]: T[K] }`,
		"{ [K in keyof T as `get${Capitalize<K & string>}`]-?: () => T[K] }",
		`T extends string ? number : never`,
		`T extends (infer U)[] ? U : never`,
		`T extends infer U extends string ? U : never`,
		`A extends B ? number : C extends D ? string : never`,
		`keyof T`,
		`keyof typeof window.document`,
		`readonly string[]`,
		`unique symbol`,
		`[first: string, second?: number, ...rest: boolean[]]`,
		`[string, number?]`,
		`import('solid-js').Component<Props>`,
		`typeof import('solid-js')`,
		"`a${B}c${D}e`",
		`-1 | 0 | 1`,
		`'a' | 'b' | 'c'`,
		`A.B.C<D.E>`,
		`x is string`,
		`asserts x is string`,
		`asserts x`,
		`this`,
		`{ a: string } & ({ b: number } | { c: boolean })`,
		`Partial<Record<'a' | 'b', number>>`,
		`(() => void) | null`,
		`10n`,
		`true | false`,
	}
	for _, src := range srcs {
		t.Run(src, func(t *testing.T) {
			typ, err := parse.Type(src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got := render(typ); got != src {
				t.Errorf("round trip:\n in  %s\n out %s", src, got)
			}
		})
	}
}

func TestNestedGenericsCloseCorrectly(t *testing.T) {
	// The scanner lexes ">>" as one token; the parser must split it.
	typ, err := parse.Type(`Map<string, Array<Set<number>>>`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := render(typ); got != `Map<string, Array<Set<number>>>` {
		t.Errorf("got %s", got)
	}
}

func TestDeclarationRoundTrip(t *testing.T) {
	src := `import type { Component } from 'solid-js';
import { createSignal } from 'solid-js';

export interface LoginProps<T = string> extends Base<T>, Other {
  readonly email: string;
  password?: string;
  onSubmit(values: { email: string; password: string }): Promise<void>;
  [key: string]: unknown;
}

export type Handler<T = unknown> = (e: T) => void;

export const enum Status {
  Active = 'active',
  Banned = 'banned',
}

declare module 'virtual:solid' {
  export type Thing = number;
}

export * as utils from './utils';
export { LoginProps as Props };
`
	f, err := parse.File("demo.ts", []byte(src))
	if err != nil {
		t.Fatalf("parse errors: %v", err)
	}
	got := printer.Print(f)
	if strings.TrimSpace(got) != strings.TrimSpace(src) {
		t.Errorf("round trip mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, src)
	}
}

func TestDocCommentsAreAttached(t *testing.T) {
	src := []byte(`
/**
 * A widget.
 * @public
 */
export interface Widget {
  /** The identifier. */
  id: string;
}
`)
	f, err := parse.File("d.ts", src)
	if err != nil {
		t.Fatalf("parse errors: %v", err)
	}
	decl, ok := f.Stmts[0].(*ast.InterfaceDecl)
	if !ok {
		t.Fatalf("expected an interface, got %T", f.Stmts[0])
	}
	if decl.Docs == nil || len(decl.Docs.Text) == 0 || decl.Docs.Text[0] != "A widget." {
		t.Errorf("interface doc not attached: %+v", decl.Docs)
	}
	if len(decl.Docs.Tags) != 1 || decl.Docs.Tags[0].Name != "public" {
		t.Errorf("interface doc tag not attached: %+v", decl.Docs)
	}
	prop := decl.Members[0].(*ast.PropertySignature)
	if prop.Docs == nil || prop.Docs.Text[0] != "The identifier." {
		t.Errorf("member doc not attached: %+v", prop.Docs)
	}
}

func TestPositionsAreRecorded(t *testing.T) {
	f, err := parse.File("p.ts", []byte("export interface A { b: string }"))
	if err != nil {
		t.Fatalf("parse errors: %v", err)
	}
	d := f.Stmts[0].(*ast.InterfaceDecl)
	if !d.Span().IsValid() {
		t.Error("declaration has no span")
	}
	prop := d.Members[0].(*ast.PropertySignature)
	if !prop.Span().IsValid() || prop.Span().Start <= d.Span().Start {
		t.Errorf("member span %v is not inside declaration span %v", prop.Span(), d.Span())
	}
}

// TestUnsupportedConstructIsPreserved documents the recovery contract: this
// frontend does not parse statements, but it must never silently lose source.
func TestUnsupportedConstructIsPreserved(t *testing.T) {
	f, err := parse.File("x.ts", []byte("const x = 1;\nexport type T = string;"))
	if err == nil {
		t.Error("expected an error for the unsupported construct")
	}
	var raw *ast.RawStmt
	for _, s := range f.Stmts {
		if r, ok := s.(*ast.RawStmt); ok {
			raw = r
		}
	}
	if raw == nil {
		t.Fatalf("unsupported construct was dropped rather than preserved: %#v", f.Stmts)
	}
	if !strings.Contains(raw.Text, "const x") {
		t.Errorf("raw text is wrong: %q", raw.Text)
	}
	// Parsing must still recover and read the declaration that follows.
	found := false
	for _, s := range f.Stmts {
		if a, ok := s.(*ast.TypeAliasDecl); ok && a.Name.Text == "T" {
			found = true
		}
	}
	if !found {
		t.Error("parser did not recover after the unsupported construct")
	}
}

func TestSyntaxErrorsReportLocations(t *testing.T) {
	_, err := parse.File("e.ts", []byte("export interface A { b: }"))
	if err == nil {
		t.Fatal("expected an error")
	}
	list, ok := err.(parse.ErrorList)
	if !ok {
		t.Fatalf("expected an ErrorList, got %T", err)
	}
	if list[0].Pos.Line != 1 || list[0].Pos.Column == 0 {
		t.Errorf("bad location: %v", list[0].Pos)
	}
}

func TestStringEscapesAreDecoded(t *testing.T) {
	typ, err := parse.Type(`'a\nb\u0041\x42'`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	lit := typ.(*ast.LiteralType)
	if lit.Value != "a\nbAB" {
		t.Errorf("got %q, want %q", lit.Value, "a\nbAB")
	}
}

func TestParserDoesNotLoopOnGarbage(t *testing.T) {
	// Progress must be guaranteed even when nothing parses.
	done := make(chan struct{})
	go func() {
		defer close(done)
		parse.File("g.ts", []byte("} ) ] @@@ ;;; <<< >>>"))
	}()
	select {
	case <-done:
	case <-timeoutAfterSeconds(5):
		t.Fatal("parser failed to terminate on garbage input")
	}
}
