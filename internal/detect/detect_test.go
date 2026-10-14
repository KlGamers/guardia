package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func touch(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, root string) []string {
	t.Helper()
	cks, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ck := range cks {
		out = append(out, ck.Name)
	}
	return out
}

func TestMonorepo(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "go.mod", "module x\n")
	touch(t, root, "backend/pyproject.toml", "")
	touch(t, root, "backend/tests/test_a.py", "")
	touch(t, root, "web/package.json", `{"scripts":{"lint":"eslint .","test":"echo \"Error: no test specified\" && exit 1","build":"vite"}}`)
	touch(t, root, "web/node_modules/dep/package.json", `{"scripts":{"lint":"x"}}`)
	touch(t, root, "app/pubspec.yaml", "name: app\ndependencies:\n  flutter:\n    sdk: flutter\n")
	touch(t, root, "app/test/a_test.dart", "")
	touch(t, root, "core/.clang-format", "")
	touch(t, root, "chain/contracts/Escrow.vy", "")
	touch(t, root, "a/b/c/go.mod", "")
	touch(t, root, ".hidden/go.mod", "")
	touch(t, root, "rs/Cargo.toml", "")

	want := []string{
		"gofmt", "go vet", "go test",
		"app: dart format", "app: flutter analyze", "app: flutter test",
		"backend: ruff check", "backend: ruff format", "backend: pytest",
		"chain: vyper",
		"core: clang-format",
		"rs: cargo fmt", "rs: cargo clippy", "rs: cargo test",
		"web: npm lint",
	}
	if got := names(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	cks, _ := Scan(root)
	for _, ck := range cks {
		if ck.Name == "backend: pytest" && ck.Dir != "backend" {
			t.Fatalf("dir not set: %+v", ck)
		}
		if ck.Name == "gofmt" && (!ck.FailOut || ck.Dir != "") {
			t.Fatalf("gofmt: %+v", ck)
		}
	}
}

func TestPlainDartAndPythonWithoutTests(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "pubspec.yaml", "name: lib\n")
	touch(t, root, "requirements.txt", "")
	got := names(t, root)
	want := []string{"ruff check", "ruff format", "dart format", "dart analyze"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestEmptyAndBrokenPackageJSON(t *testing.T) {
	root := t.TempDir()
	if got := names(t, root); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	touch(t, root, "package.json", "{not json")
	if got := names(t, root); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestUnreadableRoot(t *testing.T) {
	if _, err := Scan("/definitely/not/here"); err == nil {
		t.Fatal("want error for a missing root")
	}
	if scripts("/definitely/not/here/package.json") != nil || flutter("/definitely/not/here") {
		t.Fatal("missing files must be ignored")
	}
}
