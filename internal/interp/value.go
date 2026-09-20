package interp

import (
	"strconv"
	"strings"

	"flint/internal/types"
)

// Value is a Flint runtime value.
type Value interface {
	flintValue()
}

// The scalar runtime types. They are distinct Go types so a type switch in the
// evaluator is exhaustive and a Bool can never be mistaken for an Int.
type (
	// Int is a Flint Int.
	Int int64
	// Float is a Flint Float.
	Float float64
	// Bool is a Flint Bool.
	Bool bool
	// Str is a Flint String.
	Str string
	// Char is a Flint Char.
	Char rune
)

// Unit is the value of a Void expression.
type Unit struct{}

// Uninit is the value of a field or variable of class or interface type that
// has not been assigned yet. Using one throws NullError rather than crashing
// the host, which is spec gap 3.
type Uninit struct{}

func (Int) flintValue()    {}
func (Float) flintValue()  {}
func (Bool) flintValue()   {}
func (Str) flintValue()    {}
func (Char) flintValue()   {}
func (Unit) flintValue()   {}
func (Uninit) flintValue() {}

// ListVal is a Flint list. Lists are mutable and passed by reference.
type ListVal struct {
	Elems []Value
}

func (*ListVal) flintValue() {}

// MapVal is a Flint map. It keeps insertion order so printing a map is
// deterministic, which Go's own map iteration is not.
type MapVal struct {
	keys []Value
	m    map[Value]Value
}

func (*MapVal) flintValue() {}

// NewMap returns an empty map.
func NewMap() *MapVal {
	return &MapVal{m: map[Value]Value{}}
}

// Get returns the value stored under k.
func (m *MapVal) Get(k Value) (Value, bool) {
	v, ok := m.m[k]
	return v, ok
}

// Set stores v under k, appending k to the insertion order only if it is new.
func (m *MapVal) Set(k, v Value) {
	if _, exists := m.m[k]; !exists {
		m.keys = append(m.keys, k)
	}
	m.m[k] = v
}

// Len returns the number of entries.
func (m *MapVal) Len() int { return len(m.keys) }

// Keys returns the keys in insertion order.
func (m *MapVal) Keys() []Value { return m.keys }

// ObjectVal is an instance of a Flint class. Objects are reference values and
// compare by identity.
type ObjectVal struct {
	Class  *types.Class
	Fields map[string]Value
	ID     int
}

func (*ObjectVal) flintValue() {}

// IsException reports whether o is an Exception or a subclass of one.
func (o *ObjectVal) IsException() bool {
	for k := o.Class; k != nil; k = k.Super {
		if k.Name == "Exception" {
			return true
		}
	}
	return false
}

// Message returns an exception's message field, or the empty string.
func (o *ObjectVal) Message() string {
	if s, ok := o.Fields["message"].(Str); ok {
		return string(s)
	}
	return ""
}

// ZeroValue returns the initial value of a slot of type t.
//
// Spec gap 3: the spec never states what an unassigned field holds. Scalars get
// their natural zero and collections start empty, but a class- or
// interface-typed slot starts uninitialised, so reading one is a catchable
// NullError instead of a silent wrong answer.
func ZeroValue(t types.Type) Value {
	switch t := t.(type) {
	case *types.Primitive:
		switch t {
		case types.Int:
			return Int(0)
		case types.Float:
			return Float(0)
		case types.Bool:
			return Bool(false)
		case types.String:
			return Str("")
		case types.Char:
			return Char(0)
		}
		return Unit{}
	case *types.List:
		return &ListVal{}
	case *types.Map:
		return NewMap()
	}
	return Uninit{}
}

// Stringify renders v the way print and string concatenation show it.
func Stringify(v Value) string {
	switch v := v.(type) {
	case Int:
		return strconv.FormatInt(int64(v), 10)
	case Float:
		return formatFloat(float64(v))
	case Bool:
		if v {
			return "true"
		}
		return "false"
	case Str:
		return string(v)
	case Char:
		return string(rune(v))
	case Unit:
		return "Void"
	case Uninit:
		return "uninitialized"
	case *ListVal:
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range v.Elems {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(Stringify(e))
		}
		b.WriteByte(']')
		return b.String()
	case *MapVal:
		var b strings.Builder
		b.WriteByte('[')
		if v.Len() == 0 {
			b.WriteByte(':')
		}
		for i, k := range v.Keys() {
			if i > 0 {
				b.WriteString(", ")
			}
			val, _ := v.Get(k)
			b.WriteString(Stringify(k) + ": " + Stringify(val))
		}
		b.WriteByte(']')
		return b.String()
	case *ObjectVal:
		if v.IsException() {
			return v.Class.Name + ": " + v.Message()
		}
		return v.Class.Name + "@" + strconv.Itoa(v.ID)
	}
	return "?"
}

// formatFloat renders a Float so it always reads as one: Go's shortest
// representation of 20.0 is "20", which would be indistinguishable from an Int,
// so a bare integral result gets ".0" back.
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if strings.ContainsAny(s, ".eEnN") { // covers 1e+10, NaN, +Inf
		return s
	}
	return s + ".0"
}

// coerce widens an Int to a Float when the destination type calls for it. This
// is the runtime half of the Int-to-Float widening the checker allows.
func coerce(v Value, t types.Type) Value {
	if t == types.Float {
		if i, ok := v.(Int); ok {
			return Float(i)
		}
	}
	return v
}
