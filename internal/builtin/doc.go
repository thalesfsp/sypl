// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

// Package builtin is a minimal, mutex-guarded writer replacing Golang's
// built-in logger with only one behavioral change: it does NOT
// force-append a newline to the message.
//
// See this https://github.com/golang/go/issues/16564 for more info.
//
// V2: the v1 fork of the stdlib logger (date/time/file flags, prefixes,
// Print/Fatal/Panic families) was deleted - sypl always constructed it
// flagless, and prefixless, and formatting is the formatter's job.
package builtin
