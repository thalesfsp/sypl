// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.
//
//nolint:exhaustruct,revive
package es_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/es"
	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/shared"
)

var (
	esConfig = es.Config{
		Addresses: []string{os.Getenv("SYPL_ELASTICSEARCH_TEST_ADDRESS")},
	}
	commonPrefix       = "test"
	esIndexName        = fmt.Sprintf("%s1", commonPrefix)
	esIndexName1TagMap = fmt.Sprintf("%s2", commonPrefix)
	esIndexName2TagMap = fmt.Sprintf("%s3", commonPrefix)
	esIndexName3TagMap = fmt.Sprintf("%s-catch-all", commonPrefix)
	esTagName1TagMap   = esIndexName1TagMap
	esTagName2TagMap   = esIndexName2TagMap
	esTagName3TagMap   = "*"
)

func TestNewIntegration(t *testing.T) {
	type args struct {
		component string
		content   string
		level     level.Level
		maxLevel  level.Level

		run func(a args) string
	}

	elasticSearchOutput := args{
		component: shared.DefaultComponentNameOutput,
		content:   shared.DefaultContentOutput,
		level:     level.Info,
		maxLevel:  level.Trace,
		run: func(a args) string {
			// Creates logger, and name it.
			l := sypl.New(shared.DefaultComponentNameOutput, es.Output(
				esIndexName,
				esConfig,
				level.Trace,
			))

			l.Infoln(shared.DefaultContentOutput)

			return shared.DefaultContentOutput
		},
	}

	elasticSearchTagMapOutput := args{
		component: shared.DefaultComponentNameOutput,
		content:   shared.DefaultContentOutput,
		level:     level.Info,
		maxLevel:  level.Trace,
		run: func(a args) string {
			// Creates logger, and name it.
			l := sypl.New(shared.DefaultComponentNameOutput, es.OutputWithTagMap(
				es.TagMap{
					esTagName1TagMap: es.NewTagMapItem(a.maxLevel, func() string { return esIndexName1TagMap }),
					esTagName2TagMap: es.NewTagMapItem(a.maxLevel, func() string { return esIndexName2TagMap }),
					esTagName3TagMap: es.NewTagMapItem(a.maxLevel, func() string { return esIndexName3TagMap }),
				},
				esConfig,
			)...)

			l.PrintWithOptions(
				level.Info,
				shared.DefaultContentOutput,
				sypl.WithTags(esTagName1TagMap),
			)

			l.PrintWithOptions(
				level.Info,
				shared.DefaultContentOutput,
				sypl.WithTags(esTagName2TagMap),
			)

			l.Infoln(shared.DefaultContentOutput)

			return shared.DefaultContentOutput
		},
	}

	tests := []struct {
		name    string
		args    args
		want    func(a args) string
		CleanUp func()
	}{
		{
			name: "Should print - elasticSearchOutput",
			args: elasticSearchOutput,
			want: func(a args) string {
				return shared.DefaultContentOutput
			},
			CleanUp: func() {
			},
		},
		{
			name: "Should print - elasticSearchTagMapOutput",
			args: elasticSearchTagMapOutput,
			want: func(a args) string {
				return shared.DefaultContentOutput
			},
			CleanUp: func() {
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.EqualFold(os.Getenv("SYPL_TEST_MODE"), "integration") {
				t.SkipNow()
			}

			message := tt.args.run(tt.args)
			want := tt.want(tt.args)

			if message != want {
				t.Errorf("Got %v, want %v", message, want)
			}
		})
	}

	t.Cleanup(func() {
		if !strings.EqualFold(os.Getenv("SYPL_TEST_MODE"), "integration") {
			t.SkipNow()
		}

		t.Log("Cleaning up...")

		time.Sleep(1 * time.Second)

		_, err := es.
			New(esIndexName, esConfig).
			Client.Indices.
			Delete([]string{esIndexName, esIndexName1TagMap, esIndexName2TagMap, esIndexName3TagMap})
		if err != nil {
			t.Fatalf("Error deleting index: %s", err)
		}
	})
}
