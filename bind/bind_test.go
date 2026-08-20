package bind_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lilybw/go-solid-compiler/ast"
	"github.com/lilybw/go-solid-compiler/bind"
	"github.com/lilybw/go-solid-compiler/printer"
)

func render(n ast.Node) string {
	return strings.TrimSpace(printer.Print(n, printer.WithMaxLineWidth(0)))
}

// ---------------------------------------------------------------------------
// Scalars and containers
// ---------------------------------------------------------------------------

func TestScalarProjection(t *testing.T) {
	tests := []struct {
		name string
		got  ast.Type
		want string
	}{
		{"string", must[string](), "string"},
		{"bool", must[bool](), "boolean"},
		{"int", must[int](), "number"},
		{"float64", must[float64](), "number"},
		{"slice", must[[]string](), "string[]"},
		{"nested slice", must[[][]int](), "number[][]"},
		{"byte slice is base64 string", must[[]byte](), "string"},
		{"map", must[map[string]int](), "Record<string, number>"},
		{"int-keyed map", must[map[int]string](), "Record<number, string>"},
		{"any is unknown", must[any](), "unknown"},
		{"array", must[[3]int](), "number[]"},
		{"time is string", must[time.Time](), "string"},
		{"duration is number", must[time.Duration](), "number"},
		{"raw message", must[json.RawMessage](), "unknown"},
		{"pointer is nullable", must[*string](), "string | null"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := render(tc.got); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func must[T any](opts ...bind.Option) ast.Type {
	t, _ := bind.TypeOf[T](opts...)
	return t
}

func TestArrayAsTuplePreservesLength(t *testing.T) {
	got := render(must[[3]int](bind.WithArrays(bind.ArrayAsTuple)))
	if got != "[number, number, number]" {
		t.Errorf("got %s", got)
	}
}

func TestAnyOverride(t *testing.T) {
	got := render(must[any](bind.WithAny(ast.Any)))
	if got != "any" {
		t.Errorf("got %s", got)
	}
}

func TestInt64Override(t *testing.T) {
	got := render(must[int64](bind.WithInt64(ast.String)))
	if got != "string" {
		t.Errorf("got %s", got)
	}
}

// ---------------------------------------------------------------------------
// Struct field semantics
// ---------------------------------------------------------------------------

type tagged struct {
	Plain     string `json:"plain"`
	Renamed   string `json:"custom_name"`
	Omitted   string `json:"-"`
	Untagged  string
	OmitEmpty string  `json:"opt,omitempty"`
	Ptr       *string `json:"ptr"`
	PtrOmit   *string `json:"ptrOpt,omitempty"`
	Stringy   int64   `json:"big,string"`
	unexp     string
}

func TestJSONFieldSemantics(t *testing.T) {
	f := bind.Declare[tagged]("m.ts")
	got := render(f)

	wants := []string{
		"plain: string",
		"custom_name: string",
		"Untagged: string",
		"opt?: string",
		"ptr: string | null",
		"ptrOpt?: string",
		"big: string",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	for _, bad := range []string{"Omitted", "unexp"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
	_ = tagged{}.unexp
}

type overridden struct {
	Hidden  string `ts:"-"`
	Renamed string `json:"a" ts:"b"`
	RO      string `json:"ro" ts:",readonly"`
	Opt     string `json:"o" ts:",optional"`
	Custom  string `json:"c" ts:",type=Brand<string>"`
	Doc     string `json:"d" tsdoc:"Some documentation."`
}

func TestOverrideTag(t *testing.T) {
	got := render(bind.Declare[overridden]("m.ts"))
	wants := []string{
		"b: string", "readonly ro: string", "o?: string",
		"c: Brand<string>", "/** Some documentation. */",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if strings.Contains(got, "Hidden") {
		t.Errorf("ts:\"-\" field was not skipped:\n%s", got)
	}
}

func TestNullPolicies(t *testing.T) {
	type s struct {
		P *string `json:"p"`
	}
	tests := []struct {
		policy bind.NullPolicy
		want   string
	}{
		{bind.NullAsNull, "p: string | null"},
		{bind.NullAsUndefined, "p?: string"},
		{bind.NullAsOptional, "p?: string"},
		{bind.NullIgnore, "p: string"},
	}
	for _, tc := range tests {
		got := render(bind.Declare[s]("m.ts", bind.WithNulls(tc.policy)))
		if !strings.Contains(got, tc.want) {
			t.Errorf("policy %v: missing %q in:\n%s", tc.policy, tc.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Embedding
// ---------------------------------------------------------------------------

type embBase struct {
	ID string `json:"id"`
}

type embChild struct {
	embBase
	Name string `json:"name"`
}

func TestEmbeddedFieldsFlattenInDeclarationOrder(t *testing.T) {
	got := render(bind.Declare[embChild]("m.ts"))
	i, j := strings.Index(got, "id:"), strings.Index(got, "name:")
	if i < 0 || j < 0 {
		t.Fatalf("missing fields:\n%s", got)
	}
	if i > j {
		t.Errorf("embedded field should precede own field, matching encoding/json:\n%s", got)
	}
	if strings.Contains(got, "interface EmbBase") {
		t.Errorf("flattened embed should not emit a separate interface:\n%s", got)
	}
}

func TestEmbedExtendsPolicy(t *testing.T) {
	got := render(bind.Declare[embChild]("m.ts", bind.WithEmbeds(bind.EmbedExtends)))
	if !strings.Contains(got, "interface EmbChild extends EmbBase") {
		t.Errorf("expected extends clause:\n%s", got)
	}
	if !strings.Contains(got, "interface EmbBase") {
		t.Errorf("expected base interface to be emitted:\n%s", got)
	}
}

type shadowBase struct {
	Name string `json:"name"`
}
type shadowChild struct {
	shadowBase
	Name string `json:"name"`
}

func TestShallowFieldShadowsEmbedded(t *testing.T) {
	got := render(bind.Declare[shadowChild]("m.ts"))
	if n := strings.Count(got, "name:"); n != 1 {
		t.Errorf("expected exactly one name property, got %d:\n%s", n, got)
	}
}

// ---------------------------------------------------------------------------
// Named types, cycles, collisions
// ---------------------------------------------------------------------------

type node struct {
	Value    int     `json:"value"`
	Next     *node   `json:"next"`
	Children []*node `json:"children"`
}

func TestRecursiveTypeTerminates(t *testing.T) {
	b := bind.New()
	got := render(bind.Of[node](b))
	if got != "Node" {
		t.Errorf("expected a reference, got %s", got)
	}
	file := render(b.File("m.ts"))
	if !strings.Contains(file, "next: Node | null") {
		t.Errorf("expected self-reference:\n%s", file)
	}
	if n := strings.Count(file, "interface Node"); n != 1 {
		t.Errorf("expected one declaration, got %d:\n%s", n, file)
	}
}

func TestSharedStructDeclaredOnce(t *testing.T) {
	type inner struct {
		A string `json:"a"`
	}
	type outer struct {
		X inner `json:"x"`
		Y inner `json:"y"`
	}
	file := render(bind.Declare[outer]("m.ts"))
	if n := strings.Count(file, "interface Inner"); n != 1 {
		t.Errorf("expected one Inner declaration, got %d:\n%s", n, file)
	}
}

func TestInlineModeRejectsRecursion(t *testing.T) {
	b := bind.New(bind.WithInline(true))
	bind.Of[node](b)
	if !b.HasErrors() {
		t.Error("expected an error diagnostic for inline recursive type")
	}
}

func TestDeterministicOrder(t *testing.T) {
	type c struct {
		V string `json:"v"`
	}
	type bStruct struct {
		C c `json:"c"`
	}
	type a struct {
		B bStruct `json:"b"`
	}
	var first string
	for i := 0; i < 20; i++ {
		out := render(bind.Declare[a]("m.ts"))
		if i == 0 {
			first = out
			continue
		}
		if out != first {
			t.Fatalf("output is not deterministic across runs:\n%s\n---\n%s", first, out)
		}
	}
}

// ---------------------------------------------------------------------------
// Extension points
// ---------------------------------------------------------------------------

func TestMapExactOverridesBuiltin(t *testing.T) {
	type s struct {
		At time.Time `json:"at"`
	}
	got := render(bind.Declare[s]("m.ts",
		bind.WithMapper(bind.MapExact[time.Time](ast.Ref("Date")))))
	if !strings.Contains(got, "at: Date") {
		t.Errorf("custom mapper not applied:\n%s", got)
	}
}

func TestFieldRuleOverridesDefault(t *testing.T) {
	type s struct {
		Field string `json:"field"`
	}
	upper := bind.FieldRuleFunc(func(_ *bind.Context, f reflect.StructField) (bind.FieldSpec, bool) {
		return bind.FieldSpec{Name: strings.ToUpper(f.Name), Readonly: true}, true
	})
	got := render(bind.Declare[s]("m.ts", bind.WithFieldRule(upper)))
	if !strings.Contains(got, "readonly FIELD: string") {
		t.Errorf("field rule not applied:\n%s", got)
	}
}

func TestNamerIsUsed(t *testing.T) {
	type s struct {
		A string `json:"a"`
	}
	got := render(bind.Declare[s]("m.ts", bind.WithNamer(bind.PrefixNamer{Prefix: "Api"})))
	if !strings.Contains(got, "interface ApiS") {
		t.Errorf("namer not applied:\n%s", got)
	}
}

type Role string

func TestEnumRegistration(t *testing.T) {
	type user struct {
		Role Role `json:"role"`
	}
	b := bind.New()
	bind.Enum(b, Role("admin"), Role("viewer"))
	bind.Of[user](b)

	got := render(b.File("m.ts"))
	if !strings.Contains(got, "type Role = 'admin' | 'viewer'") {
		t.Errorf("missing enum alias:\n%s", got)
	}
	if !strings.Contains(got, "role: Role") {
		t.Errorf("field should reference the alias:\n%s", got)
	}
}

func TestUnsupportedKindsReportErrors(t *testing.T) {
	type s struct {
		Fn func() `json:"fn"`
	}
	b := bind.New()
	bind.Of[s](b)
	if !b.HasErrors() {
		t.Error("expected an error for a func field")
	}
}

type customMarshal struct {
	Hidden string
}

func (customMarshal) MarshalJSON() ([]byte, error) { return []byte(`"x"`), nil }

func TestCustomMarshalerIsReported(t *testing.T) {
	b := bind.New()
	bind.Of[customMarshal](b)
	if len(b.Diagnostics()) == 0 {
		t.Error("expected a diagnostic for a type with a custom marshaller")
	}
}

func TestReadonlyAndExportOptions(t *testing.T) {
	type s struct {
		A string `json:"a"`
	}
	got := render(bind.Declare[s]("m.ts", bind.WithReadonly(true), bind.WithExport(false)))
	if !strings.Contains(got, "readonly a: string") {
		t.Errorf("readonly not applied:\n%s", got)
	}
	if strings.Contains(got, "export ") {
		t.Errorf("export should be suppressed:\n%s", got)
	}
}

func TestTypeAliasMode(t *testing.T) {
	type s struct {
		A string `json:"a"`
	}
	got := render(bind.Declare[s]("m.ts", bind.WithTypeAliases()))
	if !strings.Contains(got, "type S = {") {
		t.Errorf("expected a type alias:\n%s", got)
	}
}

func TestGenericInstantiationIsMonomorphized(t *testing.T) {
	type box[T any] struct {
		Value T `json:"value"`
	}
	got := render(bind.Declare[box[int]]("m.ts"))
	// reflect reports "box[int]"; the name must still be a legal identifier.
	if strings.ContainsAny(got, "[]") && strings.Contains(got, "interface box[") {
		t.Errorf("generic name was not sanitized:\n%s", got)
	}
	if !strings.Contains(got, "value: number") {
		t.Errorf("type argument not projected:\n%s", got)
	}
}
