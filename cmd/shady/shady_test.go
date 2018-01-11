package main

import (
	"os"
	"testing"
)

func TestParseGeometry(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		valid := map[string]struct {
			w, h uint
		}{
			"1x2":   {w: 1, h: 2},
			"2x1":   {w: 2, h: 1},
			"30x90": {w: 30, h: 90},
			"env":   {w: 150, h: 16},
		}
		os.Setenv("LEDCAT_GEOMETRY", "150x16")

		for input, expected := range valid {
			w, h, err := parseGeometry(input)
			if err != nil {
				t.Errorf("error parsing valid geometry %q: %v", input, err)
			}
			if w != expected.w || h != expected.h {
				t.Errorf("mismatched result (%d, %d), expected (%d, %d)", w, h, expected.w, expected.h)
			}
		}
	})

	t.Run("invalid", func(t *testing.T) {
		invalid := []string{
			"0x2",
			"2x0",
			"-1x1",
			"1x-1",
			"",
			" ",
			"x",
			"fooxbar",
			"lalala",
		}

		for _, input := range invalid {
			_, _, err := parseGeometry(input)
			if err == nil {
				t.Errorf("expected an error while parsing invalid geometry %q", input)
			}
		}
	})
}
