// Package bind projects Go types into TypeScript types.
//
// The entry point is a type parameter, not a value:
//
//	b := bind.New()
//	t := bind.Of[User](b)        // a reference to "User"
//	file := b.File("models.ts")  // the declarations it implied
//
// Defaults describe what encoding/json emits rather than what the Go
// declaration looks like: a *T field without omitempty becomes T | null,
// []byte becomes string, time.Time becomes string.
//
// Every decision is overridable. [Mapper] intercepts type projection,
// [FieldRule] struct fields, and [Namer] type naming; user rules are
// consulted before the built-ins and the first to claim a type wins.
package bind

import (
	"reflect"

	"github.com/lilybw/go-solid-compiler/ast"
)

// NullPolicy decides how a Go pointer becomes a TypeScript type.
type NullPolicy uint8

const (
	// NullFromJSON follows encoding/json: an omittable pointer field becomes an
	// optional property, one that may marshal to null becomes T | null.
	NullFromJSON NullPolicy = iota
	// NullAsNull renders every pointer as T | null.
	NullAsNull
	// NullAsUndefined renders every pointer as T | undefined.
	NullAsUndefined
	// NullAsOptional renders every pointer field as an optional property.
	NullAsOptional
	// NullAsBoth renders every pointer as T | null | undefined.
	NullAsBoth
	// NullIgnore erases pointers entirely, rendering *T as T.
	NullIgnore
)

// ArrayPolicy decides how a fixed-size Go array becomes a TypeScript type.
type ArrayPolicy uint8

const (
	// ArrayAsSlice renders [N]T as T[].
	ArrayAsSlice ArrayPolicy = iota
	// ArrayAsTuple renders [N]T as a tuple of N elements, preserving length.
	ArrayAsTuple
)

// EmbedPolicy decides how an embedded Go struct becomes TypeScript.
type EmbedPolicy uint8

const (
	// EmbedFlatten inlines the embedded struct's fields, as encoding/json does.
	EmbedFlatten EmbedPolicy = iota
	// EmbedExtends emits an extends clause referencing the embedded type.
	EmbedExtends
)

// Options configures a [Binder]. Construct one via [New] with [Option]
// values so that unset fields take their defaults.
type Options struct {
	// Mappers are consulted before the built-in type rules, in order.
	Mappers []Mapper

	// FieldRules are consulted before the built-in struct field rule, in order.
	FieldRules []FieldRule

	// Namer assigns TypeScript names to named Go types.
	Namer Namer

	// TagKey is the struct tag read for field naming. Defaults to "json".
	TagKey string

	// OverrideTag is the struct tag read for TypeScript-specific overrides.
	// Defaults to "ts".
	OverrideTag string

	// DocTag is the struct tag read for a field's JSDoc text.
	// Defaults to "tsdoc".
	DocTag string

	// Nulls decides how pointers are projected.
	Nulls NullPolicy

	// Arrays decides how fixed-size arrays are projected.
	Arrays ArrayPolicy

	// Embeds decides how embedded structs are projected.
	Embeds EmbedPolicy

	// Any is the type produced for interface{} and non-empty interfaces.
	// Defaults to unknown, which forces consumers to narrow before use.
	Any ast.Type

	// Bytes is the type produced for []byte. Defaults to string, matching
	// encoding/json's base64 encoding.
	Bytes ast.Type

	// Time is the type produced for time.Time. Defaults to string.
	Time ast.Type

	// Duration is the type produced for time.Duration. Defaults to number,
	// matching encoding/json's nanosecond integer.
	Duration ast.Type

	// Int64 is the type produced for int64 and uint64. Defaults to number.
	// Set to ast.String when the wire format uses ,string, or to a union
	// when precision beyond 2^53 matters.
	Int64 ast.Type

	// Readonly marks every generated property readonly.
	Readonly bool

	// Export marks every generated declaration exported. Defaults to true.
	Export bool

	// Inline emits anonymous object types instead of named interfaces for
	// named Go structs. Recursive types are rejected in this mode, since an
	// inline type cannot refer to itself.
	Inline bool

	// Interfaces emits struct types as interface declarations. When false,
	// type aliases are emitted instead, which composes better with mapped and
	// conditional types at the cost of slower checking on large graphs.
	Interfaces bool

	// SkipUnexported skips unexported struct fields. Defaults to true;
	// unexported fields are invisible to encoding/json.
	SkipUnexported bool
}

func defaults() Options {
	return Options{
		TagKey:         "json",
		OverrideTag:    "ts",
		DocTag:         "tsdoc",
		Nulls:          NullFromJSON,
		Arrays:         ArrayAsSlice,
		Embeds:         EmbedFlatten,
		Any:            ast.Unknown,
		Bytes:          ast.String,
		Time:           ast.String,
		Duration:       ast.Number,
		Int64:          ast.Number,
		Export:         true,
		Interfaces:     true,
		SkipUnexported: true,
		Namer:          DefaultNamer{},
	}
}

// Option mutates Options.
type Option func(*Options)

// WithMapper prepends type mappers, which are consulted before the built-ins.
func WithMapper(m ...Mapper) Option {
	return func(o *Options) { o.Mappers = append(o.Mappers, m...) }
}

// WithFieldRule prepends struct field rules.
func WithFieldRule(r ...FieldRule) Option {
	return func(o *Options) { o.FieldRules = append(o.FieldRules, r...) }
}

// WithNamer sets the type namer.
func WithNamer(n Namer) Option { return func(o *Options) { o.Namer = n } }

// WithTagKey sets the struct tag used for field naming.
func WithTagKey(k string) Option { return func(o *Options) { o.TagKey = k } }

// WithNulls sets the pointer projection policy.
func WithNulls(p NullPolicy) Option { return func(o *Options) { o.Nulls = p } }

// WithArrays sets the fixed-size array policy.
func WithArrays(p ArrayPolicy) Option { return func(o *Options) { o.Arrays = p } }

// WithEmbeds sets the embedded struct policy.
func WithEmbeds(p EmbedPolicy) Option { return func(o *Options) { o.Embeds = p } }

// WithAny sets the type produced for interface{}.
func WithAny(t ast.Type) Option { return func(o *Options) { o.Any = t } }

// WithInt64 sets the type produced for 64-bit integers.
func WithInt64(t ast.Type) Option { return func(o *Options) { o.Int64 = t } }

// WithTime sets the type produced for time.Time.
func WithTime(t ast.Type) Option { return func(o *Options) { o.Time = t } }

// WithReadonly marks generated properties readonly.
func WithReadonly(b bool) Option { return func(o *Options) { o.Readonly = b } }

// WithExport controls whether declarations are exported.
func WithExport(b bool) Option { return func(o *Options) { o.Export = b } }

// WithInline emits anonymous object types rather than named interfaces.
func WithInline(b bool) Option { return func(o *Options) { o.Inline = b } }

// WithTypeAliases emits type aliases instead of interface declarations.
func WithTypeAliases() Option { return func(o *Options) { o.Interfaces = false } }

// ---------------------------------------------------------------------------
// Naming
// ---------------------------------------------------------------------------

// Namer assigns a TypeScript name to a named Go type. The binder still
// resolves collisions between distinct types that choose the same name.
type Namer interface {
	NameOf(reflect.Type) string
}

// NamerFunc adapts a function to [Namer].
type NamerFunc func(reflect.Type) string

func (f NamerFunc) NameOf(t reflect.Type) string { return f(t) }

// DefaultNamer uses the Go type name, sanitized and capitalized.
//
// Instantiated generics such as Box[int] become BoxInt: reflection reports
// only the instantiated type.
type DefaultNamer struct{}

func (DefaultNamer) NameOf(t reflect.Type) string { return pascalIdent(t.Name()) }

// VerbatimNamer uses the Go type name unchanged apart from the sanitization
// needed to make it a legal identifier.
type VerbatimNamer struct{}

func (VerbatimNamer) NameOf(t reflect.Type) string { return sanitizeIdent(t.Name()) }

// PrefixNamer prefixes the type name.
type PrefixNamer struct{ Prefix string }

func (p PrefixNamer) NameOf(t reflect.Type) string {
	return p.Prefix + pascalIdent(t.Name())
}
