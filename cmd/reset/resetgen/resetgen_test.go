package resetgen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateResetMethodsAndRunThem(t *testing.T) {
	root := newProject(t, map[string]string{
		"child/child.go": `package child

type Child struct { Count int }

func (c *Child) Reset() { c.Count = 0 }
`,
		"fixture/types.go": `package fixture

import "example.com/resetfixture/child"

type Count int

type Child struct {
	Name string
}

func (c *Child) Reset() { c.Name = "" }

type nested struct {
	Text string
	values map[string]int
}

type Embedded struct {
	Ready bool
}

// generate:reset
type Example struct {
	Count Count
	Alias intAlias
	Name *string
	PointerCount *Count
	Items []int
	Pointers []*Child
	Values map[string]int
	PointerMap *map[string]int
	Child Child
	ChildPointer *Child
	Nested nested
	NestedPointer *nested
	Array [2]Count
	PointerArray *[2]string
	ArrayPointers [2]*Count
	Embedded
	Value any
	External child.Child
	ExternalPointer *child.Child
}

type intAlias = int

// generate:reset
type Another struct { Label string }
`,
		"fixture/reset_test.go": `package fixture

import (
	"testing"
	"example.com/resetfixture/child"
)

func TestReset(t *testing.T) {
	name := "name"
	count := Count(12)
	values := map[string]int{"x": 1}
	pointerValues := map[string]int{"y": 2}
	childPointer := &Child{Name: "child"}
	nestedPointer := &nested{Text: "nested", values: map[string]int{"z": 3}}
	array := &[2]string{"a", "b"}
	arrayCount := Count(8)
	externalPointer := &child.Child{Count: 88}
	value := &Example{
		Count: 9, Alias: 10, Name: &name, PointerCount: &count,
		Items: []int{1, 2}, Pointers: []*Child{childPointer}, Values: values,
		PointerMap: &pointerValues, Child: Child{Name: "inline"}, ChildPointer: childPointer,
		Nested: nested{Text: "nested", values: map[string]int{"n": 4}}, NestedPointer: nestedPointer,
		Array: [2]Count{1, 2}, PointerArray: array,
		ArrayPointers: [2]*Count{&arrayCount, nil}, Embedded: Embedded{Ready: true}, Value: 42,
		External: child.Child{Count: 77}, ExternalPointer: externalPointer,
	}
	itemsCap := cap(value.Items)
	pointersCap := cap(value.Pointers)
	value.Reset()
	if value.Count != 0 || value.Alias != 0 || *value.Name != "" || *value.PointerCount != 0 {
		t.Fatalf("primitive fields were not reset: %+v", value)
	}
	if len(value.Items) != 0 || cap(value.Items) != itemsCap || len(value.Pointers) != 0 || cap(value.Pointers) != pointersCap {
		t.Fatal("slice length or capacity was not preserved")
	}
	if value.Values == nil || len(value.Values) != 0 || value.PointerMap == nil || *value.PointerMap == nil || len(*value.PointerMap) != 0 {
		t.Fatal("map storage was not retained and cleared")
	}
	if value.Child.Name != "" || value.ChildPointer != childPointer || childPointer.Name != "" {
		t.Fatal("nested Reset methods were not called")
	}
	if value.Nested.Text != "" || value.Nested.values == nil || len(value.Nested.values) != 0 || nestedPointer.Text != "" || len(nestedPointer.values) != 0 {
		t.Fatal("nested fields without Reset were not reset recursively")
	}
	if value.Array != ([2]Count{}) || value.PointerArray != array || *array != ([2]string{}) || *value.ArrayPointers[0] != 0 {
		t.Fatal("array fields were not reset")
	}
	if value.Embedded.Ready || value.Value != nil {
		t.Fatal("embedded or interface field was not reset")
	}
	if value.External.Count != 0 || externalPointer.Count != 0 {
		t.Fatal("exported Reset methods from another package were not called")
	}
	value.Reset()
	var nilValue *Example
	nilValue.Reset()

	another := Another{Label: "label"}
	another.Reset()
	if another.Label != "" {
		t.Fatal("second generated type was not reset")
	}
}
`,
		"plain/plain.go": "package plain\n\n// This documentation mentions generate:reset but is not a directive.\ntype Unmarked struct{ Value string }\n",
	})

	if err := Generate(root); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	generatedPath := filepath.Join(root, "fixture", outputName)
	first, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if bytes.Contains(first, []byte("import")) {
		t.Fatalf("generated file contains unnecessary imports:\n%s", first)
	}
	if err := Generate(root); err != nil {
		t.Fatalf("second Generate returned error: %v", err)
	}
	second, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read generated file after second run: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("second generation changed reset.gen.go")
	}
	if _, err := os.Stat(filepath.Join(root, "plain", outputName)); !os.IsNotExist(err) {
		t.Fatalf("unmarked package reset.gen.go stat error = %v, want not-exist", err)
	}

	command := exec.Command("go", "test", "./...")
	command.Dir = root
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run generated code: %v\n%s", err, output)
	}
}

func TestGenerateReportsUnsupportedExternalStruct(t *testing.T) {
	root := newProject(t, map[string]string{
		"fixture/types.go": `package fixture

import "time"

// generate:reset
type Example struct { Created time.Time }
`,
	})
	err := Generate(root)
	if err == nil || !strings.Contains(err.Error(), "external struct Time has no accessible Reset method") {
		t.Fatalf("Generate error = %v, want unsupported external struct error", err)
	}
}

func TestGenerateReportsUnsupportedGenericStruct(t *testing.T) {
	root := newProject(t, map[string]string{
		"fixture/types.go": `package fixture

// generate:reset
type Example[T any] struct { Value T }
`,
	})
	err := Generate(root)
	if err == nil || !strings.Contains(err.Error(), "generic struct Example is not supported") {
		t.Fatalf("Generate error = %v, want unsupported generic struct error", err)
	}
}

func TestReadGeneratedMethods(t *testing.T) {
	dir := t.TempDir()

	methods, err := readGeneratedMethods(dir)
	if err != nil {
		t.Fatalf("read missing generated file: %v", err)
	}
	if len(methods) != 0 {
		t.Fatalf("methods for missing file = %v, want empty", methods)
	}

	generated := `// Code generated by cmd/reset; DO NOT EDIT.

package fixture

func (s *First) Reset() {}
func (s Second) Reset() {}
func Reset() {}
`
	if err := os.WriteFile(filepath.Join(dir, outputName), []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}
	methods, err = readGeneratedMethods(dir)
	if err != nil {
		t.Fatalf("read generated methods: %v", err)
	}
	if !methods["First"] || !methods["Second"] || len(methods) != 2 {
		t.Fatalf("methods = %v, want First and Second", methods)
	}
}

func TestGenerateMultiplePackages(t *testing.T) {
	root := newProject(t, map[string]string{
		"first/types.go": `package first

// generate:reset
type First struct { Value string }
`,
		"second/types.go": `package second

// generate:reset
type Second struct { Value int }
`,
	})

	if err := Generate(root); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	for _, pkg := range []string{"first", "second"} {
		contents, err := os.ReadFile(filepath.Join(root, pkg, outputName))
		if err != nil {
			t.Fatalf("read %s generated file: %v", pkg, err)
		}
		if got := bytes.Count(contents, []byte(" Reset()")); got != 1 {
			t.Fatalf("%s Reset methods = %d, want 1", pkg, got)
		}
	}

	if err := Generate(root); err != nil {
		t.Fatalf("second Generate returned error: %v", err)
	}
	for _, pkg := range []string{"first", "second"} {
		contents, err := os.ReadFile(filepath.Join(root, pkg, outputName))
		if err != nil {
			t.Fatalf("read %s generated file after second run: %v", pkg, err)
		}
		if got := bytes.Count(contents, []byte(" Reset()")); got != 1 {
			t.Fatalf("%s Reset methods after second run = %d, want 1", pkg, got)
		}
	}
}

func newProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/resetfixture\n\ngo 1.26.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
