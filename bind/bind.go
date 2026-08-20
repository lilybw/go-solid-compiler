package bind

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/lilybw/go-solid-compiler/ast"
)

// Severity classifies a [Diagnostic].
type Severity uint8

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
}

// Diagnostic reports a projection that could not be performed faithfully.
//
// Diagnostics are not fatal; a generator can use [Binder.HasErrors] to fail
// a build rather than emit a wrong type.
type Diagnostic struct {
	Severity Severity
	GoType   string
	Message  string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s", d.Severity, d.Message)
}

// Binder converts Go types to TypeScript and accumulates the declarations
// those conversions imply.
//
// A Binder is stateful and not safe for concurrent use.
type Binder struct {
	opts  Options
	named map[reflect.Type]*entry
	order []reflect.Type
	used  map[string]reflect.Type
	diags []Diagnostic
}

type entry struct {
	name string
	decl ast.Decl
	rt   reflect.Type
}

// New returns a Binder configured from the supplied options.
func New(opts ...Option) *Binder {
	o := defaults()
	for _, fn := range opts {
		fn(&o)
	}
	if o.Namer == nil {
		o.Namer = DefaultNamer{}
	}
	return &Binder{
		opts:  o,
		named: make(map[reflect.Type]*entry),
		used:  make(map[string]reflect.Type),
	}
}

// Options returns a pointer to the binder's configuration.
func (b *Binder) Options() *Options { return &b.opts }

// Of projects the Go type argument into a TypeScript type.
//
// Named struct types are registered as declarations and referenced by name,
// so the result is usually an [*ast.TypeRef]; collect the declarations with
// [Binder.Declarations].
//
// On Go 1.27 the equivalent method b.Of[T]() is also available.
func Of[T any](b *Binder) ast.Type { return b.Bind(reflect.TypeFor[T]()) }

// Bind projects a reflect.Type. Prefer [Of] when the type is known statically.
func (b *Binder) Bind(rt reflect.Type) ast.Type {
	c := &Context{b: b}
	return c.Bind(rt)
}

// Diagnostics returns the projections that could not be performed faithfully.
func (b *Binder) Diagnostics() []Diagnostic { return b.diags }

// HasErrors reports whether any diagnostic has error severity.
func (b *Binder) HasErrors() bool {
	for _, d := range b.diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Declarations returns the declarations implied by everything bound so far,
// in discovery order. The order is deterministic across runs.
func (b *Binder) Declarations() []ast.Decl {
	out := make([]ast.Decl, 0, len(b.order))
	for _, rt := range b.order {
		if e := b.named[rt]; e != nil && e.decl != nil {
			out = append(out, e.decl)
		}
	}
	return out
}

// SortedDeclarations returns the declarations sorted by name.
func (b *Binder) SortedDeclarations() []ast.Decl {
	out := b.Declarations()
	sort.SliceStable(out, func(i, j int) bool {
		return declName(out[i]) < declName(out[j])
	})
	return out
}

func declName(d ast.Decl) string {
	switch x := d.(type) {
	case *ast.InterfaceDecl:
		return x.Name.Text
	case *ast.TypeAliasDecl:
		return x.Name.Text
	case *ast.EnumDecl:
		return x.Name.Text
	}
	return ""
}

// File wraps the accumulated declarations in a source file.
func (b *Binder) File(name string) *ast.SourceFile {
	f := &ast.SourceFile{Name: name, ScriptKind: ast.ScriptTS}
	for _, d := range b.Declarations() {
		f.Stmts = append(f.Stmts, d)
	}
	return f
}

// Lookup returns the TypeScript name assigned to a Go type, if it has one.
func (b *Binder) Lookup(rt reflect.Type) (string, bool) {
	if e, ok := b.named[rt]; ok {
		return e.name, true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

// Context is the per-projection state handed to [Mapper] and [FieldRule]
// implementations. Use [Context.Bind] to recurse into component types.
type Context struct {
	b     *Binder
	stack []reflect.Type
}

// Options returns the binder's configuration.
func (c *Context) Options() *Options { return &c.b.opts }

// Binder returns the owning binder.
func (c *Context) Binder() *Binder { return c.b }

// Reportf records a diagnostic.
func (c *Context) Reportf(sev Severity, rt reflect.Type, format string, args ...any) {
	name := "<nil>"
	if rt != nil {
		name = rt.String()
	}
	c.b.diags = append(c.b.diags, Diagnostic{
		Severity: sev,
		GoType:   name,
		Message:  fmt.Sprintf(format, args...),
	})
}

// Bind projects rt, consulting user mappers first and then the built-in rules.
func (c *Context) Bind(rt reflect.Type) ast.Type {
	if rt == nil {
		return c.b.opts.Any
	}
	for _, m := range c.b.opts.Mappers {
		if t, ok := m.MapType(c, rt); ok {
			return t
		}
	}
	// A type already registered — by a previous bind, or explicitly via Enum —
	// resolves to a reference. This is also what terminates recursion: the
	// entry is reserved before its members are built.
	if e, ok := c.b.named[rt]; ok {
		return ast.Ref(e.name)
	}
	if t, ok := builtinMappers(c, rt); ok {
		return t
	}
	return c.structural(rt)
}

// structural projects a type from its shape.
func (c *Context) structural(rt reflect.Type) ast.Type {
	o := &c.b.opts
	switch rt.Kind() {
	case reflect.Bool:
		return ast.Boolean

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return ast.Number

	case reflect.Int64, reflect.Uint64:
		return o.Int64

	case reflect.Float32, reflect.Float64:
		return ast.Number

	case reflect.String:
		return ast.String

	case reflect.Pointer:
		inner := c.Bind(rt.Elem())
		switch o.Nulls {
		case NullAsNull:
			return ast.Nullable(inner)
		case NullAsUndefined:
			return ast.Optional(inner)
		case NullAsBoth:
			return ast.Union(inner, ast.Null, ast.Undefined)
		default:
			// NullFromJSON and NullAsOptional are decided at the field level,
			// where omitempty is visible; a bare pointer outside a struct
			// still marshals to null.
			if o.Nulls == NullFromJSON {
				return ast.Nullable(inner)
			}
			return inner
		}

	case reflect.Slice:
		if rt.Elem().Kind() == reflect.Uint8 && rt.Elem().PkgPath() == "" {
			return o.Bytes
		}
		// A nil slice marshals to null, so the honest projection includes it.
		return ast.Array(c.Bind(rt.Elem()))

	case reflect.Array:
		if rt.Elem().Kind() == reflect.Uint8 && rt.Elem().PkgPath() == "" {
			return o.Bytes
		}
		elem := c.Bind(rt.Elem())
		if o.Arrays == ArrayAsTuple {
			elems := make([]ast.Type, rt.Len())
			for i := range elems {
				elems[i] = elem
			}
			return &ast.TupleType{Elems: elems}
		}
		return ast.Array(elem)

	case reflect.Map:
		return c.bindMap(rt)

	case reflect.Struct:
		return c.bindStruct(rt)

	case reflect.Interface:
		return o.Any

	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		c.Reportf(SeverityError, rt, "%s has no JSON representation", rt)
		return ast.Never

	case reflect.Complex64, reflect.Complex128:
		c.Reportf(SeverityError, rt, "%s is not supported by encoding/json", rt)
		return ast.Never

	default:
		c.Reportf(SeverityWarning, rt, "unhandled kind %s", rt.Kind())
		return o.Any
	}
}

func (c *Context) bindMap(rt reflect.Type) ast.Type {
	key := rt.Key()
	var keyType ast.Type
	switch {
	case key.Kind() == reflect.String:
		keyType = ast.String
	case isIntKind(key.Kind()):
		// encoding/json stringifies integer map keys, but TypeScript indexes
		// Record<number, V> with numbers at no runtime cost, so the ergonomic
		// projection is kept and the caveat documented.
		keyType = ast.Number
	case key.Implements(textMarshTyp):
		keyType = ast.String
	default:
		c.Reportf(SeverityWarning, rt,
			"map key %s is not a valid JSON object key; projecting as string", key)
		keyType = ast.String
	}
	return ast.Record(keyType, c.Bind(rt.Elem()))
}

func isIntKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Structs
// ---------------------------------------------------------------------------

func (c *Context) bindStruct(rt reflect.Type) ast.Type {
	o := &c.b.opts

	// Anonymous structs and inline mode produce a structural type.
	if rt.Name() == "" || o.Inline {
		if c.onStack(rt) {
			c.Reportf(SeverityError, rt,
				"recursive type %s cannot be projected inline; disable bind.WithInline", rt)
			return o.Any
		}
		c.stack = append(c.stack, rt)
		defer func() { c.stack = c.stack[:len(c.stack)-1] }()
		members, _ := c.structMembers(rt)
		return &ast.TypeLiteral{Members: members}
	}

	// Named: reserve the name before recursing so that a self-referential
	// field resolves to a reference rather than recursing forever. Context.Bind
	// checks the registry, so the reservation is what breaks the cycle.
	name := c.b.assignName(rt)
	e := &entry{name: name, rt: rt}
	c.b.named[rt] = e
	c.b.order = append(c.b.order, rt)

	members, extends := c.structMembers(rt)

	mods := ast.Modifier(0)
	if o.Export {
		mods = mods.With(ast.ModExport)
	}
	if o.Interfaces {
		e.decl = &ast.InterfaceDecl{
			Name:    ast.NewIdent(name),
			Extends: extends,
			Members: members,
			Mods:    mods,
		}
	} else {
		var t ast.Type = &ast.TypeLiteral{Members: members}
		if len(extends) > 0 {
			parts := make([]ast.Type, 0, len(extends)+1)
			for _, h := range extends {
				parts = append(parts, &ast.TypeRef{Name: h.Name, Args: h.Args})
			}
			parts = append(parts, t)
			t = ast.Intersection(parts...)
		}
		e.decl = &ast.TypeAliasDecl{Name: ast.NewIdent(name), Type: t, Mods: mods}
	}
	return ast.Ref(name)
}

// assignName picks a unique TypeScript name for rt, qualifying with the
// package name on collision.
func (b *Binder) assignName(rt reflect.Type) string {
	base := b.opts.Namer.NameOf(rt)
	if base == "" {
		base = "Anonymous"
	}
	if owner, taken := b.used[base]; !taken || owner == rt {
		b.used[base] = rt
		return base
	}
	if p := pkgPrefix(rt.PkgPath()); p != "" {
		q := p + base
		if owner, taken := b.used[q]; !taken || owner == rt {
			b.used[q] = rt
			return q
		}
	}
	for i := 2; ; i++ {
		q := fmt.Sprintf("%s%d", base, i)
		if owner, taken := b.used[q]; !taken || owner == rt {
			b.used[q] = rt
			return q
		}
	}
}

func (c *Context) onStack(rt reflect.Type) bool {
	for _, s := range c.stack {
		if s == rt {
			return true
		}
	}
	return false
}

// structMembers projects a struct's fields, honouring embedding policy.
func (c *Context) structMembers(rt reflect.Type) ([]ast.Member, []*ast.Heritage) {
	o := &c.b.opts
	var extends []*ast.Heritage

	fields, embedded := c.collectFields(rt)

	if o.Embeds == EmbedExtends {
		for _, emb := range embedded {
			if t := c.Bind(emb); t != nil {
				if ref, ok := t.(*ast.TypeRef); ok {
					extends = append(extends, &ast.Heritage{Name: ref.Name, Args: ref.Args})
					continue
				}
			}
			c.Reportf(SeverityWarning, emb,
				"embedded type %s could not be projected as an extends clause; flattening", emb)
		}
	}

	members := make([]ast.Member, 0, len(fields))
	for _, fe := range fields {
		spec, ok := c.resolveField(fe.field)
		if !ok || spec.Skip {
			continue
		}
		t := spec.Type
		if t == nil {
			ft := fe.field.Type
			// A field-level nullability decision already accounts for the
			// pointer, so project the pointee to avoid a doubled | null.
			if ft.Kind() == reflect.Pointer && (spec.Nullable || spec.Optional) {
				ft = ft.Elem()
			}
			t = c.Bind(ft)
		}
		if spec.Nullable {
			t = ast.Nullable(t)
		}
		members = append(members, &ast.PropertySignature{
			Name:     ast.NewIdent(spec.Name),
			Type:     t,
			Optional: spec.Optional,
			Readonly: spec.Readonly,
			Docs:     spec.Docs,
		})
	}
	return members, extends
}

func (c *Context) resolveField(f reflect.StructField) (FieldSpec, bool) {
	for _, r := range c.b.opts.FieldRules {
		if spec, ok := r.MapField(c, f); ok {
			return spec, true
		}
	}
	return jsonFieldRule(c, f)
}

// fieldEntry is a struct field resolved through embedding; index is its path
// of field indices from the outermost struct.
type fieldEntry struct {
	field reflect.StructField
	depth int
	index []int
}

// lessIndex orders field index paths, placing a flattened embedded struct's
// fields where the embedded field was declared.
func lessIndex(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// collectFields flattens embedded structs the way encoding/json does: a
// shallower field shadows a deeper one of the same name, and an ambiguous
// pair at equal depth is dropped.
func (c *Context) collectFields(rt reflect.Type) ([]fieldEntry, []reflect.Type) {
	o := &c.b.opts
	var out []fieldEntry
	var embedded []reflect.Type

	byName := map[string][]int{} // name -> indices into out
	type queued struct {
		rt    reflect.Type
		depth int
		index []int
	}
	queue := []queued{{rt, 0, nil}}
	visited := map[reflect.Type]bool{rt: true}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for i := 0; i < cur.rt.NumField(); i++ {
			f := cur.rt.Field(i)

			if f.Anonymous {
				ft := f.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				tagName, _ := parseTag(f.Tag.Get(o.TagKey))
				// An embedded struct with no tag name is flattened; with a tag
				// name it behaves like an ordinary named field.
				if ft.Kind() == reflect.Struct && tagName == "" {
					if cur.depth == 0 {
						embedded = append(embedded, ft)
						if o.Embeds == EmbedExtends {
							// The extends clause carries these fields.
							continue
						}
					}
					if !visited[ft] {
						visited[ft] = true
						queue = append(queue, queued{ft, cur.depth + 1, append(append([]int{}, cur.index...), i)})
					}
					continue
				}
				if !f.IsExported() && ft.Kind() != reflect.Struct {
					continue
				}
			}

			if o.SkipUnexported && !f.IsExported() {
				continue
			}

			name := f.Name
			if tn, _ := parseTag(f.Tag.Get(o.TagKey)); tn != "" && tn != "-" {
				name = tn
			}

			if prev, seen := byName[name]; seen {
				// Shallower depth wins; equal depth is ambiguous.
				if out[prev[0]].depth < cur.depth {
					continue
				}
				if out[prev[0]].depth == cur.depth {
					c.Reportf(SeverityWarning, cur.rt,
						"field %q is ambiguous at depth %d and was dropped, matching encoding/json",
						name, cur.depth)
					continue
				}
			}
			byName[name] = append(byName[name], len(out))
			out = append(out, fieldEntry{
				field: f,
				depth: cur.depth,
				index: append(append([]int{}, cur.index...), i),
			})
		}
	}
	// Restore declaration order: encoding/json emits an embedded struct's
	// fields at the position the embedded field was declared.
	sort.SliceStable(out, func(i, j int) bool { return lessIndex(out[i].index, out[j].index) })
	return out, embedded
}

// ---------------------------------------------------------------------------
// Package-level conveniences
// ---------------------------------------------------------------------------

// TypeOf projects T with a throwaway binder, returning the type and the
// declarations it implied. Use a [Binder] when binding several related types.
func TypeOf[T any](opts ...Option) (ast.Type, []ast.Decl) {
	b := New(opts...)
	t := Of[T](b)
	return t, b.Declarations()
}

// Declare projects T and returns a source file containing the declarations it
// implied.
func Declare[T any](name string, opts ...Option) *ast.SourceFile {
	b := New(opts...)
	Of[T](b)
	return b.File(name)
}

// Enum registers a named union alias for T over the supplied values, since
// reflection cannot enumerate Go constants.
//
//	bind.Enum(b, Active, Banned)  // type Status = 'active' | 'banned'
func Enum[T comparable](b *Binder, values ...T) ast.Type {
	rt := reflect.TypeFor[T]()
	if e, ok := b.named[rt]; ok {
		return ast.Ref(e.name)
	}
	name := b.assignName(rt)
	lits := make([]ast.Type, 0, len(values))
	for _, v := range values {
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.String:
			lits = append(lits, ast.StringLiteral(rv.String()))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			lits = append(lits, ast.NumberLiteral(fmt.Sprint(rv.Int())))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			lits = append(lits, ast.NumberLiteral(fmt.Sprint(rv.Uint())))
		case reflect.Bool:
			lits = append(lits, ast.BoolLiteral(rv.Bool()))
		default:
			b.diags = append(b.diags, Diagnostic{
				Severity: SeverityError,
				GoType:   rt.String(),
				Message:  fmt.Sprintf("enum value of kind %s is not representable as a literal type", rv.Kind()),
			})
		}
	}
	mods := ast.Modifier(0)
	if b.opts.Export {
		mods = mods.With(ast.ModExport)
	}
	b.named[rt] = &entry{
		name: name,
		rt:   rt,
		decl: &ast.TypeAliasDecl{Name: ast.NewIdent(name), Type: ast.Union(lits...), Mods: mods},
	}
	b.order = append(b.order, rt)
	return ast.Ref(name)
}
