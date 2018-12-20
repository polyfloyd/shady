package renderer

import (
	"testing"
)

func TestUnknownVar(t *testing.T) {
	initTestGL(t)

	sources := SourceBuf(`
void main() {
	a = 12;
}
	`)

	_, err := compileShader(StageVertex, sources)
	compileError, ok := err.(CompileError)
	if !ok {
		t.Fatalf("expected a CompileError, got %#v", err)
	}

	_ = compileError
}
