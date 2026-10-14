package hook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/KlGamers/guardia/internal/gitx"
)

const mark = "GUARDIA_HOOK=1"

func Script(bin string) string {
	q := quote(bin)
	return "#!/bin/sh\n" + mark + "\nexport GUARDIA_HOOK\n" +
		"if [ -x " + q + " ]; then exec " + q + " run; fi\n" +
		"exec guardia run\n"
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func paths(root string) (string, string, error) {
	p, err := gitx.Path(root, "hooks/pre-commit")
	if err != nil {
		return "", "", err
	}
	return p, p + ".guardia-backup", nil
}

func Ours(p string) bool {
	raw, err := os.ReadFile(p)
	return err == nil && strings.Contains(string(raw), mark)
}

func Install(root, bin string, force bool) (string, error) {
	p, bak, err := paths(root)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil && !Ours(p) {
		if _, err := os.Stat(bak); err == nil && !force {
			return "", fmt.Errorf("%s exists and a backup is already present; use --force to overwrite", p)
		}
		if err := os.Rename(p, bak); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(p, []byte(Script(bin)), 0o755); err != nil {
		return "", err
	}
	return p, os.Chmod(p, 0o755)
}

func Uninstall(root string) (string, error) {
	p, bak, err := paths(root)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return "", errors.New("no pre-commit hook installed")
	}
	if !Ours(p) {
		return "", fmt.Errorf("%s was not installed by guardia", p)
	}
	if err := os.Remove(p); err != nil {
		return "", err
	}
	if _, err := os.Stat(bak); err == nil {
		return p, os.Rename(bak, p)
	}
	return p, nil
}
