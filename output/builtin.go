// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package output

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/processor"
	"github.com/thalesfsp/sypl/safebuffer"
	"github.com/thalesfsp/sypl/shared"
)

// Handles the common used "-" making the output behave as a Console writing to
// stdout, and named "-".
func dashHandler(name, path string, maxLevel level.Level, processors ...processor.IProcessor) IOutput {
	if path == "-" {
		return New(name, maxLevel, os.Stdout, processors...)
	}

	return nil
}

//////
// Built-in outputs.
//////

// Console is a built-in `output` - named `Console`, that writes to `stdout`.
func Console(maxLevel level.Level, processors ...processor.IProcessor) IOutput {
	return New("Console", maxLevel, os.Stdout, processors...)
}

// StdErr is a built-in `output` - named `StdErr`, that only writes to `stderr`
// message @ Error level.
func StdErr(processors ...processor.IProcessor) IOutput {
	processors = append(processors, processor.PrintOnlyAtLevel(level.Fatal, level.Error))

	return New("StdErr", level.Error, os.Stderr, processors...)
}

// FileBased is a built-in `output`, that writes to the specified file.
func FileBased(
	name string,
	maxLevel level.Level,
	writer io.Writer,
	processors ...processor.IProcessor,
) IOutput {
	return New(name, maxLevel, writer, processors...)
}

// File is a built-in `output` - named `File`, that writes to the specified file.
//
// NOTE: If the common used "-" is used, it will behave as a Console writing to
// stdout.
// NOTE: If no path is provided, it'll create one in the OS's temp directory.
// NOTE: If the dir and/or file does not exist, it will be created.
func File(path string, maxLevel level.Level, processors ...processor.IProcessor) IOutput {
	// Should create a file in the OS temp. File name should be unique (UUIDv4).
	if path == "" {
		path = filepath.Join(os.TempDir(), fmt.Sprintf("%s.log", shared.GenerateUUID()))
	}

	if o := dashHandler("File", path, maxLevel, processors...); o != nil {
		return o
	}

	f, err := os.OpenFile(
		path,
		// Append, or create if not exists, and write only.
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		shared.DefaultFileMode,
	)
	if err != nil {
		// Should try to create the dir if it doesn't exist.
		if os.IsNotExist(err) {
			dir := filepath.Dir(path)

			if err := os.MkdirAll(dir, os.ModePerm); err != nil {
				log.Fatalf("%s File Output: Failed to create dir %s: %s", shared.ErrorPrefix, dir, err)
			}

			// Try again.
			return File(path, maxLevel, processors...)
		}

		log.Fatalf("%s File Output: Failed to create/open %s: %s", shared.ErrorPrefix, path, err)
	}

	log.Printf("%s File Output: No path provided. Created/opened \"%s\"", shared.WarnPrefix, path)

	return FileBased("File", maxLevel, f, processors...)
}

// SafeBuffer is a built-in `output` - named `Buffer`, that writes to the buffer.
func SafeBuffer(maxLevel level.Level, processors ...processor.IProcessor) (*safebuffer.Buffer, IOutput) {
	var buf safebuffer.Buffer

	o := New("Buffer", maxLevel, &buf, processors...)

	return &buf, o
}
