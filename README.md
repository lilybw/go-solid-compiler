#### DISCLAIMER
Hi, this port was made by Claude Opus 5. I can take no credit outside of architectural direction & testing methodology. 
Without this port, the features I would like to introduce to go-solid would be delayed by years if not more. 

As it stands, a full testing harness against babel is established and all suites pass successfully. 
That is the only guarantee I can give, as I have only read through some of the codebase, which is over 13 kLOC.  

# go-solid-compiler

Compile SolidJS components in Go. No Node, no Babel.

- **Lower Solid JSX** to DOM-expressions calls, matching `babel-preset-solid`.
- **Parse TypeScript and TSX** with the real TypeScript compiler.
- **Generate TypeScript types from Go types**, driven by a type parameter.
- **Embeds the solid-js runtime**, so `go get` is the whole install. No external dependencies

```go
file, err := tsx.Parse("Counter.tsx", src, tsx.TSX)
if err != nil {
    return err
}
out, err := tsx.TransformSolid(file, solid.Options{})
```

Given:

```jsx
export const Counter = () => (
  <div class="card">
    <h1>{title()}</h1>
    <button onClick={inc}>Count: {count()}</button>
  </div>
);
```

it produces:

```js
import { template as _$template, insert as _$insert,
         addEventListener as _$addEventListener,
         delegateEvents as _$delegateEvents } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div class=card><h1></h1><button>Count: </button>`);

export const Counter = () => (() => {
  var _el$ = _tmpl$(),
    _el$2 = _el$.firstChild,
    _el$3 = _el$2.nextSibling;
  _$insert(_el$2, title);
  _$insert(_el$3, count, null);
  _el$3.$$click = inc;
  return _el$;
})();

_$delegateEvents(["click"]);
```

Type annotations are left in place; only JSX is lowered. Pass the result to
esbuild with the TypeScript loader and it strips types and bundles as usual.

## Install

```
go get github.com/lilybw/go-solid-compiler
go generate ./runtime    # fetch the pinned solid-js runtime
```

## Use with esbuild

The compiler is designed to run inside an esbuild plugin, so the whole build
stays in one Go process.

```go
api.Plugin{
    Name: "solid",
    Setup: func(b api.PluginBuild) {
        b.OnLoad(api.OnLoadOptions{Filter: `\.tsx$`}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
            src, err := os.ReadFile(a.Path)
            if err != nil {
                return api.OnLoadResult{}, err
            }
            file, err := tsx.Parse(a.Path, string(src), tsx.TSX)
            if err != nil {
                return api.OnLoadResult{}, err
            }
            out, err := tsx.TransformSolid(file, solid.Options{})
            if err != nil {
                return api.OnLoadResult{}, err
            }
            return api.OnLoadResult{Contents: &out, Loader: api.LoaderTS}, nil
        })
    },
}
```

Letting esbuild drive means imports are followed automatically: a component
that imports another component gets it transformed too.

`runtime.Config` resolves `solid-js` imports from the embedded copy, so no
`node_modules` is required for the runtime itself.

## Generating types from Go

`bind` projects Go types into TypeScript, so a Go struct passed to a component
as props can be typed on both sides.

```go
b := bind.New()
props := bind.Of[LoginProps](b)          // on Go 1.27: b.Of[LoginProps]()

file := build.File("props.ts").
    ImportType("solid-js", build.Named("Component")).
    AddDecls(b.Declarations()...).
    Add(build.Alias("LoginForm", ast.Ref("Component", props)).Export().Build()).
    Build()

fmt.Print(printer.Print(file, printer.GeneratedBanner("myapp")))
```

Defaults describe what `encoding/json` emits rather than what the Go
declaration looks like:

| Go | TypeScript |
|---|---|
| `*T` without `omitempty` | `T \| null` |
| `*T` with `omitempty` | `t?: T` |
| `[]byte` | `string` (base64) |
| `time.Time` | `string` (RFC 3339) |
| `int64` with `,string` | `string` |
| embedded struct | flattened, in declaration position |

A type with a custom `MarshalJSON` produces a diagnostic rather than a
confidently wrong structural projection; `Binder.HasErrors` lets a generator
fail the build instead.

Every default is overridable:

```go
b := bind.New(
    bind.WithMapper(bind.MapExact[uuid.UUID](ast.String)),
    bind.WithNulls(bind.NullAsUndefined),
    bind.WithReadonly(true),
)
```

`Mapper` intercepts type projection, `FieldRule` struct fields, and `Namer`
type naming. User rules run before the built-ins and the first to claim a type
wins.

## Packages

| Package | Purpose |
|---|---|
| `solid` | JSX to DOM-expressions, over a small JSX IR |
| `tsx` | parse TypeScript and TSX; lower Solid JSX in a file |
| `bind` | Go types to TypeScript types |
| `ast` | TypeScript AST |
| `printer` | AST to source text, precedence-aware |
| `build` | fluent AST construction |
| `parse` | dependency-free parser for the type grammar |
| `runtime` | embedded solid-js runtime and import resolver |
| `token` | source positions |

`tsx` depends on a fork of the TypeScript compiler that re-exports its
internals; the other packages have no dependencies.

## Correctness

Output is compared against `babel-preset-solid` on a corpus of components. The
comparison is layered, because the two compilers agree on semantics but not on
spelling: templates, the imported helper set, and the delegated event set must
match exactly, while statement bodies are compared after normalizing generated
variable names, equivalent accessor forms, and DOM navigation paths.

```
go test ./...
```

The goldens are committed, so the test suite needs no Node. Re-derive them
after changing the corpus:

```
npm --prefix harness/babel ci
npm --prefix harness/babel run record
```

## Supported syntax

Host elements and components; static and dynamic attributes; text, expression,
element, component and fragment children; JSX nested inside expressions such as
`items().map(i => <li/>)`; spread on elements and components; delegated and
direct events; the `on:`, `oncapture:`, `use:`, `prop:`, `attr:`, `bool:`,
`style:` and `class:` namespaces; `ref`, `classList` and `style`; memoized
conditionals; control-flow components resolved to their `solid-js/web`
implementations.

Not yet implemented:

- **Batched attribute effects.** The reference folds every dynamic attribute on
  a template into one effect with previous-value change detection; this
  compiler emits one per attribute, which is correct but does more work.
- **SSR and hydration output.** Client rendering only.
- **Source maps.**

## Development

The toolchain is containerized, so the environment is the same everywhere:

```
cp .env.example .env
docker compose build
docker compose run --rm setup     # fetch dependencies, vendor the runtime
docker compose run --rm check     # gofmt, vet, tests
```

Other tasks: `test`, `fmt`, `record`, `vendor`, `doctor`, `shell`. `make` and
`./make.ps1` wrap them.

## License

MIT. The embedded solid-js runtime is MIT, and the TypeScript compiler fork is
Apache-2.0; both notices are retained.
