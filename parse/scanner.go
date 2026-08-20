package parse

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lilybw/go-solid-compiler/token"
)

// Tok is a token kind.
type Tok uint8

const (
	EOF Tok = iota
	Ident
	Str        // string literal
	Num        // numeric literal
	BigIntTok  // numeric literal with n suffix
	NoSubTmpl  // `text`
	TmplHead   // `text${
	TmplMiddle // }text${
	TmplTail   // }text`
	Punct      // any operator or delimiter; Text holds the exact spelling
)

// Token is a lexed token. Text is the source spelling for identifiers and
// punctuation, and the decoded value for string and template literals.
type Token struct {
	Kind Tok
	Text string
	Span token.Span
	// Raw is the undecoded source text, retained for literals so that a
	// numeric type literal round-trips its original spelling.
	Raw string
	// NewlineBefore records whether a line terminator preceded this token,
	// which TypeScript's grammar is sensitive to in several places.
	NewlineBefore bool
	// Docs holds JSDoc comment blocks immediately preceding this token.
	Docs []string
}

// Scanner turns TypeScript source into tokens.
type Scanner struct {
	src    []byte
	file   *token.File
	offset int
	errs   *ErrorList

	// Template substitutions are brace-delimited, so a closing brace is
	// ambiguous: it may end a block or resume a template. The scanner tracks
	// brace depth and the depth at which each open substitution began, which
	// lets Next resolve the ambiguity without help from the parser.
	braceDepth int
	tmplStack  []int
}

// NewScanner returns a Scanner over src.
func NewScanner(name string, src []byte, errs *ErrorList) *Scanner {
	return &Scanner{src: src, file: token.NewFile(name, src), errs: errs}
}

// File returns the position table for the scanned source.
func (s *Scanner) File() *token.File { return s.file }

func (s *Scanner) errorf(pos token.Pos, format string, args ...any) {
	if s.errs == nil {
		return
	}
	*s.errs = append(*s.errs, Error{
		Pos: s.file.Position(pos),
		Msg: fmt.Sprintf(format, args...),
	})
}

// pos converts a byte offset to a 1-based token.Pos.
func (s *Scanner) pos(off int) token.Pos { return token.Pos(off + 1) }

// text returns the source between a 1-based start position and the current
// offset.
func (s *Scanner) text(start token.Pos) string {
	return string(s.src[int(start)-1 : s.offset])
}

func (s *Scanner) peekByte(off int) byte {
	if s.offset+off < len(s.src) {
		return s.src[s.offset+off]
	}
	return 0
}

// Next returns the next token.
func (s *Scanner) Next() Token {
	nl, docs := s.skipTrivia()
	start := s.pos(s.offset)

	// A closing brace at the depth where a substitution began resumes the
	// template rather than closing a block.
	if n := len(s.tmplStack); n > 0 && s.braceDepth == s.tmplStack[n-1] &&
		s.offset < len(s.src) && s.src[s.offset] == '}' {
		s.tmplStack = s.tmplStack[:n-1]
		s.offset++
		t := s.scanTemplatePart(start, false, nl, docs)
		if t.Kind == TmplMiddle {
			s.tmplStack = append(s.tmplStack, s.braceDepth)
		}
		return t
	}

	if s.offset >= len(s.src) {
		return Token{Kind: EOF, Span: token.Span{Start: start, End: start},
			NewlineBefore: nl, Docs: docs}
	}

	c := s.src[s.offset]
	switch {
	case isIdentStart(rune(c)) || c >= utf8.RuneSelf:
		return s.scanIdent(start, nl, docs)
	case c >= '0' && c <= '9':
		return s.scanNumber(start, nl, docs)
	case c == '.' && s.peekByte(1) >= '0' && s.peekByte(1) <= '9':
		return s.scanNumber(start, nl, docs)
	case c == '"' || c == '\'':
		return s.scanString(start, nl, docs)
	case c == '`':
		s.offset++
		t := s.scanTemplatePart(start, true, nl, docs)
		if t.Kind == TmplHead {
			s.tmplStack = append(s.tmplStack, s.braceDepth)
		}
		return t
	default:
		t := s.scanPunct(start, nl, docs)
		switch t.Text {
		case "{":
			s.braceDepth++
		case "}":
			s.braceDepth--
		}
		return t
	}
}

// ScanTemplateContinue resumes scanning a template literal at a closing brace.
// The parser calls it after consuming a substitution expression.
func (s *Scanner) ScanTemplateContinue() Token {
	start := s.pos(s.offset)
	if s.offset < len(s.src) && s.src[s.offset] == '}' {
		s.offset++
	}
	return s.scanTemplatePart(start, false, false, nil)
}

// skipTrivia consumes whitespace and comments, reporting whether a newline
// was crossed and collecting JSDoc blocks.
func (s *Scanner) skipTrivia() (newline bool, docs []string) {
	for s.offset < len(s.src) {
		c := s.src[s.offset]
		switch {
		case c == '\n':
			newline = true
			s.offset++
		case c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f':
			s.offset++
		case c == '/' && s.peekByte(1) == '/':
			for s.offset < len(s.src) && s.src[s.offset] != '\n' {
				s.offset++
			}
		case c == '/' && s.peekByte(1) == '*':
			isDoc := s.peekByte(2) == '*' && s.peekByte(3) != '/'
			begin := s.offset + 2
			s.offset += 2
			closed := false
			for s.offset < len(s.src) {
				if s.src[s.offset] == '\n' {
					newline = true
				}
				if s.src[s.offset] == '*' && s.peekByte(1) == '/' {
					if isDoc {
						docs = append(docs, string(s.src[begin+1:s.offset]))
					}
					s.offset += 2
					closed = true
					break
				}
				s.offset++
			}
			if !closed {
				s.errorf(s.pos(begin), "unterminated block comment")
			}
		default:
			return
		}
	}
	return
}

func isIdentStart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (s *Scanner) scanIdent(start token.Pos, nl bool, docs []string) Token {
	for s.offset < len(s.src) {
		r, size := rune(s.src[s.offset]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRune(s.src[s.offset:])
		}
		if !isIdentPart(r) {
			break
		}
		s.offset += size
	}
	text := s.text(start)
	return Token{Kind: Ident, Text: text, Raw: text,
		Span:          token.Span{Start: start, End: s.pos(s.offset)},
		NewlineBefore: nl, Docs: docs}
}

func (s *Scanner) scanNumber(start token.Pos, nl bool, docs []string) Token {
	kind := Num
	if s.src[s.offset] == '0' && s.offset+1 < len(s.src) {
		switch s.src[s.offset+1] {
		case 'x', 'X', 'b', 'B', 'o', 'O':
			s.offset += 2
			for s.offset < len(s.src) && (isHex(s.src[s.offset]) || s.src[s.offset] == '_') {
				s.offset++
			}
			goto suffix
		}
	}
	for s.offset < len(s.src) && (isDigit(s.src[s.offset]) || s.src[s.offset] == '_') {
		s.offset++
	}
	if s.offset < len(s.src) && s.src[s.offset] == '.' {
		s.offset++
		for s.offset < len(s.src) && (isDigit(s.src[s.offset]) || s.src[s.offset] == '_') {
			s.offset++
		}
	}
	if s.offset < len(s.src) && (s.src[s.offset] == 'e' || s.src[s.offset] == 'E') {
		s.offset++
		if s.offset < len(s.src) && (s.src[s.offset] == '+' || s.src[s.offset] == '-') {
			s.offset++
		}
		for s.offset < len(s.src) && isDigit(s.src[s.offset]) {
			s.offset++
		}
	}
suffix:
	if s.offset < len(s.src) && s.src[s.offset] == 'n' {
		s.offset++
		kind = BigIntTok
	}
	raw := s.text(start)
	return Token{Kind: kind, Text: raw, Raw: raw,
		Span:          token.Span{Start: start, End: s.pos(s.offset)},
		NewlineBefore: nl, Docs: docs}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func (s *Scanner) scanString(start token.Pos, nl bool, docs []string) Token {
	quote := s.src[s.offset]
	s.offset++
	var b strings.Builder
	for {
		if s.offset >= len(s.src) {
			s.errorf(start, "unterminated string literal")
			break
		}
		c := s.src[s.offset]
		if c == quote {
			s.offset++
			break
		}
		if c == '\n' {
			s.errorf(start, "unterminated string literal")
			break
		}
		if c == '\\' {
			s.offset++
			s.scanEscape(&b)
			continue
		}
		b.WriteByte(c)
		s.offset++
	}
	return Token{Kind: Str, Text: b.String(), Raw: s.text(start),
		Span:          token.Span{Start: start, End: s.pos(s.offset)},
		NewlineBefore: nl, Docs: docs}
}

// scanTemplatePart scans from after a backtick or closing brace to the next
// ${ or terminating backtick.
func (s *Scanner) scanTemplatePart(start token.Pos, head bool, nl bool, docs []string) Token {
	var b strings.Builder
	for {
		if s.offset >= len(s.src) {
			s.errorf(start, "unterminated template literal")
			break
		}
		c := s.src[s.offset]
		if c == '`' {
			s.offset++
			kind := TmplTail
			if head {
				kind = NoSubTmpl
			}
			return Token{Kind: kind, Text: b.String(), Raw: s.text(start),
				Span:          token.Span{Start: start, End: s.pos(s.offset)},
				NewlineBefore: nl, Docs: docs}
		}
		if c == '$' && s.peekByte(1) == '{' {
			s.offset += 2
			kind := TmplMiddle
			if head {
				kind = TmplHead
			}
			return Token{Kind: kind, Text: b.String(), Raw: s.text(start),
				Span:          token.Span{Start: start, End: s.pos(s.offset)},
				NewlineBefore: nl, Docs: docs}
		}
		if c == '\\' {
			s.offset++
			s.scanEscape(&b)
			continue
		}
		b.WriteByte(c)
		s.offset++
	}
	kind := TmplTail
	if head {
		kind = NoSubTmpl
	}
	return Token{Kind: kind, Text: b.String(), Raw: s.text(start),
		Span:          token.Span{Start: start, End: s.pos(s.offset)},
		NewlineBefore: nl, Docs: docs}
}

func (s *Scanner) scanEscape(b *strings.Builder) {
	if s.offset >= len(s.src) {
		return
	}
	c := s.src[s.offset]
	s.offset++
	switch c {
	case 'n':
		b.WriteByte('\n')
	case 'r':
		b.WriteByte('\r')
	case 't':
		b.WriteByte('\t')
	case 'b':
		b.WriteByte('\b')
	case 'f':
		b.WriteByte('\f')
	case 'v':
		b.WriteByte('\v')
	case '0':
		b.WriteByte(0)
	case 'x':
		b.WriteRune(s.hexRune(2))
	case 'u':
		if s.offset < len(s.src) && s.src[s.offset] == '{' {
			s.offset++
			n := 0
			for s.offset < len(s.src) && s.src[s.offset] != '}' {
				n = n*16 + hexVal(s.src[s.offset])
				s.offset++
			}
			if s.offset < len(s.src) {
				s.offset++ // closing brace
			}
			b.WriteRune(rune(n))
		} else {
			b.WriteRune(s.hexRune(4))
		}
	case '\n':
		// line continuation: contributes nothing
	default:
		b.WriteByte(c)
	}
}

func (s *Scanner) hexRune(n int) rune {
	v := 0
	for i := 0; i < n && s.offset < len(s.src); i++ {
		v = v*16 + hexVal(s.src[s.offset])
		s.offset++
	}
	return rune(v)
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

// punctuators is ordered longest-first for maximal munch.
var punctuators = []string{
	">>>=", "...", "===", "!==", "**=", "<<=", ">>=", ">>>", "&&=", "||=", "??=",
	"=>", "==", "!=", "<=", ">=", "&&", "||", "??", "?.", "++", "--", "+=", "-=",
	"*=", "/=", "%=", "&=", "|=", "^=", "**", "<<", ">>",
	"{", "}", "(", ")", "[", "]", ";", ",", "<", ">", "+", "-", "*", "/", "%",
	"&", "|", "^", "!", "~", "?", ":", "=", ".", "@", "#",
}

func (s *Scanner) scanPunct(start token.Pos, nl bool, docs []string) Token {
	rest := s.src[s.offset:]
	for _, p := range punctuators {
		if len(rest) >= len(p) && string(rest[:len(p)]) == p {
			s.offset += len(p)
			return Token{Kind: Punct, Text: p, Raw: p,
				Span:          token.Span{Start: start, End: s.pos(s.offset)},
				NewlineBefore: nl, Docs: docs}
		}
	}
	r, size := utf8.DecodeRune(rest)
	s.offset += size
	s.errorf(start, "unexpected character %q", r)
	return Token{Kind: Punct, Text: string(r),
		Span:          token.Span{Start: start, End: s.pos(s.offset)},
		NewlineBefore: nl, Docs: docs}
}
