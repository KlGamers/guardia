package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/KlGamers/guardia/internal/conf"
	"github.com/KlGamers/guardia/internal/detect"
	"github.com/KlGamers/guardia/internal/gitx"
	"github.com/KlGamers/guardia/internal/hook"
	"github.com/KlGamers/guardia/internal/report"
	"github.com/KlGamers/guardia/internal/run"
)

var version = "dev"

const usage = `guardia keeps broken code out of your commits.

usage:
  guardia init [--force] [--write-config]   install the pre-commit hook
  guardia uninstall                         remove the hook and restore any previous one
  guardia run [-p N] [--no-isolate] [-v]    check staged files (what the hook runs)
  guardia ci [-p N] [-v]                    check every tracked file, for CI
  guardia list                              show the checks that would run
  guardia version
`

func main() {
	os.Exit(Main(os.Args[1:], os.Stdout, os.Stderr))
}

func Main(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(errw, usage)
		return 2
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(errw, "guardia:", err)
		return 2
	}
	var code int
	switch args[0] {
	case "init":
		code, err = cmdInit(wd, args[1:], out)
	case "uninstall":
		code, err = cmdUninstall(wd, out)
	case "run":
		code, err = cmdCheck(wd, args[1:], out, false)
	case "ci":
		code, err = cmdCheck(wd, args[1:], out, true)
	case "list":
		code, err = cmdList(wd, out)
	case "version", "--version", "-v":
		fmt.Fprintln(out, "guardia", version)
	case "help", "--help", "-h":
		fmt.Fprint(out, usage)
	default:
		fmt.Fprintf(errw, "guardia: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(errw, "guardia:", err)
		return 2
	}
	return code
}

func load(wd string) (string, *conf.Conf, []conf.Check, error) {
	root, err := gitx.Root(wd)
	if err != nil {
		return "", nil, nil, err
	}
	c, _, err := conf.Load(root)
	if err != nil {
		return "", nil, nil, err
	}
	chks := append([]conf.Check(nil), c.Checks...)
	if *c.Detect {
		found, err := detect.Scan(root)
		if err != nil {
			return "", nil, nil, err
		}
		taken := map[string]bool{}
		for _, ck := range chks {
			taken[ck.Name] = true
		}
		for _, ck := range found {
			if !taken[ck.Name] {
				chks = append(chks, ck)
			}
		}
	}
	return root, c, chks, nil
}

func cmdInit(wd string, args []string, out io.Writer) (int, error) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "replace an existing backup")
	write := fs.Bool("write-config", false, "write detected checks to guardia.json")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	root, err := gitx.Root(wd)
	if err != nil {
		return 2, err
	}
	bin, err := os.Executable()
	if err != nil {
		return 2, err
	}
	if abs, err := filepath.EvalSymlinks(bin); err == nil {
		bin = abs
	}
	p, err := hook.Install(root, bin, *force)
	if err != nil {
		return 2, err
	}
	fmt.Fprintln(out, "installed", p)
	if *write {
		dst := filepath.Join(root, conf.File)
		if _, err := os.Stat(dst); err == nil {
			return 2, fmt.Errorf("%s already exists", dst)
		}
		found, err := detect.Scan(root)
		if err != nil {
			return 2, err
		}
		raw, _ := json.MarshalIndent(conf.Conf{Checks: found}, "", "  ")
		if err := os.WriteFile(dst, append(raw, '\n'), 0o644); err != nil {
			return 2, err
		}
		fmt.Fprintf(out, "wrote %s with %d checks\n", dst, len(found))
	}
	return 0, nil
}

func cmdUninstall(wd string, out io.Writer) (int, error) {
	root, err := gitx.Root(wd)
	if err != nil {
		return 2, err
	}
	p, err := hook.Uninstall(root)
	if err != nil {
		return 2, err
	}
	fmt.Fprintln(out, "removed", p)
	return 0, nil
}

func cmdList(wd string, out io.Writer) (int, error) {
	_, _, chks, err := load(wd)
	if err != nil {
		return 2, err
	}
	if len(chks) == 0 {
		fmt.Fprintln(out, "no checks configured or detected")
		return 0, nil
	}
	for _, ck := range chks {
		fmt.Fprintf(out, "%-24s %s\n", ck.Name, ck.Run)
	}
	return 0, nil
}

func cmdCheck(wd string, args []string, out io.Writer, all bool) (code int, err error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	par := fs.Int("p", 0, "checks to run at once")
	noIso := fs.Bool("no-isolate", false, "check the working tree as is")
	verb := fs.Bool("v", false, "show output of passing checks")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	root, c, chks, err := load(wd)
	if err != nil {
		return 2, err
	}
	if *par > 0 {
		c.Parallel = *par
	}

	var files []string
	if all {
		files, err = gitx.Tracked(root)
	} else {
		files, err = gitx.Staged(root)
	}
	if err != nil {
		return 2, err
	}
	jobs := run.Plan(chks, files)
	if len(jobs) == 0 {
		fmt.Fprintln(out, "guardia: nothing to check")
		return 0, nil
	}

	if !all && *c.Isolate && !*noIso && !gitx.Busy(root) {
		stash, err := gitx.Hide(root)
		if err != nil {
			return 2, err
		}
		defer func() {
			if rerr := stash.Restore(); rerr != nil {
				code, err = 2, errors.Join(err, rerr)
			}
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	t0 := time.Now()
	res := run.Exec(ctx, root, jobs, c.Parallel, c.Timeout)

	kind := "staged"
	if all {
		kind = "tracked"
	}
	head := plural(len(jobs), "check") + " · " + plural(len(files), kind+" file")
	report.Write(out, res, head, report.Style{Color: tty(out), Verbose: *verb}, time.Since(t0))
	if ctx.Err() != nil {
		return 130, nil
	}
	if report.Failed(res) > 0 {
		if !all {
			fmt.Fprintln(out, "commit cancelled. fix the problems above, or bypass with git commit --no-verify")
		}
		return 1, nil
	}
	return 0, nil
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func tty(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
