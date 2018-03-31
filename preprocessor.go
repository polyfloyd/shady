package glsl

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

var ppIncludeRe = regexp.MustCompile(`(?im)^#pragma\s+use\s+"([^"]+)"$`)

type includedSource struct {
	filename string
	source   string
}

func processRecursive(filename string, sources []includedSource) ([]includedSource, error) {
	// Read the current file.
	shaderSourceFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer shaderSourceFile.Close()
	shaderSource, err := ioutil.ReadAll(shaderSourceFile)
	if err != nil {
		return nil, err
	}

	currentFile := includedSource{
		filename: filename,
		source:   string(shaderSource),
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
			if inc.filename == includedFile {
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

func ProcessSourceFile(filename string) ([]string, error) {
	absFilename, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	sourceMap, err := processRecursive(absFilename, []includedSource{})
	if err != nil {
		return nil, err
	}
	sources := make([]string, 0, len(sourceMap))
	for _, inc := range sourceMap {
		sources = append(sources, inc.source)
	}
	return sources, nil
}
