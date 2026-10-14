package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KlGamers/guardia/internal/conf"
)

type Status int

const (
	Pass Status = iota
	Fail
	Skip
)

const keep = 64 << 10

type Job struct {
	Chk   conf.Check
	Files []string
}

type Res struct {
	Name   string
	Status Status
	Why    string
	Out    []byte
	Dur    time.Duration
}

func Plan(chks []conf.Check, files []string) []Job {
	var jobs []Job
	for _, ck := range chks {
		if fs := ck.Pick(files); len(fs) > 0 {
			jobs = append(jobs, Job{Chk: ck, Files: fs})
		}
	}
	return jobs
}

func Quote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == '+' || r == ':' || r == '@' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func Line(ck conf.Check, files []string) string {
	if !strings.Contains(ck.Run, "{files}") {
		return ck.Run
	}
	qs := make([]string, len(files))
	for i, f := range files {
		qs[i] = Quote(f)
	}
	return strings.ReplaceAll(ck.Run, "{files}", strings.Join(qs, " "))
}

func Env(root, dir string) []string {
	var extra []string
	for _, base := range []string{dir, root} {
		for _, sub := range []string{".venv/bin", "venv/bin", "node_modules/.bin"} {
			p := filepath.Join(base, sub)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				extra = append(extra, p)
			}
		}
	}
	path := strings.Join(append(extra, os.Getenv("PATH")), string(os.PathListSeparator))
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	return append(env, "PATH="+path)
}

func find(tool string, env []string) bool {
	for _, kv := range env {
		if !strings.HasPrefix(kv, "PATH=") {
			continue
		}
		for _, d := range filepath.SplitList(strings.TrimPrefix(kv, "PATH=")) {
			if st, err := os.Stat(filepath.Join(d, tool)); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
				return true
			}
		}
	}
	return false
}

func Exec(ctx context.Context, root string, jobs []Job, par int, def conf.Dur) []Res {
	if par < 1 {
		par = 1
	}
	out := make([]Res, len(jobs))
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for i, jb := range jobs {
		wg.Add(1)
		go func(i int, jb Job) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				out[i] = Res{Name: jb.Chk.Name, Status: Skip, Why: "cancelled"}
				return
			}
			defer func() { <-sem }()
			out[i] = one(ctx, root, jb, def)
		}(i, jb)
	}
	wg.Wait()
	return out
}

func one(ctx context.Context, root string, jb Job, def conf.Dur) Res {
	ck := jb.Chk
	res := Res{Name: ck.Name}
	dir := filepath.Join(root, filepath.FromSlash(ck.Base()))
	env := Env(root, dir)
	if ck.Need != "" && !find(ck.Need, env) {
		res.Status = Skip
		res.Why = ck.Need + " not installed"
		return res
	}
	ctx, stop := context.WithTimeout(ctx, ck.Wait(def))
	defer stop()

	cmd := exec.CommandContext(ctx, "sh", "-c", Line(ck, jb.Files))
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	t0 := time.Now()
	err := cmd.Run()
	res.Dur = time.Since(t0)
	res.Out = tail(buf.Bytes())

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Status = Fail
		res.Why = fmt.Sprintf("timed out after %s", ck.Wait(def))
	case ctx.Err() != nil:
		res.Status = Skip
		res.Why = "cancelled"
	case err != nil:
		res.Status = Fail
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.Why = fmt.Sprintf("exit status %d", ee.ExitCode())
		} else {
			res.Why = err.Error()
		}
	case ck.FailOut && len(bytes.TrimSpace(buf.Bytes())) > 0:
		res.Status = Fail
		res.Why = "reported problems"
	default:
		res.Status = Pass
	}
	return res
}

func tail(b []byte) []byte {
	if len(b) <= keep {
		return b
	}
	b = b[len(b)-keep:]
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[i+1:]
	}
	return b
}
