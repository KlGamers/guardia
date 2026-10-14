package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const File = "guardia.json"

type Dur time.Duration

func (d *Dur) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Dur(v)
	return nil
}

func (d Dur) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d Dur) Std() time.Duration { return time.Duration(d) }

type Check struct {
	Name    string   `json:"name"`
	Run     string   `json:"run"`
	Dir     string   `json:"dir,omitempty"`
	Glob    []string `json:"glob,omitempty"`
	FailOut bool     `json:"fail_on_output,omitempty"`
	Need    string   `json:"need,omitempty"`
	Timeout Dur      `json:"timeout,omitempty"`
}

type Conf struct {
	Parallel int     `json:"parallel,omitempty"`
	Timeout  Dur     `json:"timeout,omitempty"`
	Isolate  *bool   `json:"isolate,omitempty"`
	Detect   *bool   `json:"detect,omitempty"`
	Checks   []Check `json:"checks,omitempty"`
}

func Load(root string) (*Conf, bool, error) {
	raw, err := os.ReadFile(filepath.Join(root, File))
	if errors.Is(err, os.ErrNotExist) {
		c := &Conf{}
		c.fill()
		return c, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	c, err := Parse(raw)
	return c, true, err
}

func Parse(raw []byte) (*Conf, error) {
	c := &Conf{}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	c.fill()
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return c, nil
}

func (c *Conf) fill() {
	if c.Parallel <= 0 {
		c.Parallel = 4
	}
	if c.Timeout == 0 {
		c.Timeout = Dur(10 * time.Minute)
	}
	if c.Isolate == nil {
		on := true
		c.Isolate = &on
	}
	if c.Detect == nil {
		on := len(c.Checks) == 0
		c.Detect = &on
	}
}

func (c *Conf) check() error {
	seen := map[string]bool{}
	for i, ck := range c.Checks {
		if strings.TrimSpace(ck.Name) == "" {
			return fmt.Errorf("check %d has no name", i+1)
		}
		if seen[ck.Name] {
			return fmt.Errorf("duplicate check %q", ck.Name)
		}
		seen[ck.Name] = true
		if strings.TrimSpace(ck.Run) == "" {
			return fmt.Errorf("check %q has no run command", ck.Name)
		}
		if ck.Dir != "" && (filepath.IsAbs(ck.Dir) || strings.HasPrefix(path.Clean(filepath.ToSlash(ck.Dir)), "..")) {
			return fmt.Errorf("check %q: dir must stay inside the repository", ck.Name)
		}
		for _, g := range ck.Glob {
			if _, err := path.Match(g, ""); err != nil {
				return fmt.Errorf("check %q: bad glob %q", ck.Name, g)
			}
		}
	}
	return nil
}

func (ck Check) Wait(def Dur) time.Duration {
	if ck.Timeout > 0 {
		return ck.Timeout.Std()
	}
	return def.Std()
}

func (ck Check) Base() string {
	if ck.Dir == "" {
		return "."
	}
	return path.Clean(filepath.ToSlash(ck.Dir))
}

func (ck Check) Pick(files []string) []string {
	base := ck.Base()
	var out []string
	for _, f := range files {
		rel := f
		if base != "." {
			if !strings.HasPrefix(f, base+"/") {
				continue
			}
			rel = strings.TrimPrefix(f, base+"/")
		}
		if ck.Hit(rel) {
			out = append(out, rel)
		}
	}
	return out
}

func (ck Check) Hit(rel string) bool {
	if len(ck.Glob) == 0 {
		return true
	}
	for _, g := range ck.Glob {
		tgt := path.Base(rel)
		if strings.Contains(g, "/") {
			tgt = rel
		}
		if ok, _ := path.Match(g, tgt); ok {
			return true
		}
	}
	return false
}
