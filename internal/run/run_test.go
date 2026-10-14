package run

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KlGamers/guardia/internal/conf"
)

func TestQuoteAndLine(t *testing.T) {
	for in, want := range map[string]string{
		"a/b.go": "a/b.go", "mi archivo.go": "'mi archivo.go'", "it's": `'it'\''s'`, "": "''", "ñ.py": "'ñ.py'", "$(x)": "'$(x)'",
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s want %s", in, got, want)
		}
	}
	ck := conf.Check{Run: "lint {files} --strict"}
	if got := Line(ck, []string{"a.go", "b c.go"}); got != "lint a.go 'b c.go' --strict" {
		t.Fatal(got)
	}
	if got := Line(conf.Check{Run: "go test ./..."}, []string{"a.go"}); got != "go test ./..." {
		t.Fatal(got)
	}
}

func TestPlan(t *testing.T) {
	chks := []conf.Check{
		{Name: "go", Glob: []string{"*.go"}},
		{Name: "py", Glob: []string{"*.py"}},
		{Name: "web", Dir: "web", Glob: []string{"*.ts"}},
	}
	jobs := Plan(chks, []string{"main.go", "web/a.ts", "docs/x.md"})
	if len(jobs) != 2 || jobs[0].Chk.Name != "go" || !reflect.DeepEqual(jobs[1].Files, []string{"a.ts"}) {
		t.Fatalf("%+v", jobs)
	}
}

func job(name, run string) Job {
	return Job{Chk: conf.Check{Name: name, Run: run}, Files: []string{"x"}}
}

func TestExecStatuses(t *testing.T) {
	root := t.TempDir()
	jobs := []Job{
		job("ok", "echo fine"),
		job("bad", "echo broken >&2; exit 3"),
		{Chk: conf.Check{Name: "fmt", Run: "echo needs-format.go", FailOut: true}, Files: []string{"x"}},
		{Chk: conf.Check{Name: "fmt-clean", Run: "true", FailOut: true}, Files: []string{"x"}},
		{Chk: conf.Check{Name: "slow", Run: "sleep 5", Timeout: conf.Dur(200 * time.Millisecond)}, Files: []string{"x"}},
		{Chk: conf.Check{Name: "tool", Run: "nope", Need: "definitely-not-a-real-tool"}, Files: []string{"x"}},
	}
	t0 := time.Now()
	res := Exec(context.Background(), root, jobs, 6, conf.Dur(time.Minute))
	if time.Since(t0) > 4*time.Second {
		t.Fatal("timeout did not kill the process")
	}
	want := []Status{Pass, Fail, Fail, Pass, Fail, Skip}
	for i, r := range res {
		if r.Status != want[i] {
			t.Errorf("%s: got %v want %v (%s)", r.Name, r.Status, want[i], r.Why)
		}
	}
	if res[1].Why != "exit status 3" || !strings.Contains(string(res[1].Out), "broken") {
		t.Errorf("bad: %+v", res[1])
	}
	if !strings.Contains(res[4].Why, "timed out") || !strings.Contains(res[5].Why, "not installed") {
		t.Errorf("why: %q %q", res[4].Why, res[5].Why)
	}
}

func TestParallelism(t *testing.T) {
	jobs := []Job{job("a", "sleep 0.4"), job("b", "sleep 0.4"), job("c", "sleep 0.4"), job("d", "sleep 0.4")}
	t0 := time.Now()
	Exec(context.Background(), t.TempDir(), jobs, 4, conf.Dur(time.Minute))
	if d := time.Since(t0); d > 1200*time.Millisecond {
		t.Fatalf("parallel run took %s", d)
	}
	t0 = time.Now()
	Exec(context.Background(), t.TempDir(), jobs[:3], 1, conf.Dur(time.Minute))
	if d := time.Since(t0); d < 1100*time.Millisecond {
		t.Fatalf("serial run took only %s", d)
	}
}

func TestCancel(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		stop()
	}()
	t0 := time.Now()
	res := Exec(ctx, t.TempDir(), []Job{job("a", "sleep 5"), job("b", "sleep 5")}, 1, conf.Dur(time.Minute))
	if time.Since(t0) > 3*time.Second {
		t.Fatal("cancel did not stop the checks")
	}
	for _, r := range res {
		if r.Status != Skip || r.Why != "cancelled" {
			t.Fatalf("%+v", r)
		}
	}
}

func TestProjectToolsOnPath(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "api", ".venv", "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "mytool"), []byte("#!/bin/sh\necho from-venv \"$PWD\"\n"), 0o755)
	res := Exec(context.Background(), root, []Job{{Chk: conf.Check{Name: "t", Run: "mytool", Need: "mytool", Dir: "api"}, Files: []string{"a"}}}, 1, conf.Dur(time.Minute))
	if res[0].Status != Pass || !strings.Contains(string(res[0].Out), "from-venv") || !strings.Contains(string(res[0].Out), "/api") {
		t.Fatalf("%+v %s", res[0], res[0].Out)
	}
}

func TestTail(t *testing.T) {
	big := strings.Repeat("line\n", keep/4)
	got := tail([]byte(big))
	if len(got) > keep || !strings.HasPrefix(string(got), "line") {
		t.Fatalf("len %d", len(got))
	}
	if string(tail([]byte("short"))) != "short" {
		t.Fatal("short output changed")
	}
}
