// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// Package sypltest carries test-only fixtures shared across the sypl
// modules' test suites. It is internal on purpose: these values were
// exported from `shared` in v1 by accident - they are not consumer API.
package sypltest

// Default values used in tests.
const (
	DefaultComponentNameOutput = "componentNameTest"
	DefaultContentOutput       = "contentTest"
	DefaultPrefixValue         = "My Prefix - "
	DefaultTimestampFormat     = "2006"
)
