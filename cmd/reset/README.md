# reset

Run the generator from the repository root with:

```sh
go run ./cmd/reset
```

It scans Go packages below the current module for structs marked directly on a type declaration or type specification:

```go
// generate:reset
type Request struct {
	Name string
}
```

For each package with marked structs, the command writes `reset.gen.go` next to the source. The generated `Reset` method safely handles nil receivers. Primitive fields become zero values, slices retain their capacity with length set to zero, maps are cleared, and interfaces, channels, and functions become nil. Pointers are checked before their values are reset. Nested structs use an accessible `Reset` method when available; local structs without one are reset field by field. Arrays are reset element by element.

Unsupported field types, external structs without an accessible `Reset` method, and generic marked structs produce an error instead of invalid generated code. Generated files are formatted and rewritten only when their contents change.
