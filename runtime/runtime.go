// Package runtime embeds the solid-js client runtime so that bundling a Solid
// application needs no npm install and no node_modules directory.
//
// The embedded files are not committed by hand. Run
//
//	go generate ./runtime
//
// to fetch the pinned release. Transform output is coupled to the runtime
// version, so changing [Version] means re-running the test suite.
//
// Use [github.com/lilybw/go-solid-compiler/esbuildsolid.Runtime] to serve these
// files to esbuild; [Config.Resolve] is the underlying lookup for other
// bundlers.
package runtime

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:generate ./vendor.sh

// Version is the solid-js release this runtime was vendored from.
const Version = "1.9.14"

// dist holds the vendored solid-js ESM builds. The all: prefix is required
// because some solid-js files begin with an underscore.
//
//go:embed all:dist
var dist embed.FS

// Only the client builds are embedded. The server build imports seroval and
// the storage build imports node:async_hooks, whereas the client builds import
// nothing outside solid-js itself, which is what lets them be served without a
// package manager.
var (
	production = map[string]string{
		"solid-js":                 "dist/solid.js",
		"solid-js/web":             "dist/web.js",
		"solid-js/store":           "dist/store.js",
		"solid-js/html":            "dist/html.js",
		"solid-js/h":               "dist/h.js",
		"solid-js/jsx-runtime":     "dist/solid.js",
		"solid-js/jsx-dev-runtime": "dist/solid.js",
	}
	development = map[string]string{
		"solid-js":       "dist/solid.dev.js",
		"solid-js/web":   "dist/web.dev.js",
		"solid-js/store": "dist/store.dev.js",
	}
)

// Config selects which build to serve.
type Config struct {
	// Development serves the development builds, which carry Solid's runtime
	// warnings. Entry points without a development build fall back to the
	// production one.
	Development bool

	// Override replaces a module's source entirely, keyed by import specifier.
	// It is the escape hatch for pinning a different solid-js version without
	// abandoning the embedded runtime.
	Override map[string]string
}

// Handles reports whether a specifier is served by the embedded runtime.
func Handles(specifier string) bool {
	return specifier == "solid-js" || strings.HasPrefix(specifier, "solid-js/")
}

// Resolve returns the source of an embedded module.
//
// It reports false for a specifier the runtime does not serve, and an error
// for one it should serve but cannot, which distinguishes "not mine" from
// "mine and broken".
func (c Config) Resolve(specifier string) (string, bool, error) {
	if src, ok := c.Override[specifier]; ok {
		return src, true, nil
	}
	if !Handles(specifier) {
		return "", false, nil
	}

	path := ""
	if c.Development {
		path = development[specifier]
	}
	if path == "" {
		path = production[specifier]
	}
	if path == "" {
		return "", false, fmt.Errorf(
			"runtime: %q is not part of the embedded solid-js %s (available: %s)",
			specifier, Version, strings.Join(Specifiers(), ", "))
	}

	src, err := fs.ReadFile(dist, path)
	if err != nil {
		return "", false, fmt.Errorf(
			"runtime: %s is missing from the embedded solid-js %s; run `go generate ./runtime`",
			path, Version)
	}
	return string(src), true, nil
}

// Specifiers lists the import specifiers the embedded runtime serves.
func Specifiers() []string {
	out := make([]string, 0, len(production))
	for s := range production {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Available reports whether the embedded runtime has been populated by
// go generate.
func Available() bool {
	entries, err := fs.ReadDir(dist, "dist")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".js") {
			return true
		}
	}
	return false
}

// Files lists the embedded module paths, which is useful for diagnostics.
func Files() []string {
	var out []string
	_ = fs.WalkDir(dist, "dist", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}
