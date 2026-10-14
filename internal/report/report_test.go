package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/KlGamers/guardia/internal/run"
)

func TestWrite(t *testing.T) {
	res := []run.Res{
		{Name: "gofmt", Status: run.Pass, Dur: 120 * time.Millisecond, Out: []byte("noise")},
		{Name: "go test", Status: run.Fail, Dur: 2300 * time.Millisecond, Why: "exit status 1", Out: []byte(strings.Repeat("x\n", 60) + "FAIL pkg\n")},
		{Name: "ruff", Status: run.Skip, Why: "ruff not installed"},
	}
	var buf bytes.Buffer
	Write(&buf, res, "3 checks · 2 staged files", Style{}, time.Second)
	out := buf.String()
	for _, want := range []string{"✓ gofmt    120ms", "✗ go test  2.3s  exit status 1", "- ruff     skipped: ruff not installed", "── go test ──", "FAIL pkg", "earlier lines hidden", "1 check failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "noise") || strings.Contains(out, "\x1b[") {
		t.Error("passing output or color leaked")
	}
	buf.Reset()
	Write(&buf, res[:1], "1 check", Style{Color: true, Verbose: true}, time.Second)
	if !strings.Contains(buf.String(), "noise") || !strings.Contains(buf.String(), "\x1b[32m") || !strings.Contains(buf.String(), "all checks passed") {
		t.Errorf("verbose/color:\n%s", buf.String())
	}
	if Failed(append(res, res[1])) != 2 {
		t.Error("count")
	}
	buf.Reset()
	Write(&buf, []run.Res{res[1], res[1]}, "", Style{}, 0)
	if !strings.Contains(buf.String(), "2 checks failed") {
		t.Error("plural")
	}
}
