package gitx

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/KlGamers/guardia/internal/testgit"
)

func TestRootAndFiles(t *testing.T) {
	dir := testgit.New(t)
	if _, err := Root(t.TempDir()); err == nil {
		t.Fatal("want error outside a repo")
	}
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	root, err := Root(sub)
	if err != nil {
		t.Fatal(err)
	}
	if real, _ := filepath.EvalSymlinks(dir); root != real && root != dir {
		t.Fatalf("root %q, want %q", root, dir)
	}

	testgit.Write(t, dir, "keep.txt", "a")
	testgit.Write(t, dir, "gone.txt", "a")
	testgit.Write(t, dir, "old name.txt", "same content here")
	testgit.Git(t, dir, "add", ".")
	testgit.Git(t, dir, "commit", "-qm", "init")

	testgit.Write(t, dir, "keep.txt", "b")
	testgit.Write(t, dir, "nuevo archivo ñ.go", "package x")
	testgit.Git(t, dir, "rm", "-q", "gone.txt")
	testgit.Git(t, dir, "mv", "old name.txt", "new name.txt")
	testgit.Git(t, dir, "add", ".")
	testgit.Write(t, dir, "untracked.txt", "x")

	got, err := Staged(dir)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"keep.txt", "new name.txt", "nuevo archivo ñ.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staged %q", got)
	}
	all, _ := Tracked(dir)
	if len(all) != 3 {
		t.Fatalf("tracked %q", all)
	}
}

func setup(t *testing.T) string {
	t.Helper()
	dir := testgit.New(t)
	testgit.Write(t, dir, "a.txt", "one\n")
	testgit.Write(t, dir, "b.txt", "keep\n")
	testgit.Write(t, dir, "bin.dat", "\x00\x01\x02")
	testgit.Git(t, dir, "add", ".")
	testgit.Git(t, dir, "commit", "-qm", "init")
	return dir
}

func TestHideRestorePartialStaging(t *testing.T) {
	dir := setup(t)
	testgit.Write(t, dir, "a.txt", "one\nstaged\n")
	testgit.Git(t, dir, "add", "a.txt")
	testgit.Write(t, dir, "a.txt", "one\nstaged\nunstaged\n")
	os.Remove(filepath.Join(dir, "b.txt"))
	testgit.Write(t, dir, "bin.dat", "\x00\x09\x09\x09")
	testgit.Write(t, dir, "new.txt", "untracked\n")

	st, err := Hide(dir)
	if err != nil || st == nil {
		t.Fatalf("hide: %v %v", st, err)
	}
	if got := testgit.Read(t, dir, "a.txt"); got != "one\nstaged\n" {
		t.Fatalf("during check a.txt = %q", got)
	}
	if got := testgit.Read(t, dir, "b.txt"); got != "keep\n" {
		t.Fatalf("deleted file not restored for the check: %q", got)
	}
	if got := testgit.Read(t, dir, "new.txt"); got != "untracked\n" {
		t.Fatal("untracked file touched")
	}
	if err := st.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := testgit.Read(t, dir, "a.txt"); got != "one\nstaged\nunstaged\n" {
		t.Fatalf("after restore a.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("deletion not restored")
	}
	if got := testgit.Read(t, dir, "bin.dat"); got != "\x00\x09\x09\x09" {
		t.Fatalf("binary not restored: %q", got)
	}
	if out := testgit.Git(t, dir, "diff", "--cached", "--name-only"); strings.TrimSpace(out) != "a.txt" {
		t.Fatalf("index changed: %q", out)
	}
	p, _ := Path(dir, "guardia-unstaged.patch")
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("patch left behind")
	}
}

func TestHideNothingAndLeftover(t *testing.T) {
	dir := setup(t)
	st, err := Hide(dir)
	if err != nil || st != nil {
		t.Fatalf("clean tree: %v %v", st, err)
	}
	if err := st.Restore(); err != nil {
		t.Fatal(err)
	}
	p, _ := Path(dir, "guardia-unstaged.patch")
	os.WriteFile(p, []byte("x"), 0o600)
	testgit.Write(t, dir, "a.txt", "changed\n")
	if _, err := Hide(dir); err == nil || !strings.Contains(err.Error(), "previous run") {
		t.Fatalf("want leftover error, got %v", err)
	}
	if got := testgit.Read(t, dir, "a.txt"); got != "changed\n" {
		t.Fatal("touched the tree despite the error")
	}
}

func TestRestoreConflictKeepsPatch(t *testing.T) {
	dir := setup(t)
	testgit.Write(t, dir, "a.txt", "one\nmine\n")
	st, err := Hide(dir)
	if err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, dir, "a.txt", "totally different\n")
	if err := st.Restore(); err == nil || !strings.Contains(err.Error(), "saved in") {
		t.Fatalf("want conflict error, got %v", err)
	}
	p, _ := Path(dir, "guardia-unstaged.patch")
	if _, err := os.Stat(p); err != nil {
		t.Fatal("patch must be kept on failure")
	}
}

func TestBusy(t *testing.T) {
	dir := setup(t)
	if Busy(dir) {
		t.Fatal("clean repo reported busy")
	}
	p, _ := Path(dir, "MERGE_HEAD")
	os.WriteFile(p, []byte("x"), 0o644)
	if !Busy(dir) {
		t.Fatal("merge not detected")
	}
}

func TestErrorsOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Path(dir, "x"); err == nil {
		t.Error("path")
	}
	if _, err := Staged(dir); err == nil {
		t.Error("staged")
	}
	if _, err := Hide(dir); err == nil {
		t.Error("hide")
	}
	if Busy(dir) {
		t.Error("busy outside a repo")
	}
}
