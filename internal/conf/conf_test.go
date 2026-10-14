package conf

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultsWithoutFile(t *testing.T) {
	c, found, err := Load(t.TempDir())
	if err != nil || found {
		t.Fatalf("err=%v found=%v", err, found)
	}
	if c.Parallel != 4 || c.Timeout.Std() != 10*time.Minute || !*c.Isolate || !*c.Detect {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestParse(t *testing.T) {
	c, err := Parse([]byte(`{"parallel":2,"isolate":false,"checks":[{"name":"lint","run":"x {files}","glob":["*.go"],"timeout":"30s"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Parallel != 2 || *c.Isolate || *c.Detect || c.Checks[0].Wait(c.Timeout) != 30*time.Second {
		t.Fatalf("parsed: %+v", c)
	}
	d, _ := Parse([]byte(`{"detect":true,"checks":[{"name":"a","run":"b"}]}`))
	if !*d.Detect {
		t.Fatal("explicit detect ignored")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, File), []byte(`{"checks":[{"name":"a","run":"true"}]}`), 0o644)
	c, found, err := Load(dir)
	if err != nil || !found || len(c.Checks) != 1 {
		t.Fatalf("err=%v found=%v c=%+v", err, found, c)
	}
}

func TestRejects(t *testing.T) {
	bad := map[string]string{
		"has no name":      `{"checks":[{"run":"x"}]}`,
		"duplicate":        `{"checks":[{"name":"a","run":"x"},{"name":"a","run":"y"}]}`,
		"no run command":   `{"checks":[{"name":"a"}]}`,
		"inside the":       `{"checks":[{"name":"a","run":"x","dir":"../up"}]}`,
		"bad glob":         `{"checks":[{"name":"a","run":"x","glob":["[a"]}]}`,
		"unknown field":    `{"chekcs":[]}`,
		"invalid duration": `{"timeout":"soon"}`,
	}
	for want, raw := range bad {
		if _, err := Parse([]byte(raw)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
}

func TestPick(t *testing.T) {
	files := []string{"main.go", "a/b.go", "web/src/app.ts", "web/src/app.go", "webby/x.ts", "README.md"}
	cases := []struct {
		ck   Check
		want []string
	}{
		{Check{Glob: []string{"*.go"}}, []string{"main.go", "a/b.go", "web/src/app.go"}},
		{Check{Dir: "web", Glob: []string{"*.ts"}}, []string{"src/app.ts"}},
		{Check{Dir: "web/", Glob: []string{"src/*.go"}}, []string{"src/app.go"}},
		{Check{Dir: "web"}, []string{"src/app.ts", "src/app.go"}},
		{Check{Glob: []string{"*.rs"}}, nil},
	}
	for i, c := range cases {
		if got := c.ck.Pick(files); !reflect.DeepEqual(got, c.want) {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

func TestDurJSON(t *testing.T) {
	raw, err := Dur(90 * time.Second).MarshalJSON()
	if err != nil || string(raw) != `"1m30s"` {
		t.Fatalf("%s %v", raw, err)
	}
}
