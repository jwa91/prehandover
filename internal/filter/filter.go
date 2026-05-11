package filter

import (
	"fmt"
	"regexp"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/jwa91/prehandover/internal/config"
)

type Matcher struct {
	include []matchFn
	exclude []matchFn
}

type matchFn func(path string) bool

func New(include, exclude config.Pattern) (*Matcher, error) {
	m := &Matcher{}
	if err := add(&m.include, include); err != nil {
		return nil, fmt.Errorf("include: %w", err)
	}
	if err := add(&m.exclude, exclude); err != nil {
		return nil, fmt.Errorf("exclude: %w", err)
	}
	return m, nil
}

func add(fns *[]matchFn, p config.Pattern) error {
	if p.Regex != "" {
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			return err
		}
		*fns = append(*fns, func(s string) bool { return re.MatchString(s) })
	}
	for _, g := range p.Globs {
		glob := g
		if _, err := doublestar.Match(glob, ""); err != nil {
			return fmt.Errorf("invalid glob %q: %w", glob, err)
		}
		*fns = append(*fns, func(s string) bool {
			ok, _ := doublestar.PathMatch(glob, s)
			return ok
		})
	}
	return nil
}

func (m *Matcher) Match(path string) bool {
	if len(m.include) > 0 {
		matched := false
		for _, fn := range m.include {
			if fn(path) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, fn := range m.exclude {
		if fn(path) {
			return false
		}
	}
	return true
}

func (m *Matcher) Filter(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if m.Match(p) {
			out = append(out, p)
		}
	}
	return out
}
