// Package runtime embeds a pinned copy of the solid-js client runtime and
// resolves imports of it during bundling, so consumers need no npm install
// and no node_modules directory.
//
// The embedded files are not committed by hand. Run
//
//	go generate ./runtime
//
// to fetch the pinned release. Transform output is coupled to the runtime
// version, so changing [Version] means re-running the test suite.
package runtime

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:generate ./vendor.sh

// Version is the solid-js release this runtime was vendored from.
const Version = "1.9.14"

// dist holds the vendored solid-js ESM build. The all: prefix is required
// because some solid-js files begin with an underscore.
var dist embed.FS

// moduleAliases maps import specifiers to paths inside the embedded tree.
var moduleAliases = map[string]string{
	"solid-js":       "dist/solid.js",
	"solid-js/web":   "dist/web.js",
	"solid-js/store": "dist/store.js",
	"solid-js/html":  "dist/html.js",
	"solid-js/h":     "dist/h.js",
}

// Config controls resolution.
type Config struct {
	// Fallback is consulted when a specifier is not part of the embedded
	// runtime. Returning ("", false) lets the bundler resolve it normally,
	// which is what allows a consumer to keep a node_modules directory for
	// their own dependencies while still getting solid-js from here.
	Fallback func(specifier string) (contents string, ok bool)

	// Override replaces an embedded module wholesale, which is the escape
	// hatch for a consumer pinned to a different solid-js version.
	Override map[string]string
}

// IsRuntimeModule reports whether a specifier is served by the embedded
// runtime.
func IsRuntimeModule(specifier string) bool {
	if _, ok := moduleAliases[specifier]; ok {
		return true
	}
	return strings.HasPrefix(specifier, "solid-js/")
}

// Resolve returns the source of an embedded module. Relative specifiers are
// resolved against the importer.
func (c Config) Resolve(specifier, importer string) (string, bool, error) {
	if c.Override != nil {
		if src, ok := c.Override[specifier]; ok {
			return src, true, nil
		}
	}

	var target string
	switch {
	case strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../"):
		if !strings.HasPrefix(importer, "dist/") {
			return "", false, nil
		}
		target = path.Join(path.Dir(importer), specifier)
	default:
		alias, ok := moduleAliases[specifier]
		if !ok {
			if !strings.HasPrefix(specifier, "solid-js/") {
				break
			}
			alias = "dist/" + strings.TrimPrefix(specifier, "solid-js/") + ".js"
		}
		target = alias
	}

	if target != "" {
		if src, err := fs.ReadFile(dist, target); err == nil {
			return string(src), true, nil
		}
		if !strings.HasSuffix(target, ".js") {
			if src, err := fs.ReadFile(dist, target+".js"); err == nil {
				return string(src), true, nil
			}
		}
	}

	if c.Fallback != nil {
		if src, ok := c.Fallback(specifier); ok {
			return src, true, nil
		}
	}

	if IsRuntimeModule(specifier) {
		return "", false, fmt.Errorf(
			"runtime: %q is not present in the embedded solid-js %s; "+
				"run `go generate ./runtime` to populate it, or supply Config.Override",
			specifier, Version)
	}
	return "", false, nil
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
	return out
}
