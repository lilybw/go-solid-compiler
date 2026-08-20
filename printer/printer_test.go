package printer_test

import (
	"strings"
	"testing"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/printer"
)

// print renders t with wrapping disabled, so that these tests assert on
// precedence and spelling rather than on layout.
func print1(n ast.Node) string {
	return strings.TrimSpace(printer.Print(n, printer.WithMaxLineWidth(0)))
}

func TestTypePrecedence(t *testing.T) {
	fn := func(ret ast.Type) *ast.FunctionType {
		return &ast.FunctionType{Signature: ast.Signature{Return: ret}}
	}

	tests := []struct {
		name string
		in   ast.Type
		want string
	}{
		{"array of union parenthesizes",
			ast.Array(ast.Union(ast.String, ast.Null)),
			"(string | null)[]"},
		{"function in union parenthesizes",
			ast.Union(fn(ast.Void), ast.Null),
			"(() => void) | null"},
		{"union in intersection parenthesizes",
			ast.Intersection(ast.Union(ast.String, ast.Number), ast.Ref("A")),
			"(string | number) & A"},
		{"intersection in union does not parenthesize",
			ast.Union(ast.Intersection(ast.Ref("A"), ast.Ref("B")), ast.Null),
			"A & B | null"},
		{"nested union flattens",
			ast.Union(ast.Ref("A"), ast.Union(ast.Ref("B"), ast.Ref("C"))),
			"A | B | C"},
		{"keyof binds tighter than array",
			ast.KeyOf(ast.Array(ast.Ref("User"))),
			"keyof User[]"},
		{"array of keyof parenthesizes",
			ast.Array(ast.KeyOf(ast.Ref("T"))),
			"(keyof T)[]"},
		{"conditional in array parenthesizes",
			ast.Array(&ast.ConditionalType{
				Check: ast.Ref("T"), Extends: ast.String, True: ast.Number, False: ast.Never}),
			"(T extends string ? number : never)[]"},
		{"function in check position parenthesizes",
			&ast.ConditionalType{
				Check: fn(ast.Void), Extends: ast.Ref("F"), True: ast.Number, False: ast.Never},
			"(() => void) extends F ? number : never"},
		{"nested conditional right-associates without parens",
			&ast.ConditionalType{
				Check: ast.Ref("A"), Extends: ast.Ref("B"), True: ast.Number,
				False: &ast.ConditionalType{
					Check: ast.Ref("C"), Extends: ast.Ref("D"), True: ast.String, False: ast.Never}},
			"A extends B ? number : C extends D ? string : never"},
		{"indexed access",
			ast.Index(ast.Ref("User"), ast.StringLiteral("id")),
			"User['id']"},
		{"union collapses duplicates",
			ast.Union(ast.String, ast.String, ast.Null),
			"string | null"},
		{"empty union is never", ast.Union(), "never"},
		{"single member union unwraps", ast.Union(ast.String), "string"},
		{"rest and optional tuple members",
			&ast.TupleType{Elems: []ast.Type{
				ast.String,
				&ast.OptionalType{Elem: ast.Number},
				&ast.RestType{Elem: ast.Array(ast.Boolean)},
			}},
			"[string, number?, ...boolean[]]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := print1(tc.in); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestExpressionPrecedence(t *testing.T) {
	bin := func(op string, l, r ast.Expr) ast.Expr {
		return &ast.BinaryExpr{Op: op, Left: l, Right: r}
	}
	n := func(s string) ast.Expr { return ast.Num(s) }

	tests := []struct {
		name string
		in   ast.Expr
		want string
	}{
		{"sum in product parenthesizes",
			bin("*", bin("+", n("1"), n("2")), n("3")), "(1 + 2) * 3"},
		{"product in sum does not",
			bin("+", n("1"), bin("*", n("2"), n("3"))), "1 + 2 * 3"},
		{"right operand of left-assoc parenthesizes",
			bin("-", n("1"), bin("-", n("2"), n("3"))), "1 - (2 - 3)"},
		{"exponent is right-associative",
			bin("**", n("2"), bin("**", n("3"), n("4"))), "2 ** 3 ** 4"},
		{"unary minus does not form decrement",
			&ast.UnaryExpr{Op: "-", Expr: &ast.UnaryExpr{Op: "-", Expr: n("1")}}, "- -1"},
		{"word operator gets a space",
			&ast.UnaryExpr{Op: "typeof", Expr: ast.NewIdent("x")}, "typeof x"},
		{"object literal arrow body parenthesizes",
			&ast.ArrowFunc{Expr: &ast.ObjectLit{Props: []ast.ObjectProp{
				{Name: ast.NewIdent("a"), Value: n("1")}}}}, "() => ({ a: 1 })"},
		{"nested conditional chains",
			&ast.CondExpr{Cond: ast.NewIdent("a"), Then: n("1"),
				Else: &ast.CondExpr{Cond: ast.NewIdent("b"), Then: n("2"), Else: n("3")}},
			"a ? 1 : b ? 2 : 3"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := print1(tc.in); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestPropertyNameQuoting(t *testing.T) {
	lit := &ast.TypeLiteral{Members: []ast.Member{
		ast.Prop("plain", ast.String),
		ast.Prop("kebab-case", ast.String),
		ast.Prop("has space", ast.String),
		ast.Prop("$dollar", ast.String),
		ast.Prop("_under", ast.String),
		ast.Prop("123", ast.Number),
	}}
	got := print1(lit)
	for _, want := range []string{
		"plain: string", "'kebab-case': string", "'has space': string",
		"$dollar: string", "_under: string", "'123': number",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestStringEscaping(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", `'plain'`},
		{"it's", `'it\'s'`},
		{"line\nbreak", `'line\nbreak'`},
		{`back\slash`, `'back\\slash'`},
		{"tab\there", `'tab\there'`},
	}
	for _, tc := range tests {
		got := print1(ast.StringLiteral(tc.in))
		if got != tc.want {
			t.Errorf("quote(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestNoTrailingWhitespace(t *testing.T) {
	long := ast.Union(
		ast.StringLiteral("aaaaaaaaaaaaaaa"), ast.StringLiteral("bbbbbbbbbbbbbbb"),
		ast.StringLiteral("ccccccccccccccc"), ast.StringLiteral("ddddddddddddddd"))
	out := printer.Print(
		&ast.TypeAliasDecl{Name: ast.NewIdent("Big"), Type: long, Mods: ast.ModExport},
		printer.WithMaxLineWidth(40))
	for i, line := range strings.Split(out, "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("line %d has trailing whitespace: %q", i+1, line)
		}
	}
	if !strings.Contains(out, "\n  | 'aaaaaaaaaaaaaaa'") {
		t.Errorf("expected leading-delimiter wrapping, got:\n%s", out)
	}
}

func TestInlineObjectTypeStaysOnOneLine(t *testing.T) {
	lit := &ast.TypeLiteral{Members: []ast.Member{
		ast.Prop("a", ast.String), ast.Prop("b", ast.Number)}}
	got := print1(lit)
	if got != "{ a: string; b: number }" {
		t.Errorf("got %q", got)
	}
}

func TestInterfaceBodyAlwaysBlock(t *testing.T) {
	d := &ast.InterfaceDecl{
		Name:    ast.NewIdent("Tiny"),
		Members: []ast.Member{ast.Prop("a", ast.String)},
	}
	got := print1(d)
	if !strings.Contains(got, "\n  a: string;\n") {
		t.Errorf("interface body should be block-formatted, got:\n%s", got)
	}
}

func TestOptionsAreHonoured(t *testing.T) {
	d := &ast.TypeAliasDecl{Name: ast.NewIdent("X"), Type: ast.StringLiteral("v")}

	if got := print1(d); got != `type X = 'v';` {
		t.Errorf("default: got %s", got)
	}
	got := strings.TrimSpace(printer.Print(d,
		printer.WithQuote(printer.DoubleQuote), printer.WithSemicolons(false)))
	if got != `type X = "v"` {
		t.Errorf("double quotes, no semicolons: got %s", got)
	}
}

func TestGeneratedBanner(t *testing.T) {
	f := &ast.SourceFile{Stmts: []ast.Stmt{
		&ast.TypeAliasDecl{Name: ast.NewIdent("X"), Type: ast.String}}}
	out := printer.Print(f, printer.GeneratedBanner("tool"))
	if !strings.HasPrefix(out, "// Code generated by tool. DO NOT EDIT.\n") {
		t.Errorf("missing banner, got:\n%s", out)
	}
	if !strings.Contains(out, "@generated") {
		t.Errorf("missing @generated marker")
	}
}

func TestExportWrapperDoesNotMutate(t *testing.T) {
	inner := &ast.TypeAliasDecl{Name: ast.NewIdent("X"), Type: ast.String}
	out := print1(&ast.ExportDecl{Decl: inner})
	if !strings.HasPrefix(out, "export type X") {
		t.Errorf("got %s", out)
	}
	if inner.Mods.Has(ast.ModExport) {
		t.Error("printing mutated the caller's declaration")
	}
}
