package detect

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/KlGamers/guardia/internal/conf"
)

var skip = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "venv": true, "vendor": true,
	"build": true, "dist": true, "target": true, ".dart_tool": true, "__pycache__": true,
	".next": true, "Pods": true,
}

const depth = 2

func Scan(root string) ([]conf.Check, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if rel != "." && (skip[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if rel != "." && strings.Count(filepath.ToSlash(rel), "/") >= depth {
			return filepath.SkipDir
		}
		dirs = append(dirs, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	var out []conf.Check
	var vy []string
	for _, rel := range dirs {
		cks := inDir(root, rel, under(rel, vy))
		for _, ck := range cks {
			if strings.HasSuffix(ck.Name, "vyper") {
				vy = append(vy, rel)
			}
		}
		out = append(out, cks...)
	}
	return out, nil
}

func under(rel string, roots []string) bool {
	for _, r := range roots {
		if r == "." || strings.HasPrefix(filepath.ToSlash(rel)+"/", filepath.ToSlash(r)+"/") {
			return true
		}
	}
	return false
}

func inDir(root, rel string, noVy bool) []conf.Check {
	abs := filepath.Join(root, rel)
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(abs, name))
		return err == nil
	}
	var cks []conf.Check
	add := func(name, run, need string, failOut bool, globs ...string) {
		cks = append(cks, conf.Check{Name: name, Run: run, Need: need, FailOut: failOut, Glob: globs})
	}

	if has("go.mod") {
		add("gofmt", "gofmt -l {files}", "gofmt", true, "*.go")
		add("go vet", "go vet ./...", "go", false, "*.go", "go.mod", "go.sum")
		add("go test", "go test ./...", "go", false, "*.go", "go.mod", "go.sum")
	}
	if has("pyproject.toml") || has("setup.py") || has("requirements.txt") || has("requirements-dev.txt") {
		add("ruff check", "ruff check {files}", "ruff", false, "*.py")
		add("ruff format", "ruff format --check {files}", "ruff", false, "*.py")
		if has("tests") || has("test") || globAny(abs, "test_*.py") {
			add("pytest", "pytest -q", "pytest", false, "*.py", "*.vy", "pyproject.toml")
		}
	}
	if has("package.json") {
		for _, sc := range scripts(filepath.Join(abs, "package.json")) {
			add("npm "+sc, "npm run --silent "+sc, "npm", false,
				"*.js", "*.jsx", "*.ts", "*.tsx", "*.mjs", "*.cjs", "*.vue", "*.svelte", "*.css", "*.json")
		}
	}
	if has("Cargo.toml") {
		add("cargo fmt", "cargo fmt --all -- --check", "cargo", false, "*.rs")
		add("cargo clippy", "cargo clippy --all-targets -- -D warnings", "cargo", false, "*.rs", "Cargo.toml")
		add("cargo test", "cargo test", "cargo", false, "*.rs", "Cargo.toml")
	}
	if has("pubspec.yaml") {
		add("dart format", "dart format --output=none --set-exit-if-changed {files}", "dart", false, "*.dart")
		tool := "dart"
		if flutter(filepath.Join(abs, "pubspec.yaml")) {
			tool = "flutter"
		}
		add(tool+" analyze", tool+" analyze", tool, false, "*.dart", "pubspec.yaml", "analysis_options.yaml")
		if has("test") {
			add(tool+" test", tool+" test", tool, false, "*.dart", "pubspec.yaml")
		}
	}
	if has(".clang-format") {
		add("clang-format", "clang-format --dry-run -Werror {files}", "clang-format", false,
			"*.c", "*.cc", "*.cpp", "*.cxx", "*.h", "*.hh", "*.hpp")
	}
	if !noVy && (globAny(abs, "*.vy") || globAny(filepath.Join(abs, "contracts"), "*.vy")) {
		add("vyper", "vyper -f abi {files} > /dev/null", "vyper", false, "*.vy")
	}

	for i := range cks {
		if rel != "." {
			cks[i].Dir = filepath.ToSlash(rel)
			cks[i].Name = filepath.ToSlash(rel) + ": " + cks[i].Name
		}
	}
	return cks
}

func globAny(dir, pat string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, pat))
	return len(m) > 0
}

func scripts(p string) []string {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return nil
	}
	var out []string
	for _, sc := range []string{"lint", "format:check", "typecheck", "test"} {
		if body, ok := pkg.Scripts[sc]; ok && !strings.Contains(body, "no test specified") {
			out = append(out, sc)
		}
	}
	return out
}

func flutter(p string) bool {
	raw, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	for _, ln := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(ln) == "sdk: flutter" {
			return true
		}
	}
	return false
}
