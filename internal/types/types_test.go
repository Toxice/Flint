package types

import "testing"

// shapeHierarchy builds the spec's sample hierarchy:
//
//	interface Shape
//	abstract class BaseShape : Shape
//	class Rectangle : BaseShape
//	class Circle : BaseShape
//	class Unrelated
func shapeHierarchy() (shape *Interface, base, rect, circ, unrel *Class) {
	shape = NewInterface("Shape")
	shape.Methods["area"] = &Method{Name: "area", Sig: &Signature{Result: Float}}

	base = NewClass("BaseShape")
	base.Abstract = true
	base.Interfaces = []*Interface{shape}

	rect = NewClass("Rectangle")
	rect.Super = base

	circ = NewClass("Circle")
	circ.Super = base

	unrel = NewClass("Unrelated")
	return
}

func TestAssignable(t *testing.T) {
	shape, base, rect, _, unrel := shapeHierarchy()

	tests := []struct {
		name string
		dst  Type
		src  Type
		want bool
	}{
		{"identical Int", Int, Int, true},
		{"identical String", String, String, true},
		{"Int widens to Float", Float, Int, true},
		{"Float does not narrow to Int", Int, Float, false},
		{"Bool is not Int", Int, Bool, false},
		{"Char is not String", String, Char, false},

		{"subclass to superclass", base, rect, true},
		{"superclass to subclass", rect, base, false},
		{"class to same class", rect, rect, true},
		{"unrelated classes", rect, unrel, false},

		{"class to interface via superclass", shape, rect, true},
		{"abstract class to its interface", shape, base, true},
		{"unrelated class to interface", shape, unrel, false},
		{"interface to class", rect, shape, false},

		{"same list type", &List{Elem: Int}, &List{Elem: Int}, true},
		{"list is invariant", &List{Elem: shape}, &List{Elem: rect}, false},
		{"list of different primitive", &List{Elem: Int}, &List{Elem: Float}, false},
		{"list is not its element", Int, &List{Elem: Int}, false},

		{"same map type", &Map{Key: String, Val: Int}, &Map{Key: String, Val: Int}, true},
		{"map is invariant in the value", &Map{Key: String, Val: Float}, &Map{Key: String, Val: Int}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Assignable(tt.dst, tt.src); got != tt.want {
				t.Fatalf("Assignable(%s, %s) = %v, want %v", tt.dst, tt.src, got, tt.want)
			}
		})
	}
}

func TestIsSubclassOnADeepChain(t *testing.T) {
	a := NewClass("A")
	b := NewClass("B")
	b.Super = a
	c := NewClass("C")
	c.Super = b

	if !IsSubclass(c, a) {
		t.Fatal("C should be a subclass of A three levels up")
	}
	if !IsSubclass(c, c) {
		t.Fatal("a class should be a subclass of itself")
	}
	if IsSubclass(a, c) {
		t.Fatal("A should not be a subclass of C")
	}
}

func TestLookupFieldWalksAncestors(t *testing.T) {
	_, base, rect, _, _ := shapeHierarchy()
	base.Fields["name"] = &Field{Name: "name", Type: String, Owner: base}
	rect.Fields["width"] = &Field{Name: "width", Type: Float, Owner: rect}

	if f := rect.LookupField("name"); f == nil || f.Owner != base {
		t.Fatalf("inherited field not found: %#v", f)
	}
	if f := rect.LookupField("width"); f == nil || f.Owner != rect {
		t.Fatalf("own field not found: %#v", f)
	}
	if f := base.LookupField("width"); f != nil {
		t.Fatal("a superclass must not see a subclass field")
	}
}

func TestLookupMethodPrefersTheSubclassOverride(t *testing.T) {
	_, base, rect, _, _ := shapeHierarchy()
	base.Methods["describe"] = &Method{Name: "describe", Owner: base}
	base.Methods["area"] = &Method{Name: "area", Owner: base, Abstract: true}
	rect.Methods["area"] = &Method{Name: "area", Owner: rect}

	if m := rect.LookupMethod("area"); m == nil || m.Owner != rect {
		t.Fatalf("override not preferred: %#v", m)
	}
	if m := rect.LookupMethod("describe"); m == nil || m.Owner != base {
		t.Fatalf("inherited method not found: %#v", m)
	}
	if m := rect.LookupMethod("missing"); m != nil {
		t.Fatal("found a method that does not exist")
	}
}

// Spec gap 2: a class with no constructor inherits its nearest ancestor's.
func TestLookupCtorIsInherited(t *testing.T) {
	exc := NewClass("Exception")
	exc.Ctor = &Signature{Params: []Type{String}, ParamNames: []string{"message"}}
	sub := NewClass("NegativeAreaError")
	sub.Super = exc

	got := sub.LookupCtor()
	if got == nil || len(got.Params) != 1 || got.Params[0] != String {
		t.Fatalf("inherited ctor = %#v, want (String)", got)
	}

	own := NewClass("Own")
	own.Super = exc
	own.Ctor = &Signature{}
	if own.LookupCtor() != own.Ctor {
		t.Fatal("a declared ctor must win over the inherited one")
	}
}

func TestString(t *testing.T) {
	shape, _, _, _, _ := shapeHierarchy()
	tests := []struct {
		typ  Type
		want string
	}{
		{Int, "Int"},
		{Void, "Void"},
		{&List{Elem: shape}, "[Shape]"},
		{&Map{Key: String, Val: Int}, "[String: Int]"},
		{&List{Elem: &Map{Key: String, Val: Int}}, "[[String: Int]]"},
		{&Signature{Params: []Type{Int, Float}, Result: Bool}, "(Int, Float): Bool"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPredicates(t *testing.T) {
	shape, _, rect, _, _ := shapeHierarchy()
	if !IsNumeric(Int) || !IsNumeric(Float) || IsNumeric(String) || IsNumeric(Bool) {
		t.Fatal("IsNumeric is wrong")
	}
	if !IsOrdered(String) || !IsOrdered(Char) || IsOrdered(Bool) {
		t.Fatal("IsOrdered is wrong")
	}
	if !IsReference(rect) || !IsReference(shape) || IsReference(Int) {
		t.Fatal("IsReference is wrong")
	}
	if IsReference(&List{Elem: Int}) {
		t.Fatal("a list is not a reference type for default-value purposes")
	}
}
