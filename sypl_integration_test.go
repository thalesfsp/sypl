// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.
//
//nolint:exhaustruct
package sypl

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/thalesfsp/sypl/elasticsearch"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/shared"
)

var (
	esConfig = output.ElasticSearchConfig{
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
			l := New(shared.DefaultComponentNameOutput, output.ElasticSearch(
				esIndexName,
				esConfig,
				level.Trace,
			))

			l.Infoln(shared.DefaultContentOutput)

			return shared.DefaultContentOutput
		},
	}

	ElasticSearchTagMapOutput := args{
		component: shared.DefaultComponentNameOutput,
		content:   shared.DefaultContentOutput,
		level:     level.Info,
		maxLevel:  level.Trace,
		run: func(a args) string {
			// Creates logger, and name it.
			l := New(shared.DefaultComponentNameOutput, output.ElasticSearchWithTagMap(
				map[string]output.ElasticSearchTagMapItem{
					esTagName1TagMap: output.NewElasticSearchTagMapItem(a.maxLevel, func() string { return esIndexName1TagMap }),
					esTagName2TagMap: output.NewElasticSearchTagMapItem(a.maxLevel, func() string { return esIndexName2TagMap }),
					esTagName3TagMap: output.NewElasticSearchTagMapItem(a.maxLevel, func() string { return esIndexName3TagMap }),
				},
				esConfig,
			)...)

			l.PrintWithOptions(
				level.Info,
				shared.DefaultContentOutput,
				WithTags(esTagName1TagMap),
			)

			l.PrintWithOptions(
				level.Info,
				shared.DefaultContentOutput,
				WithTags(esTagName2TagMap),
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
			name: "Should print - ElasticSearchTagMapOutput",
			args: ElasticSearchTagMapOutput,
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

		_, err := elasticsearch.
			New(esIndexName, esConfig).
			Client.Indices.
			Delete([]string{esIndexName, esIndexName1TagMap, esIndexName2TagMap, esIndexName3TagMap})
		if err != nil {
			t.Fatalf("Error deleting index: %s", err)
		}
	})
}
