// Package sane finds a scanner among those SANE's scanimage lists.
package sane

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sync"
)

// Finder keeps what the last look for one scanner found. A scanner
// found is kept; one not found is looked for again the next time it is
// wanted (story 174).
type Finder struct {
	look sync.Mutex // one look at a time

	mu     sync.Mutex // what the last look found
	looked bool
	dev    string
	err    error
}

// absent is the error of a look that listed the scanners and found
// none of ours; it reads as the driver worded it.
type absent string

func (a absent) Error() string { return string(a) }

// Find is the SANE device of the first of patterns to match a line of
// command -L, each pattern's first group being the device. With no
// match the error is missing, which says the scanner is not connected.
func (f *Finder) Find(ctx context.Context, command, missing string, patterns ...*regexp.Regexp) (string, error) {
	f.look.Lock()
	defer f.look.Unlock()
	if dev, _, _ := f.last(); dev != "" {
		return dev, nil
	}
	dev, err := find(ctx, command, missing, patterns)
	f.mu.Lock()
	f.looked, f.dev, f.err = true, dev, err
	f.mu.Unlock()
	return dev, err
}

func find(ctx context.Context, command, missing string, patterns []*regexp.Regexp) (string, error) {
	out, err := exec.CommandContext(ctx, command, "-L").Output()
	if err != nil {
		return "", fmt.Errorf("the scanners could not be listed: %w", err)
	}
	for _, re := range patterns {
		if m := re.FindSubmatch(out); m != nil {
			return string(m[1]), nil
		}
	}
	return "", absent(missing)
}

func (f *Finder) last() (dev string, err error, looked bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dev, f.err, f.looked
}

// Status is what the last look found, in words: the device, that the
// scanner is not connected, or why the scanners could not be listed. It
// does not wait for a look still running.
func (f *Finder) Status() string {
	dev, err, looked := f.last()
	if !looked {
		return "not yet looked for"
	}
	if errors.As(err, new(absent)) {
		return "not connected"
	}
	if err != nil {
		return err.Error()
	}
	return At(dev)
}

// At is the status of a scanner at dev.
func At(dev string) string { return "at " + dev }
