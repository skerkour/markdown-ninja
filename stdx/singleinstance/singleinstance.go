// Package singleinstance provides a mechanism to ensure, that only one instance of a program is running
package singleinstance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Option configures Single
type Option func(*SingleInstance)

// WithLockPath configures the path for the lockfile
func WithLockPath(lockpath string) Option {
	return func(s *SingleInstance) {
		s.path = lockpath
	}
}

var (
	// ErrAlreadyRunning another instance of is already running
	ErrAlreadyRunning = errors.New("another instance is already running")
)

// SingleInstance represents the name and the open file descriptor
type SingleInstance struct {
	name string
	path string
	file *os.File
}

// New creates a SingleInstance instance where name is the basename of the lock file (<name>.lock)
// if no path is given (WithLockPath option) the lock will be created in an operating specific path as <name>.lock
// panics if namr is empty
func New(name string, opts ...Option) *SingleInstance {
	if name == "" {
		panic("singleinstance: name cannot be empty")
	}

	s := &SingleInstance{
		name: name,
	}

	for _, o := range opts {
		o(s)
	}

	if s.path == "" {
		s.path = os.TempDir()
	}

	return s
}

// Lockfile returns the full path of the lock file
func (s *SingleInstance) Lockfile() string {
	return filepath.Join(s.path, fmt.Sprintf("%s.lock", s.name))
}
