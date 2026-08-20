// Package solid lowers JSX into the DOM-expressions calls that solid-js/web
// provides.
//
// The transform operates on the JSX intermediate representation defined in
// this file rather than on any parser's AST. Build the IR directly, or use
// the tsx package to produce it from TypeScript source.
//
//	m := solid.NewModule(solid.Options{})
//	expr := m.Compile(node)
//	src := m.Prelude() + "const C = () => " + expr + ";" + m.Postlude()
//
// Compile returns a JavaScript expression. Prelude returns the runtime import
// and hoisted templates the compiled expressions refer to, and Postlude the
// event delegation registration; both must be included in the emitted module.
package solid

import (
	"strings"
)

// Node is a node of the JSX intermediate representation.
type Node interface{ jsxNode() }

// Element is a host element: a lowercase tag that becomes real DOM.
type Element struct {
	Tag      string
	Attrs    []Attr
	Children []Node
}

// Component is a capitalized or dotted tag, which compiles to a
// createComponent call rather than to DOM.
type Component struct {
	Tag      string
	Attrs    []Attr
	Children []Node
}

// Fragment is <>...</>, which compiles to an array.
type Fragment struct {
	Children []Node
}

// Text is literal text between tags. Value is the already-normalized text;
// see [NormalizeJSXText].
type Text struct {
	Value string
}

// Expr is a {...} interpolation. Code is the original TypeScript source of
// the expression, which is emitted verbatim.
type Expr struct {
	Code string
}

// Spread is a {...props} attribute position.
type Spread struct {
	Code string
}

func (*Element) jsxNode()   {}
func (*Component) jsxNode() {}
func (*Fragment) jsxNode()  {}
func (*Text) jsxNode()      {}
func (*Expr) jsxNode()      {}
func (*Spread) jsxNode()    {}

// AttrKind discriminates how an attribute's value was written.
type AttrKind uint8

const (
	// AttrBare is a valueless attribute, as in <input disabled />, which means
	// the boolean true.
	AttrBare AttrKind = iota
	// AttrString is a literal string value, as in class="card".
	AttrString
	// AttrExpr is an interpolated value, as in class={x}.
	AttrExpr
)

// Attr is a JSX attribute. Namespace holds the part before a colon, as in
// on:click, use:directive, prop:value, attr:id, bool:x, style:color,
// class:active.
type Attr struct {
	Namespace string
	Name      string
	Kind      AttrKind
	// Value is the string content for AttrString, or the expression source
	// for AttrExpr.
	Value string
}

// IsDynamic reports whether the value must be re-evaluated when reactive
// state changes.
func (a Attr) IsDynamic() bool {
	if a.Kind != AttrExpr {
		return false
	}
	return !isStaticExpr(a.Value)
}

// isStaticExpr reports whether an expression cannot change once evaluated:
// a literal, a bare identifier, or a function expression.
func isStaticExpr(src string) bool {
	s := strings.TrimSpace(src)
	if s == "" {
		return true
	}
	return isLiteralExpr(s) || isPlainIdent(s) || hasTopLevelArrow(s)
}

// hasTopLevelArrow reports whether src is itself a function expression, as
// opposed to merely containing one.
func hasTopLevelArrow(src string) bool {
	if strings.HasPrefix(strings.TrimSpace(src), "function") {
		return true
	}
	depth := 0
	var inStr byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '=':
			if depth == 0 && i+1 < len(src) && src[i+1] == '>' {
				return true
			}
		}
	}
	return false
}

// lazyValue keeps a spread argument callable, so that mergeProps can re-read
// it. Evaluating the call here would snapshot the props and drop reactivity.
func lazyValue(code string) string {
	s := strings.TrimSpace(code)
	if strings.HasSuffix(s, "()") {
		if inner := strings.TrimSpace(s[:len(s)-2]); isPlainIdent(inner) {
			return inner
		}
	}
	return s
}

// isLiteralExpr reports whether source is a literal that cannot change.
func isLiteralExpr(src string) bool {
	s := strings.TrimSpace(src)
	if s == "" {
		return true
	}
	switch s {
	case "true", "false", "null", "undefined":
		return true
	}
	// A quoted string with no interior quote of the same kind.
	if len(s) >= 2 {
		q := s[0]
		if (q == '"' || q == '\'') && s[len(s)-1] == q &&
			!strings.ContainsRune(s[1:len(s)-1], rune(q)) {
			return true
		}
	}
	// A plain numeric literal.
	hasDigit := false
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '.' || r == '_':
		case (r == '-' || r == '+') && i == 0:
		default:
			return false
		}
	}
	return hasDigit
}

// IsDynamic reports whether an interpolated child must be wrapped so that it
// re-runs on change.
func (e *Expr) IsDynamic() bool { return !isStaticExpr(e.Code) }

// ---------------------------------------------------------------------------
// Tag classification
// ---------------------------------------------------------------------------

// IsComponentTag reports whether a JSX tag names a component rather than a
// host element: capitalized, dotted, or namespaced.
func IsComponentTag(tag string) bool {
	if tag == "" {
		return false
	}
	if strings.ContainsAny(tag, ".") {
		return true
	}
	c := tag[0]
	return !(c >= 'a' && c <= 'z')
}

// voidElements are HTML elements that must not be given a closing tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// IsVoidElement reports whether tag is an HTML void element.
func IsVoidElement(tag string) bool { return voidElements[strings.ToLower(tag)] }

// delegatedEvents are the events solid-js routes through one document-level
// listener rather than attaching per element.
var delegatedEvents = map[string]bool{
	"beforeinput": true, "click": true, "dblclick": true, "contextmenu": true,
	"focusin": true, "focusout": true, "input": true, "keydown": true,
	"keyup": true, "mousedown": true, "mousemove": true, "mouseout": true,
	"mouseover": true, "mouseup": true, "pointerdown": true,
	"pointermove": true, "pointerout": true, "pointerover": true,
	"pointerup": true, "touchend": true, "touchmove": true, "touchstart": true,
}

// IsDelegatedEvent reports whether an event name is delegated by solid-js.
func IsDelegatedEvent(name string) bool { return delegatedEvents[strings.ToLower(name)] }

// propertyAttrs must be assigned as DOM properties rather than through
// setAttribute, because attribute and property diverge after user input.
var propertyAttrs = map[string]bool{
	"value": true, "checked": true, "selected": true, "muted": true,
	"volume": true, "playbackRate": true, "srcObject": true,
	"indeterminate": true, "textContent": true, "innerHTML": true,
}

// ---------------------------------------------------------------------------
// Text normalization
// ---------------------------------------------------------------------------

// NormalizeJSXText applies JSX's whitespace rules to raw text between tags:
// whitespace-only lines vanish, indentation is stripped, and interior line
// breaks collapse to a single space.
//
// The empty string means the text contributes nothing.
func NormalizeJSXText(s string) string {
	if s == "" {
		return ""
	}
	if !strings.ContainsAny(s, "\n\r") {
		// A single-line run of text is preserved exactly, including any
		// significant leading or trailing spaces.
		if strings.TrimSpace(s) == "" {
			return " "
		}
		return s
	}

	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		trimmed := line
		// Every line except the first loses its leading indentation, and every
		// line except the last loses its trailing whitespace.
		if i > 0 {
			trimmed = strings.TrimLeft(trimmed, " \t")
		}
		if i < len(lines)-1 {
			trimmed = strings.TrimRight(trimmed, " \t")
		}
		if trimmed == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, " ")
}

// ---------------------------------------------------------------------------
// HTML escaping
// ---------------------------------------------------------------------------

// escapeHTMLText escapes text destined for a template's element content.
func escapeHTMLText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case 0xA0:
			b.WriteString("&nbsp;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeHTMLAttr escapes a value destined for a double-quoted attribute.
func escapeHTMLAttr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case 0xA0:
			b.WriteString("&nbsp;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// quoteJS renders s as a double-quoted JavaScript string literal.
func quoteJS(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// quoteTemplate renders s as a backtick template literal.
func quoteTemplate(s string) string {
	var b strings.Builder
	b.WriteByte('`')
	rs := []rune(s)
	for i, r := range rs {
		switch r {
		case '`':
			b.WriteString("\\`")
		case '\\':
			b.WriteString(`\\`)
		case '$':
			if i+1 < len(rs) && rs[i+1] == '{' {
				b.WriteString(`\$`)
			} else {
				b.WriteByte('$')
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('`')
	return b.String()
}

// attrNeedsQuotes reports whether an attribute value must be quoted in the
// template.
func attrNeedsQuotes(v string) bool {
	if v == "" {
		return true
	}
	for _, r := range v {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '"', '\'', '=', '<', '>', '`':
			return true
		}
	}
	return false
}

// elideTrailingCloseTags removes closing tags at the end of a template. The
// HTML parser closes any element left open at the end of a fragment.
func elideTrailingCloseTags(html string) string {
	for {
		if !strings.HasSuffix(html, ">") {
			return html
		}
		open := strings.LastIndex(html, "<")
		if open < 0 || !strings.HasPrefix(html[open:], "</") {
			return html
		}
		html = html[:open]
	}
}

// attrHTMLName maps a JSX attribute name to its HTML attribute name.
func attrHTMLName(name string) string {
	switch name {
	case "className":
		return "class"
	case "htmlFor":
		return "for"
	}
	return name
}
