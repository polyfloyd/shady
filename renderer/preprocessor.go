package renderer

import (
	"io/ioutil"
	"path/filepath"
	"regexp"
)

var ppIncludeRe = regexp.MustCompile(`(?im)^#pragma\s+use\s+"([^"]+)"$`)

// Includes recursively resolves dependencies in the specified file.
//
// The argument file is returned included in the returned list of files.
func Includes(filenames ...string) ([]string, error) {
	return processRecursive(filenames, []string{})
}

func processRecursive(filenames []string, sources []string) ([]string, error) {
	for _, filename := range filenames {
		absFilename, err := filepath.Abs(filename)
		if err != nil {
			return nil, err
		}
		currentFile := absFilename
		shaderSource, err := ioutil.ReadFile(currentFile)
		if err != nil {
			return nil, err
		}

		// We need to check for recursion using a set that includes the current
		// file. But we need to append the current file after all included sources
		// in the list of files. Create a new temporary set of included source
		// files for the recursion check.
		checkset := append(sources, currentFile)

		// Check for files being included in the current file so we can later
		// recurse into all of them.
		includeMatches := ppIncludeRe.FindAllSubmatch(shaderSource, -1)
		includes := make([]string, 0, len(includeMatches))
	outer:
		for _, submatch := range includeMatches {
			includedFile := string(submatch[1])
			if !filepath.IsAbs(includedFile) {
				includedFile = filepath.Join(filepath.Dir(absFilename), includedFile)
			} else {
				includedFile = filepath.Clean(includedFile)
			}

			// Check whether we have already included the referred file. This stops
			// infinite recursions.
			for _, inc := range checkset {
				if inc == includedFile {
					continue outer
				}
			}
			includes = append(includes, includedFile)
		}

		sources, err = processRecursive(includes, sources)
		if err != nil {
			return nil, err
		}
		sources = append(sources, currentFile)
	}

	return sources, nil
}
