// Package esbuildsolid provides the esbuild plugins for building a Solid
// application entirely in Go.
//
//	result := api.Build(api.BuildOptions{
//	    EntryPoints: []string{entry},
//	    Bundle:      true,
//	    Plugins: []api.Plugin{
//	        esbuildsolid.Transform(solid.Options{}),
//	        esbuildsolid.Runtime(runtime.Config{Development: dev}),
//	    },
//	})
//
// [Transform] lowers Solid JSX; [Runtime] serves solid-js from the embedded
// copy so that no node_modules directory is required. Use Transform alone to
// keep resolving solid-js from disk.
package esbuildsolid

import (
	"fmt"
	"os"
	"path/filepath"

	esbuild "github.com/evanw/esbuild/pkg/api"

	"github.com/lilybw/go-solid-compiler/runtime"
	"github.com/lilybw/go-solid-compiler/solid"
	"github.com/lilybw/go-solid-compiler/tsx"
)

// Transform returns a plugin that lowers Solid JSX in every JSX and TSX file.
//
// esbuild's own JSX transform is React-shaped and cannot produce Solid's
// template calls, so this must own every such file in the graph rather than
// just the entry point.
//
// esbuild calls OnLoad concurrently across files. That is safe here: each call
// builds its own compiler state.
func Transform(opts solid.Options) esbuild.Plugin {
	return esbuild.Plugin{
		Name: "solid-transform",
		Setup: func(build esbuild.PluginBuild) {
			build.OnLoad(esbuild.OnLoadOptions{Filter: `\.[jt]sx$`},
				func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
					src, err := os.ReadFile(args.Path)
					if err != nil {
						return esbuild.OnLoadResult{}, err
					}

					file, err := tsx.Parse(args.Path, string(src), tsx.ScriptKindOf(args.Path))
					if err != nil {
						return esbuild.OnLoadResult{}, fmt.Errorf("solid parse %s: %w", args.Path, err)
					}
					contents, err := tsx.TransformSolid(file, opts)
					if err != nil {
						return esbuild.OnLoadResult{}, fmt.Errorf("solid transform %s: %w", args.Path, err)
					}

					// JSX is lowered but type annotations remain, so esbuild
					// strips those. LoaderTS rather than LoaderTSX on purpose:
					// no JSX should survive, and the TS loader fails loudly if
					// any does instead of applying the React transform to it.
					return esbuild.OnLoadResult{
						Contents:   &contents,
						Loader:     esbuild.LoaderTS,
						ResolveDir: filepath.Dir(args.Path),
					}, nil
				})
		},
	}
}

// runtimeNamespace keeps the embedded modules out of the file namespace, so
// esbuild never looks for them on disk.
const runtimeNamespace = "solid-runtime"

// Runtime returns a plugin that serves solid-js from the embedded copy,
// removing the need for a node_modules directory.
//
// Both consumer code and the embedded modules themselves import bare
// specifiers such as "solid-js/web", so resolution is registered for the file
// namespace and for the plugin's own namespace.
func Runtime(cfg runtime.Config) esbuild.Plugin {
	return esbuild.Plugin{
		Name: "solid-runtime",
		Setup: func(build esbuild.PluginBuild) {
			if !runtime.Available() {
				build.OnStart(func() (esbuild.OnStartResult, error) {
					return esbuild.OnStartResult{
						Errors: []esbuild.Message{{
							Text: "the embedded solid-js runtime is empty; run `go generate ./runtime` " +
								"in go-solid-compiler, or drop the Runtime plugin to resolve solid-js from disk",
						}},
					}, nil
				})
				return
			}

			claim := func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				if !runtime.Handles(args.Path) {
					return esbuild.OnResolveResult{}, nil
				}
				return esbuild.OnResolveResult{
					Path:      args.Path,
					Namespace: runtimeNamespace,
					// solid-js declares "sideEffects": false in its package
					// manifest, which esbuild cannot see once node resolution
					// is bypassed. Restating it here preserves tree shaking.
					SideEffects: esbuild.SideEffectsFalse,
				}, nil
			}

			// Imports from the application's own files.
			build.OnResolve(esbuild.OnResolveOptions{Filter: `^solid-js(/.*)?$`}, claim)
			// Imports between the embedded modules, which reference each other
			// by bare specifier rather than by relative path.
			build.OnResolve(esbuild.OnResolveOptions{
				Filter:    `^solid-js(/.*)?$`,
				Namespace: runtimeNamespace,
			}, claim)

			build.OnLoad(esbuild.OnLoadOptions{
				Filter:    `.*`,
				Namespace: runtimeNamespace,
			}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
				src, ok, err := cfg.Resolve(args.Path)
				if err != nil {
					return esbuild.OnLoadResult{}, err
				}
				if !ok {
					return esbuild.OnLoadResult{}, fmt.Errorf(
						"runtime: no embedded module for %q", args.Path)
				}
				return esbuild.OnLoadResult{
					Contents: &src,
					Loader:   esbuild.LoaderJS,
				}, nil
			})
		},
	}
}

// Plugins returns the transform and, when embed is true, the embedded runtime.
func Plugins(opts solid.Options, cfg runtime.Config, embed bool) []esbuild.Plugin {
	out := []esbuild.Plugin{Transform(opts)}
	if embed {
		out = append(out, Runtime(cfg))
	}
	return out
}
