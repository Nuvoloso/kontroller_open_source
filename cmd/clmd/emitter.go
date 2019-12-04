// Copyright 2019 Tad Lebeck
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


package main

import (
	"encoding/json"
	"fmt"

	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v2"
)

// Emitter provides an abstraction of output emitters
type Emitter interface {
	// JSONEmitter is an interface that emits JSON
	EmitJSON(data interface{}) error
	// YAMLEmitter is an interface that emits YAML
	EmitYAML(data interface{}) error
	// TableEmitter is an interface that emits a table. The callback is optional.
	EmitTable(headers []string, data [][]string, customizeTable func(t *tablewriter.Table)) error
}

var emptyStruct = struct{}{}

// StdoutEmitter emits output to stdout
type StdoutEmitter struct{}

// EmitJSON emits JSON to stdout
func (e *StdoutEmitter) EmitJSON(data interface{}) error {
	b, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}
	fmt.Fprintln(outputWriter, string(b))
	return nil
}

// EmitYAML emits YAML to stdout
func (e *StdoutEmitter) EmitYAML(data interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = fmt.Errorf("%s", x)
			default:
				panic(r)
			}
		}
	}()
	b, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(outputWriter, "%s", string(b))
	return nil
}

// EmitTable emits a table to stdout
func (e *StdoutEmitter) EmitTable(headers []string, data [][]string, customizeTable func(t *tablewriter.Table)) error {
	table := tablewriter.NewWriter(outputWriter)
	if len(headers) > 0 {
		table.SetHeader(headers)
		table.SetAutoFormatHeaders(false)
	}
	if customizeTable != nil {
		customizeTable(table)
	}
	table.AppendBulk(data)
	table.Render()
	return nil
}
