// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.
//
//nolint:exhaustruct
package sypl

import (
	"os"
	"strings"
	"testing"

	"github.com/thalesfsp/sypl/elasticsearch"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/sypl/output"
	"github.com/thalesfsp/sypl/shared"
)

var (
	esConfig = output.ElasticSearchConfig{
		Addresses: []string{"http://localhost:9200"},
	}
	esIndexName = "test"
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
				_, err := elasticsearch.New(esIndexName, esConfig).Client.Indices.Delete([]string{esIndexName})
				if err != nil {
					t.Fatalf("Error deleting index: %s", err)
				}
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

			tt.CleanUp()
		})
	}
}
