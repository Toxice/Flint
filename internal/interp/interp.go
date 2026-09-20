// Package interp is a tree-walking evaluator for Flint.
//
// It runs a program the checker has already accepted, so it trusts the static
// types and only re-checks what static typing cannot decide: division by zero,
// list bounds, and use of an uninitialised object reference. Each of those
// raises an ordinary Flint exception the program can catch.
//
// Non-local control flow -- return and throw -- uses panic with the unexported
// signal types below, recovered at the function-call, try, and top-level
// boundaries. This is the usual Go idiom for an evaluator; the signals never
// escape this package, and any panic that is not one of them is re-panicked
// unchanged so genuine bugs still surface.
package interp

import (
	"fmt"
	"io"
	"math"

	"flint/internal/ast"
	"flint/internal/checker"
	"flint/internal/token"
	"flint/internal/types"
)

// returnSignal unwinds to the enclosing function call.
type returnSignal struct {
	value Value
}

// throwSignal unwinds to the enclosing try or to the top level.
type throwSignal struct {
	exc *ObjectVal
}

// Interp executes a checked Flint program.
type Interp struct {
	info   *checker.Info
	out    io.Writer
	nextID int
}

// New returns an interpreter for a program the checker has accepted. Output
// from print goes to out.
func New(prog *ast.Program, info *checker.Info, out io.Writer) *Interp {
	return &Interp{info: info, out: out}
}

// env is a lexical scope chain of local variables.
type env struct {
	parent *env
	vars   map[string]Value
}

func newEnv(parent *env) *env {
	return &env{parent: parent, vars: map[string]Value{}}
}

func (e *env) lookup(name string) (Value, bool) {
	for k := e; k != nil; k = k.parent {
		if v, ok := k.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// assign stores into the nearest scope that already binds name.
func (e *env) assign(name string, v Value) bool {
	for k := e; k != nil; k = k.parent {
		if _, ok := k.vars[name]; ok {
			k.vars[name] = v
			return true
		}
	}
	return false
}

// frame is one function activation: its locals, its receiver, and the return
// type a `return` must widen to.
type frame struct {
	env    *env
	self   *ObjectVal
	result types.Type
}

func (f *frame) child() *frame {
	return &frame{env: newEnv(f.env), self: f.self, result: f.result}
}

// Run executes main. It returns a non-nil error if a Flint exception escapes.
func (i *Interp) Run() (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		ts, ok := r.(throwSignal)
		if !ok {
			panic(r)
		}
		err = fmt.Errorf("uncaught %s: %s", ts.exc.Class.Name, ts.exc.Message())
	}()

	main, ok := i.info.FuncDecls["main"]
	if !ok {
		return fmt.Errorf("no main function")
	}
	i.callFunc(main, nil, nil)
	return nil
}

// ---------- exceptions ----------

// throwBuiltin raises one of the runtime's own exception classes.
func (i *Interp) throwBuiltin(class, format string, args ...any) {
	cl := i.info.Classes[class]
	if cl == nil {
		// The built-ins are registered by the checker; if one is missing the
		// pipeline was assembled wrongly, which is a host bug, not a Flint one.
		panic(fmt.Sprintf("interp: built-in class %s is not registered", class))
	}
	obj := i.alloc(cl)
	obj.Fields["message"] = Str(fmt.Sprintf(format, args...))
	panic(throwSignal{exc: obj})
}

// alloc creates an instance with every inherited field at its zero value.
// Fields are initialised from the root ancestor down, so a subclass field that
// shadows nothing still lands in the same flat map.
func (i *Interp) alloc(cl *types.Class) *ObjectVal {
	i.nextID++
	obj := &ObjectVal{Class: cl, Fields: map[string]Value{}, ID: i.nextID}

	var chain []*types.Class
	for k := cl; k != nil; k = k.Super {
		chain = append(chain, k)
	}
	for n := len(chain) - 1; n >= 0; n-- {
		k := chain[n]
		for _, name := range k.FieldOrder {
			obj.Fields[name] = ZeroValue(k.Fields[name].Type)
		}
	}
	return obj
}

// ---------- statements ----------

func (i *Interp) execBlock(b *ast.Block, fr *frame) {
	for _, s := range b.Stmts {
		i.execStmt(s, fr)
	}
}

func (i *Interp) execStmt(s ast.Stmt, fr *frame) {
	switch s := s.(type) {
	case *ast.Let:
		v := i.eval(s.Value, fr)
		fr.env.vars[s.Name] = coerce(v, i.info.Resolved[s.Type])

	case *ast.Assign:
		i.execAssign(s, fr)

	case *ast.ExprStmt:
		i.eval(s.X, fr)

	case *ast.Block:
		i.execBlock(s, fr.child())

	case *ast.If:
		if bool(i.evalBool(s.Cond, fr)) {
			i.execBlock(s.Then, fr.child())
		} else if s.Else != nil {
			i.execStmt(s.Else, fr)
		}

	case *ast.While:
		for bool(i.evalBool(s.Cond, fr)) {
			i.execBlock(s.Body, fr.child())
		}

	case *ast.For:
		i.execFor(s, fr)

	case *ast.Return:
		var v Value = Unit{}
		if s.Value != nil {
			v = coerce(i.eval(s.Value, fr), fr.result)
		}
		panic(returnSignal{value: v})

	case *ast.Throw:
		v := i.eval(s.Value, fr)
		obj, ok := v.(*ObjectVal)
		if !ok {
			i.throwBuiltin("NullError", "cannot throw an uninitialized value")
		}
		panic(throwSignal{exc: obj})

	case *ast.Try:
		i.execTry(s, fr)
	}
}

func (i *Interp) execAssign(s *ast.Assign, fr *frame) {
	v := i.eval(s.Value, fr)
	if t, ok := i.info.ExprType[s.Target]; ok {
		v = coerce(v, t)
	}

	switch target := s.Target.(type) {
	case *ast.Ident:
		if !fr.env.assign(target.Name, v) {
			// The checker guarantees the name exists; if it did not, binding it
			// here is still the least surprising behaviour.
			fr.env.vars[target.Name] = v
		}

	case *ast.Field:
		obj := i.evalObject(target.X, fr, target.Pos())
		obj.Fields[target.Name] = v

	case *ast.Index:
		i.assignIndex(target, v, fr)
	}
}

func (i *Interp) assignIndex(target *ast.Index, v Value, fr *frame) {
	recv := i.eval(target.X, fr)
	idx := i.eval(target.Index, fr)
	switch recv := recv.(type) {
	case *ListVal:
		n, ok := idx.(Int)
		if !ok {
			i.throwBuiltin("IndexError", "list index must be an Int")
			return
		}
		if n < 0 || int(n) >= len(recv.Elems) {
			i.throwBuiltin("IndexError", "list index %d out of range for a list of length %d", int(n), len(recv.Elems))
			return
		}
		recv.Elems[n] = v
	case *MapVal:
		recv.Set(idx, v)
	case Uninit:
		i.throwBuiltin("NullError", "cannot index an uninitialized value")
	}
}

func (i *Interp) execFor(s *ast.For, fr *frame) {
	iter := i.eval(s.Iter, fr)
	list, ok := iter.(*ListVal)
	if !ok {
		i.throwBuiltin("NullError", "cannot iterate over an uninitialized value")
		return
	}
	declared := i.info.Resolved[s.Type]
	// Iterate over a snapshot so mutating the list inside the body cannot make
	// the loop run forever or read past the end.
	elems := make([]Value, len(list.Elems))
	copy(elems, list.Elems)
	for _, e := range elems {
		inner := fr.child()
		inner.env.vars[s.Var] = coerce(e, declared)
		i.execBlock(s.Body, inner)
	}
}

// execTry runs the body and hands a thrown exception to the first catch clause
// whose type matches. A return crossing the try is a returnSignal, not a
// throwSignal, so it passes straight through.
func (i *Interp) execTry(s *ast.Try, fr *frame) {
	caught := i.runProtected(s.Body, fr)
	if caught == nil {
		return
	}
	for _, clause := range s.Catches {
		ct, ok := i.info.Resolved[clause.Type].(*types.Class)
		if !ok || !types.IsSubclass(caught.Class, ct) {
			continue
		}
		inner := fr.child()
		inner.env.vars[clause.Name] = caught
		i.execBlock(clause.Body, inner)
		return
	}
	// No clause matched: keep unwinding to an outer try or to Run.
	panic(throwSignal{exc: caught})
}

// runProtected executes b, returning the exception it threw, or nil.
func (i *Interp) runProtected(b *ast.Block, fr *frame) (exc *ObjectVal) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		ts, ok := r.(throwSignal)
		if !ok {
			panic(r) // a return signal, or a real bug
		}
		exc = ts.exc
	}()
	i.execBlock(b, fr.child())
	return nil
}

// ---------- calls ----------

// callFunc runs a function, method, or constructor body and returns its result.
func (i *Interp) callFunc(d *ast.FuncDecl, args []Value, self *ObjectVal) (result Value) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		rs, ok := r.(returnSignal)
		if !ok {
			panic(r)
		}
		result = rs.value
	}()

	fr := &frame{env: newEnv(nil), self: self}
	if d.Result != nil {
		fr.result = i.info.Resolved[d.Result]
	}
	for n, p := range d.Params {
		if n < len(args) {
			fr.env.vars[p.Name] = coerce(args[n], i.info.Resolved[p.Type])
		}
	}
	if d.Body != nil {
		i.execBlock(d.Body, fr)
	}
	return Unit{}
}

// construct allocates an instance of cl and runs its constructor, which may be
// inherited from an ancestor (spec gap 2).
func (i *Interp) construct(cl *types.Class, args []Value) *ObjectVal {
	obj := i.alloc(cl)
	for k := cl; k != nil; k = k.Super {
		if body, ok := i.info.CtorBody[k]; ok {
			i.callFunc(body, args, obj)
			return obj
		}
	}
	// Exception and its built-in subclasses have no source body; their one
	// parameter is the message.
	if sig := cl.LookupCtor(); sig != nil && len(sig.ParamNames) == len(args) {
		for n, name := range sig.ParamNames {
			obj.Fields[name] = args[n]
		}
	}
	return obj
}

// callMethod dispatches on the receiver's runtime class, which is what makes an
// override take effect.
func (i *Interp) callMethod(recv *ObjectVal, name string, args []Value) Value {
	m := recv.Class.LookupMethod(name)
	if m == nil {
		i.throwBuiltin("NullError", "class %s has no method %s", recv.Class.Name, name)
	}
	body, ok := i.info.MethodBody[m]
	if !ok {
		i.throwBuiltin("NullError", "method %s on class %s has no implementation", name, recv.Class.Name)
	}
	return i.callFunc(body, args, recv)
}

// ---------- expressions ----------

func (i *Interp) eval(e ast.Expr, fr *frame) Value {
	switch e := e.(type) {
	case *ast.IntLit:
		return Int(e.Value)
	case *ast.FloatLit:
		return Float(e.Value)
	case *ast.StringLit:
		return Str(e.Value)
	case *ast.CharLit:
		return Char(e.Value)
	case *ast.BoolLit:
		return Bool(e.Value)

	case *ast.ObjectExpr:
		if fr.self == nil {
			i.throwBuiltin("NullError", "object is not bound here")
		}
		return fr.self

	case *ast.Ident:
		if v, ok := fr.env.lookup(e.Name); ok {
			return v
		}
		i.throwBuiltin("NullError", "undefined variable %s", e.Name)

	case *ast.Unary:
		return i.evalUnary(e, fr)

	case *ast.Binary:
		return i.evalBinary(e, fr)

	case *ast.Field:
		obj := i.evalObject(e.X, fr, e.Pos())
		v, ok := obj.Fields[e.Name]
		if !ok {
			i.throwBuiltin("NullError", "class %s has no field %s", obj.Class.Name, e.Name)
		}
		return v

	case *ast.Index:
		return i.evalIndex(e, fr)

	case *ast.Call:
		return i.evalCall(e, fr)

	case *ast.ListLit:
		elem := elemTypeOf(i.info.ExprType[e])
		list := &ListVal{Elems: make([]Value, 0, len(e.Elems))}
		for _, el := range e.Elems {
			list.Elems = append(list.Elems, coerce(i.eval(el, fr), elem))
		}
		return list

	case *ast.MapLit:
		m := NewMap()
		mt, _ := i.info.ExprType[e].(*types.Map)
		for n := range e.Keys {
			k := i.eval(e.Keys[n], fr)
			v := i.eval(e.Vals[n], fr)
			if mt != nil {
				k = coerce(k, mt.Key)
				v = coerce(v, mt.Val)
			}
			m.Set(k, v)
		}
		return m
	}
	panic(fmt.Sprintf("interp: unhandled expression %T", e))
}

func elemTypeOf(t types.Type) types.Type {
	if l, ok := t.(*types.List); ok {
		return l.Elem
	}
	return nil
}

// evalObject evaluates e and requires an object, so a field access or method
// call on an uninitialised reference is a catchable NullError.
func (i *Interp) evalObject(e ast.Expr, fr *frame, pos token.Pos) *ObjectVal {
	v := i.eval(e, fr)
	obj, ok := v.(*ObjectVal)
	if !ok {
		i.throwBuiltin("NullError", "use of an uninitialized value at %s", pos)
	}
	return obj
}

func (i *Interp) evalBool(e ast.Expr, fr *frame) Bool {
	b, ok := i.eval(e, fr).(Bool)
	if !ok {
		i.throwBuiltin("NullError", "condition is not a Bool")
	}
	return b
}

func (i *Interp) evalUnary(e *ast.Unary, fr *frame) Value {
	x := i.eval(e.X, fr)
	switch e.Op {
	case token.MINUS:
		switch x := x.(type) {
		case Int:
			return -x
		case Float:
			return -x
		}
	case token.BANG:
		if b, ok := x.(Bool); ok {
			return !b
		}
	}
	i.throwBuiltin("NullError", "cannot apply %s to this value", e.Op)
	return Unit{}
}

func (i *Interp) evalBinary(e *ast.Binary, fr *frame) Value {
	// && and || short-circuit, so the right side is not evaluated eagerly.
	switch e.Op {
	case token.ANDAND:
		if !bool(i.evalBool(e.X, fr)) {
			return Bool(false)
		}
		return i.evalBool(e.Y, fr)
	case token.OROR:
		if bool(i.evalBool(e.X, fr)) {
			return Bool(true)
		}
		return i.evalBool(e.Y, fr)
	}

	x := i.eval(e.X, fr)
	y := i.eval(e.Y, fr)
	result := i.info.ExprType[e]

	switch e.Op {
	case token.PLUS:
		// The checker types + as String when either side is a String, which is
		// how the spec concatenates a Float onto a name.
		if result == types.String {
			return Str(Stringify(x) + Stringify(y))
		}
		return i.arith(e, x, y, result)

	case token.MINUS, token.STAR, token.SLASH, token.PERCENT:
		return i.arith(e, x, y, result)

	case token.EQ:
		return Bool(equalValues(x, y))
	case token.NE:
		return Bool(!equalValues(x, y))

	case token.LT, token.GT, token.LE, token.GE:
		return i.compare(e, x, y)
	}
	panic(fmt.Sprintf("interp: unhandled binary operator %v", e.Op))
}

// arith evaluates a numeric operation at the width the checker chose.
func (i *Interp) arith(e *ast.Binary, x, y Value, result types.Type) Value {
	if result == types.Float {
		a, b := toFloat(x), toFloat(y)
		switch e.Op {
		case token.PLUS:
			return Float(a + b)
		case token.MINUS:
			return Float(a - b)
		case token.STAR:
			return Float(a * b)
		case token.SLASH:
			return Float(a / b) // IEEE: division by zero gives Inf or NaN
		case token.PERCENT:
			return Float(math.Mod(a, b))
		}
	}

	a, b := toInt(x), toInt(y)
	switch e.Op {
	case token.PLUS:
		return Int(a + b)
	case token.MINUS:
		return Int(a - b)
	case token.STAR:
		return Int(a * b)
	case token.SLASH:
		if b == 0 {
			i.throwBuiltin("ArithmeticError", "division by zero")
		}
		return Int(a / b)
	case token.PERCENT:
		if b == 0 {
			i.throwBuiltin("ArithmeticError", "modulo by zero")
		}
		return Int(a % b)
	}
	panic(fmt.Sprintf("interp: unhandled arithmetic operator %v", e.Op))
}

func (i *Interp) compare(e *ast.Binary, x, y Value) Value {
	var cmp int
	switch a := x.(type) {
	case Int:
		if b, ok := y.(Float); ok {
			cmp = cmpFloat(float64(a), float64(b))
		} else {
			cmp = cmpInt(int64(a), toInt(y))
		}
	case Float:
		cmp = cmpFloat(float64(a), toFloat(y))
	case Str:
		b, ok := y.(Str)
		if !ok {
			i.throwBuiltin("NullError", "cannot compare a String with this value")
		}
		cmp = cmpStr(string(a), string(b))
	case Char:
		b, ok := y.(Char)
		if !ok {
			i.throwBuiltin("NullError", "cannot compare a Char with this value")
		}
		cmp = cmpInt(int64(a), int64(b))
	default:
		i.throwBuiltin("NullError", "values of this type cannot be ordered")
	}

	switch e.Op {
	case token.LT:
		return Bool(cmp < 0)
	case token.GT:
		return Bool(cmp > 0)
	case token.LE:
		return Bool(cmp <= 0)
	case token.GE:
		return Bool(cmp >= 0)
	}
	panic(fmt.Sprintf("interp: unhandled comparison %v", e.Op))
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func toInt(v Value) int64 {
	switch v := v.(type) {
	case Int:
		return int64(v)
	case Float:
		return int64(v)
	case Char:
		return int64(v)
	}
	return 0
}

func toFloat(v Value) float64 {
	switch v := v.(type) {
	case Int:
		return float64(v)
	case Float:
		return float64(v)
	}
	return 0
}

// equalValues compares two values. Scalars compare by value; objects, lists,
// and maps compare by identity, as they do in Java.
func equalValues(x, y Value) bool {
	switch a := x.(type) {
	case Int:
		if b, ok := y.(Float); ok {
			return float64(a) == float64(b)
		}
	case Float:
		if b, ok := y.(Int); ok {
			return float64(a) == float64(b)
		}
	}
	return x == y
}

func (i *Interp) evalIndex(e *ast.Index, fr *frame) Value {
	recv := i.eval(e.X, fr)
	idx := i.eval(e.Index, fr)

	switch recv := recv.(type) {
	case *ListVal:
		n, ok := idx.(Int)
		if !ok {
			i.throwBuiltin("IndexError", "list index must be an Int")
		}
		if n < 0 || int(n) >= len(recv.Elems) {
			i.throwBuiltin("IndexError", "list index %d out of range for a list of length %d", int(n), len(recv.Elems))
		}
		return recv.Elems[n]

	case *MapVal:
		v, ok := recv.Get(idx)
		if !ok {
			i.throwBuiltin("IndexError", "no entry for key %s", Stringify(idx))
		}
		return v
	}
	i.throwBuiltin("NullError", "cannot index an uninitialized value")
	return Unit{}
}

func (i *Interp) evalCall(e *ast.Call, fr *frame) Value {
	target, ok := i.info.Calls[e]
	if !ok {
		panic("interp: call was not resolved by the checker")
	}

	args := make([]Value, len(e.Args))
	for n, a := range e.Args {
		args[n] = i.eval(a, fr)
		if target.Sig != nil && n < len(target.Sig.Params) {
			args[n] = coerce(args[n], target.Sig.Params[n])
		}
	}

	switch target.Kind {
	case checker.CallBuiltin:
		return i.callBuiltin(target.Builtin, args)

	case checker.CallCtor:
		return i.construct(target.Class, args)

	case checker.CallFunc:
		return i.callFunc(target.Func, args, nil)

	case checker.CallMethod:
		field := e.Fn.(*ast.Field)
		recv := i.evalObject(field.X, fr, field.Pos())
		return i.callMethod(recv, target.Method, args)
	}
	panic(fmt.Sprintf("interp: unhandled call kind %v", target.Kind))
}
