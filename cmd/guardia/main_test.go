package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KlGamers/guardia/internal/testgit"
)

const cfg = `{"checks":[
 {"name":"no-todo","run":"! grep -n TODO {files}","glob":["*.txt"]},
 {"name":"has-ok","run":"for f in {files}; do grep -q OK \"$f\" || exit 1; done","glob":["*.txt"]}
]}`

func repo(t *testing.T) string {
	t.Helper()
	dir := testgit.New(t)
	testgit.Write(t, dir, "guardia.json", cfg)
	testgit.Write(t, dir, "a.txt", "OK\n")
	testgit.Git(t, dir, "add", ".")
	testgit.Git(t, dir, "commit", "-qm", "init")
	return dir
}

func in(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	var out, errb bytes.Buffer
	code := Main(args, &out, &errb)
	return code, out.String() + errb.String()
}

func TestRunPassFailAndNothing(t *testing.T) {
	dir := repo(t)
	if code, out := in(t, dir, "run"); code != 0 || !strings.Contains(out, "nothing to check") {
		t.Fatalf("%d %s", code, out)
	}
	testgit.Write(t, dir, "a.txt", "OK\nmore\n")
	testgit.Git(t, dir, "add", "a.txt")
	if code, out := in(t, dir, "run"); code != 0 || !strings.Contains(out, "all checks passed") {
		t.Fatalf("%d %s", code, out)
	}
	testgit.Write(t, dir, "a.txt", "OK\nTODO fix\n")
	testgit.Git(t, dir, "add", "a.txt")
	code, out := in(t, dir, "run")
	if code != 1 || !strings.Contains(out, "✗ no-todo") || !strings.Contains(out, "2:TODO fix") || !strings.Contains(out, "commit cancelled") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestChecksSeeOnlyStagedContent(t *testing.T) {
	dir := repo(t)
	testgit.Write(t, dir, "a.txt", "OK\nclean\n")
	testgit.Git(t, dir, "add", "a.txt")
	testgit.Write(t, dir, "a.txt", "OK\nclean\nTODO not staged yet\n")
	if code, out := in(t, dir, "run"); code != 0 {
		t.Fatalf("unstaged TODO must not fail the commit: %d %s", code, out)
	}
	if got := testgit.Read(t, dir, "a.txt"); got != "OK\nclean\nTODO not staged yet\n" {
		t.Fatalf("unstaged work lost: %q", got)
	}
	if code, _ := in(t, dir, "run", "--no-isolate"); code != 1 {
		t.Fatal("--no-isolate should see the working tree")
	}

	testgit.Write(t, dir, "a.txt", "TODO staged\n")
	testgit.Git(t, dir, "add", "a.txt")
	testgit.Write(t, dir, "a.txt", "OK fixed but unstaged\n")
	if code, _ := in(t, dir, "run"); code != 1 {
		t.Fatal("staged TODO must fail even if the working copy is fixed")
	}
	if got := testgit.Read(t, dir, "a.txt"); got != "OK fixed but unstaged\n" {
		t.Fatalf("working copy not restored after failure: %q", got)
	}
}

func TestCIAndList(t *testing.T) {
	dir := repo(t)
	if code, out := in(t, dir, "ci"); code != 0 || !strings.Contains(out, "tracked files") {
		t.Fatalf("%d %s", code, out)
	}
	testgit.Write(t, dir, "b.txt", "no marker\n")
	testgit.Git(t, dir, "add", "b.txt")
	testgit.Git(t, dir, "commit", "-qm", "b", "--no-verify")
	if code, out := in(t, dir, "ci"); code != 1 || strings.Contains(out, "commit cancelled") {
		t.Fatalf("%d %s", code, out)
	}
	code, out := in(t, dir, "list")
	if code != 0 || !strings.Contains(out, "no-todo") || !strings.Contains(out, "for f in {files}") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestUsageAndErrors(t *testing.T) {
	if code, _ := in(t, t.TempDir()); code != 2 {
		t.Fatal("no args")
	}
	if code, out := in(t, t.TempDir(), "frobnicate"); code != 2 || !strings.Contains(out, "unknown command") {
		t.Fatal(out)
	}
	if code, out := in(t, t.TempDir(), "run"); code != 2 || !strings.Contains(out, "not inside a git repository") {
		t.Fatal(out)
	}
	dir := repo(t)
	testgit.Write(t, dir, "guardia.json", "{broken")
	if code, out := in(t, dir, "run"); code != 2 || !strings.Contains(out, "guardia.json") {
		t.Fatal(out)
	}
	if code, out := in(t, dir, "version"); code != 0 || !strings.Contains(out, "guardia") {
		t.Fatal(out)
	}
	if code, _ := in(t, dir, "help"); code != 0 {
		t.Fatal("help")
	}
}

func TestWriteConfig(t *testing.T) {
	dir := testgit.New(t)
	testgit.Write(t, dir, "go.mod", "module x\n")
	code, out := in(t, dir, "init", "--write-config")
	if code != 0 || !strings.Contains(out, "with 3 checks") {
		t.Fatalf("%d %s", code, out)
	}
	if !strings.Contains(testgit.Read(t, dir, "guardia.json"), `"gofmt -l {files}"`) {
		t.Fatal("config content")
	}
	if code, _ := in(t, dir, "init", "--write-config"); code != 2 {
		t.Fatal("must not overwrite guardia.json")
	}
	if code, out := in(t, dir, "uninstall"); code != 0 || !strings.Contains(out, "removed") {
		t.Fatal(out)
	}
}

func TestRealCommitThroughHook(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "guardia")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	dir := repo(t)
	cmd := exec.Command(bin, "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	commit := func(msg string) (string, error) {
		c := exec.Command("git", "commit", "-qm", msg)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "NO_COLOR=1")
		out, err := c.CombinedOutput()
		return string(out), err
	}
	testgit.Write(t, dir, "a.txt", "OK\nTODO\n")
	testgit.Git(t, dir, "add", "a.txt")
	out, err := commit("bad")
	if err == nil || !strings.Contains(out, "commit cancelled") {
		t.Fatalf("broken commit went through: %v\n%s", err, out)
	}
	if n := strings.TrimSpace(testgit.Git(t, dir, "rev-list", "--count", "HEAD")); n != "1" {
		t.Fatalf("commits: %s", n)
	}
	testgit.Write(t, dir, "a.txt", "OK\ndone\n")
	testgit.Git(t, dir, "add", "a.txt")
	if out, err := commit("good"); err != nil {
		t.Fatalf("good commit rejected: %v\n%s", err, out)
	}
	if n := strings.TrimSpace(testgit.Git(t, dir, "rev-list", "--count", "HEAD")); n != "2" {
		t.Fatalf("commits: %s", n)
	}
}
