package ast_test

import (
	"testing"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/token"
)

func sample() *ast.SourceFile {
	return &ast.SourceFile{
		Name: "s.ts",
		Stmts: []ast.Stmt{
			&ast.InterfaceDecl{
				Name: ast.NewIdent("User"),
				Members: []ast.Member{
					ast.Prop("id", ast.String),
					ast.Prop("tags", ast.Array(ast.Ref("Tag"))),
					&ast.MethodSignature{
						Name: ast.NewIdent("save"),
						Signature: ast.Signature{
							Params: []*ast.Param{{Name: ast.NewIdent("opts"), Type: ast.Ref("Opts")}},
							Return: ast.Promise(ast.Void),
						},
					},
				},
			},
		},
	}
}

func TestChildrenSkipsNils(t *testing.T) {
	// A property with no type must not yield a nil child.
	p := &ast.PropertySignature{Name: ast.NewIdent("x")}
	for _, c := range ast.Children(p) {
		if c == nil {
			t.Fatal("Children returned a nil node")
		}
	}
	if got := len(ast.Children(p)); got != 1 {
		t.Errorf("expected 1 child (the name), got %d", got)
	}
}

func TestChildrenHandlesTypedNilPointers(t *testing.T) {
	// A typed nil stored in an interface is not == nil; Children must still
	// filter it, or consumers crash on a value that looks non-nil.
	d := &ast.FunctionDecl{Name: ast.NewIdent("f"), Body: nil}
	for _, c := range ast.Children(d) {
		if c == nil {
			t.Fatal("nil child")
		}
		if _, ok := c.(*ast.BlockStmt); ok {
			t.Fatal("nil body was yielded as a child")
		}
	}
}

func TestInspectVisitsEveryNode(t *testing.T) {
	var refs, props int
	ast.Inspect(sample(), func(n ast.Node) bool {
		switch n.(type) {
		case *ast.TypeRef:
			refs++
		case *ast.PropertySignature:
			props++
		}
		return true
	})
	if props != 2 {
		t.Errorf("expected 2 property signatures, got %d", props)
	}
	// Tag, Opts, Promise
	if refs != 3 {
		t.Errorf("expected 3 type references, got %d", refs)
	}
}

func TestInspectPrunesOnFalse(t *testing.T) {
	var seen int
	ast.Inspect(sample(), func(n ast.Node) bool {
		seen++
		_, isIface := n.(*ast.InterfaceDecl)
		return !isIface // do not descend into the interface
	})
	if seen != 2 { // the file and the interface
		t.Errorf("expected traversal to stop at the interface, visited %d nodes", seen)
	}
}

func TestWalkVisitor(t *testing.T) {
	var idents int
	ast.Walk(ast.VisitorFunc(func(n ast.Node) bool {
		if _, ok := n.(*ast.Ident); ok {
			idents++
		}
		return true
	}), sample())
	if idents == 0 {
		t.Error("visitor saw no identifiers")
	}
}

func TestFind(t *testing.T) {
	got := ast.Find(sample(), func(n ast.Node) bool {
		r, ok := n.(*ast.TypeRef)
		if !ok {
			return false
		}
		id, ok := r.Name.(*ast.Ident)
		return ok && id.Text == "Opts"
	})
	if got == nil {
		t.Fatal("Find returned nil")
	}
	if got.Kind() != ast.KindTypeRef {
		t.Errorf("unexpected kind %v", got.Kind())
	}
}

func TestUnionNormalization(t *testing.T) {
	tests := []struct {
		name string
		got  ast.Type
		want ast.Kind
		size int
	}{
		{"empty collapses to never", ast.Union(), ast.KindKeywordType, 0},
		{"single unwraps", ast.Union(ast.String), ast.KindKeywordType, 0},
		{"nils are dropped", ast.Union(ast.String, nil), ast.KindKeywordType, 0},
		{"nested flattens", ast.Union(ast.String, ast.Union(ast.Number, ast.Null)), ast.KindUnionType, 3},
		{"duplicate keywords collapse", ast.Union(ast.String, ast.String), ast.KindKeywordType, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.Kind() != tc.want {
				t.Fatalf("kind = %v, want %v", tc.got.Kind(), tc.want)
			}
			if tc.size > 0 {
				u := tc.got.(*ast.UnionType)
				if len(u.Types) != tc.size {
					t.Errorf("len = %d, want %d", len(u.Types), tc.size)
				}
			}
		})
	}
}

func TestRefSplitsDottedNames(t *testing.T) {
	r := ast.Ref("A.B.C")
	q, ok := r.Name.(*ast.QualifiedName)
	if !ok {
		t.Fatalf("expected a qualified name, got %T", r.Name)
	}
	if q.Right.Text != "C" {
		t.Errorf("rightmost segment = %q", q.Right.Text)
	}
}

func TestModifierBits(t *testing.T) {
	m := ast.ModExport.With(ast.ModDeclare)
	if !m.Has(ast.ModExport) || !m.Has(ast.ModDeclare) {
		t.Error("With did not set both bits")
	}
	if m.Without(ast.ModExport).Has(ast.ModExport) {
		t.Error("Without did not clear the bit")
	}
}

func TestSpanPromotionAcrossCategories(t *testing.T) {
	// Loc is promoted through two levels of embedding, so a parser can set a
	// position without knowing the concrete node type.
	want := token.Span{Start: 5, End: 11}
	nodes := []ast.Positioner{
		&ast.KeywordType{Keyword: ast.KwString},
		&ast.PropertySignature{Name: ast.NewIdent("x")},
		&ast.InterfaceDecl{Name: ast.NewIdent("I")},
		&ast.StringLit{Value: "s"},
	}
	for _, n := range nodes {
		if n.Span().IsValid() {
			t.Errorf("%T: synthesized node should report an invalid span", n)
		}
		n.SetSpan(want)
		if n.Span() != want {
			t.Errorf("%T: span = %v, want %v", n, n.Span(), want)
		}
	}
}
