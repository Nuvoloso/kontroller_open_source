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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/olekukonko/tablewriter"
	"github.com/stretchr/testify/assert"
)

type TestEmitter struct {
	jsonData     interface{}
	yamlData     interface{}
	tableData    [][]string
	tableHeaders []string
}

func (e *TestEmitter) EmitJSON(data interface{}) error {
	e.jsonData = data
	return nil
}

func (e *TestEmitter) EmitYAML(data interface{}) error {
	e.yamlData = data
	return nil
}

func (e *TestEmitter) EmitTable(headers []string, data [][]string, customizeTable func(t *tablewriter.Table)) error {
	e.tableData = data
	e.tableHeaders = headers
	if customizeTable != nil {
		var b bytes.Buffer
		ow := outputWriter
		defer func() {
			outputWriter = ow
		}()
		outputWriter = &b
		se := StdoutEmitter{}
		se.EmitTable(headers, data, customizeTable)
	}
	return nil
}

func TestStdoutEmitter(t *testing.T) {
	assert := assert.New(t)

	defer func() {
		outputWriter = os.Stdout
	}()
	var b bytes.Buffer
	outputWriter = &b

	// json ok
	type tE struct {
		Field1   int `json:"fieldOne"`
		Field2   string
		IsACamel bool
	}
	o := tE{2, "2humps", true}
	j := "{\n" +
		`    "fieldOne": 2,` + "\n" +
		`    "Field2": "2humps",` + "\n" +
		`    "IsACamel": true` + "\n" +
		"}\n"
	e := &StdoutEmitter{}

	err := e.EmitJSON(o)
	assert.Nil(err)
	assert.True(b.Len() > 0)
	t.Log(b.String())
	assert.Equal(j, b.String())

	// json failure (test case from source)
	b = bytes.Buffer{}
	outputWriter = &b
	err = e.EmitJSON(&struct {
		N json.Number
	}{json.Number(`invalid`)})
	t.Log(err)
	t.Log(b.String())
	assert.NotNil(err)

	// yaml ok
	e.Reset()
	y := "field1: 2\n" +
		"field2: 2humps\n" +
		"isacamel: true\n"
	b = bytes.Buffer{}
	outputWriter = &b
	err = e.EmitYAML(o)
	assert.Nil(err)
	assert.True(b.Len() > 0)
	t.Log(b.String())
	assert.Equal(y, b.String())

	// multi-doc yaml
	e.Reset()
	y = "field1: 2\n" +
		"field2: 2humps\n" +
		"isacamel: true\n" +
		"---\n" +
		"field1: 2\n" +
		"field2: 2humps\n" +
		"isacamel: true\n"
	b = bytes.Buffer{}
	outputWriter = &b
	err = e.EmitYAML(o)
	assert.Nil(err)
	err = e.EmitYAML(o)
	assert.Nil(err)
	assert.True(b.Len() > 0)
	t.Log(b.String())
	assert.Equal(y, b.String())

	// yaml error (test case from source)
	e.Reset()
	b = bytes.Buffer{}
	outputWriter = &b
	err = e.EmitYAML(&failingMarshaler{})
	t.Log(err)
	t.Log(b.String())
	assert.NotNil(err)
	assert.Equal("YAML MARSHAL FAILED", err.Error())

	// yaml panic caught (test case from source)
	e.Reset()
	b = bytes.Buffer{}
	outputWriter = &b
	err = e.EmitYAML(&struct {
		A int
		B map[string]int `yaml:",inline"`
	}{1, map[string]int{"a": 2}})
	t.Log(err)
	t.Log(b.String())
	assert.NotNil(err)
	assert.Regexp("conflicts with struct field", err.Error())

	// table ok
	exp := "+--------+--------+----------+\n" +
		"| Field1 | Field2 | IsACamel |\n" +
		"+--------+--------+----------+\n" +
		"|      2 | 2humps | true     |\n" +
		"+--------+--------+----------+\n"
	b = bytes.Buffer{}
	outputWriter = &b
	err = e.EmitTable([]string{"Field1", "Field2", "IsACamel"},
		[][]string{{fmt.Sprintf("%d", o.Field1), o.Field2, fmt.Sprintf("%v", o.IsACamel)}}, nil)
	t.Log(err)
	t.Log(b.String())
	assert.Nil(err)
	assert.Equal(exp, b.String())

	// table ok, customized
	exp = "+--------+--------+----------+\n" +
		"| Field1 | Field2 | IsACamel |\n" +
		"|      2 | 2humps | true     |\n" +
		"+--------+--------+----------+\n"
	b = bytes.Buffer{}
	outputWriter = &b
	err = e.EmitTable([]string{"Field1", "Field2", "IsACamel"},
		[][]string{{fmt.Sprintf("%d", o.Field1), o.Field2, fmt.Sprintf("%v", o.IsACamel)}},
		func(tb *tablewriter.Table) { tb.SetHeaderLine(false) })
	t.Log(err)
	t.Log(b.String())
	assert.Nil(err)
	assert.Equal(exp, b.String())
}

type failingMarshaler struct{}

func (ft *failingMarshaler) MarshalYAML() (interface{}, error) {
	return nil, fmt.Errorf("YAML MARSHAL FAILED")
}
