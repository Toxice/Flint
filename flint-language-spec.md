# Flint — A Language Specification

## Overview

Flint pairs Java's structural discipline — classes, interfaces, strict typing — with Python's lack of ceremony. Every declaration states its type explicitly (no inference), but nothing else demands boilerplate: no forced getters/setters, no checked exceptions, no diamond-operator generics.

Design goals:

- Read like Java in structure, read like Python in weight
- Type annotations are mandatory everywhere: variables, parameters, return types, fields
- Cut Java's ceremony — no `public static void main(String[] args)` ritual, no required access modifiers, unchecked exceptions
- Stay scoped to core OOP and typing; no generics beyond built-in collections, no concurrency, no pattern matching

This is a design exercise: a full specification and worked pseudo-code, not a working compiler or interpreter.

## Type System

**Primitives:** `Int`, `Float`, `Bool`, `String`, `Char`, `Void` (no return value).

**Annotations are mandatory** on every variable, parameter, return type, and field — there is no type inference. Syntax is Python-style, `name: Type`, rather than Java's `Type name`:

```
let age: Int = 21
let label: String = "active"
```

**Collections use bracket types, not `<>`:**

- `[Type]` for a list, e.g. `[Shape]` instead of `List<Shape>`
- `[Key: Value]` for a map, e.g. `[String: Int]` instead of `Map<String, Int>`

Collection literals use the same brackets:

```
let shapes: [Shape] = [Rectangle("Rect1", 4.0, 5.0), Circle("Circ1", 3.0)]
let counts: [String: Int] = ["a": 1, "b": 2]
```

Only these built-in parameterized types exist — there is no user-defined generics system. That keeps the type system's scope to what a class hierarchy and a couple of collections need, without a full generics feature.

## Classes and Interfaces

Fields require type annotations, and the constructor is a method with the same name as the class:

```
class Dog {
    name: String

    Dog(name: String) { object.name = name }
}
```

**`object` refers to the current instance** — Flint's replacement for Java's `this` and Python's `self`.

**Inheritance and interface conformance share one syntax** — a single `:` list, rather than Java's separate `extends`/`implements` keywords:

```
interface Shape {
    func area(): Float
}

class Rectangle : Shape {
    width: Float
    height: Float

    Rectangle(width: Float, height: Float) {
        object.width = width
        object.height = height
    }

    func area(): Float { return object.width * object.height }
}
```

A class can list a parent class and any number of interfaces together: `class Circle : BaseShape, Comparable { ... }`.

**Visibility:** members are `public` by default — no need to write it. Mark a member `private` only when it should be hidden. There's no forced getter/setter boilerplate; direct field access is fine unless a field is marked `private`.

**Abstract members:** `abstract class` and `abstract func` declare a method with no body, to be filled in by a subclass.

## Functions and Control Flow

Functions are declared with `func`, and always state a return type (`Void` if there isn't one):

```
func add(a: Int, b: Int): Int {
    return a + b
}
```

`if`, `while`, and `for` all drop the parentheses around their condition but keep braces around the body — lighter than Java, still visually structured:

```
if x > 5 {
    print("big")
}

for (shape: Shape in shapes) {
    print(shape.area())
}
```

## Error Handling

Exception handling looks like Java's `try`/`catch`, and custom exceptions are ordinary classes that inherit from `Exception`:

```
class NegativeAreaError : Exception { }

try {
    let a: Float = shape.area()
} catch (e: NegativeAreaError) {
    print("Error: " + e.message)
}
```

**Exceptions are unchecked** — there's no `throws` declaration on a function signature. Checked exceptions are one of Java's biggest sources of ceremony (every function up the call chain has to declare or handle them), and dropping that requirement is one of the more deliberate departures from Java in this design.

## Sample Program

A small shape hierarchy that exercises every feature above: interfaces, abstract classes, single-colon inheritance, the `object` keyword, bracket-typed collections, and unchecked exceptions.

```
interface Shape {
    func area(): Float
    func describe(): String
}

abstract class BaseShape : Shape {
    name: String

    BaseShape(name: String) { object.name = name }

    func describe(): String {
        return object.name + " has area " + object.area()
    }
}

class Rectangle : BaseShape {
    width: Float
    height: Float

    Rectangle(name: String, width: Float, height: Float) {
        object.width = width
        object.height = height
    }

    func area(): Float { return object.width * object.height }
}

class Circle : BaseShape {
    radius: Float

    Circle(name: String, radius: Float) { object.radius = radius }

    func area(): Float { return 3.14159 * object.radius * object.radius }
}

class NegativeAreaError : Exception { }

func totalArea(shapes: [Shape]): Float {
    let total: Float = 0.0
    for (shape: Shape in shapes) {
        let a: Float = shape.area()
        if a < 0 {
            throw NegativeAreaError("Area cannot be negative")
        }
        total = total + a
    }
    return total
}

func main(): Void {
    let shapes: [Shape] = [Rectangle("Rect1", 4.0, 5.0), Circle("Circ1", 3.0)]

    try {
        print(totalArea(shapes))
    } catch (e: NegativeAreaError) {
        print("Error: " + e.message)
    }
}
```
