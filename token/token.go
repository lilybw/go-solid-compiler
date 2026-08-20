// Package token defines source positions for the TypeScript AST.
//
// Synthesized nodes carry [NoPos], so a tree may mix parsed and generated
// subtrees freely.
package token

import "fmt"

// Pos is a 1-based byte offset into a source file, so that the zero value
// can mean "synthesized".
type Pos int

// NoPos is the zero Pos, reported by every synthesized node.
const NoPos Pos = 0

// IsValid reports whether p refers to a real source location.
func (p Pos) IsValid() bool { return p != NoPos }

// Span is a half-open byte range [Start, End).
type Span struct {
	Start Pos
	End   Pos
}

// NoSpan is the span of a synthesized node.
var NoSpan = Span{}

// Between returns the span covering both a and b, ignoring invalid endpoints.
func Between(a, b Span) Span {
	s := a
	if !s.Start.IsValid() || (b.Start.IsValid() && b.Start < s.Start) {
		s.Start = b.Start
	}
	if b.End > s.End {
		s.End = b.End
	}
	return s
}

// IsValid reports whether the span refers to real source text.
func (s Span) IsValid() bool { return s.Start.IsValid() }

func (s Span) String() string { return fmt.Sprintf("%d:%d", s.Start, s.End) }

// File maps byte offsets to line and column pairs. It is safe for concurrent
// reads once built.
type File struct {
	Name  string
	Size  int
	lines []Pos // offset of the first byte of each line; lines[0] == 0
}

// NewFile records line offsets for src and returns the resulting File.
func NewFile(name string, src []byte) *File {
	f := &File{Name: name, Size: len(src), lines: []Pos{0}}
	for i, b := range src {
		if b == '\n' {
			f.lines = append(f.lines, Pos(i+1))
		}
	}
	return f
}

// Location is a human-readable source position. Line and Column are 1-based,
// and Column counts bytes.
type Location struct {
	File   string
	Line   int
	Column int
}

func (l Location) String() string {
	if l.File == "" {
		return fmt.Sprintf("%d:%d", l.Line, l.Column)
	}
	return fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Column)
}

// Position converts a byte offset to a [Location].
func (f *File) Position(p Pos) Location {
	if f == nil {
		return Location{Line: 0, Column: 0}
	}
	off := Pos(int(p) - 1)
	if off < 0 {
		off = 0
	}
	// Binary search for the greatest line start <= off.
	lo, hi := 0, len(f.lines)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if f.lines[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return Location{File: f.Name, Line: lo + 1, Column: int(off-f.lines[lo]) + 1}
}
