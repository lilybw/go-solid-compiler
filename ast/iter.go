package ast

import "iter"

// Range-over-func traversal:
//
//	for prop := range ast.OfType[*ast.PropertySignature](file) {
//	}
//
// Every iterator honours early termination.

// Preorder yields every node depth first, parents before children.
func Preorder(root Node) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		var walk func(Node) bool
		walk = func(n Node) bool {
			if isNil(n) {
				return true
			}
			if !yield(n) {
				return false
			}
			for _, c := range Children(n) {
				if !walk(c) {
					return false
				}
			}
			return true
		}
		walk(root)
	}
}

// Postorder yields every node depth first, children before parents. This is
// the order for bottom-up rewriting.
func Postorder(root Node) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		var walk func(Node) bool
		walk = func(n Node) bool {
			if isNil(n) {
				return true
			}
			for _, c := range Children(n) {
				if !walk(c) {
					return false
				}
			}
			return yield(n)
		}
		walk(root)
	}
}

// WithParent yields each node with its parent; the root has a nil parent.
func WithParent(root Node) iter.Seq2[Node, Node] {
	return func(yield func(node, parent Node) bool) {
		var walk func(n, parent Node) bool
		walk = func(n, parent Node) bool {
			if isNil(n) {
				return true
			}
			if !yield(n, parent) {
				return false
			}
			for _, c := range Children(n) {
				if !walk(c, n) {
					return false
				}
			}
			return true
		}
		walk(root, nil)
	}
}

// OfType yields every node in the tree with dynamic type T.
func OfType[T Node](root Node) iter.Seq[T] {
	return func(yield func(T) bool) {
		for n := range Preorder(root) {
			if t, ok := n.(T); ok {
				if !yield(t) {
					return
				}
			}
		}
	}
}

// Filter yields the nodes of seq satisfying pred.
func Filter[T Node](seq iter.Seq[T], pred func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for n := range seq {
			if pred(n) && !yield(n) {
				return
			}
		}
	}
}

// Collect drains seq into a slice.
func Collect[T Node](seq iter.Seq[T]) []T {
	var out []T
	for n := range seq {
		out = append(out, n)
	}
	return out
}

// ChildSeq yields the direct children of n.
func ChildSeq(n Node) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		for _, c := range Children(n) {
			if !yield(c) {
				return
			}
		}
	}
}
