// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/message"
	"github.com/thalesfsp/sypl/v2/processor"
)

//////
// Test helpers.
//////

// newRotatingFile creates a RotatingFile output, failing the test on error.
func newRotatingFile(t *testing.T, path string, cfg RotationConfig, processors ...processor.IProcessor) IOutput {
	t.Helper()

	o, err := RotatingFile("RotatingFile", path, level.Trace, cfg, processors...)
	if err != nil {
		t.Fatalf("RotatingFile() error = %v, want nil", err)
	}

	return o
}

// writeString writes `content` - as-is - through the output, failing the
// test on error.
func writeString(t *testing.T, o IOutput, content string) {
	t.Helper()

	if err := o.Write(message.New(level.Info, content)); err != nil {
		t.Fatalf("Write(%q) error = %v, want nil", content, err)
	}
}

// listBackups returns the backup files - sorted, oldest first - for `path`.
func listBackups(t *testing.T, path string) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Failed listing the log dir: %v", err)
	}

	backups := []string{}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), filepath.Base(path)+".") {
			backups = append(backups, filepath.Join(filepath.Dir(path), entry.Name()))
		}
	}

	sort.Strings(backups)

	return backups
}

// readFile reads a file, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed reading %q: %v", path, err)
	}

	return string(content)
}

// fakeClock makes rotation timestamps deterministic: every call advances
// the clock by one second.
type fakeClock struct {
	base  time.Time
	calls int
}

func (c *fakeClock) now() time.Time {
	c.calls++

	return c.base.Add(time.Duration(c.calls) * time.Second)
}

// withFakeClock overrides the rotation clock for the test duration.
func withFakeClock(t *testing.T, clock func() time.Time) {
	t.Helper()

	original := rotateNow

	rotateNow = clock

	t.Cleanup(func() { rotateNow = original })
}

//////
// Basic behavior.
//////

func TestRotatingFile_WritesAndAppends(t *testing.T) {
	// The missing parent directories must be created.
	path := filepath.Join(t.TempDir(), "a", "b", "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 1024},
		processor.Prefixer("p: "))

	if o.GetName() != "RotatingFile" {
		t.Errorf("GetName() = %q, want %q", o.GetName(), "RotatingFile")
	}

	writeString(t, o, "first\n")
	writeString(t, o, "second\n")

	if got := readFile(t, path); got != "p: first\np: second\n" {
		t.Errorf("File = %q, want %q", got, "p: first\np: second\n")
	}

	if err := o.(interface{ Close() error }).Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestRotatingFile_AppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	// Pre-existing content: its size must count toward the threshold.
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatalf("Failed seeding the log file: %v", err)
	}

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 10})

	// 5 (existing) + 5 = 10: exact fit, no rotation.
	writeString(t, o, "67890")

	if backups := listBackups(t, path); len(backups) != 0 {
		t.Fatalf("Expected no backups, got %v", backups)
	}

	// The next write would exceed the limit - it must rotate first.
	writeString(t, o, "x")

	backups := listBackups(t, path)

	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup, got %v", backups)
	}

	if got := readFile(t, backups[0]); got != "1234567890" {
		t.Errorf("Backup = %q, want %q", got, "1234567890")
	}

	if got := readFile(t, path); got != "x" {
		t.Errorf("Live file = %q, want %q", got, "x")
	}
}

//////
// Rotation boundary.
//////

func TestRotatingFile_RotatesWhenSizeWouldExceed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 10})

	// Exact-fit edge case: a write landing exactly AT the limit must NOT
	// rotate.
	writeString(t, o, "12345")
	writeString(t, o, "67890")

	if backups := listBackups(t, path); len(backups) != 0 {
		t.Fatalf("Exact-fit write should not rotate, got backups %v", backups)
	}

	// Exceeding the limit rotates: the full old content moves to the
	// backup, the live file starts fresh.
	writeString(t, o, "abc")

	backups := listBackups(t, path)

	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup, got %v", backups)
	}

	if got := readFile(t, backups[0]); got != "1234567890" {
		t.Errorf("Backup = %q, want %q", got, "1234567890")
	}

	if got := readFile(t, path); got != "abc" {
		t.Errorf("Live file = %q, want %q", got, "abc")
	}
}

func TestRotatingFile_OversizedSingleWriteOnEmptyFileDoesNotRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 3})

	// A single write larger than the limit, on an empty file: rotation
	// would loop forever on an empty file - the write must land.
	writeString(t, o, "0123456789")

	if backups := listBackups(t, path); len(backups) != 0 {
		t.Fatalf("Expected no backups, got %v", backups)
	}

	if got := readFile(t, path); got != "0123456789" {
		t.Errorf("Live file = %q, want %q", got, "0123456789")
	}
}

//////
// Backup naming.
//////

func TestRotatingFile_BackupNamingAndCollision(t *testing.T) {
	// A frozen clock forces two rotations onto the SAME timestamp - the
	// second backup must get a collision suffix, not overwrite the first.
	frozen := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)

	withFakeClock(t, func() time.Time { return frozen })

	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2})

	writeString(t, o, "aa")
	writeString(t, o, "bb") // Rotation 1.
	writeString(t, o, "cc") // Rotation 2 - same timestamp.

	backups := listBackups(t, path)

	if len(backups) != 2 {
		t.Fatalf("Expected 2 backups, got %v", backups)
	}

	wantBase := path + "." + frozen.Format("20060102T150405.000000000Z0700")

	if backups[0] != wantBase {
		t.Errorf("Backup[0] = %q, want %q", backups[0], wantBase)
	}

	if backups[1] != wantBase+"-1" {
		t.Errorf("Backup[1] = %q, want %q", backups[1], wantBase+"-1")
	}

	// No data lost to an overwrite.
	if got := readFile(t, backups[0]); got != "aa" {
		t.Errorf("Backup[0] = %q, want %q", got, "aa")
	}

	if got := readFile(t, backups[1]); got != "bb" {
		t.Errorf("Backup[1] = %q, want %q", got, "bb")
	}
}

//////
// Pruning.
//////

func TestRotatingFile_PrunesByCount(t *testing.T) {
	clock := &fakeClock{base: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)}

	withFakeClock(t, clock.now)

	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxBackups: 2})

	// 4 rotations: only the 2 NEWEST backups survive.
	for _, content := range []string{"b0", "b1", "b2", "b3", "b4"} {
		writeString(t, o, content)
	}

	backups := listBackups(t, path)

	if len(backups) != 2 {
		t.Fatalf("Expected 2 backups, got %v", backups)
	}

	// Oldest first: b2, then b3 - b0, and b1 were pruned; b4 is live.
	if got := readFile(t, backups[0]); got != "b2" {
		t.Errorf("Backup[0] = %q, want %q", got, "b2")
	}

	if got := readFile(t, backups[1]); got != "b3" {
		t.Errorf("Backup[1] = %q, want %q", got, "b3")
	}

	if got := readFile(t, path); got != "b4" {
		t.Errorf("Live file = %q, want %q", got, "b4")
	}
}

func TestRotatingFile_PrunesByAge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxAgeDays: 1})

	writeString(t, o, "b0")
	writeString(t, o, "b1") // Rotation 1: backup "b0".

	backups := listBackups(t, path)

	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup, got %v", backups)
	}

	// Make the backup 3 days old - older than MaxAgeDays.
	old := time.Now().Add(-3 * 24 * time.Hour)

	if err := os.Chtimes(backups[0], old, old); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	writeString(t, o, "b2") // Rotation 2 prunes the aged backup.

	backups = listBackups(t, path)

	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup after age pruning, got %v", backups)
	}

	if got := readFile(t, backups[0]); got != "b1" {
		t.Errorf("Surviving backup = %q, want %q", got, "b1")
	}
}

func TestRotatingFile_PruneSkipsUnstatableBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxAgeDays: 1})

	writeString(t, o, "b0")
	writeString(t, o, "b1") // Rotation 1: backup "b0".

	// Stat failures on a backup: the file can't be assessed - it must be
	// SKIPPED, never removed.
	originalStat := rotateStat

	rotateStat = func(string) (os.FileInfo, error) {
		return nil, errors.New("stat failed")
	}

	t.Cleanup(func() { rotateStat = originalStat })

	writeString(t, o, "b2") // Rotation 2: age pruning can't assess b0.

	rotateStat = originalStat

	if backups := listBackups(t, path); len(backups) != 2 {
		t.Fatalf("Unstatable backups should be kept, got %v", backups)
	}
}

func TestRotatingFile_PruneErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	t.Run("Should fail - unlistable directory", func(t *testing.T) {
		dir := t.TempDir()

		path := filepath.Join(dir, "rotate.log")

		o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxBackups: 1})

		writeString(t, o, "b0")

		// An unlistable directory: prune - and therefore the rotation -
		// must surface the error.
		if err := os.Chmod(dir, 0o000); err != nil {
			t.Fatalf("Chmod failed: %v", err)
		}

		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		err := o.Write(message.New(level.Info, "b1"))

		if err == nil || !strings.Contains(err.Error(), "failed") {
			t.Errorf("Write() error = %v, want a rotation failure", err)
		}
	})

	t.Run("Should fail - unremovable backup", func(t *testing.T) {
		dir := t.TempDir()

		path := filepath.Join(dir, "rotate.log")

		o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxBackups: 1})

		writeString(t, o, "b0")
		writeString(t, o, "b1") // Rotation 1: backup "b0" - within MaxBackups.

		// Remove needs directory write permission: r-x makes pruning
		// fail, while listing still works.
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("Chmod failed: %v", err)
		}

		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		// This rotation exceeds MaxBackups - pruning fails. NOTE: The
		// rename in the rotation itself also needs a writable dir, so
		// the error surfaces at the rename step - still a rotation
		// failure reaching the caller.
		err := o.Write(message.New(level.Info, "b2"))

		if err == nil {
			t.Error("Write() should surface the rotation failure")
		}
	})
}

func TestRotatingFile_PruneRemoveError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	dir := t.TempDir()

	path := filepath.Join(dir, "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxBackups: 1})

	writeString(t, o, "b0")
	writeString(t, o, "b1") // Rotation 1: backup "b0".

	// A second - older-named - backup puts the count over the cap.
	stale := path + ".00000101T000000.000000000Z"

	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatalf("Failed seeding a stale backup: %v", err)
	}

	// Directly exercise prune with an undeletable backup.
	concrete, ok := o.(*rotatingFileOutput)

	if !ok {
		t.Fatal("RotatingFile should return a *rotatingFileOutput")
	}

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := concrete.writer.prune()

	if err == nil || !strings.Contains(err.Error(), "failed pruning backup") {
		t.Errorf("prune() error = %v, want a prune failure", err)
	}
}

func TestRotatingFile_PruneListError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	dir := t.TempDir()

	path := filepath.Join(dir, "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxBackups: 1})

	concrete, ok := o.(*rotatingFileOutput)

	if !ok {
		t.Fatal("RotatingFile should return a *rotatingFileOutput")
	}

	// An unlistable directory: prune can't enumerate the backups.
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := concrete.writer.prune()

	if err == nil || !strings.Contains(err.Error(), "failed listing backups") {
		t.Errorf("prune() error = %v, want a listing failure", err)
	}
}

func TestRotatingFile_PruneByAgeRemoveError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	dir := t.TempDir()

	path := filepath.Join(dir, "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2, MaxAgeDays: 1})

	writeString(t, o, "b0")
	writeString(t, o, "b1") // Rotation 1: backup "b0".

	backups := listBackups(t, path)

	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup, got %v", backups)
	}

	// Make the backup old enough to prune - then make it undeletable.
	old := time.Now().Add(-3 * 24 * time.Hour)

	if err := os.Chtimes(backups[0], old, old); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	concrete, ok := o.(*rotatingFileOutput)

	if !ok {
		t.Fatal("RotatingFile should return a *rotatingFileOutput")
	}

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := concrete.writer.prune()

	if err == nil || !strings.Contains(err.Error(), "failed pruning backup") {
		t.Errorf("prune() error = %v, want an age-prune failure", err)
	}
}

//////
// Flush, and Close.
//////

func TestRotatingFile_FlushAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 1024})

	writeString(t, o, "data\n")

	f, ok := o.(interface{ Flush() error })

	if !ok {
		t.Fatal("RotatingFile should implement Flush() error")
	}

	if err := f.Flush(); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}

	c, ok := o.(interface{ Close() error })

	if !ok {
		t.Fatal("RotatingFile should implement Close() error")
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	// Idempotent Close.
	if err := c.Close(); err != nil {
		t.Fatalf("Second Close() error = %v, want nil", err)
	}

	// Flush after Close: documented no-op.
	if err := f.Flush(); err != nil {
		t.Errorf("Flush() after Close error = %v, want nil", err)
	}

	// Write after Close: typed error, no panic.
	err := o.Write(message.New(level.Info, "late\n"))

	if !errors.Is(err, ErrRotatingFileClosed) {
		t.Errorf("Write() after Close error = %v, want ErrRotatingFileClosed", err)
	}
}

//////
// Constructor errors.
//////

func TestRotatingFile_ConstructorErrors(t *testing.T) {
	tests := []struct {
		name      string
		path      func(t *testing.T) string
		cfg       RotationConfig
		wantInErr string
	}{
		{
			name:      "Should fail - empty path",
			path:      func(t *testing.T) string { t.Helper(); return "" },
			cfg:       RotationConfig{MaxSizeBytes: 10},
			wantInErr: "path is required",
		},
		{
			name: "Should fail - non-positive MaxSizeBytes",
			path: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "rotate.log")
			},
			cfg:       RotationConfig{MaxSizeBytes: 0},
			wantInErr: "MaxSizeBytes must be positive",
		},
		{
			name: "Should fail - path is a directory",
			path: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			cfg:       RotationConfig{MaxSizeBytes: 10},
			wantInErr: "failed creating/opening",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := RotatingFile("RotatingFile", tt.path(t), level.Trace, tt.cfg)

			if o != nil {
				t.Error("RotatingFile() output should be nil on error")
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("RotatingFile() error = %v, want it to contain %q", err, tt.wantInErr)
			}
		})
	}
}

func TestRotatingFile_ConstructorUnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	parent := t.TempDir()

	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	// MkdirAll can't create the sub directory.
	o, err := RotatingFile(
		"RotatingFile",
		filepath.Join(parent, "sub", "rotate.log"),
		level.Trace,
		RotationConfig{MaxSizeBytes: 10},
	)

	if o != nil {
		t.Error("RotatingFile() output should be nil on error")
	}

	if err == nil || !strings.Contains(err.Error(), "failed creating the log directory") {
		t.Errorf("RotatingFile() error = %v, want a directory creation failure", err)
	}
}

//////
// Mid-rotation failures.
//////

func TestRotatingFile_ReopenFailureMidRotateSurfacesError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2})

	writeString(t, o, "b0")

	// Fail the post-rotation reopen.
	originalOpen := rotateOpenFile

	rotateOpenFile = func(string) (*os.File, error) {
		return nil, errors.New("reopen denied")
	}

	t.Cleanup(func() { rotateOpenFile = originalOpen })

	err := o.Write(message.New(level.Info, "b1"))

	if err == nil || !strings.Contains(err.Error(), "reopen denied") {
		t.Errorf("Write() error = %v, want the reopen failure", err)
	}
}

func TestRotatingFile_CloseFailureMidRotateSurfacesError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2})

	writeString(t, o, "b0")

	concrete, ok := o.(*rotatingFileOutput)

	if !ok {
		t.Fatal("RotatingFile should return a *rotatingFileOutput")
	}

	// Close the underlying file behind the writer's back: the rotation's
	// close step must surface the failure.
	//
	// NOTE: Asserted on the WRITER: the output layer - by design - treats
	// `os.ErrClosed` as a benign closed-writer condition (warn, nil).
	if err := concrete.writer.file.Close(); err != nil {
		t.Fatalf("Failed pre-closing the file: %v", err)
	}

	_, err := concrete.writer.Write([]byte("b1"))

	if err == nil || !strings.Contains(err.Error(), "failed closing the log file") {
		t.Errorf("Write() error = %v, want the close failure", err)
	}
}

//////
// Self-healing.
//////

// A failed rename must NOT leave the writer on a closed file: the original
// path is reopened, the triggering write lands, and subsequent writes keep
// landing - with the size still counting, so rotation retries, and succeeds
// once the filesystem heals.
func TestRotatingFile_RenameFailureSelfHealsKeepsWriting(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	dir := t.TempDir()

	path := filepath.Join(dir, "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 4})

	writeString(t, o, "b0b0")

	// A read-only directory: rotation can't rename the live file away.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	// The rotation fails - the error is surfaced - but the write itself
	// lands in the reopened original file: no message loss.
	err := o.Write(message.New(level.Info, "b1b1"))

	if err == nil || !strings.Contains(err.Error(), "failed renaming the log file") {
		t.Fatalf("Write() error = %v, want the rename failure", err)
	}

	if got := readFile(t, path); got != "b0b0b1b1" {
		t.Errorf("Live file = %q, want %q - the write must land despite the failed rotation",
			got, "b0b0b1b1")
	}

	// Subsequent writes keep landing in the original file - the size keeps
	// counting, so rotation is retried (and keeps failing) on each.
	if err := o.Write(message.New(level.Info, "b2b2")); err == nil {
		t.Error("Write() error = nil, want the retried rotation failure")
	}

	if got := readFile(t, path); got != "b0b0b1b1b2b2" {
		t.Errorf("Live file = %q, want %q", got, "b0b0b1b1b2b2")
	}

	// The filesystem heals: the next size-threshold write rotates - the
	// accumulated content becomes the backup, the live file starts fresh.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	writeString(t, o, "b3b3")

	backups := listBackups(t, path)

	if len(backups) != 1 {
		t.Fatalf("Expected 1 backup after healing, got %v", backups)
	}

	if got := readFile(t, backups[0]); got != "b0b0b1b1b2b2" {
		t.Errorf("Backup = %q, want the accumulated content %q", got, "b0b0b1b1b2b2")
	}

	if got := readFile(t, path); got != "b3b3" {
		t.Errorf("Live file = %q, want %q", got, "b3b3")
	}
}

// When BOTH the post-rotation reopen, and the recovery reopen fail, the
// writer has no live file: writes return the TYPED unavailability error -
// never an `os.ErrClosed`-classed one, which the output layer silently
// swallows - and each write retries the reopen first, self-healing once
// the filesystem recovers.
func TestRotatingFile_NoLiveFileTypedErrorAndSelfHeal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 4})

	writeString(t, o, "b0b0")

	// Every reopen - post-rotation, recovery, and self-heal - fails.
	originalOpen := rotateOpenFile

	rotateOpenFile = func(string) (*os.File, error) {
		return nil, errors.New("reopen denied")
	}

	t.Cleanup(func() { rotateOpenFile = originalOpen })

	// The rotation-triggering write: the rename succeeds, both reopens
	// fail - the writer is left with NO live file.
	err := o.Write(message.New(level.Info, "b1b1"))

	if err == nil || !strings.Contains(err.Error(), "failed reopening the log file after rotation") {
		t.Fatalf("Write() error = %v, want the reopen failure", err)
	}

	if !strings.Contains(err.Error(), "failed reopening the original log file") {
		t.Errorf("Write() error = %v, want the recovery failure joined in", err)
	}

	// Subsequent writes return the TYPED error - visible to callers, and
	// error handlers - and attempt a reopen first.
	err = o.Write(message.New(level.Info, "b2b2"))

	if !errors.Is(err, ErrRotatingFileUnavailable) {
		t.Fatalf("Write() error = %v, want ErrRotatingFileUnavailable", err)
	}

	if errors.Is(err, os.ErrClosed) {
		t.Error("Write() error must NOT be os.ErrClosed-classed - it would be silently swallowed")
	}

	// Flush has no live file to sync either.
	if err := o.(interface{ Flush() error }).Flush(); !errors.Is(err, ErrRotatingFileUnavailable) {
		t.Errorf("Flush() error = %v, want ErrRotatingFileUnavailable", err)
	}

	// The filesystem recovers: the next write self-heals - reopening a
	// fresh live file (the original was renamed away) - and lands.
	rotateOpenFile = originalOpen

	writeString(t, o, "b3b3")

	if got := readFile(t, path); got != "b3b3" {
		t.Errorf("Live file = %q, want %q after self-healing", got, "b3b3")
	}

	if err := o.(interface{ Close() error }).Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

// Close on a writer left with no live file is clean, and the closed guard
// still wins over the self-heal path afterward.
func TestRotatingFile_CloseWithNoLiveFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2})

	writeString(t, o, "b0")

	originalOpen := rotateOpenFile

	rotateOpenFile = func(string) (*os.File, error) {
		return nil, errors.New("reopen denied")
	}

	t.Cleanup(func() { rotateOpenFile = originalOpen })

	if err := o.Write(message.New(level.Info, "b1")); err == nil {
		t.Fatal("Write() error = nil, want the mid-rotation failure")
	}

	// Close with no live file: clean, idempotent.
	if err := o.(interface{ Close() error }).Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}

	if err := o.Write(message.New(level.Info, "late")); !errors.Is(err, ErrRotatingFileClosed) {
		t.Errorf("Write() after Close error = %v, want ErrRotatingFileClosed", err)
	}
}

func TestRotatingFile_RenameFailureMidRotateSurfacesError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Running as root - permission-based failures can't be simulated")
	}

	dir := t.TempDir()

	path := filepath.Join(dir, "rotate.log")

	o := newRotatingFile(t, path, RotationConfig{MaxSizeBytes: 2})

	writeString(t, o, "b0")

	// A read-only directory makes the rename fail.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := o.Write(message.New(level.Info, "b1"))

	if err == nil || !strings.Contains(err.Error(), "failed renaming the log file") {
		t.Errorf("Write() error = %v, want the rename failure", err)
	}
}
