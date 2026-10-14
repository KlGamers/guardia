package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func git(dir string, in []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	if in != nil {
		cmd.Stdin = bytes.NewReader(in)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.Bytes(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return out.Bytes(), nil
}

func Root(dir string) (string, error) {
	out, err := git(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("not inside a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

func Path(root, name string) (string, error) {
	out, err := git(root, nil, "rev-parse", "--git-path", name)
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p, nil
}

func split(raw []byte) []string {
	var out []string
	for _, p := range bytes.Split(raw, []byte{0}) {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out
}

func Staged(root string) ([]string, error) {
	out, err := git(root, nil, "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z")
	return split(out), err
}

func Tracked(root string) ([]string, error) {
	out, err := git(root, nil, "ls-files", "-z")
	return split(out), err
}

func Busy(root string) bool {
	for _, name := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		if p, err := Path(root, name); err == nil {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	return false
}

type Stash struct {
	root  string
	patch string
}

func Hide(root string) (*Stash, error) {
	patch, err := Path(root, "guardia-unstaged.patch")
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(patch); err == nil {
		return nil, fmt.Errorf("a previous run left unstaged changes in %s; restore them with: git apply %q", patch, patch)
	}
	diff, err := git(root, nil, "diff", "--binary", "--no-color", "--no-ext-diff", "--no-renames")
	if err != nil {
		return nil, err
	}
	if len(diff) == 0 {
		return nil, nil
	}
	if err := os.WriteFile(patch, diff, 0o600); err != nil {
		return nil, err
	}
	if _, err := git(root, nil, "checkout", "--", "."); err != nil {
		return nil, fmt.Errorf("could not hide unstaged changes (saved in %s): %w", patch, err)
	}
	return &Stash{root: root, patch: patch}, nil
}

func (s *Stash) Restore() error {
	if s == nil {
		return nil
	}
	if _, err := git(s.root, nil, "apply", "--binary", "--whitespace=nowarn", s.patch); err != nil {
		if _, err3 := git(s.root, nil, "apply", "--binary", "--3way", "--whitespace=nowarn", s.patch); err3 != nil {
			return fmt.Errorf("could not restore unstaged changes, they are saved in %s: %w", s.patch, err)
		}
	}
	return os.Remove(s.patch)
}
