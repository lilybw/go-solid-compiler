// Package gots_test exercises the library end to end, across package
// boundaries, the way a consumer would.
package compiler_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/bind"
	"github.com/lilybw/go-solid-compiler/build"
	"github.com/lilybw/go-solid-compiler/parse"
	"github.com/lilybw/go-solid-compiler/printer"
)

// The scenario go-solid needs: a Go struct is passed to a Solid component as
// props, and the component should be typed against it rather than against any.
type Theme string

type Session struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type LoginProps struct {
	Title       string   `json:"title"`
	Theme       Theme    `json:"theme"`
	Session     *Session `json:"session"`
	Redirect    string   `json:"redirect,omitempty"`
	Attempts    int      `json:"attempts"`
	Permissions []string `json:"permissions"`
}

// TestGeneratedOutputReparses is the load-bearing invariant of the library:
// anything the generator emits must be readable by the parser. If the two ever
// disagree, one of them is wrong, and this test says so without needing tsc.
func TestGeneratedOutputReparses(t *testing.T) {
	b := bind.New()
	bind.Enum(b, Theme("light"), Theme("dark"))
	propsRef := bind.Of[LoginProps](b)

	file := build.File("LoginForm.props.ts").
		ImportType("solid-js", build.Named("Component")).
		AddDecls(b.Declarations()...).
		Add(build.Alias("LoginForm", ast.Ref("Component", propsRef)).Export().Build()).
		Build()

	src := printer.Print(file, printer.GeneratedBanner("go-solid-compiler"))

	if b.HasErrors() {
		t.Fatalf("binder reported errors: %v", b.Diagnostics())
	}

	reparsed, err := parse.File("LoginForm.props.ts", []byte(src))
	if err != nil {
		t.Fatalf("generated output does not parse: %v\n\n%s", err, src)
	}

	// Printing the reparsed tree must reproduce the original text.
	again := printer.Print(reparsed, printer.GeneratedBanner("go-solid-compiler"))
	if again != src {
		t.Errorf("generate -> parse -> print is not stable:\n--- first ---\n%s\n--- second ---\n%s", src, again)
	}

	for _, want := range []string{
		"export type Theme = 'light' | 'dark';",
		"theme: Theme;",
		"session: Session | null;",
		"redirect?: string;",
		"permissions: string[];",
		"expiresAt: string;",
		"export type LoginForm = Component<LoginProps>;",
		"import type { Component } from 'solid-js';",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in generated output:\n%s", want, src)
		}
	}
}

// TestParsedTypesAreReusable checks the other direction: a type read from
// existing TypeScript can be spliced into generated output. This is what a
// component-props workflow needs when the source of truth is a .d.ts file.
func TestParsedTypesAreReusable(t *testing.T) {
	existing, err := parse.Type(`{ onClose: () => void; variant: 'modal' | 'inline' }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	b := bind.New()
	generated := bind.Of[LoginProps](b)

	merged := build.Alias("DialogProps", ast.Intersection(generated, existing)).Export().Build()
	got := strings.TrimSpace(printer.Print(merged, printer.WithMaxLineWidth(0)))

	want := `export type DialogProps = LoginProps & { onClose: () => void; variant: 'modal' | 'inline' };`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// TestBuilderAndLiteralNodesInterop confirms the builder layer is genuinely
// optional: hand-built nodes and builder output are the same thing.
func TestBuilderAndLiteralNodesInterop(t *testing.T) {
	fromBuilder := build.Interface("A").
		Export().
		Doc("Built fluently.").
		TypeParam("T", build.Constraint(ast.String)).
		Prop("id", ast.String).
		Prop("note", ast.String, build.Optional, build.Readonly).
		Method("run", func(s *build.SigBuilder) {
			s.Param("x", ast.Ref("T")).Returns(ast.Promise(ast.Void))
		}).
		Build()

	fromLiteral := &ast.InterfaceDecl{
		Name: ast.NewIdent("A"),
		Mods: ast.ModExport,
		Docs: ast.Comment("Built fluently."),
		TypeParams: []*ast.TypeParam{
			{Name: ast.NewIdent("T"), Constraint: ast.String},
		},
		Members: []ast.Member{
			ast.Prop("id", ast.String),
			&ast.PropertySignature{
				Name: ast.NewIdent("note"), Type: ast.String,
				Optional: true, Readonly: true,
			},
			&ast.MethodSignature{
				Name: ast.NewIdent("run"),
				Signature: ast.Signature{
					Params: []*ast.Param{{Name: ast.NewIdent("x"), Type: ast.Ref("T")}},
					Return: ast.Promise(ast.Void),
				},
			},
		},
	}

	a := printer.Print(fromBuilder)
	l := printer.Print(fromLiteral)
	if a != l {
		t.Errorf("builder and literal construction diverge:\n--- builder ---\n%s\n--- literal ---\n%s", a, l)
	}
}

// TestCloneIsolatesBuilders guards the template use case.
func TestCloneIsolatesBuilders(t *testing.T) {
	base := build.Interface("Base").Prop("id", ast.String)
	a := base.Clone().Prop("a", ast.Number).Build()
	bIface := base.Clone().Prop("b", ast.Boolean).Build()

	if len(a.Members) != 2 || len(bIface.Members) != 2 {
		t.Fatalf("clone leaked members: a=%d b=%d", len(a.Members), len(bIface.Members))
	}
	if len(base.Build().Members) != 1 {
		t.Error("clone mutated the template")
	}
}
