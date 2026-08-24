package portscan

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// maxPackageJSONDepth limits how far up the tree we look for a package.json,
// so a stray file in $HOME does not label every unrelated process.
const maxPackageJSONDepth = 4

// ProjectName resolves a human readable project name for a working directory:
// the "name" field of the nearest package.json, or the directory name.
func ProjectName(cwd string) string {
	if cwd == "" {
		return ""
	}
	if name := packageJSONName(cwd); name != "" {
		return name
	}

	// System daemons usually run from "/", which is not a useful project name.
	base := filepath.Base(cwd)
	if base == "/" || base == "." {
		return ""
	}
	return base
}

// packageJSONName walks up from dir looking for a package.json with a name.
func packageJSONName(dir string) string {
	home, _ := os.UserHomeDir()

	for depth := 0; depth <= maxPackageJSONDepth; depth++ {
		if name := readPackageName(filepath.Join(dir, "package.json")); name != "" {
			return name
		}

		parent := filepath.Dir(dir)
		if parent == dir || dir == home || parent == string(filepath.Separator) {
			return ""
		}
		dir = parent
	}
	return ""
}

func readPackageName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Name
}
