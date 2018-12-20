package renderer

import (
	"testing"
)

func TestPlain(t *testing.T) {
	sources, err := Includes("../testdata/preprocessor/include-none.glsl")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("unexpected number of sources: exp %v, got %v", 1, len(sources))
	}
}

func TestIncludeSingle(t *testing.T) {
	sources, err := Includes("../testdata/preprocessor/include-single.glsl")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("unexpected number of sources: exp %v, got %v", 2, len(sources))
	}
}

func TestIncludeRecursive(t *testing.T) {
	sources, err := Includes("../testdata/preprocessor/include-recursive.glsl")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 3 {
		t.Fatalf("unexpected number of sources: exp %v, got %v", 3, len(sources))
	}
}

func TestStopRecursionCycle(t *testing.T) {
	sources, err := Includes("../testdata/preprocessor/include-cycle.glsl")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("unexpected number of sources: exp %v, got %v", 1, len(sources))
	}
}
