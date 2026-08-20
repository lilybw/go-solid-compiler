#!/usr/bin/env bash
# Repair the missing localization assets in the typescript-go fork.
#
# # The defect
#
# github.com/Zzzen/typescript-go re-exports the compiler's internals by copying
# them out of internal/ and rewriting import paths. Its sync script skips every
# file that does not end in .go:
#
#     if !strings.HasSuffix(path, ".go") { return nil }
#
# The diagnostics package embeds one gzipped JSON file per locale:
#
#     //go:embed loc/cs-CZ.json.gz
#     var csCZData string
#
# Those assets are not .go files, so they never get copied, and the package
# fails to build:
#
#     loc_generated.go:76:12: pattern loc/cs-CZ.json.gz: no matching files found
#
# ast imports diagnostics, so nothing downstream builds either. This is almost
# certainly why the fork shows zero importers: as published, it does not
# compile.
#
# # Why empty files are the right repair
#
# diagnostics.Localize looks a key up in the locale map and falls back to the
# English message text when it is absent:
#
#     text := message.text
#     if localized, ok := getLocalizedMessages(...)[message.key]; ok {
#         text = localized
#     }
#
# So a locale file containing an empty JSON object is not a stub that papers
# over a problem — it produces exactly the behaviour this project wants, which
# is English diagnostics. Nothing is lost; only translations we never use.
#
# # Why it writes into the module cache
#
# The alternative is maintaining a fork of the fork, which is the durable fix
# and is described in the README. This is the cheap one: it is idempotent, it
# runs from setup, and it is confined to a directory the container owns. It has
# to be re-run after `go clean -modcache` or a version bump, which is why setup
# calls it every time rather than only once.
set -euo pipefail

module="github.com/Zzzen/typescript-go"

dir="$(go list -m -f '{{.Dir}}' "$module" 2>/dev/null || true)"
if [ -z "$dir" ] || [ ! -d "$dir" ]; then
    echo "patch-tsgo-loc: $module is not in the module graph yet" >&2
    echo "patch-tsgo-loc: run 'go get $module@main' first" >&2
    exit 1
fi

diagnostics="$dir/use-at-your-own-risk/diagnostics"
generated="$diagnostics/loc_generated.go"

if [ ! -f "$generated" ]; then
    echo "patch-tsgo-loc: no loc_generated.go under $diagnostics" >&2
    echo "patch-tsgo-loc: the fork's layout changed; this script needs updating" >&2
    exit 1
fi

# Read the locale list out of the embed directives rather than hard-coding it,
# so that a locale added upstream is picked up without editing this script.
locales="$(grep -o 'loc/[A-Za-z0-9_-]*\.json\.gz' "$generated" | sort -u || true)"
if [ -z "$locales" ]; then
    echo "patch-tsgo-loc: no //go:embed loc/... directives found; nothing to do"
    exit 0
fi

missing=0
for rel in $locales; do
    [ -f "$diagnostics/$rel" ] || missing=$((missing + 1))
done

if [ "$missing" -eq 0 ]; then
    echo "patch-tsgo-loc: locale assets already present; nothing to do"
    exit 0
fi

# The module cache is checked out read-only. The container owns the volume, so
# widening the mode is permitted; only this one directory is touched.
chmod -R u+w "$diagnostics"
mkdir -p "$diagnostics/loc"

created=0
for rel in $locales; do
    target="$diagnostics/$rel"
    [ -f "$target" ] && continue
    # An empty JSON object gunzips to a map with no entries, which sends every
    # lookup down the English fallback path.
    printf '{}' | gzip -c > "$target"
    created=$((created + 1))
done

echo "patch-tsgo-loc: created $created empty locale asset(s) in $diagnostics/loc"
echo "patch-tsgo-loc: diagnostics will render in English, which is the fallback anyway"
