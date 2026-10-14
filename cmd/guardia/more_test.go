package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KlGamers/guardia/internal/gitx"
	"github.com/KlGamers/guardia/internal/testgit"
)

func TestDetectMergesWithConfig(t *testing.T) {
	dir := testgit.New(t)
	testgit.Write(t, dir, "go.mod", "module x\n")
	testgit.Write(t, dir, "guardia.json", `{"detect":true,"checks":[{"name":"gofmt","run":"echo custom"},{"name":"extra","run":"true"}]}`)
	code, out := in(t, dir, "list")
	if code != 0 || strings.Count(out, "gofmt") != 1 || !strings.Contains(out, "echo custom") || !strings.Contains(out, "go vet") || !strings.Contains(out, "extra") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestListEmptyAndOutsideRepo(t *testing.T) {
	dir := testgit.New(t)
	if code, out := in(t, dir, "list"); code != 0 || !strings.Contains(out, "no checks") {
		t.Fatalf("%d %s", code, out)
	}
	outside := t.TempDir()
	for _, cmd := range []string{"init", "uninstall", "list"} {
		if code, out := in(t, outside, cmd); code != 2 || !strings.Contains(out, "not inside a git repository") {
			t.Errorf("%s: %d %s", cmd, code, out)
		}
	}
	if code, out := in(t, dir, "uninstall"); code != 2 || !strings.Contains(out, "no pre-commit hook") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestBadFlags(t *testing.T) {
	dir := repo(t)
	for _, args := range [][]string{{"init", "--nope"}, {"run", "--nope"}, {"ci", "-p", "x"}} {
		if code, _ := in(t, dir, args...); code != 2 {
			t.Errorf("%v: got %d", args, code)
		}
	}
}

func TestParallelFlagAndVerbose(t *testing.T) {
	dir := repo(t)
	testgit.Write(t, dir, "a.txt", "OK\nfine\n")
	testgit.Git(t, dir, "add", "a.txt")
	code, out := in(t, dir, "run", "-p", "1", "-v")
	if code != 0 || !strings.Contains(out, "all checks passed") {
		t.Fatalf("%d %s", code, out)
	}
	if !strings.Contains(out, "1 staged file") || !strings.Contains(out, "2 checks") {
		t.Fatalf("plural: %s", out)
	}
}

func TestNoIsolationDuringMerge(t *testing.T) {
	dir := repo(t)
	testgit.Write(t, dir, "a.txt", "OK\nclean\n")
	testgit.Git(t, dir, "add", "a.txt")
	testgit.Write(t, dir, "a.txt", "OK\nclean\nTODO unstaged\n")
	p, _ := gitx.Path(dir, "MERGE_HEAD")
	os.WriteFile(p, []byte("x"), 0o644)
	if code, _ := in(t, dir, "run"); code != 1 {
		t.Fatal("during a merge the working tree is checked as is")
	}
	if got := testgit.Read(t, dir, "a.txt"); !strings.Contains(got, "TODO unstaged") {
		t.Fatal("working tree changed during merge")
	}
}

func TestLeftoverPatchStopsRun(t *testing.T) {
	dir := repo(t)
	testgit.Write(t, dir, "a.txt", "OK\nx\n")
	testgit.Git(t, dir, "add", "a.txt")
	testgit.Write(t, dir, "a.txt", "OK\nx\ny\n")
	p, _ := gitx.Path(dir, "guardia-unstaged.patch")
	os.WriteFile(p, []byte("old"), 0o600)
	if code, out := in(t, dir, "run"); code != 2 || !strings.Contains(out, "previous run") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestTTY(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if tty(f) {
		t.Fatal("regular file is not a terminal")
	}
	t.Setenv("NO_COLOR", "1")
	if tty(os.Stdout) {
		t.Fatal("NO_COLOR must disable color")
	}
	if tty(&strings.Builder{}) {
		t.Fatal("non-file writer")
	}
}
