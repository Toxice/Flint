// Package checker performs static analysis on a Flint program.
//
// It runs in two passes. Pass 1 registers every class, interface, and function
// and resolves their types, so a declaration may refer to another declared
// later in the file. Pass 2 walks bodies, records the static type of every
// expression, and checks conformance.
//
// The checker collects every error it finds rather than stopping at the first,
// and the Info it returns is what lets the interpreter run without re-deriving
// any of this.
package checker

import (
	"fmt"

	"flint/internal/ast"
	"flint/internal/token"
	"flint/internal/types"
)

// CallKind says what a call expression resolves to.
type CallKind int

// The kinds of call a Flint program can contain.
const (
	CallFunc    CallKind = iota // a top-level function
	CallMethod                  // a method on an object
	CallCtor                    // a constructor, which allocates
	CallBuiltin                 // print or len
)

// CallTarget records what a call resolves to, so the interpreter does not
// repeat the checker's name resolution.
type CallTarget struct {
	Kind    CallKind
	Func    *ast.FuncDecl // CallFunc
	Method  string        // CallMethod: the method name to dispatch
	Recv    types.Type    // CallMethod: the receiver's static type
	Class   *types.Class  // CallCtor
	Builtin string        // CallBuiltin
	Sig     *types.Signature
}

// Info is everything the interpreter needs from the checker.
type Info struct {
	Classes    map[string]*types.Class
	Interfaces map[string]*types.Interface
	Funcs      map[string]*types.Signature
	FuncDecls  map[string]*ast.FuncDecl

	// MethodBody and CtorBody give the interpreter the code behind a resolved
	// method or constructor.
	MethodBody map[*types.Method]*ast.FuncDecl
	CtorBody   map[*types.Class]*ast.FuncDecl

	// ExprType holds the static type of every expression in the program.
	ExprType map[ast.Expr]types.Type
	// Resolved holds the type each source type annotation denotes.
	Resolved map[ast.TypeExpr]types.Type
	// Calls records what each call expression resolves to.
	Calls map[*ast.Call]*CallTarget
	// FieldOwner records the class that declares each accessed field, so the
	// interpreter can read shadowed fields correctly.
	FieldOwner map[*ast.Field]*types.Field
}

type checker struct {
	file string
	info *Info
	errs []error

	// Per-function context.
	class  *types.Class // the enclosing class, or nil at top level
	result types.Type   // the enclosing function's return type
	scope  *scope
}

type scope struct {
	parent *scope
	vars   map[string]types.Type
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, vars: map[string]types.Type{}}
}

func (s *scope) lookup(name string) (types.Type, bool) {
	for k := s; k != nil; k = k.parent {
		if t, ok := k.vars[name]; ok {
			return t, true
		}
	}
	return nil, false
}

// declaredHere reports whether name is bound in this scope, ignoring parents.
// Shadowing an outer binding is legal; redeclaring in the same scope is not.
func (s *scope) declaredHere(name string) bool {
	_, ok := s.vars[name]
	return ok
}

// Check analyses prog and returns the information the interpreter needs plus
// every error found.
func Check(prog *ast.Program, filename string) (*Info, []error) {
	c := &checker{
		file: filename,
		info: &Info{
			Classes:    map[string]*types.Class{},
			Interfaces: map[string]*types.Interface{},
			Funcs:      map[string]*types.Signature{},
			FuncDecls:  map[string]*ast.FuncDecl{},
			MethodBody: map[*types.Method]*ast.FuncDecl{},
			CtorBody:   map[*types.Class]*ast.FuncDecl{},
			ExprType:   map[ast.Expr]types.Type{},
			Resolved:   map[ast.TypeExpr]types.Type{},
			Calls:      map[*ast.Call]*CallTarget{},
			FieldOwner: map[*ast.Field]*types.Field{},
		},
	}
	c.registerBuiltins()
	c.collect(prog)
	// Pass 2 is only meaningful once every name resolves; a program with an
	// unknown base class would otherwise produce a cascade of noise.
	if len(c.errs) == 0 {
		c.checkBodies(prog)
	}
	return c.info, c.errs
}

func (c *checker) errorf(pos token.Pos, format string, args ...any) {
	c.errs = append(c.errs, fmt.Errorf("%s:%d:%d: %s", c.file, pos.Line, pos.Col,
		fmt.Sprintf(format, args...)))
}

// ---------- built-ins ----------

// BuiltinExceptions names the exception classes the runtime itself throws.
var BuiltinExceptions = []string{"ArithmeticError", "IndexError", "NullError"}

// registerBuiltins installs the Exception hierarchy. The spec's `e.message`
// requires an Exception class with a message field, but never declares one;
// the runtime error classes let a Flint program catch a division by zero or a
// bad index instead of the host crashing.
func (c *checker) registerBuiltins() {
	exc := types.NewClass("Exception")
	exc.Fields["message"] = &types.Field{Name: "message", Type: types.String, Owner: exc}
	exc.FieldOrder = []string{"message"}
	exc.Ctor = &types.Signature{Params: []types.Type{types.String}, ParamNames: []string{"message"}}
	c.info.Classes["Exception"] = exc

	for _, name := range BuiltinExceptions {
		sub := types.NewClass(name)
		sub.Super = exc
		c.info.Classes[name] = sub
	}
}

// ---------- pass 1: declaration collection ----------

func (c *checker) collect(prog *ast.Program) {
	classDecls := map[string]*ast.ClassDecl{}
	ifaceDecls := map[string]*ast.InterfaceDecl{}

	// 1a. Register every name first, so declarations may refer forward.
	for _, d := range prog.Decls {
		switch d := d.(type) {
		case *ast.ClassDecl:
			if c.nameTaken(d.Name) {
				c.errorf(d.Pos(), "%s is already declared", d.Name)
				continue
			}
			cl := types.NewClass(d.Name)
			cl.Abstract = d.Abstract
			c.info.Classes[d.Name] = cl
			classDecls[d.Name] = d

		case *ast.InterfaceDecl:
			if c.nameTaken(d.Name) {
				c.errorf(d.Pos(), "%s is already declared", d.Name)
				continue
			}
			c.info.Interfaces[d.Name] = types.NewInterface(d.Name)
			ifaceDecls[d.Name] = d

		case *ast.FuncDecl:
			if _, dup := c.info.Funcs[d.Name]; dup {
				c.errorf(d.Pos(), "function %s is already declared", d.Name)
				continue
			}
			c.info.Funcs[d.Name] = nil // reserve the name; filled in below
			c.info.FuncDecls[d.Name] = d
		}
	}

	// 1b. Resolve base lists, which need every class and interface name known.
	for name, d := range classDecls {
		cl := c.info.Classes[name]
		for _, base := range d.Bases {
			switch {
			case c.info.Classes[base] != nil:
				if cl.Super != nil {
					c.errorf(d.Pos(), "class %s lists more than one superclass; Flint has single inheritance", name)
					continue
				}
				cl.Super = c.info.Classes[base]
			case c.info.Interfaces[base] != nil:
				cl.Interfaces = append(cl.Interfaces, c.info.Interfaces[base])
			default:
				c.errorf(d.Pos(), "unknown type %s in the bases of class %s", base, name)
			}
		}
	}

	// 1c. Break inheritance cycles before anything walks a Super chain.
	c.checkInheritanceCycles(classDecls)

	// 1d. Resolve interface method signatures.
	for name, d := range ifaceDecls {
		iface := c.info.Interfaces[name]
		for _, m := range d.Methods {
			if _, dup := iface.Methods[m.Name]; dup {
				c.errorf(m.Pos(), "interface %s declares method %s more than once", name, m.Name)
				continue
			}
			iface.Methods[m.Name] = &types.Method{
				Name: m.Name, Sig: c.signature(m), Abstract: true,
			}
		}
	}

	// 1e. Resolve class members.
	for name, d := range classDecls {
		c.collectClassMembers(c.info.Classes[name], d)
	}

	// 1f. Resolve top-level function signatures.
	for name, d := range c.info.FuncDecls {
		c.info.Funcs[name] = c.signature(d)
	}
}

func (c *checker) nameTaken(name string) bool {
	if _, ok := types.Primitives[name]; ok {
		return true
	}
	_, isClass := c.info.Classes[name]
	_, isIface := c.info.Interfaces[name]
	return isClass || isIface
}

// checkInheritanceCycles severs any Super chain that loops, so later passes can
// walk ancestors without spinning forever.
func (c *checker) checkInheritanceCycles(decls map[string]*ast.ClassDecl) {
	for name, d := range decls {
		cl := c.info.Classes[name]
		seen := map[*types.Class]bool{cl: true}
		for k := cl.Super; k != nil; k = k.Super {
			if seen[k] {
				c.errorf(d.Pos(), "inheritance cycle involving class %s", name)
				cl.Super = nil // sever it so nothing loops later
				break
			}
			seen[k] = true
		}
	}
}

func (c *checker) collectClassMembers(cl *types.Class, d *ast.ClassDecl) {
	for _, f := range d.Fields {
		if _, dup := cl.Fields[f.Name]; dup {
			c.errorf(f.Pos(), "class %s declares field %s more than once", cl.Name, f.Name)
			continue
		}
		cl.Fields[f.Name] = &types.Field{
			Name: f.Name, Type: c.resolveType(f.Type), Private: f.Private, Owner: cl,
		}
		cl.FieldOrder = append(cl.FieldOrder, f.Name)
	}

	for _, m := range d.Methods {
		if _, dup := cl.Methods[m.Name]; dup {
			c.errorf(m.Pos(), "class %s declares method %s more than once", cl.Name, m.Name)
			continue
		}
		if m.Abstract && !d.Abstract {
			c.errorf(m.Pos(), "class %s is not abstract, so it cannot declare abstract func %s", cl.Name, m.Name)
		}
		method := &types.Method{
			Name: m.Name, Sig: c.signature(m), Private: m.Private,
			Abstract: m.Abstract, Owner: cl,
		}
		cl.Methods[m.Name] = method
		if m.Body != nil {
			c.info.MethodBody[method] = m
		}
	}

	if d.Ctor != nil {
		cl.Ctor = c.signature(d.Ctor)
		c.info.CtorBody[cl] = d.Ctor
	}
}

// signature resolves a function declaration's parameter and result types.
func (c *checker) signature(d *ast.FuncDecl) *types.Signature {
	sig := &types.Signature{}
	for _, p := range d.Params {
		sig.Params = append(sig.Params, c.resolveType(p.Type))
		sig.ParamNames = append(sig.ParamNames, p.Name)
	}
	if d.Result != nil {
		sig.Result = c.resolveType(d.Result)
	}
	return sig
}

// resolveType turns a source type annotation into a types.Type, reporting and
// returning a placeholder when the name is unknown.
func (c *checker) resolveType(te ast.TypeExpr) types.Type {
	if t, ok := c.info.Resolved[te]; ok {
		return t
	}
	var t types.Type
	switch te := te.(type) {
	case *ast.NamedType:
		switch {
		case types.Primitives[te.Name] != nil:
			t = types.Primitives[te.Name]
		case c.info.Classes[te.Name] != nil:
			t = c.info.Classes[te.Name]
		case c.info.Interfaces[te.Name] != nil:
			t = c.info.Interfaces[te.Name]
		default:
			c.errorf(te.Pos(), "unknown type %s", te.Name)
			t = types.Void
		}
	case *ast.ListType:
		t = &types.List{Elem: c.resolveType(te.Elem)}
	case *ast.MapType:
		key := c.resolveType(te.Key)
		if types.IsReference(key) {
			c.errorf(te.Pos(), "map key type %s is not allowed; keys must be Int, Float, Bool, String, or Char", key)
		}
		t = &types.Map{Key: key, Val: c.resolveType(te.Val)}
	default:
		t = types.Void
	}
	c.info.Resolved[te] = t
	return t
}

// ---------- pass 2: bodies and conformance ----------

func (c *checker) checkBodies(prog *ast.Program) {
	for _, d := range prog.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			c.checkFunc(d, nil)
		case *ast.ClassDecl:
			cl := c.info.Classes[d.Name]
			c.checkConformance(cl, d)
			if d.Ctor != nil {
				c.checkFunc(d.Ctor, cl)
			}
			for _, m := range d.Methods {
				if m.Body != nil {
					c.checkFunc(m, cl)
				}
			}
		}
	}

	main, ok := c.info.FuncDecls["main"]
	if !ok {
		c.errorf(prog.Pos(), "no main function; a Flint program needs `func main(): Void`")
		return
	}
	sig := c.info.Funcs["main"]
	if len(sig.Params) != 0 || sig.Result != types.Void {
		c.errorf(main.Pos(), "main must be declared `func main(): Void`, found %s", sig)
	}
}

// checkConformance verifies that a concrete class implements every method its
// interfaces and abstract ancestors require, and that overrides match.
func (c *checker) checkConformance(cl *types.Class, d *ast.ClassDecl) {
	required := map[string]*types.Method{}
	for _, iface := range cl.AllInterfaces() {
		for name, m := range iface.Methods {
			required[name] = m
		}
	}
	for k := cl; k != nil; k = k.Super {
		for name, m := range k.Methods {
			if m.Abstract {
				required[name] = m
			}
		}
	}

	for name, want := range required {
		got := cl.LookupMethod(name)
		if got == nil || got.Abstract {
			// An abstract class is allowed to leave these open for a subclass.
			if !cl.Abstract {
				c.errorf(d.Pos(), "class %s does not implement %s%s", cl.Name, name, want.Sig)
			}
			continue
		}
		if !sameSignature(got.Sig, want.Sig) {
			c.errorf(d.Pos(), "class %s implements %s%s, but %s%s is required",
				cl.Name, name, got.Sig, name, want.Sig)
		}
	}

	// An override must keep its superclass's signature.
	if cl.Super != nil {
		for name, m := range cl.Methods {
			if parent := cl.Super.LookupMethod(name); parent != nil {
				if !sameSignature(m.Sig, parent.Sig) {
					c.errorf(d.Pos(), "class %s overrides %s%s, but %s declares %s%s",
						cl.Name, name, m.Sig, cl.Super.Name, name, parent.Sig)
				}
			}
		}
	}
}

func sameSignature(a, b *types.Signature) bool {
	if len(a.Params) != len(b.Params) {
		return false
	}
	for i := range a.Params {
		if !types.Identical(a.Params[i], b.Params[i]) {
			return false
		}
	}
	if a.Result == nil || b.Result == nil {
		return a.Result == b.Result
	}
	return types.Identical(a.Result, b.Result)
}

func (c *checker) checkFunc(d *ast.FuncDecl, cl *types.Class) {
	prevClass, prevResult, prevScope := c.class, c.result, c.scope
	defer func() { c.class, c.result, c.scope = prevClass, prevResult, prevScope }()

	c.class = cl
	c.scope = newScope(nil)

	sig := c.signature(d)
	c.result = sig.Result
	if d.IsCtor {
		c.result = types.Void
	}

	for i, p := range d.Params {
		if c.scope.declaredHere(p.Name) {
			c.errorf(p.Pos(), "parameter %s is declared more than once", p.Name)
		}
		c.scope.vars[p.Name] = sig.Params[i]
	}

	if d.Body == nil {
		return
	}
	c.checkBlock(d.Body, true)

	if c.result != nil && c.result != types.Void && !alwaysReturns(d.Body) {
		c.errorf(d.Pos(), "function %s must return %s on every path", d.Name, c.result)
	}
}

// alwaysReturns reports whether every path through s ends in a return or a
// throw. A while loop never counts: its condition may be false on entry.
func alwaysReturns(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.Return, *ast.Throw:
		return true
	case *ast.Block:
		for _, st := range s.Stmts {
			if alwaysReturns(st) {
				return true
			}
		}
		return false
	case *ast.If:
		if s.Else == nil {
			return false
		}
		return alwaysReturns(s.Then) && alwaysReturns(s.Else)
	case *ast.Try:
		if !alwaysReturns(s.Body) {
			return false
		}
		for _, cl := range s.Catches {
			if !alwaysReturns(cl.Body) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *checker) checkBlock(b *ast.Block, reuseScope bool) {
	if !reuseScope {
		prev := c.scope
		c.scope = newScope(prev)
		defer func() { c.scope = prev }()
	}
	for _, s := range b.Stmts {
		c.checkStmt(s)
	}
}

func (c *checker) checkStmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.Let:
		declared := c.resolveType(s.Type)
		if declared == types.Void {
			c.errorf(s.Pos(), "variable %s cannot have type Void", s.Name)
		}
		got := c.checkExprExpecting(s.Value, declared)
		if got != nil && !types.Assignable(declared, got) {
			c.errorf(s.Value.Pos(), "cannot use %s as %s in the declaration of %s", got, declared, s.Name)
		}
		if c.scope.declaredHere(s.Name) {
			c.errorf(s.Pos(), "%s is already declared in this scope", s.Name)
		}
		c.scope.vars[s.Name] = declared

	case *ast.Assign:
		target := c.checkAssignTarget(s.Target)
		got := c.checkExprExpecting(s.Value, target)
		if target != nil && got != nil && !types.Assignable(target, got) {
			c.errorf(s.Value.Pos(), "cannot assign %s to %s", got, target)
		}

	case *ast.ExprStmt:
		c.checkExpr(s.X)

	case *ast.Block:
		c.checkBlock(s, false)

	case *ast.If:
		c.requireBool(s.Cond, "if condition")
		c.checkBlock(s.Then, false)
		if s.Else != nil {
			c.checkStmt(s.Else)
		}

	case *ast.While:
		c.requireBool(s.Cond, "while condition")
		c.checkBlock(s.Body, false)

	case *ast.For:
		c.checkFor(s)

	case *ast.Return:
		c.checkReturn(s)

	case *ast.Throw:
		got := c.checkExpr(s.Value)
		if got != nil && !c.isExceptionType(got) {
			c.errorf(s.Value.Pos(), "can only throw an Exception or a subclass of it, found %s", got)
		}

	case *ast.Try:
		c.checkBlock(s.Body, false)
		for _, cl := range s.Catches {
			c.checkCatch(cl)
		}
	}
}

func (c *checker) checkFor(s *ast.For) {
	declared := c.resolveType(s.Type)
	iter := c.checkExpr(s.Iter)

	var elem types.Type
	switch it := iter.(type) {
	case *types.List:
		elem = it.Elem
	case nil:
		// The iterable already failed to type-check; do not pile on.
	default:
		c.errorf(s.Iter.Pos(), "cannot iterate over %s; for-in needs a list", iter)
	}

	if elem != nil && !types.Assignable(declared, elem) {
		c.errorf(s.Pos(), "loop variable %s is declared %s, but the list holds %s", s.Var, declared, elem)
	}

	prev := c.scope
	c.scope = newScope(prev)
	c.scope.vars[s.Var] = declared
	c.checkBlock(s.Body, true)
	c.scope = prev
}

func (c *checker) checkReturn(s *ast.Return) {
	if s.Value == nil {
		if c.result != nil && c.result != types.Void {
			c.errorf(s.Pos(), "this function must return %s", c.result)
		}
		return
	}
	got := c.checkExprExpecting(s.Value, c.result)
	if c.result == types.Void {
		c.errorf(s.Pos(), "cannot return a value from a Void function")
		return
	}
	if got != nil && c.result != nil && !types.Assignable(c.result, got) {
		c.errorf(s.Value.Pos(), "cannot return %s from a function declared to return %s", got, c.result)
	}
}

func (c *checker) checkCatch(cl *ast.CatchClause) {
	caught := c.resolveType(cl.Type)
	if !c.isExceptionType(caught) {
		c.errorf(cl.Type.Pos(), "catch type must be Exception or a subclass of it, found %s", caught)
	}
	prev := c.scope
	c.scope = newScope(prev)
	c.scope.vars[cl.Name] = caught
	c.checkBlock(cl.Body, true)
	c.scope = prev
}

func (c *checker) isExceptionType(t types.Type) bool {
	cl, ok := t.(*types.Class)
	return ok && types.IsSubclass(cl, c.info.Classes["Exception"])
}

func (c *checker) requireBool(e ast.Expr, what string) {
	got := c.checkExpr(e)
	if got != nil && got != types.Bool {
		c.errorf(e.Pos(), "%s must be Bool, found %s", what, got)
	}
}

// checkAssignTarget types the left side of an assignment.
func (c *checker) checkAssignTarget(e ast.Expr) types.Type {
	switch e := e.(type) {
	case *ast.Ident:
		t, ok := c.scope.lookup(e.Name)
		if !ok {
			c.errorf(e.Pos(), "undefined variable %s", e.Name)
			return nil
		}
		c.info.ExprType[e] = t
		return t
	case *ast.Field, *ast.Index:
		return c.checkExpr(e)
	}
	c.errorf(e.Pos(), "cannot assign to this expression")
	return nil
}

// ---------- expressions ----------

// checkExpr types e, records the result in Info.ExprType, and returns it. It
// returns nil when the expression could not be typed, and callers must treat a
// nil as "already reported" rather than reporting again.
func (c *checker) checkExpr(e ast.Expr) types.Type {
	t := c.exprType(e)
	if t != nil {
		c.info.ExprType[e] = t
	}
	return t
}

// checkExprExpecting types e in a context that wants type want.
//
// This matters only for collection literals. Lists and maps are invariant, so
// inferring [BaseShape] from [Rectangle(...), Circle(...)] and only then
// comparing it against the declared [Shape] would reject the spec's own sample
// program. Typing each element against the expected element type instead is
// both correct and what makes an empty [] literal usable.
func (c *checker) checkExprExpecting(e ast.Expr, want types.Type) types.Type {
	switch e := e.(type) {
	case *ast.ListLit:
		w, ok := want.(*types.List)
		if !ok {
			break
		}
		for _, el := range e.Elems {
			got := c.checkExprExpecting(el, w.Elem)
			if got != nil && !types.Assignable(w.Elem, got) {
				c.errorf(el.Pos(), "cannot use %s as %s in a %s literal", got, w.Elem, want)
			}
		}
		c.info.ExprType[e] = want
		return want

	case *ast.MapLit:
		w, ok := want.(*types.Map)
		if !ok {
			break
		}
		for i := range e.Keys {
			if got := c.checkExprExpecting(e.Keys[i], w.Key); got != nil && !types.Assignable(w.Key, got) {
				c.errorf(e.Keys[i].Pos(), "cannot use %s as the key type %s in a %s literal", got, w.Key, want)
			}
			if got := c.checkExprExpecting(e.Vals[i], w.Val); got != nil && !types.Assignable(w.Val, got) {
				c.errorf(e.Vals[i].Pos(), "cannot use %s as the value type %s in a %s literal", got, w.Val, want)
			}
		}
		c.info.ExprType[e] = want
		return want
	}
	return c.checkExpr(e)
}

func (c *checker) exprType(e ast.Expr) types.Type {
	switch e := e.(type) {
	case *ast.IntLit:
		return types.Int
	case *ast.FloatLit:
		return types.Float
	case *ast.StringLit:
		return types.String
	case *ast.CharLit:
		return types.Char
	case *ast.BoolLit:
		return types.Bool

	case *ast.ObjectExpr:
		if c.class == nil {
			c.errorf(e.Pos(), "object is only valid inside a class body")
			return nil
		}
		return c.class

	case *ast.Ident:
		return c.identType(e)

	case *ast.Unary:
		return c.unaryType(e)

	case *ast.Binary:
		return c.binaryType(e)

	case *ast.Field:
		return c.fieldType(e)

	case *ast.Index:
		return c.indexType(e)

	case *ast.Call:
		return c.callType(e)

	case *ast.ListLit:
		return c.listLitType(e)

	case *ast.MapLit:
		return c.mapLitType(e)
	}
	c.errorf(e.Pos(), "unsupported expression")
	return nil
}

func (c *checker) identType(e *ast.Ident) types.Type {
	if t, ok := c.scope.lookup(e.Name); ok {
		return t
	}
	// A class name used as a value is almost always a forgotten call.
	if _, ok := c.info.Classes[e.Name]; ok {
		c.errorf(e.Pos(), "%s is a class; write %s(...) to construct one", e.Name, e.Name)
		return nil
	}
	if _, ok := c.info.Funcs[e.Name]; ok {
		c.errorf(e.Pos(), "%s is a function; Flint has no function values, call it with %s(...)", e.Name, e.Name)
		return nil
	}
	c.errorf(e.Pos(), "undefined variable %s", e.Name)
	return nil
}

func (c *checker) unaryType(e *ast.Unary) types.Type {
	x := c.checkExpr(e.X)
	if x == nil {
		return nil
	}
	switch e.Op {
	case token.MINUS:
		if !types.IsNumeric(x) {
			c.errorf(e.Pos(), "cannot negate %s", x)
			return nil
		}
		return x
	case token.BANG:
		if x != types.Bool {
			c.errorf(e.Pos(), "cannot apply ! to %s", x)
			return nil
		}
		return types.Bool
	}
	return nil
}

func (c *checker) binaryType(e *ast.Binary) types.Type {
	x := c.checkExpr(e.X)
	y := c.checkExpr(e.Y)
	if x == nil || y == nil {
		return nil
	}

	switch e.Op {
	case token.PLUS:
		// The spec concatenates a Float onto a String in BaseShape.describe,
		// so + with a String operand is concatenation and stringifies the other.
		if x == types.String || y == types.String {
			return types.String
		}
		return c.arithType(e, x, y)

	case token.MINUS, token.STAR, token.SLASH, token.PERCENT:
		return c.arithType(e, x, y)

	case token.LT, token.GT, token.LE, token.GE:
		if !types.IsOrdered(x) || !types.IsOrdered(y) {
			c.errorf(e.Pos(), "cannot compare %s with %s", x, y)
			return nil
		}
		if !types.Assignable(x, y) && !types.Assignable(y, x) {
			c.errorf(e.Pos(), "cannot compare %s with %s", x, y)
			return nil
		}
		return types.Bool

	case token.EQ, token.NE:
		if !types.Assignable(x, y) && !types.Assignable(y, x) {
			c.errorf(e.Pos(), "cannot compare %s with %s for equality", x, y)
			return nil
		}
		return types.Bool

	case token.ANDAND, token.OROR:
		if x != types.Bool || y != types.Bool {
			c.errorf(e.Pos(), "operator %s needs Bool operands, found %s and %s", e.Op, x, y)
			return nil
		}
		return types.Bool
	}
	return nil
}

func (c *checker) arithType(e *ast.Binary, x, y types.Type) types.Type {
	if !types.IsNumeric(x) || !types.IsNumeric(y) {
		c.errorf(e.Pos(), "operator %s needs numeric operands, found %s and %s", e.Op, x, y)
		return nil
	}
	if x == types.Float || y == types.Float {
		return types.Float
	}
	return types.Int
}

func (c *checker) fieldType(e *ast.Field) types.Type {
	recv := c.checkExpr(e.X)
	if recv == nil {
		return nil
	}
	cl, ok := recv.(*types.Class)
	if !ok {
		if iface, isIface := recv.(*types.Interface); isIface {
			if _, isMethod := iface.Methods[e.Name]; isMethod {
				c.errorf(e.Pos(), "%s is a method on %s; call it with %s(...)", e.Name, iface.Name, e.Name)
				return nil
			}
			c.errorf(e.Pos(), "interface %s has no member %s", iface.Name, e.Name)
			return nil
		}
		c.errorf(e.Pos(), "type %s has no member %s", recv, e.Name)
		return nil
	}

	f := cl.LookupField(e.Name)
	if f == nil {
		if m := cl.LookupMethodOrInterface(e.Name); m != nil {
			c.errorf(e.Pos(), "%s is a method on %s; call it with %s(...)", e.Name, cl.Name, e.Name)
			return nil
		}
		c.errorf(e.Pos(), "class %s has no field %s", cl.Name, e.Name)
		return nil
	}
	if f.Private && c.class != f.Owner {
		c.errorf(e.Pos(), "field %s is private to class %s", e.Name, f.Owner.Name)
		return nil
	}
	c.info.FieldOwner[e] = f
	return f.Type
}

func (c *checker) indexType(e *ast.Index) types.Type {
	recv := c.checkExpr(e.X)
	idx := c.checkExpr(e.Index)
	if recv == nil || idx == nil {
		return nil
	}
	switch r := recv.(type) {
	case *types.List:
		if idx != types.Int {
			c.errorf(e.Index.Pos(), "a list index must be Int, found %s", idx)
			return nil
		}
		return r.Elem
	case *types.Map:
		if !types.Assignable(r.Key, idx) {
			c.errorf(e.Index.Pos(), "cannot index %s with %s", recv, idx)
			return nil
		}
		return r.Val
	}
	c.errorf(e.Pos(), "cannot index a value of type %s", recv)
	return nil
}

func (c *checker) listLitType(e *ast.ListLit) types.Type {
	if len(e.Elems) == 0 {
		// An empty literal has no element type of its own; the declared type
		// on the left of the `let` supplies it, and Assignable is lenient here
		// via the Void placeholder never matching anything else.
		return &types.List{Elem: emptyElem}
	}
	elem := c.checkExpr(e.Elems[0])
	if elem == nil {
		return nil
	}
	for _, el := range e.Elems[1:] {
		got := c.checkExpr(el)
		if got == nil {
			return nil
		}
		elem = c.join(el.Pos(), elem, got)
		if elem == nil {
			return nil
		}
	}
	return &types.List{Elem: elem}
}

func (c *checker) mapLitType(e *ast.MapLit) types.Type {
	if len(e.Keys) == 0 {
		return &types.Map{Key: emptyElem, Val: emptyElem}
	}
	key := c.checkExpr(e.Keys[0])
	val := c.checkExpr(e.Vals[0])
	if key == nil || val == nil {
		return nil
	}
	for i := 1; i < len(e.Keys); i++ {
		k := c.checkExpr(e.Keys[i])
		v := c.checkExpr(e.Vals[i])
		if k == nil || v == nil {
			return nil
		}
		key = c.join(e.Keys[i].Pos(), key, k)
		val = c.join(e.Vals[i].Pos(), val, v)
		if key == nil || val == nil {
			return nil
		}
	}
	return &types.Map{Key: key, Val: val}
}

// join finds a type both a and b fit into, which is how a mixed literal like
// [Rectangle(...), Circle(...)] becomes a [BaseShape].
func (c *checker) join(pos token.Pos, a, b types.Type) types.Type {
	if types.Assignable(a, b) {
		return a
	}
	if types.Assignable(b, a) {
		return b
	}
	// Walk a's ancestors and interfaces for a common supertype.
	if ac, ok := a.(*types.Class); ok {
		for k := ac.Super; k != nil; k = k.Super {
			if types.Assignable(k, b) {
				return k
			}
		}
		for _, i := range ac.AllInterfaces() {
			if types.Assignable(i, b) {
				return i
			}
		}
	}
	c.errorf(pos, "collection elements have incompatible types %s and %s", a, b)
	return nil
}

// emptyElem is the element type of an empty literal: it is assignable to
// nothing, so the declared type on the left always wins.
var emptyElem types.Type = &types.Primitive{Name: "<empty>"}

// ---------- calls ----------

func (c *checker) callType(e *ast.Call) types.Type {
	switch fn := e.Fn.(type) {
	case *ast.Ident:
		return c.callByName(e, fn)
	case *ast.Field:
		return c.callMethod(e, fn)
	}
	c.errorf(e.Pos(), "this expression is not callable")
	return nil
}

func (c *checker) callByName(e *ast.Call, fn *ast.Ident) types.Type {
	// A local variable shadowing a function or class name is not callable.
	if _, isVar := c.scope.lookup(fn.Name); isVar {
		c.errorf(e.Pos(), "%s is a variable, not a function", fn.Name)
		return nil
	}

	if fn.Name == "print" || fn.Name == "len" {
		return c.callBuiltin(e, fn.Name)
	}

	if cl, ok := c.info.Classes[fn.Name]; ok {
		if cl.Abstract {
			c.errorf(e.Pos(), "cannot instantiate abstract class %s", cl.Name)
			return nil
		}
		sig := cl.LookupCtor()
		if sig == nil {
			if len(e.Args) != 0 {
				c.errorf(e.Pos(), "class %s has no constructor, so %s() takes no arguments", cl.Name, cl.Name)
			}
			sig = &types.Signature{}
		}
		c.checkArgs(e, cl.Name, sig)
		c.info.Calls[e] = &CallTarget{Kind: CallCtor, Class: cl, Sig: sig}
		return cl
	}

	if _, ok := c.info.Interfaces[fn.Name]; ok {
		c.errorf(e.Pos(), "cannot instantiate interface %s", fn.Name)
		return nil
	}

	sig, ok := c.info.Funcs[fn.Name]
	if !ok {
		c.errorf(e.Pos(), "undefined function %s", fn.Name)
		return nil
	}
	c.checkArgs(e, fn.Name, sig)
	c.info.Calls[e] = &CallTarget{Kind: CallFunc, Func: c.info.FuncDecls[fn.Name], Sig: sig}
	return sig.Result
}

// callBuiltin types print and len, which the spec uses but never declares.
func (c *checker) callBuiltin(e *ast.Call, name string) types.Type {
	if len(e.Args) != 1 {
		c.errorf(e.Pos(), "%s takes exactly 1 argument, found %d", name, len(e.Args))
		return nil
	}
	arg := c.checkExpr(e.Args[0])
	if arg == nil {
		return nil
	}
	c.info.Calls[e] = &CallTarget{Kind: CallBuiltin, Builtin: name}

	if name == "print" {
		if arg == types.Void {
			c.errorf(e.Args[0].Pos(), "cannot print a Void value")
			return nil
		}
		return types.Void
	}
	switch arg.(type) {
	case *types.List, *types.Map:
		return types.Int
	}
	if arg == types.String {
		return types.Int
	}
	c.errorf(e.Args[0].Pos(), "len needs a list, a map, or a String, found %s", arg)
	return nil
}

func (c *checker) callMethod(e *ast.Call, fn *ast.Field) types.Type {
	recv := c.checkExpr(fn.X)
	if recv == nil {
		return nil
	}

	var m *types.Method
	switch r := recv.(type) {
	case *types.Class:
		m = r.LookupMethodOrInterface(fn.Name)
		if m == nil {
			if f := r.LookupField(fn.Name); f != nil {
				c.errorf(e.Pos(), "%s is a field on %s, not a method", fn.Name, r.Name)
				return nil
			}
			c.errorf(e.Pos(), "class %s has no method %s", r.Name, fn.Name)
			return nil
		}
		if m.Private && c.class != m.Owner {
			c.errorf(e.Pos(), "method %s is private to class %s", fn.Name, m.Owner.Name)
			return nil
		}
	case *types.Interface:
		var ok bool
		m, ok = r.Methods[fn.Name]
		if !ok {
			c.errorf(e.Pos(), "interface %s has no method %s", r.Name, fn.Name)
			return nil
		}
	default:
		c.errorf(e.Pos(), "type %s has no methods", recv)
		return nil
	}

	c.checkArgs(e, fn.Name, m.Sig)
	c.info.Calls[e] = &CallTarget{Kind: CallMethod, Method: fn.Name, Recv: recv, Sig: m.Sig}
	return m.Sig.Result
}

func (c *checker) checkArgs(e *ast.Call, name string, sig *types.Signature) {
	if len(e.Args) != len(sig.Params) {
		c.errorf(e.Pos(), "%s takes %d argument(s), found %d", name, len(sig.Params), len(e.Args))
		// Still type the arguments so errors inside them are reported.
		for _, a := range e.Args {
			c.checkExpr(a)
		}
		return
	}
	for i, a := range e.Args {
		got := c.checkExprExpecting(a, sig.Params[i])
		if got != nil && !types.Assignable(sig.Params[i], got) {
			c.errorf(a.Pos(), "cannot use %s as %s in argument %d of %s", got, sig.Params[i], i+1, name)
		}
	}
}
