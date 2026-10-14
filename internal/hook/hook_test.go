package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KlGamers/guardia/internal/testgit"
)

func TestInstallUninstall(t *testing.T) {
	dir := testgit.New(t)
	p, err := Install(dir, "/opt/my tools/guardia", false)
	if err != nil {
		t.Fatal(err)
	}
	raw := testgit.Read(t, dir, ".git/hooks/pre-commit")
	if !strings.Contains(raw, `'/opt/my tools/guardia' run`) || !Ours(p) {
		t.Fatalf("script:\n%s", raw)
	}
	if st, _ := os.Stat(p); st.Mode()&0o111 == 0 {
		t.Fatal("hook not executable")
	}
	if _, err := Install(dir, "/x", false); err != nil {
		t.Fatal("reinstall over our own hook should work:", err)
	}
	if _, err := Uninstall(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("hook still there")
	}
	if _, err := Uninstall(dir); err == nil {
		t.Fatal("want error when nothing is installed")
	}
}

func TestBackupAndRestoreForeignHook(t *testing.T) {
	dir := testgit.New(t)
	testgit.Write(t, dir, ".git/hooks/pre-commit", "#!/bin/sh\necho mine\n")
	if _, err := Install(dir, "/g", false); err != nil {
		t.Fatal(err)
	}
	if got := testgit.Read(t, dir, ".git/hooks/pre-commit.guardia-backup"); !strings.Contains(got, "echo mine") {
		t.Fatal("backup missing")
	}
	testgit.Write(t, dir, ".git/hooks/pre-commit", "#!/bin/sh\necho other\n")
	if _, err := Install(dir, "/g", false); err == nil {
		t.Fatal("want refusal when a backup already exists")
	}
	if _, err := Uninstall(dir); err == nil {
		t.Fatal("must not remove a hook guardia did not write")
	}
	if _, err := Install(dir, "/g", true); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(dir); err != nil {
		t.Fatal(err)
	}
	if got := testgit.Read(t, dir, ".git/hooks/pre-commit"); !strings.Contains(got, "echo other") {
		t.Fatalf("previous hook not restored: %q", got)
	}
}

func TestHooksPath(t *testing.T) {
	dir := testgit.New(t)
	testgit.Git(t, dir, "config", "core.hooksPath", ".githooks")
	p, err := Install(dir, "/g", false)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".githooks", "pre-commit"); p != want {
		t.Fatalf("got %s want %s", p, want)
	}
}
