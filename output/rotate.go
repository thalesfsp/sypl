// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/shared"
)

//////
// Consts, vars, and types.
//////

const (
	// backupTimeFormat is the - lexicographically sortable - UTC timestamp
	// appended to rotated backup names.
	backupTimeFormat = "20060102T150405.000000000Z0700"

	// hoursPerDay converts `MaxAgeDays` into a duration.
	hoursPerDay = 24
)

// ErrRotatingFileClosed is returned when writing to a closed rotating file
// output.
var ErrRotatingFileClosed = errors.New("rotating file output is closed")

// Seams for deterministic tests.
var (
	// rotateNow returns the current time - backup naming, and age pruning.
	rotateNow = time.Now

	// rotateOpenFile reopens the live log file after a rotation.
	rotateOpenFile = openLogFile

	// rotateStat inspects backups for age pruning.
	rotateStat = os.Stat
)

// RotationConfig configures the rotating file output.
type RotationConfig struct {
	// MaxSizeBytes is the size threshold: a write that would push the live
	// file BEYOND it triggers a rotation first. A write landing exactly at
	// the limit does not rotate. Must be positive.
	MaxSizeBytes int64

	// MaxBackups caps how many rotated backups are kept - the oldest
	// beyond the cap are pruned on rotation. Zero keeps all.
	MaxBackups int

	// MaxAgeDays prunes - on rotation - backups whose modification time is
	// older than this many days. Zero keeps all.
	MaxAgeDays int
}

// rotatingWriter is a concurrency-safe, size-rotating file writer.
type rotatingWriter struct {
	// mu guards the state below - rotation is atomic vs. Write, Sync, and
	// Close.
	mu sync.Mutex

	cfg    RotationConfig
	closed bool
	file   *os.File
	path   string
	size   int64
}

//////
// rotatingWriter methods.
//////

// Write appends to the live file - rotating first when the write would push
// it beyond `MaxSizeBytes`.
//
// io.Writer interface implementation.
func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, ErrRotatingFileClosed
	}

	// The `size > 0` guard keeps a single oversized write on a fresh file
	// from rotating forever.
	if w.size > 0 && w.size+int64(len(p)) > w.cfg.MaxSizeBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)

	w.size += int64(n)

	return n, err
}

// Sync flushes the live file to stable storage. After Close it's a no-op.
func (w *rotatingWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	return w.file.Sync()
}

// Close closes the live file. It's idempotent. Writes after Close return
// `ErrRotatingFileClosed` - never panic.
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true

	return w.file.Close()
}

// rotate closes the live file, renames it to a timestamped backup, reopens
// a fresh live file, and prunes backups. The caller must hold `mu`.
func (w *rotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed closing the log file for rotation: %w", err)
	}

	if err := os.Rename(w.path, w.backupPath()); err != nil {
		return fmt.Errorf("failed renaming the log file for rotation: %w", err)
	}

	f, err := rotateOpenFile(w.path)
	if err != nil {
		return fmt.Errorf("failed reopening the log file after rotation: %w", err)
	}

	w.file = f
	w.size = 0

	return w.prune()
}

// backupPath returns a free backup path: `<path>.<UTC timestamp>` - with a
// `-N` suffix on - unlikely - collisions.
func (w *rotatingWriter) backupPath() string {
	base := fmt.Sprintf("%s.%s", w.path, rotateNow().UTC().Format(backupTimeFormat))

	candidate := base

	for i := 1; ; i++ {
		// Any Lstat error - not just "does not exist" - frees the
		// candidate: a real filesystem problem surfaces at the rename.
		if _, err := os.Lstat(candidate); err != nil {
			return candidate
		}

		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

// prune removes backups beyond `MaxBackups`, and older than `MaxAgeDays`.
// Any file named `<path>.*` is considered a backup. The caller must hold
// `mu`.
func (w *rotatingWriter) prune() error {
	dir := filepath.Dir(w.path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed listing backups: %w", err)
	}

	prefix := filepath.Base(w.path) + "."

	backups := []string{}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			backups = append(backups, entry.Name())
		}
	}

	// Newest first - backup names embed a sortable UTC timestamp.
	slices.Sort(backups)
	slices.Reverse(backups)

	errs := []error{}

	// Prune by count.
	if w.cfg.MaxBackups > 0 && len(backups) > w.cfg.MaxBackups {
		for _, name := range backups[w.cfg.MaxBackups:] {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				errs = append(errs, fmt.Errorf("failed pruning backup: %w", err))
			}
		}

		backups = backups[:w.cfg.MaxBackups]
	}

	// Prune by age - based on the backup's modification time.
	if w.cfg.MaxAgeDays > 0 {
		cutoff := rotateNow().Add(-time.Duration(w.cfg.MaxAgeDays) * hoursPerDay * time.Hour)

		for _, name := range backups {
			fullPath := filepath.Join(dir, name)

			info, err := rotateStat(fullPath)
			if err != nil {
				// The backup can't be assessed - never remove what
				// can't be inspected.
				continue
			}

			if info.ModTime().Before(cutoff) {
				if err := os.Remove(fullPath); err != nil {
					errs = append(errs, fmt.Errorf("failed pruning backup: %w", err))
				}
			}
		}
	}

	return errors.Join(errs...)
}

//////
// Output wrapper.
//////

// rotatingFileOutput is a rotating-file-backed `IOutput` carrying the Flush,
// and Close capabilities.
type rotatingFileOutput struct {
	*proxyOutput

	writer *rotatingWriter
}

// Flush syncs the live file to stable storage. After Close it's a no-op.
func (o *rotatingFileOutput) Flush() error {
	return o.writer.Sync()
}

// Close closes the live file. It's idempotent. Writes after Close return
// `ErrRotatingFileClosed`.
func (o *rotatingFileOutput) Close() error {
	return o.writer.Close()
}

//////
// Helpers.
//////

// openLogFile opens - creating if needed - a log file for appending.
func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(
		path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		shared.DefaultFileMode,
	)
}

//////
// Factory.
//////

// RotatingFile is a built-in `output` that writes to the specified file,
// rotating it by size: when a write would push the live file beyond
// `cfg.MaxSizeBytes`, the file is closed, renamed to `<path>.<UTC
// timestamp>`, and reopened fresh - then backups beyond `cfg.MaxBackups`,
// or older than `cfg.MaxAgeDays`, are pruned (inline, no goroutines).
//
// Capabilities: `Flush() error` (file sync), and idempotent `Close() error`.
// Writes after Close return `ErrRotatingFileClosed`.
//
// Notes:
// - Unlike `File`, it returns an error - it never calls log.Fatalf.
// - Missing parent directories are created.
// - Any file named `<path>.*` is treated as a backup by pruning.
func RotatingFile(
	name, path string,
	maxLevel level.Level,
	cfg RotationConfig,
	processors ...processor.IProcessor,
) (IOutput, error) {
	if path == "" {
		return nil, errors.New("rotating file output: path is required")
	}

	if cfg.MaxSizeBytes <= 0 {
		return nil, errors.New("rotating file output: MaxSizeBytes must be positive")
	}

	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return nil, fmt.Errorf("rotating file output: failed creating the log directory: %w", err)
	}

	// The pre-existing size counts toward the rotation threshold.
	size := int64(0)

	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}

	f, err := openLogFile(path)
	if err != nil {
		return nil, fmt.Errorf(`rotating file output: failed creating/opening "%s": %w`, path, err)
	}

	w := &rotatingWriter{
		cfg:  cfg,
		file: f,
		path: path,
		size: size,
	}

	o := &rotatingFileOutput{writer: w}

	o.proxyOutput = newProxyOutput(New(name, maxLevel, w, processors...), o)

	return o, nil
}
