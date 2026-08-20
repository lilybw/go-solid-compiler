package ast

// Traversal.
//
// Children is the single source of truth: Inspect, Walk, and the iterator
// helpers in iter.go are all defined in terms of it. Adding a node type
// therefore requires updating exactly one switch.

// Children returns the direct child nodes of n in source order, omitting
// nils. Nodes carrying verbatim text report no children.
func Children(n Node) []Node {
	if n == nil {
		return nil
	}
	var c childCollector
	switch x := n.(type) {

	// --- names -------------------------------------------------------------
	case *Ident:
	case *QualifiedName:
		c.add(x.Left, x.Right)
	case *ComputedName:
		c.add(x.Expr)

	// --- types -------------------------------------------------------------
	case *KeywordType, *LiteralType, *ThisType, *RawType:
	case *TypeRef:
		c.add(x.Name)
		c.addTypes(x.Args)
	case *ArrayType:
		c.add(x.Elem)
	case *TupleType:
		c.addTypes(x.Elems)
	case *OptionalType:
		c.add(x.Elem)
	case *RestType:
		c.add(x.Elem)
	case *NamedTupleMember:
		c.add(x.Name, x.Type)
	case *UnionType:
		c.addTypes(x.Types)
	case *IntersectionType:
		c.addTypes(x.Types)
	case *ParenType:
		c.add(x.Inner)
	case *TypeLiteral:
		c.addMembers(x.Members)
	case *FunctionType:
		c.addSignature(x.Signature)
	case *ConstructorType:
		c.addSignature(x.Signature)
	case *IndexedAccessType:
		c.add(x.Object, x.Index)
	case *MappedType:
		c.add(x.Param, x.As, x.Type)
	case *ConditionalType:
		c.add(x.Check, x.Extends, x.True, x.False)
	case *InferType:
		c.add(x.Param)
	case *TypeOperator:
		c.add(x.Type)
	case *TypeQuery:
		c.add(x.Name)
		c.addTypes(x.Args)
	case *TemplateLiteralType:
		c.addTypes(x.Types)
	case *ImportType:
		c.add(x.Qualifier)
		c.addTypes(x.Args)
	case *PredicateType:
		c.add(x.ParamName, x.Type)

	// --- structural --------------------------------------------------------
	case *TypeParam:
		c.add(x.Name, x.Constraint, x.Default)
	case *Param:
		c.add(x.Name, x.Type, x.Default)
	case *Heritage:
		c.add(x.Name)
		c.addTypes(x.Args)
	case *Binding:
		c.add(x.Name, x.Type, x.Value)
	case *EnumMember:
		c.add(x.Name, x.Value)

	// --- members -----------------------------------------------------------
	case *PropertySignature:
		c.add(x.Name, x.Type)
	case *MethodSignature:
		c.add(x.Name)
		c.addSignature(x.Signature)
	case *IndexSignature:
		c.add(x.KeyType, x.Type)
	case *CallSignature:
		c.addSignature(x.Signature)
	case *ConstructSignature:
		c.addSignature(x.Signature)
	case *GetAccessor:
		c.add(x.Name)
		c.addSignature(x.Signature)
		c.add(x.Body)
	case *SetAccessor:
		c.add(x.Name)
		c.addSignature(x.Signature)
		c.add(x.Body)
	case *PropertyDecl:
		c.add(x.Name, x.Type, x.Value)
	case *MethodDecl:
		c.add(x.Name)
		c.addSignature(x.Signature)
		c.add(x.Body)

	// --- declarations ------------------------------------------------------
	case *InterfaceDecl:
		c.add(x.Name)
		c.addTypeParams(x.TypeParams)
		for _, h := range x.Extends {
			c.add(h)
		}
		c.addMembers(x.Members)
	case *TypeAliasDecl:
		c.add(x.Name)
		c.addTypeParams(x.TypeParams)
		c.add(x.Type)
	case *EnumDecl:
		c.add(x.Name)
		for _, m := range x.Members {
			c.add(m)
		}
	case *ClassDecl:
		c.add(x.Name)
		c.addTypeParams(x.TypeParams)
		c.add(x.Extends)
		for _, h := range x.Implements {
			c.add(h)
		}
		c.addMembers(x.Members)
	case *FunctionDecl:
		c.add(x.Name)
		c.addSignature(x.Signature)
		c.add(x.Body)
	case *VarDecl:
		for _, b := range x.Bindings {
			c.add(b)
		}
	case *ModuleDecl:
		c.addStmts(x.Body)
	case *ImportDecl:
	case *ExportDecl:
		c.add(x.Decl)
	case *ExportAssign:
		c.add(x.Expr)

	// --- statements --------------------------------------------------------
	case *BlockStmt:
		c.addStmts(x.Stmts)
	case *ReturnStmt:
		c.add(x.Value)
	case *ExprStmt:
		c.add(x.Expr)
	case *RawStmt:

	// --- expressions -------------------------------------------------------
	case *StringLit, *NumberLit, *BigIntLit, *BoolLit, *NullLit, *UndefinedLit, *RawExpr:
	case *ArrayLit:
		c.addExprs(x.Elems)
	case *ObjectLit:
		for _, p := range x.Props {
			c.add(p.Name, p.Value)
		}
	case *CallExpr:
		c.add(x.Callee)
		c.addTypes(x.TypeArgs)
		c.addExprs(x.Args)
	case *NewExpr:
		c.add(x.Callee)
		c.addTypes(x.TypeArgs)
		c.addExprs(x.Args)
	case *MemberExpr:
		c.add(x.Object, x.Prop)
	case *IndexExpr:
		c.add(x.Object, x.Index)
	case *ArrowFunc:
		c.addSignature(x.Signature)
		c.add(x.Body, x.Expr)
	case *AsExpr:
		c.add(x.Expr, x.Type)
	case *SatisfiesExpr:
		c.add(x.Expr, x.Type)
	case *NonNullExpr:
		c.add(x.Expr)
	case *TemplateLit:
		c.add(x.Tag)
		c.addExprs(x.Exprs)
	case *UnaryExpr:
		c.add(x.Expr)
	case *BinaryExpr:
		c.add(x.Left, x.Right)
	case *CondExpr:
		c.add(x.Cond, x.Then, x.Else)
	case *SpreadExpr:
		c.add(x.Expr)

	// --- files -------------------------------------------------------------
	case *SourceFile:
		c.addStmts(x.Stmts)
	}
	return c.out
}

// childCollector accumulates non-nil children, filtering typed nil pointers
// stored in non-nil interfaces.
type childCollector struct{ out []Node }

func (c *childCollector) add(ns ...Node) {
	for _, n := range ns {
		if !isNil(n) {
			c.out = append(c.out, n)
		}
	}
}

func (c *childCollector) addTypes(ts []Type) {
	for _, t := range ts {
		if !isNil(t) {
			c.out = append(c.out, t)
		}
	}
}

func (c *childCollector) addExprs(es []Expr) {
	for _, e := range es {
		if !isNil(e) {
			c.out = append(c.out, e)
		}
	}
}

func (c *childCollector) addStmts(ss []Stmt) {
	for _, s := range ss {
		if !isNil(s) {
			c.out = append(c.out, s)
		}
	}
}

func (c *childCollector) addMembers(ms []Member) {
	for _, m := range ms {
		if !isNil(m) {
			c.out = append(c.out, m)
		}
	}
}

func (c *childCollector) addTypeParams(ps []*TypeParam) {
	for _, p := range ps {
		if p != nil {
			c.out = append(c.out, p)
		}
	}
}

func (c *childCollector) addSignature(s Signature) {
	c.addTypeParams(s.TypeParams)
	for _, p := range s.Params {
		if p != nil {
			c.out = append(c.out, p)
		}
	}
	c.add(s.Return)
}

// isNil reports whether n is nil, including a typed nil pointer held in a
// non-nil interface.
func isNil(n Node) bool {
	if n == nil {
		return true
	}
	switch x := n.(type) {
	case *Ident:
		return x == nil
	case *BlockStmt:
		return x == nil
	case *TypeParam:
		return x == nil
	case *Heritage:
		return x == nil
	}
	return false
}

// ---------------------------------------------------------------------------
// Visiting
// ---------------------------------------------------------------------------

// Inspect walks the tree in depth-first preorder, calling f for each node.
// Returning false skips that node's children.
func Inspect(n Node, f func(Node) bool) {
	if isNil(n) || !f(n) {
		return
	}
	for _, c := range Children(n) {
		Inspect(c, f)
	}
}

// Visitor is called for each node; a non-nil result visits that node's
// children, allowing a scoped visitor to take over.
type Visitor interface {
	Visit(Node) Visitor
}

// Walk traverses the tree rooted at n, driving v.
func Walk(v Visitor, n Node) {
	if isNil(n) {
		return
	}
	next := v.Visit(n)
	if next == nil {
		return
	}
	for _, c := range Children(n) {
		Walk(next, c)
	}
}

// VisitorFunc adapts a function to the Visitor interface. The function is
// re-used for children when it returns true.
type VisitorFunc func(Node) bool

func (f VisitorFunc) Visit(n Node) Visitor {
	if f(n) {
		return f
	}
	return nil
}

// Find returns the first node in preorder for which pred is true, or nil.
func Find(n Node, pred func(Node) bool) Node {
	var found Node
	Inspect(n, func(x Node) bool {
		if found != nil {
			return false
		}
		if pred(x) {
			found = x
			return false
		}
		return true
	})
	return found
}
