// Package types is the Flint type model, shared by the checker and the
// interpreter.
//
// The checker builds it from source; the interpreter reads it back for method
// dispatch, field defaults, and catch-clause matching. Keeping it in its own
// package is what lets the interpreter stay independent of the checker.
package types

import "strings"

// Type is a Flint type.
type Type interface {
	String() string
}

// Primitive is one of the six built-in scalar types.
type Primitive struct {
	Name string
}

// String returns the primitive's source spelling.
func (p *Primitive) String() string { return p.Name }

// The primitive types. They are compared by pointer identity, so there is
// exactly one of each.
var (
	Int    = &Primitive{Name: "Int"}
	Float  = &Primitive{Name: "Float"}
	Bool   = &Primitive{Name: "Bool"}
	String = &Primitive{Name: "String"}
	Char   = &Primitive{Name: "Char"}
	Void   = &Primitive{Name: "Void"}
)

// Primitives maps each primitive's name to its type.
var Primitives = map[string]*Primitive{
	"Int": Int, "Float": Float, "Bool": Bool,
	"String": String, "Char": Char, "Void": Void,
}

// List is a list type, written [Elem].
type List struct {
	Elem Type
}

// String renders the list type as [Elem].
func (l *List) String() string { return "[" + l.Elem.String() + "]" }

// Map is a map type, written [Key: Val].
type Map struct {
	Key Type
	Val Type
}

// String renders the map type as [Key: Val].
func (m *Map) String() string { return "[" + m.Key.String() + ": " + m.Val.String() + "]" }

// Signature is a function, method, or constructor type.
type Signature struct {
	Params     []Type
	ParamNames []string
	Result     Type // nil for a constructor
}

// String renders the signature as (A, B): R.
func (s *Signature) String() string {
	var b strings.Builder
	b.WriteString("(")
	for i, p := range s.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.String())
	}
	b.WriteString(")")
	if s.Result != nil {
		b.WriteString(": " + s.Result.String())
	}
	return b.String()
}

// Field is one declared instance field.
type Field struct {
	Name    string
	Type    Type
	Private bool
	Owner   *Class
}

// Method is one declared method.
type Method struct {
	Name     string
	Sig      *Signature
	Private  bool
	Abstract bool
	Owner    *Class
}

// Class is a class type. Super is the single optional superclass; Interfaces
// holds every interface named directly on this class.
type Class struct {
	Name       string
	Super      *Class
	Interfaces []*Interface
	Fields     map[string]*Field
	FieldOrder []string // declaration order, for deterministic initialisation
	Methods    map[string]*Method
	Ctor       *Signature
	Abstract   bool
}

// NewClass returns an empty class with its maps ready.
func NewClass(name string) *Class {
	return &Class{Name: name, Fields: map[string]*Field{}, Methods: map[string]*Method{}}
}

// String returns the class name.
func (c *Class) String() string { return c.Name }

// Interface is an interface type.
type Interface struct {
	Name    string
	Methods map[string]*Method
}

// NewInterface returns an empty interface with its map ready.
func NewInterface(name string) *Interface {
	return &Interface{Name: name, Methods: map[string]*Method{}}
}

// String returns the interface name.
func (i *Interface) String() string { return i.Name }

// LookupField finds a field on c or any ancestor, nearest first.
func (c *Class) LookupField(name string) *Field {
	for k := c; k != nil; k = k.Super {
		if f, ok := k.Fields[name]; ok {
			return f
		}
	}
	return nil
}

// LookupMethod finds a method on c or any ancestor, nearest first. A subclass
// method shadows the superclass one, which is how overriding works.
func (c *Class) LookupMethod(name string) *Method {
	for k := c; k != nil; k = k.Super {
		if m, ok := k.Methods[name]; ok {
			return m
		}
	}
	return nil
}

// LookupMethodOrInterface finds a method on c or an ancestor, and failing that
// on any interface c claims.
//
// The spec's BaseShape.describe() calls object.area(), which no class in the
// chain declares -- it exists only on the Shape interface BaseShape conforms
// to. Static lookup therefore has to see interface methods; dynamic dispatch at
// run time still finds the concrete override via LookupMethod.
func (c *Class) LookupMethodOrInterface(name string) *Method {
	if m := c.LookupMethod(name); m != nil {
		return m
	}
	for _, i := range c.AllInterfaces() {
		if m, ok := i.Methods[name]; ok {
			return m
		}
	}
	return nil
}

// LookupCtor finds the constructor on c, or inherits the nearest ancestor's.
//
// Spec gap 2: the spec's sample constructs NegativeAreaError("...") though that
// class declares no constructor, so a class with none inherits its parent's.
func (c *Class) LookupCtor() *Signature {
	for k := c; k != nil; k = k.Super {
		if k.Ctor != nil {
			return k.Ctor
		}
	}
	return nil
}

// AllInterfaces returns every interface c claims, directly or through an
// ancestor.
func (c *Class) AllInterfaces() []*Interface {
	var out []*Interface
	seen := map[*Interface]bool{}
	for k := c; k != nil; k = k.Super {
		for _, i := range k.Interfaces {
			if !seen[i] {
				seen[i] = true
				out = append(out, i)
			}
		}
	}
	return out
}

// IsSubclass reports whether sub is super or descends from it.
func IsSubclass(sub, super *Class) bool {
	for k := sub; k != nil; k = k.Super {
		if k == super {
			return true
		}
	}
	return false
}

// Implements reports whether c claims i, directly or through an ancestor.
func Implements(c *Class, i *Interface) bool {
	for _, got := range c.AllInterfaces() {
		if got == i {
			return true
		}
	}
	return false
}

// Identical reports whether two types are the same type.
func Identical(a, b Type) bool {
	if a == b {
		return true
	}
	switch x := a.(type) {
	case *List:
		y, ok := b.(*List)
		return ok && Identical(x.Elem, y.Elem)
	case *Map:
		y, ok := b.(*Map)
		return ok && Identical(x.Key, y.Key) && Identical(x.Val, y.Val)
	}
	return false
}

// Assignable reports whether a value of type src may be stored in a slot of
// type dst.
//
// The rules, in order: identical types; Int widens to Float (the spec's sample
// compares a Float against the Int literal 0); a subclass is assignable to its
// superclass; a class is assignable to any interface it implements. Lists and
// maps are invariant -- [Rectangle] is not a [Shape], because writing a Circle
// through the wider view would break the narrower one.
func Assignable(dst, src Type) bool {
	if dst == nil || src == nil {
		return false
	}
	if Identical(dst, src) {
		return true
	}
	if dst == Float && src == Int {
		return true
	}
	switch d := dst.(type) {
	case *Class:
		s, ok := src.(*Class)
		return ok && IsSubclass(s, d)
	case *Interface:
		if s, ok := src.(*Class); ok {
			return Implements(s, d)
		}
		// An interface value is assignable to the same interface only, which
		// Identical already covered.
		return false
	}
	return false
}

// IsNumeric reports whether t is Int or Float.
func IsNumeric(t Type) bool { return t == Int || t == Float }

// IsOrdered reports whether t supports the < > <= >= operators.
func IsOrdered(t Type) bool { return t == Int || t == Float || t == String || t == Char }

// IsReference reports whether values of t are object references, and so start
// out uninitialised rather than at a zero value.
func IsReference(t Type) bool {
	switch t.(type) {
	case *Class, *Interface:
		return true
	}
	return false
}
