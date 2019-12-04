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
	"os"
	"os/exec"
	"testing"

	"github.com/alecthomas/units"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestRemainingArgsCatcher(t *testing.T) {
	assert := assert.New(t)

	rac := &remainingArgsCatcher{}
	assert.NoError(rac.verifyNoRemainingArgs())
	assert.Error(rac.verifyNRemainingArgs(1))
	assert.Equal(0, rac.numRemainingArgs())

	rac = &remainingArgsCatcher{
		RemainingArgs: remainingArgsField{
			Rest: []string{"arg0", "arg1"},
		},
	}
	assert.Error(rac.verifyNoRemainingArgs())
	assert.Equal(2, rac.numRemainingArgs())
	n := rac.numRemainingArgs()
	for i := 0; i < n; i++ {
		assert.Error(rac.verifyNRemainingArgs(i))
		assert.NotEmpty(rac.remainingArg(i))
	}
	assert.NoError(rac.verifyNRemainingArgs(n))
	assert.Error(rac.verifyNRemainingArgs(n + 1))

	reqIDRac := &requiredIDRemainingArgsCatcher{}
	assert.Error(reqIDRac.verifyRequiredIDAndNoRemainingArgs())
	reqIDRac.ID = "foo"
	assert.NoError(reqIDRac.verifyRequiredIDAndNoRemainingArgs())
	reqIDRac.RemainingArgs = remainingArgsField{Rest: []string{"bar"}}
	assert.Error(reqIDRac.verifyRequiredIDAndNoRemainingArgs())
	reqIDRac.ID = ""
	assert.NoError(reqIDRac.verifyRequiredIDAndNoRemainingArgs())
	assert.Equal("bar", reqIDRac.ID)

	opIDRac := &optionalIDRemainingArgsCatcher{}
	assert.NoError(opIDRac.verifyOptionalIDAndNoRemainingArgs())
	opIDRac.ID = "foo"
	assert.NoError(opIDRac.verifyOptionalIDAndNoRemainingArgs())
	opIDRac.RemainingArgs = remainingArgsField{Rest: []string{"bar"}}
	assert.Error(opIDRac.verifyOptionalIDAndNoRemainingArgs())
	opIDRac.ID = ""
	assert.NoError(opIDRac.verifyOptionalIDAndNoRemainingArgs())
	assert.Equal("bar", opIDRac.ID)
	opIDRac.ID = ""
	opIDRac.RemainingArgs = remainingArgsField{Rest: []string{"foo", "bar"}}
	assert.Error(opIDRac.verifyOptionalIDAndNoRemainingArgs())

	opNidRac := &optionalNidsRemainingArgsCatcher{}
	assert.NoError(opNidRac.verifyOptionalNidsAndNoRemainingArgs())
	opNidRac.Nids = []string{"foo"}
	assert.NoError(opNidRac.verifyOptionalNidsAndNoRemainingArgs())
	opNidRac.RemainingArgs = remainingArgsField{Rest: []string{"foo", "bar"}}
	assert.Error(opNidRac.verifyOptionalNidsAndNoRemainingArgs())
	opNidRac.Nids = nil
	assert.NoError(opNidRac.verifyOptionalNidsAndNoRemainingArgs())
	assert.Equal([]string{"foo", "bar"}, opNidRac.Nids)
}

func TestSizeToString(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("20TiB", sizeToString(20*int64(units.TiB)))
	assert.Equal("1TiB", sizeToString(1*int64(units.TiB)))
	assert.Equal("100TB", sizeToString(100*int64(units.TB)))
	assert.Equal("1TB", sizeToString(1*int64(units.TB)))

	assert.Equal("20GiB", sizeToString(20*int64(units.GiB)))
	assert.Equal("1GiB", sizeToString(1*int64(units.GiB)))
	assert.Equal("100GB", sizeToString(100*int64(units.GB)))
	assert.Equal("1GB", sizeToString(1*int64(units.GB)))

	assert.Equal("20MiB", sizeToString(20*int64(units.MiB)))
	assert.Equal("1MiB", sizeToString(1*int64(units.MiB)))
	assert.Equal("100MB", sizeToString(100*int64(units.MB)))
	assert.Equal("1MB", sizeToString(1*int64(units.MB)))

	assert.Equal("20KiB", sizeToString(20*int64(units.KiB)))
	assert.Equal("1KiB", sizeToString(1*int64(units.KiB)))
	assert.Equal("100KB", sizeToString(100*int64(units.KB)))
	assert.Equal("1KB", sizeToString(1*int64(units.KB)))

	assert.Equal("999B", sizeToString(999))
	assert.Equal("1B", sizeToString(1))
	assert.Equal("0B", sizeToString(0))
}

func TestAGMapAndOutput(t *testing.T) {
	assert := assert.New(t)

	agm := agFieldMap{
		"eN1": agField{"iN1", true},
		"eN2": agField{"iN2", false},
	}
	s, err := agm.validateSumFields([]string{"eN1"})
	assert.NoError(err)
	assert.Equal([]string{"iN1"}, s)
	s, err = agm.validateSumFields([]string{"eN2"})
	assert.Error(err)

	defer func() {
		outputWriter = os.Stdout
	}()
	var b bytes.Buffer
	outputWriter = &b

	e := &StdoutEmitter{}
	appCtx = &AppCtx{}
	appCtx.Emitter = e

	type tRes struct {
		TotalCount   int64
		Aggregations []string
	}
	res := tRes{10, []string{"iN1:sum:999"}}
	appCtx.OutputFormat = "table"
	expT := "+-------+-----------+-------+\n" +
		"| Field | Operation | Value |\n" +
		"+-------+-----------+-------+\n" +
		"|       | count     |    10 |\n" +
		"| eN1   | sum       |   999 |\n" +
		"+-------+-----------+-------+\n"
	assert.NoError(appCtx.EmitAggregation(agm, res))
	t.Log(b.String())
	assert.Equal(expT, b.String())

	e.Reset()
	appCtx.OutputFormat = "json"
	expJ := "{\n" +
		"    \"numberObjects\": 10,\n" +
		"    \"aggregations\": [\n" +
		"        {\n" +
		"            \"fieldName\": \"iN1\",\n" +
		"            \"operation\": \"sum\",\n" +
		"            \"value\": \"999\"\n" +
		"        }\n" +
		"    ]\n" +
		"}\n"
	b = bytes.Buffer{}
	outputWriter = &b
	assert.NoError(appCtx.EmitAggregation(agm, res))
	t.Log(b.String())
	assert.Equal(expJ, b.String())

	e.Reset()
	appCtx.OutputFormat = "yaml"
	expY := "numberObjects: 10\n" +
		"aggregations:\n" +
		"- fieldName: iN1\n" +
		"  operation: sum\n" +
		"  value: \"999\"\n"
	b = bytes.Buffer{}
	outputWriter = &b
	assert.NoError(appCtx.EmitAggregation(agm, res))
	t.Log(b.String())
	assert.Equal(expY, b.String())
}

func TestParseColumns(t *testing.T) {
	assert := assert.New(t)

	appCtx = &AppCtx{}
	validCols := []string{"A", "B", "C", "D", "E", "F", "G"}
	defaultCols := []string{"A", "B", "C"}

	errTCs := []string{
		"h,I,j", " H, I ,J ", "*,*",
	}
	for _, tc := range errTCs {
		exp, err := appCtx.parseColumns(tc, validCols, defaultCols)
		assert.Errorf(err, "Case: %s", tc)
		assert.Nil(exp, "Case: %s", tc)
	}

	defTCs := []string{
		"", ",", " ", " ,", " , ", "*", ", *", " , , * , ", "-D,-e,-f,-g",
	}
	for _, tc := range defTCs {
		exp, err := appCtx.parseColumns(tc, validCols, defaultCols)
		assert.NoError(err, "Case: %s", tc)
		assert.Equal(defaultCols, exp, "Case: %s", tc)
	}

	exp, err := appCtx.parseColumns("a,C,g", validCols, defaultCols)
	assert.NoError(err)
	assert.Equal([]string{"A", "C", "G"}, exp)

	exp, err = appCtx.parseColumns("+g,+F", validCols, defaultCols)
	assert.NoError(err)
	assert.Equal([]string{"G", "F", "A", "B", "C"}, exp)

	exp, err = appCtx.parseColumns("*,+g,+F", validCols, defaultCols)
	assert.NoError(err)
	assert.Equal([]string{"A", "B", "C", "G", "F"}, exp)

	exp, err = appCtx.parseColumns("+g,*,+F", validCols, defaultCols)
	assert.NoError(err)
	assert.Equal([]string{"G", "A", "B", "C", "F"}, exp)

	exp, err = appCtx.parseColumns("+g,-b, *,+F", validCols, defaultCols)
	assert.NoError(err)
	assert.Equal([]string{"G", "A", "C", "F"}, exp)

	exp, err = appCtx.parseColumns("+g,-b, *,+F,B", validCols, defaultCols)
	assert.NoError(err)
	assert.Equal([]string{"G", "A", "C", "F", "B"}, exp)
}

func TestShowCols(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	cmd, _ := parser.AddCommand("testCommand", "testCommand short", "testCommand long", &testCmd{})

	headers := map[string]string{
		"head1": "Description one.",
		"head2": "Description two.",
		"head3": "Description three.",
		"head4": "Description fours.",
	}
	cmd.AddCommand("show-columns", "Show table columns", "Show names of columns used in table format", &showColsCmd{columns: headers})

	// show-columns (table)
	te := &TestEmitter{}
	appCtx.Emitter = te
	err := parseAndRun([]string{"testCommand", "show-columns"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	t.Log(te.tableData)
	assert.Len(te.tableHeaders, 2)
	assert.Len(te.tableData, len(headers))
	cN := make(map[string]string, len(te.tableData))
	for _, r := range te.tableData {
		cN[r[0]] = r[1]
	}
	assert.Equal(headers, cN)

	// show-columns (json, alias)
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = parseAndRun([]string{"testCommand", "show-columns", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(headers, te.jsonData)

	// show-columns (yaml)
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = parseAndRun([]string{"testCommand", "show-columns", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(headers, te.yamlData)
}

func TestClearTerminal(t *testing.T) {
	assert := assert.New(t)

	fos := &fakeOSExecutor{}
	appCtx = &AppCtx{}
	appCtx.osExec = fos

	appCtx.ClearTerminal()
	assert.NotNil(fos.Cmd)
	assert.Equal("clear", fos.Cmd.Path)
	assert.Empty(fos.Cmd.Args)
	assert.Equal(os.Stdout, fos.Cmd.Stdout)
	assert.True(fos.RunCalled)
}

func TestOSExecutor(t *testing.T) {
	assert := assert.New(t)

	// Must run something real to get code coverage on OSCommand
	cmd := appCtx.OSCommand("ls")
	err := appCtx.OSCommandRun(cmd)
	assert.NoError(err)
}

type fakeOSExecutor struct {
	Cmd       *exec.Cmd
	RunCalled bool
	RetRunErr error
}

// OSCommand creates an exec.Cmd
func (o *fakeOSExecutor) OSCommand(name string, arg ...string) *exec.Cmd {
	o.Cmd = &exec.Cmd{}
	o.Cmd.Path = name
	o.Cmd.Args = append([]string{}, arg...)
	return o.Cmd
}

// OSCommandRun invokes the cmd.Run method
func (o *fakeOSExecutor) OSCommandRun(c *exec.Cmd) error {
	o.RunCalled = true
	return o.RetRunErr
}
