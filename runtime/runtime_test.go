package runtime

import (
	"strings"
	"testing"
)

func TestEmbeddedRuntimeIsPopulated(t *testing.T) {
	if !Available() {
		t.Fatal("dist is empty; run `go generate ./runtime`")
	}
}

func TestResolvesEveryAdvertisedSpecifier(t *testing.T) {
	var cfg Config
	for _, spec := range Specifiers() {
		src, ok, err := cfg.Resolve(spec)
		if err != nil || !ok {
			t.Errorf("%s: ok=%v err=%v", spec, ok, err)
			continue
		}
		if len(src) == 0 {
			t.Errorf("%s: empty source", spec)
		}
	}
}

// The embedded modules import each other by bare specifier. Anything they
// reference must itself resolve, or the bundle breaks with an unresolvable
// import at build time rather than here.
func TestEmbeddedModulesOnlyImportResolvableSpecifiers(t *testing.T) {
	for _, dev := range []bool{false, true} {
		cfg := Config{Development: dev}
		for _, spec := range Specifiers() {
			src, ok, _ := cfg.Resolve(spec)
			if !ok {
				continue
			}
			for _, imported := range bareSpecifiers(src) {
				if _, ok, err := cfg.Resolve(imported); !ok || err != nil {
					t.Errorf("development=%v: %s imports %q, which does not resolve (err=%v)",
						dev, spec, imported, err)
				}
			}
		}
	}
}

// The client builds must not reach outside solid-js; the server build imports
// seroval and the storage build imports node:async_hooks, and embedding either
// would drag in dependencies that defeat the point.
func TestNoExternalDependencies(t *testing.T) {
	var cfg Config
	for _, spec := range Specifiers() {
		src, ok, _ := cfg.Resolve(spec)
		if !ok {
			continue
		}
		for _, imported := range bareSpecifiers(src) {
			if !Handles(imported) {
				t.Errorf("%s imports %q, which is outside solid-js", spec, imported)
			}
		}
	}
}

func TestDevelopmentBuildsDifferFromProduction(t *testing.T) {
	prod, _, _ := Config{}.Resolve("solid-js")
	dev, _, _ := Config{Development: true}.Resolve("solid-js")
	if prod == dev {
		t.Error("development and production builds are identical; vendoring picked the wrong files")
	}
}

// An entry point with no development build falls back rather than failing.
func TestDevelopmentFallsBackToProduction(t *testing.T) {
	src, ok, err := Config{Development: true}.Resolve("solid-js/h")
	if !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(src) == 0 {
		t.Error("empty source")
	}
}

func TestHandles(t *testing.T) {
	for _, s := range []string{"solid-js", "solid-js/web", "solid-js/store"} {
		if !Handles(s) {
			t.Errorf("%s should be handled", s)
		}
	}
	for _, s := range []string{"react", "solid-jsx", "./local", "seroval"} {
		if Handles(s) {
			t.Errorf("%s should not be handled", s)
		}
	}
}

func TestUnknownSolidSubpathIsAnError(t *testing.T) {
	// Distinguishing "not mine" from "mine and broken" is what lets a bundler
	// report something useful.
	_, ok, err := Config{}.Resolve("solid-js/nonexistent")
	if ok {
		t.Fatal("should not resolve")
	}
	if err == nil {
		t.Fatal("an unknown solid-js subpath should error, not be declined silently")
	}
	if !strings.Contains(err.Error(), "solid-js/web") {
		t.Errorf("the error should list what is available, got: %v", err)
	}
}

func TestOverrideWins(t *testing.T) {
	cfg := Config{Override: map[string]string{"solid-js": "export const x = 1;"}}
	src, ok, err := cfg.Resolve("solid-js")
	if !ok || err != nil || src != "export const x = 1;" {
		t.Errorf("override not applied: %q ok=%v err=%v", src, ok, err)
	}
}

func TestLicenseIsShipped(t *testing.T) {
	found := false
	for _, f := range Files() {
		if strings.HasSuffix(f, "LICENSE") {
			found = true
		}
	}
	if !found {
		t.Error("solid-js is MIT licensed; its LICENSE must ship alongside the embedded code")
	}
}

// bareSpecifiers extracts non-relative import sources from ESM source.
func bareSpecifiers(src string) []string {
	var out []string
	for _, part := range strings.Split(src, "from") {
		part = strings.TrimLeft(part, " \t")
		if part == "" {
			continue
		}
		q := part[0]
		if q != '"' && q != '\'' {
			continue
		}
		end := strings.IndexByte(part[1:], q)
		if end < 0 {
			continue
		}
		spec := part[1 : 1+end]
		if spec == "" || spec[0] == '.' || spec[0] == '/' {
			continue
		}
		out = append(out, spec)
	}
	return out
}
