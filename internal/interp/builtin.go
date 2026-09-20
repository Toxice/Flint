package interp

import "fmt"

// callBuiltin runs print or len.
//
// Neither is declared in the spec, which nonetheless calls print throughout;
// len is the minimum needed to make lists, maps, and strings usable. The
// checker has already validated the argument count and type.
func (i *Interp) callBuiltin(name string, args []Value) Value {
	switch name {
	case "print":
		fmt.Fprintln(i.out, Stringify(args[0]))
		return Unit{}

	case "len":
		switch a := args[0].(type) {
		case *ListVal:
			return Int(len(a.Elems))
		case *MapVal:
			return Int(a.Len())
		case Str:
			return Int(len([]rune(a)))
		case Uninit:
			i.throwBuiltin("NullError", "len of an uninitialized value")
		}
		i.throwBuiltin("NullError", "len does not apply to this value")
	}
	panic("interp: unknown builtin " + name)
}
