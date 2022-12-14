// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package options

import (
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/flag"
)

// Options extends printer's capabilities.
//
// NOTE: Changes in the `Message` or `Options` data structure may trigger
// changes in the `New`, `Copy` (from `Message`), `mergeOptions` (from `Sypl`),
// `New` methods, and the formatters.
type Options struct {
	// Structured fields.
	Fields fields.Fields

	// Flags define behaviors.
	Flag flag.Flag

	// OutputsNames are the names of the outputs to be used.
	OutputsNames []string

	// ProcessorsNames are the names of the processors to be used.
	ProcessorsNames []string

	// Tags are indicators consumed by `Output`s and `Processor`s.
	Tags []string
}

//////
// Factory.
//////

// New is the `Options` factory.
//
// NOTE: Changes in the `Message` or `Options` data structure may reflects here.
func New() *Options {
	return &Options{
		Fields:          fields.Fields{},
		Flag:            flag.None,
		OutputsNames:    []string{},
		ProcessorsNames: []string{},
		Tags:            []string{},
	}
}
