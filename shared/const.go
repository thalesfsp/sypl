// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package shared

// Standard logging prefix for internal errors.
const (
	ErrorPrefix = "[sypl] [Error]"
	WarnPrefix  = "[sypl] [Warn]"
)

// Default values.
const (
	DefaultFileMode = 0o644
)

// Env vars that affects Sypl.
const (
	FilterEnvVar = "SYPL_FILTER"
	LevelEnvVar  = "SYPL_LEVEL"
)
