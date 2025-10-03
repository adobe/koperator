// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package docgen_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/banzaicloud/operator-tools/pkg/docgen"
	"github.com/banzaicloud/operator-tools/pkg/utils"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
)

var logger logr.Logger

func init() {
	logger = utils.Log
}

// normalizeString trims trailing whitespace from each line and the overall string
func normalizeString(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func TestGenParse(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)

	var testData = []struct {
		docItem  docgen.DocItem
		expected string
	}{
		{
			docItem: docgen.DocItem{
				Name:       "sample",
				SourcePath: filepath.Join(currentDir, "testdata", "sample.go"),
				DestPath:   filepath.Join(currentDir, "../../build/_test/docgen"),
			},
			expected: heredoc.Doc(`
					## Sample

					### field1 (string, optional) {#sample-field1}

					Default: -
			`),
		},
		{
			docItem: docgen.DocItem{
				Name:       "sample-default",
				SourcePath: filepath.Join(currentDir, "testdata", "sample_default.go"),
				DestPath:   filepath.Join(currentDir, "../../build/_test/docgen"),
				DefaultValueFromTagExtractor: func(tag string) string {
					return docgen.GetPrefixedValue(tag, `asd:\"default:(.*)\"`)
				},
			},
			expected: heredoc.Doc(`
				## SampleDefault

				### field1 (string, optional) {#sampledefault-field1}

				Default: testval
			`),
		},
		{
			docItem: docgen.DocItem{
				Name:       "sample-codeblock",
				SourcePath: filepath.Join(currentDir, "testdata", "sample_codeblock.go"),
				DestPath:   filepath.Join(currentDir, "../../build/_test/docgen"),
				DefaultValueFromTagExtractor: func(tag string) string {
					return docgen.GetPrefixedValue(tag, `asd:\"default:(.*)\"`)
				},
			},
			expected: heredoc.Doc(`
				## Sample

				### field1 (string, optional) {#sample-field1}

				Description
				{{< highlight yaml >}}
				test: code block
				some: more lines
				    indented: line
				{{< /highlight >}}


				Default: -
			`),
		},
	}

	for _, item := range testData {
		parser := docgen.GetDocumentParser(item.docItem, logger)
		err := parser.Generate()
		if err != nil {
			t.Fatalf("%+v", err)
		}

		bytes, err := os.ReadFile(filepath.Join(item.docItem.DestPath, item.docItem.Name+".md"))
		if err != nil {
			t.Fatalf("%+v", err)
		}

		actual := normalizeString(string(bytes))
		expected := normalizeString(item.expected)
		if diff := cmp.Diff(expected, actual); diff != "" {
			t.Errorf("Result mismatch (-want +got):\n%s", diff)
		}
	}
}
