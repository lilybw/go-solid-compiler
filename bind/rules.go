package bind

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/lilybw/go-solid-compiler/ast"
)

// ---------------------------------------------------------------------------
// Mappers
// ---------------------------------------------------------------------------

// Mapper projects a Go type into a TypeScript type. Returning false declines
// the type, passing it to the next mapper and then to the built-in rules.
type Mapper interface {
	MapType(c *Context, t reflect.Type) (ast.Type, bool)
}

// MapperFunc adapts a function to [Mapper].
type MapperFunc func(*Context, reflect.Type) (ast.Type, bool)

func (f MapperFunc) MapType(c *Context, t reflect.Type) (ast.Type, bool) { return f(c, t) }

// MapExact returns a Mapper claiming exactly the Go type T.
//
//	bind.MapExact[uuid.UUID](ast.String)
func MapExact[T any](ts ast.Type) Mapper {
	want := reflect.TypeFor[T]()
	return MapperFunc(func(_ *Context, t reflect.Type) (ast.Type, bool) {
		if t == want {
			return ts, true
		}
		return nil, false
	})
}

// MapKind returns a Mapper claiming every type of the given reflect kind.
func MapKind(k reflect.Kind, ts ast.Type) Mapper {
	return MapperFunc(func(_ *Context, t reflect.Type) (ast.Type, bool) {
		if t.Kind() == k {
			return ts, true
		}
		return nil, false
	})
}

// MapNamed returns a Mapper claiming types by package path and name, for
// third-party types that cannot be named at compile time.
func MapNamed(pkgPath, name string, ts ast.Type) Mapper {
	return MapperFunc(func(_ *Context, t reflect.Type) (ast.Type, bool) {
		if t.PkgPath() == pkgPath && t.Name() == name {
			return ts, true
		}
		return nil, false
	})
}

// ---------------------------------------------------------------------------
// Built-in type mappers
// ---------------------------------------------------------------------------

var (
	timeType     = reflect.TypeFor[time.Time]()
	durationType = reflect.TypeFor[time.Duration]()
	rawMsgType   = reflect.TypeFor[json.RawMessage]()
	marshalerTyp = reflect.TypeFor[json.Marshaler]()
	textMarshTyp = reflect.TypeFor[interface{ MarshalText() ([]byte, error) }]()
)

// builtinMappers handles standard library types whose JSON encoding does not
// follow from their structure.
func builtinMappers(c *Context, t reflect.Type) (ast.Type, bool) {
	o := c.Options()
	switch t {
	case timeType:
		return o.Time, true
	case durationType:
		return o.Duration, true
	case rawMsgType:
		return o.Any, true
	}

	// A type with a custom marshaller does not serialize as its fields, so
	// projecting its structure would be a lie. Report it and fall back rather
	// than emitting a confidently wrong shape.
	if t.Kind() == reflect.Struct && t != timeType {
		if t.Implements(marshalerTyp) || reflect.PointerTo(t).Implements(marshalerTyp) {
			c.Reportf(SeverityWarning, t,
				"%s implements json.Marshaler; its wire shape cannot be derived from its fields — supply a bind.Mapper", t)
			return o.Any, true
		}
		if t.Implements(textMarshTyp) || reflect.PointerTo(t).Implements(textMarshTyp) {
			return ast.String, true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Field rules
// ---------------------------------------------------------------------------

// FieldSpec describes how one Go struct field is projected.
type FieldSpec struct {
	// Name is the TypeScript property name.
	Name string
	// Skip omits the field entirely.
	Skip bool
	// Optional emits name?: T.
	Optional bool
	// Readonly emits readonly name: T.
	Readonly bool
	// Nullable adds | null to the projected type.
	Nullable bool
	// Type overrides the projected type. When nil the type is derived from
	// the Go field type.
	Type ast.Type
	// Docs attaches a JSDoc comment.
	Docs *ast.Doc
}

// FieldRule projects a Go struct field. Returning false declines the field,
// passing it to the next rule and then to the built-in JSON rule.
type FieldRule interface {
	MapField(c *Context, f reflect.StructField) (FieldSpec, bool)
}

// FieldRuleFunc adapts a function to [FieldRule].
type FieldRuleFunc func(*Context, reflect.StructField) (FieldSpec, bool)

func (fn FieldRuleFunc) MapField(c *Context, f reflect.StructField) (FieldSpec, bool) {
	return fn(c, f)
}

// jsonFieldRule is the built-in rule: the json tag for naming and omission,
// then the ts tag for TypeScript-specific overrides.
func jsonFieldRule(c *Context, f reflect.StructField) (FieldSpec, bool) {
	o := c.Options()
	spec := FieldSpec{Name: f.Name}

	if o.SkipUnexported && !f.IsExported() {
		spec.Skip = true
		return spec, true
	}

	// --- json tag ---------------------------------------------------------
	name, opts := parseTag(f.Tag.Get(o.TagKey))
	if name == "-" && opts == "" {
		spec.Skip = true
		return spec, true
	}
	if name != "" {
		spec.Name = name
	}
	omitempty := hasOpt(opts, "omitempty") || hasOpt(opts, "omitzero")
	asString := hasOpt(opts, "string")

	// --- pointer and omission semantics -----------------------------------
	isPtr := f.Type.Kind() == reflect.Pointer
	switch o.Nulls {
	case NullFromJSON:
		// A field that may be omitted is optional. A pointer that is always
		// present marshals to null when nil.
		if omitempty {
			spec.Optional = true
		} else if isPtr {
			spec.Nullable = true
		}
	case NullAsNull:
		spec.Nullable = isPtr
		spec.Optional = omitempty
	case NullAsUndefined:
		spec.Optional = isPtr || omitempty
	case NullAsOptional:
		spec.Optional = isPtr || omitempty
	case NullAsBoth:
		spec.Nullable = isPtr
		spec.Optional = isPtr || omitempty
	case NullIgnore:
		spec.Optional = omitempty
	}

	if asString {
		spec.Type = ast.String
		if spec.Nullable {
			spec.Type = ast.Nullable(spec.Type)
			spec.Nullable = false
		}
	}

	if o.Readonly {
		spec.Readonly = true
	}

	// --- ts override tag --------------------------------------------------
	if o.OverrideTag != "" {
		if tsName, tsOpts := parseTag(f.Tag.Get(o.OverrideTag)); tsName != "" || tsOpts != "" {
			if tsName == "-" {
				spec.Skip = true
				return spec, true
			}
			if tsName != "" {
				spec.Name = tsName
			}
			for _, opt := range strings.Split(tsOpts, ",") {
				switch {
				case opt == "":
				case opt == "optional":
					spec.Optional = true
				case opt == "required":
					spec.Optional = false
				case opt == "readonly":
					spec.Readonly = true
				case opt == "nullable":
					spec.Nullable = true
				case strings.HasPrefix(opt, "type="):
					spec.Type = ast.Raw(strings.TrimPrefix(opt, "type="))
				}
			}
		}
	}

	// --- doc tag ----------------------------------------------------------
	if o.DocTag != "" {
		if d := f.Tag.Get(o.DocTag); d != "" {
			spec.Docs = ast.Comment(d)
		}
	}

	return spec, true
}

// parseTag splits a struct tag value into its name and comma-separated
// options.
func parseTag(tag string) (name, opts string) {
	if i := strings.IndexByte(tag, ','); i >= 0 {
		return tag[:i], tag[i+1:]
	}
	return tag, ""
}

func hasOpt(opts, want string) bool {
	for len(opts) > 0 {
		var cur string
		if i := strings.IndexByte(opts, ','); i >= 0 {
			cur, opts = opts[:i], opts[i+1:]
		} else {
			cur, opts = opts, ""
		}
		if cur == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Identifier sanitization
// ---------------------------------------------------------------------------

// sanitizeIdent converts a Go type name into a valid TypeScript identifier.
// Instantiated generics such as Box[int] become BoxInt.
func sanitizeIdent(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	upper := false
	for i, r := range name {
		switch {
		case r == '[' || r == ']' || r == ',' || r == ' ' || r == '*':
			upper = true
		case r == '.' || r == '/' || r == '-':
			upper = true
		case unicode.IsLetter(r) || r == '_' || r == '$':
			if upper {
				b.WriteRune(unicode.ToUpper(r))
				upper = false
			} else {
				b.WriteRune(r)
			}
		case unicode.IsDigit(r):
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			upper = false
		default:
			upper = true
		}
	}
	out := b.String()
	if out == "" {
		return "Anonymous"
	}
	return out
}

// pascalIdent sanitizes a Go type name and capitalizes it, matching
// TypeScript convention.
func pascalIdent(name string) string {
	s := sanitizeIdent(name)
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// pkgPrefix derives a disambiguating prefix from a package path.
func pkgPrefix(pkgPath string) string {
	if pkgPath == "" {
		return ""
	}
	seg := pkgPath
	if i := strings.LastIndexByte(seg, '/'); i >= 0 {
		seg = seg[i+1:]
	}
	seg = sanitizeIdent(seg)
	if seg == "" {
		return ""
	}
	return strings.ToUpper(seg[:1]) + seg[1:]
}
