package renderer

import (
	"testing"
)

func TestSingleFileMarkerPosition(t *testing.T) {
	initTestGL(t)

	source := SourceBuf(`
void main() {
	#error meh
}
	`)

	_, err := compileShader(StageVertex, source)
	cerr := err.(CompileError)

	t.Logf("\n%s\n", cerr.log)

	m := cerr.markers()
	if len(m) == 0 {
		t.Fatalf("Expected at least one error marker")
	}
	if m[0].fileno != 0 {
		t.Fatalf("Unexpected fileno")
	}
	if m[0].lineno != 3 {
		t.Fatalf("Unexpected lineno")
	}
}

func TestMultiFileMarkerPosition(t *testing.T) {
	initTestGL(t)

	source1 := SourceBuf(`
// A bunch of text to offset the line number.
	`)
	source2 := SourceBuf(`
void main() {
	#error meh
}
	`)

	_, err := compileShader(StageVertex, source1, source2)
	cerr := err.(CompileError)

	t.Logf("\n%s\n", cerr.log)

	m := cerr.markers()
	if len(m) == 0 {
		t.Fatalf("Expected at least one error marker")
	}
	if m[0].fileno != 1 {
		t.Fatalf("Unexpected fileno")
	}
	if m[0].lineno != 3 {
		t.Fatalf("Unexpected lineno")
	}
}
