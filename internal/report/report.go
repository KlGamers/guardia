package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/KlGamers/guardia/internal/run"
)

const lines = 40

type Style struct {
	Color   bool
	Verbose bool
}

func (s Style) paint(code, txt string) string {
	if !s.Color {
		return txt
	}
	return "\x1b[" + code + "m" + txt + "\x1b[0m"
}

func Failed(res []run.Res) int {
	n := 0
	for _, r := range res {
		if r.Status == run.Fail {
			n++
		}
	}
	return n
}

func Write(w io.Writer, res []run.Res, head string, st Style, total time.Duration) {
	fmt.Fprintln(w, st.paint("1", "guardia")+" "+st.paint("2", "· "+head))
	wide := 0
	for _, r := range res {
		if len(r.Name) > wide {
			wide = len(r.Name)
		}
	}
	for _, r := range res {
		var mark, info string
		switch r.Status {
		case run.Pass:
			mark, info = st.paint("32", "✓"), st.paint("2", dur(r.Dur))
		case run.Fail:
			mark, info = st.paint("31", "✗"), st.paint("31", dur(r.Dur)+"  "+r.Why)
		default:
			mark, info = st.paint("33", "-"), st.paint("33", "skipped: "+r.Why)
		}
		fmt.Fprintf(w, "  %s %-*s  %s\n", mark, wide, r.Name, info)
	}
	for _, r := range res {
		if (r.Status == run.Fail || st.Verbose && r.Status == run.Pass) && len(strings.TrimSpace(string(r.Out))) > 0 {
			fmt.Fprintf(w, "\n%s\n%s", st.paint("1", "── "+r.Name+" ──"), last(string(r.Out), lines))
		}
	}
	n := Failed(res)
	fmt.Fprintln(w)
	switch {
	case n == 0:
		fmt.Fprintln(w, st.paint("32", "all checks passed")+st.paint("2", " in "+dur(total)))
	case n == 1:
		fmt.Fprintln(w, st.paint("31", "1 check failed"))
	default:
		fmt.Fprintln(w, st.paint("31", fmt.Sprintf("%d checks failed", n)))
	}
}

func dur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func last(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	parts := strings.Split(s, "\n")
	if len(parts) > n {
		parts = append([]string{fmt.Sprintf("… %d earlier lines hidden", len(parts)-n)}, parts[len(parts)-n:]...)
	}
	return strings.Join(parts, "\n") + "\n"
}
