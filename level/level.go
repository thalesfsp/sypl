// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package level

import (
	"fmt"
	"log"
	"strings"

	"github.com/thalesfsp/sypl/shared"
)

// Level specification.
type Level int

// Available levels.
const (
	None Level = iota
	Fatal
	Error
	Info
	Warn
	Debug
	Trace
)

var names = []string{"none", "fatal", "error", "info", "warn", "debug", "trace"}

// String interface implementation.
func (l Level) String() string {
	if l < None || l > Trace {
		return "Unknown"
	}

	return names[l]
}

// FromInt returns a `Level` from a given integer.
//
// NOTE: Failure will return "Unknown".
func FromInt(level int) Level {
	return Level(level)
}

// FromString returns a `Level` from a given string. It can also be used to
// validate if a given string, is a `Level`. An invalid level will return `None`
// as `Level`, and not ok (`false`).
func FromString(level string) (Level, error) {
	if level == "" {
		return None, fmt.Errorf("%w: No level specified. Available: %s", ErrInvalidLevel, strings.Join(LevelsNames(), ", "))
	}

	for i, levelString := range names {
		if strings.EqualFold(level, levelString) {
			return Level(i), nil
		}
	}

	return None, fmt.Errorf("%w: %s. Available: %s", ErrInvalidLevel, level, strings.Join(LevelsNames(), ", "))
}

// MustFromString returns a `Level` from a given string. Failure will log, and
// exit printing available levels.
func MustFromString(level string) Level {
	if level == "" {
		log.Fatalf("%s No level specified. Available: %s", shared.ErrorPrefix, strings.Join(LevelsNames(), ", "))
	}

	for i, levelString := range names {
		if strings.EqualFold(level, levelString) {
			return Level(i)
		}
	}

	log.Fatalf("%s Invalid level: %s. Available: %s", shared.ErrorPrefix, level, strings.Join(names, ", "))

	return None
}

// LevelsToString converts a slice of levels to string (concatenated).
func LevelsToString(levels []Level) string {
	names := []string{}

	for _, level := range levels {
		names = append(names, level.String())
	}

	return strings.Join(names, ",")
}

// LevelsNames returns the name of all available levels.
func LevelsNames() []string {
	finalNames := []string{}

	for _, name := range names {
		finalNames = append(finalNames, strings.ToLower(name))
	}

	return finalNames
}
