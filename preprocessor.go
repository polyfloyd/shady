package glsl

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var ppIncludeRe = regexp.MustCompile(`(?im)^#pragma\s+use\s+"([^"]+)"$`)

type Source interface {
	Contents() ([]byte, error)
}

type SourceBuf string

func (s SourceBuf) Contents() ([]byte, error) {
	return []byte(s), nil
}

type SourceFile struct {
	Filename string
}

// Includes recursively resolves dependencies in the specified file.
//
// The argument file is returned included in the returned list of files.
func Includes(filename string) ([]SourceFile, error) {
	absFilename, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	return processRecursive(absFilename, []SourceFile{})
}

func (s SourceFile) Contents() ([]byte, error) {
	fd, err := os.Open(s.Filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	return ioutil.ReadAll(fd)
}

func processRecursive(filename string, sources []SourceFile) ([]SourceFile, error) {
	currentFile := SourceFile{
		Filename: filename,
	}
	shaderSource, err := currentFile.Contents()
	if err != nil {
		return nil, err
	}

	// We need to check for recursion using a set that includes the current
	// file. But we need to append the current file after all included sources
	// in the list of files. Create a new temporary set of included source
	// files for the recursion check.
	checkset := append(sources, currentFile)

	// Check for files being included in the current file and recurse into
	// them.
	includes := ppIncludeRe.FindAllSubmatch(shaderSource, -1)
outer:
	for _, submatch := range includes {
		includedFile := string(submatch[1])
		if !filepath.IsAbs(includedFile) {
			includedFile = filepath.Join(filepath.Dir(filename), includedFile)
		} else {
			includedFile = filepath.Clean(includedFile)
		}

		// Check whether we have already included the referred file. This stops
		// infinite recursions.
		for _, inc := range checkset {
			if inc.Filename == includedFile {
				continue outer
			}
		}

		sources, err = processRecursive(includedFile, sources)
		if err != nil {
			return nil, err
		}
	}

	return append(sources, currentFile), nil
}
