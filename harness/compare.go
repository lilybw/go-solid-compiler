// Package harness compares this compiler's output against the reference
// implementation, babel-preset-solid.
//
// The two agree on semantics but not on spelling, so comparison is layered:
// templates, the helper set, and the delegated event set must match exactly;
// statement bodies are compared after normalizing generated variable names,
// equivalent accessor forms, and DOM navigation paths.
package harness

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Output is the structured content extracted from a generated module.
type Output struct {
	// Templates are the hoisted HTML strings, in declaration order, with
	// template-literal escaping removed.
	Templates []string
	// Helpers are the runtime helpers imported, sorted.
	Helpers []string
	// Delegated are the event names registered for delegation, sorted.
	Delegated []string
	// Body is everything else, normalized.
	Body string
}

// Extract parses a generated module into its comparable parts.
func Extract(src string) Output {
	var out Output
	rest := src

	out.Templates, rest = extractTemplates(rest)
	out.Helpers, rest = extractHelpers(rest)
	out.Delegated, rest = extractDelegated(rest)
	out.Body = NormalizeBody(rest)
	return out
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

var templateCall = regexp.MustCompile(`_\$?template\s*\(`)

// extractTemplates pulls out every template literal passed to the template
// helper, returning the unescaped HTML and the remaining source.
func extractTemplates(src string) ([]string, string) {
	var templates []string
	var kept strings.Builder
	pos := 0

	for {
		loc := templateCall.FindStringIndex(src[pos:])
		if loc == nil {
			break
		}
		callStart := pos + loc[0]
		argStart := pos + loc[1]

		content, end, ok := scanTemplateLiteral(src, argStart)
		if !ok {
			// Not a template literal argument; leave it alone.
			kept.WriteString(src[pos:argStart])
			pos = argStart
			continue
		}
		templates = append(templates, content)

		// Drop the whole declaration line containing this call.
		lineStart := strings.LastIndexByte(src[:callStart], '\n') + 1
		lineEnd := end
		if i := strings.IndexByte(src[end:], '\n'); i >= 0 {
			lineEnd = end + i + 1
		} else {
			lineEnd = len(src)
		}
		kept.WriteString(src[pos:lineStart])
		pos = lineEnd
	}
	kept.WriteString(src[pos:])
	return templates, kept.String()
}

// scanTemplateLiteral reads a backtick literal at or after i, returning its
// unescaped content and the index past the closing backtick.
func scanTemplateLiteral(src string, i int) (string, int, bool) {
	for i < len(src) && (src[i] == ' ' || src[i] == '\n' || src[i] == '\t') {
		i++
	}
	// Skip a leading /*#__PURE__*/ style comment if present.
	if strings.HasPrefix(src[i:], "/*") {
		if j := strings.Index(src[i:], "*/"); j >= 0 {
			i += j + 2
			for i < len(src) && (src[i] == ' ' || src[i] == '\n' || src[i] == '\t') {
				i++
			}
		}
	}
	if i >= len(src) || src[i] != '`' {
		return "", i, false
	}
	i++
	var b strings.Builder
	for i < len(src) {
		c := src[i]
		if c == '\\' && i+1 < len(src) {
			// Unescape so that differing escape conventions do not register
			// as differing templates.
			b.WriteByte(src[i+1])
			i += 2
			continue
		}
		if c == '`' {
			return b.String(), i + 1, true
		}
		b.WriteByte(c)
		i++
	}
	return "", i, false
}

// ---------------------------------------------------------------------------
// Imports
// ---------------------------------------------------------------------------

var importClause = regexp.MustCompile(`(?s)import\s*\{(.*?)\}\s*from\s*["'][^"']*solid-js[^"']*["']\s*;?`)

// extractHelpers collects the runtime helpers imported from solid-js.
func extractHelpers(src string) ([]string, string) {
	seen := map[string]bool{}
	rest := importClause.ReplaceAllStringFunc(src, func(m string) string {
		inner := importClause.FindStringSubmatch(m)[1]
		for _, spec := range strings.Split(inner, ",") {
			spec = strings.TrimSpace(spec)
			if spec == "" {
				continue
			}
			// "insert as _$insert" -> the imported name is what matters.
			if j := strings.Index(spec, " as "); j >= 0 {
				spec = strings.TrimSpace(spec[:j])
			}
			seen[spec] = true
		}
		return ""
	})

	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out, rest
}

// ---------------------------------------------------------------------------
// Delegated events
// ---------------------------------------------------------------------------

var delegateCall = regexp.MustCompile(`_\$?delegateEvents\s*\(\s*\[(.*?)\]\s*\)\s*;?`)

func extractDelegated(src string) ([]string, string) {
	seen := map[string]bool{}
	rest := delegateCall.ReplaceAllStringFunc(src, func(m string) string {
		inner := delegateCall.FindStringSubmatch(m)[1]
		for _, e := range strings.Split(inner, ",") {
			e = strings.Trim(strings.TrimSpace(e), `"'`)
			if e != "" {
				seen[e] = true
			}
		}
		return ""
	})

	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out, rest
}

// ---------------------------------------------------------------------------
// Body normalization
// ---------------------------------------------------------------------------

var (
	generatedVar = regexp.MustCompile(`_(?:el|tmpl|p|v|c)\$\d*`)
	// The trailing delimiter is captured rather than looked ahead, because RE2
	// has no lookahead; the replacement puts it back.
	arrowCall    = regexp.MustCompile(`\(\s*\)\s*=>\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*\(\s*\)($|[,;)\]}])`)
	singleParam  = regexp.MustCompile(`\(\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*\)\s*=>`)
	pureComment  = regexp.MustCompile(`/\*\s*#__PURE__\s*\*/`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComment  = regexp.MustCompile(`(?m)//[^\n]*$`)
	whitespace   = regexp.MustCompile(`\s+`)
)

// NormalizeBody reduces generated code to a form where only meaningful
// differences remain: generated variables are alpha-renamed, equivalent
// accessor spellings are collapsed, comments and whitespace are removed, and
// DOM navigation is expanded to root-relative paths.
func NormalizeBody(src string) string {
	s := pureComment.ReplaceAllString(src, "")
	s = blockComment.ReplaceAllString(s, "")
	s = lineComment.ReplaceAllString(s, "")

	s = arrowCall.ReplaceAllString(s, "$1$2")
	s = singleParam.ReplaceAllString(s, "$1 =>")
	s = alphaRename(s)

	s = whitespace.ReplaceAllString(s, " ")
	// Punctuation spacing carries no meaning once whitespace is collapsed.
	for _, p := range []string{",", ";", "{", "}", "(", ")", "[", "]", ":", "="} {
		s = strings.ReplaceAll(s, " "+p, p)
		s = strings.ReplaceAll(s, p+" ", p)
	}
	s = canonicalizeNavigation(s)
	return strings.TrimSpace(s)
}

var (
	navAssign = regexp.MustCompile(`(_v\d+)=(_v\d+(?:\.(?:firstChild|nextSibling))+)`)
	navRef    = regexp.MustCompile(`_v\d+`)
)

// canonicalizeNavigation rewrites every DOM navigation variable into its
// full path from the cloned root, then drops the intermediate declarations,
// so that the node addressed is compared rather than the route taken.
func canonicalizeNavigation(s string) string {
	paths := map[string]string{}

	// Resolve transitively: a chain may be defined in terms of an earlier one,
	// and declarations appear in dependency order.
	for _, m := range navAssign.FindAllStringSubmatch(s, -1) {
		name, expr := m[1], m[2]
		base := navRef.FindString(expr)
		if root, ok := paths[base]; ok {
			expr = root + strings.TrimPrefix(expr, base)
		}
		paths[name] = expr
	}
	if len(paths) == 0 {
		return s
	}

	// Remove the now-redundant declarations. They always follow the root
	// binding, so each is preceded by a comma.
	out := navAssign.ReplaceAllStringFunc(s, func(m string) string {
		name := navAssign.FindStringSubmatch(m)[1]
		if _, ok := paths[name]; ok {
			return "\x00"
		}
		return m
	})
	out = strings.ReplaceAll(out, ",\x00", "")
	out = strings.ReplaceAll(out, "\x00,", "")
	out = strings.ReplaceAll(out, "\x00", "")
	// A declaration list that held nothing but navigation is now empty.
	out = strings.ReplaceAll(out, "const;", "")

	// Substitute the remaining references with their full paths.
	return navRef.ReplaceAllStringFunc(out, func(name string) string {
		if p, ok := paths[name]; ok {
			return p
		}
		return name
	})
}

// alphaRename replaces generated identifiers with canonical ones in order of
// first appearance.
func alphaRename(src string) string {
	next := 0
	seen := map[string]string{}
	return generatedVar.ReplaceAllStringFunc(src, func(name string) string {
		if canon, ok := seen[name]; ok {
			return canon
		}
		next++
		canon := fmt.Sprintf("_v%d", next)
		seen[name] = canon
		return canon
	})
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

// Report describes how one fixture's output differs from the reference.
type Report struct {
	Name       string
	Mismatches []Mismatch
}

// Mismatch is a single difference, categorized as "template", "helpers",
// "events", or "body".
type Mismatch struct {
	// Kind is "template", "helpers", "events", or "body".
	Kind      string
	Reference string
	Actual    string
	Detail    string
}

// OK reports whether the outputs agree.
func (r Report) OK() bool { return len(r.Mismatches) == 0 }

func (r Report) String() string {
	if r.OK() {
		return r.Name + ": ok"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d mismatch(es)\n", r.Name, len(r.Mismatches))
	for _, m := range r.Mismatches {
		fmt.Fprintf(&b, "  [%s] %s\n", m.Kind, m.Detail)
		if m.Reference != "" || m.Actual != "" {
			fmt.Fprintf(&b, "    babel: %s\n", truncate(m.Reference, 400))
			fmt.Fprintf(&b, "    ours:  %s\n", truncate(m.Actual, 400))
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Compare checks generated output against the reference.
func Compare(name, reference, actual string) Report {
	ref := Extract(reference)
	act := Extract(actual)
	rep := Report{Name: name}

	// Templates are compared as a multiset: the reference hoists them as each
	// subtree finishes, which is not document order.
	sortedRef := append([]string(nil), ref.Templates...)
	sortedAct := append([]string(nil), act.Templates...)
	sort.Strings(sortedRef)
	sort.Strings(sortedAct)
	ref.Templates, act.Templates = sortedRef, sortedAct

	if len(ref.Templates) != len(act.Templates) {
		rep.Mismatches = append(rep.Mismatches, Mismatch{
			Kind:      "template",
			Detail:    fmt.Sprintf("template count: babel %d, ours %d", len(ref.Templates), len(act.Templates)),
			Reference: strings.Join(ref.Templates, " | "),
			Actual:    strings.Join(act.Templates, " | "),
		})
	} else {
		for i := range ref.Templates {
			if ref.Templates[i] != act.Templates[i] {
				rep.Mismatches = append(rep.Mismatches, Mismatch{
					Kind:      "template",
					Detail:    fmt.Sprintf("template %d differs", i+1),
					Reference: ref.Templates[i],
					Actual:    act.Templates[i],
				})
			}
		}
	}

	if d := setDiff(ref.Helpers, act.Helpers); d != "" {
		rep.Mismatches = append(rep.Mismatches, Mismatch{
			Kind:      "helpers",
			Detail:    d,
			Reference: strings.Join(ref.Helpers, ", "),
			Actual:    strings.Join(act.Helpers, ", "),
		})
	}

	if d := setDiff(ref.Delegated, act.Delegated); d != "" {
		rep.Mismatches = append(rep.Mismatches, Mismatch{
			Kind:      "events",
			Detail:    d,
			Reference: strings.Join(ref.Delegated, ", "),
			Actual:    strings.Join(act.Delegated, ", "),
		})
	}

	if ref.Body != act.Body {
		rep.Mismatches = append(rep.Mismatches, Mismatch{
			Kind:      "body",
			Detail:    "normalized bodies differ",
			Reference: ref.Body,
			Actual:    act.Body,
		})
	}
	return rep
}

// setDiff describes how two sorted sets differ, or "" if they match.
func setDiff(want, got []string) string {
	inWant := map[string]bool{}
	for _, w := range want {
		inWant[w] = true
	}
	inGot := map[string]bool{}
	for _, g := range got {
		inGot[g] = true
	}
	var missing, extra []string
	for _, w := range want {
		if !inGot[w] {
			missing = append(missing, w)
		}
	}
	for _, g := range got {
		if !inWant[g] {
			extra = append(extra, g)
		}
	}
	switch {
	case len(missing) == 0 && len(extra) == 0:
		return ""
	case len(extra) == 0:
		return "missing: " + strings.Join(missing, ", ")
	case len(missing) == 0:
		return "unexpected: " + strings.Join(extra, ", ")
	default:
		return "missing: " + strings.Join(missing, ", ") +
			"; unexpected: " + strings.Join(extra, ", ")
	}
}
