// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package es

import (
	"fmt"
	"slices"

	"github.com/thalesfsp/sypl/v2/formatter"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/output"
	"github.com/thalesfsp/sypl/v2/processor"
)

//////
// Consts, vars, and types.
//////

// TagMapItem is an item of the ElasticSearch tag map.
//
// NOTE: Formerly `output.ElasticSearchTagMapItem`.
type TagMapItem struct {
	DynamicIndexFunc DynamicIndexFunc
	Level            level.Level
}

// TagMap is a map of tags to index names.
//
// NOTE: Formerly `output.ElasticSearchTagMap`.
type TagMap = map[string]TagMapItem

//////
// Helpers.
//////

// NewTagMapItem is a helper to create `TagMapItem`.
//
// NOTE: Formerly `output.NewElasticSearchTagMapItem`.
func NewTagMapItem(
	l level.Level,
	dynamicIndexFunc DynamicIndexFunc,
) TagMapItem {
	return TagMapItem{
		DynamicIndexFunc: dynamicIndexFunc,
		Level:            l,
	}
}

//////
// Factory.
//////

// outputFactory builds ElasticSearch outputs.
func outputFactory(
	outputName string,
	dynamicIndexFunc DynamicIndexFunc,
	esConfig Config,
	maxLevel level.Level,
	processors ...processor.IProcessor,
) output.IOutput {
	o := output.New(outputName,
		maxLevel,
		NewWithDynamicIndex(dynamicIndexFunc, esConfig),
		processors...,
	).SetFormatter(formatter.JSONPretty())

	return o
}

//////
// Builtins.
//////

// Output is a built-in `output` - named `ElasticSearch`, that writes to
// ElasticSearch.
//
// NOTE: Formerly `output.ElasticSearch`.
// NOTE: By default, data is JSON-formatted.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
func Output(
	indexName string,
	esConfig Config,
	maxLevel level.Level,
	processors ...processor.IProcessor,
) output.IOutput {
	return outputFactory(
		"ElasticSearch",
		func() string { return indexName },
		esConfig,
		maxLevel,
		processors...,
	)
}

// OutputWithTagMap is a built-in `output` - named
// `ElasticSearchWithTagMap-{tag}` that writes to ElasticSearch. It allows to
// define a map of tags and indexes. The index name is a function which defines
// the name of the index and is evaluated at the index time.
//
// IT'S THE CALLER'S RESPONSIBILITY TO DEFINE A CATCH-ALL INDEX - IF DESIRED.
// TO ACHIEVE THIS, USE `*` AS THE TAG NAME.
//
// NOTE: Formerly `output.ElasticSearchWithTagMap`.
// NOTE: By default, data is JSON-formatted.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
func OutputWithTagMap(
	tagMap TagMap,
	esConfig Config,
	processors ...processor.IProcessor,
) []output.IOutput {
	outputs := make([]output.IOutput, 0, len(tagMap))
	tags := make([]string, 0, len(tagMap))

	var eSTMI TagMapItem

	for tag, tagMapItem := range tagMap {
		if tag == "*" {
			eSTMI = tagMapItem
		} else {
			outputs = append(outputs, outputFactory(
				fmt.Sprintf("ElasticSearchWithTagMap-%s", tag),
				tagMapItem.DynamicIndexFunc,
				esConfig,
				tagMapItem.Level,
				// NOTE: Clone before appending - `append(processors, ...)`
				// would alias the caller's backing array when it has spare
				// capacity, so later iterations would overwrite earlier
				// outputs' tag processor.
				append(slices.Clone(processors), processor.PrintOnlyIfTagged(tag))...,
			))

			tags = append(tags, tag)
		}
	}

	if eSTMI.DynamicIndexFunc != nil {
		outputs = append(outputs, outputFactory(
			"ElasticSearchWithTagMap-*",
			eSTMI.DynamicIndexFunc,
			esConfig,
			eSTMI.Level,
			append(slices.Clone(processors), processor.PrintOnlyIfNotTaggedWith(tags...))...,
		))
	}

	return outputs
}

// OutputWithDynamicIndex is a built-in `output` - named
// `ElasticSearchWithDynamicIndex` that writes to ElasticSearch. It allows to
// define a function that returns the index name to be used, evaluated at the
// index time.
//
// NOTE: Formerly `output.ElasticSearchWithDynamicIndex`.
// NOTE: By default, data is JSON-formatted.
// NOTE: It's the caller's responsibility to create the index, define its
// mapping, and settings.
func OutputWithDynamicIndex(
	dynamicIndexFunc DynamicIndexFunc,
	esConfig Config,
	maxLevel level.Level,
	processors ...processor.IProcessor,
) output.IOutput {
	return outputFactory(
		fmt.Sprintf("ElasticSearchWithDynamicIndex-%s", dynamicIndexFunc()),
		dynamicIndexFunc,
		esConfig,
		maxLevel,
		processors...,
	)
}
