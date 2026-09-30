// SPDX-License-Identifier: GPL-3.0-or-later

// Package bans implements the substring-match ban list from the C tracker.
//
// Compatibility notes: the C tracker called bansCreate(NULL) so the list was
// always empty in practice. We support loading from a file for parity but
// the default is an empty list. Match semantics: a name is banned if any
// loaded ban string is a substring (strstr) of the name. Empty input is
// treated as "never banned" to avoid the C strstr-with-empty-needle gotcha
// (where an empty input would trivially match every ban).
package bans

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
)

type List struct {
	mu      sync.RWMutex
	entries []string
}

// New returns an empty ban list.
func New() *List {
	return &List{}
}

// Load reads bans from path (one per line, blank lines skipped). A missing
// file is not an error — returns an empty list, matching the C tracker which
// silently no-ops if the file is absent.
func Load(path string) (*List, error) {
	l := New()
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return l, nil
		}
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimRight(s.Text(), "\r\n")
		if line == "" {
			continue
		}
		l.entries = append(l.entries, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return l, nil
}

// Exists reports whether name is banned. Mirrors bansExist: any loaded
// entry containing name as a substring matches.
func (l *List) Exists(name string) bool {
	if name == "" {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, e := range l.entries {
		if strings.Contains(e, name) {
			return true
		}
	}
	return false
}

// Add appends a ban entry. Mirrors bansAddBan.
func (l *List) Add(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, name)
}
