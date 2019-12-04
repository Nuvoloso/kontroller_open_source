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
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/olekukonko/tablewriter"
)

type remainingArgsField struct {
	Rest []string `positional-arg-name:"[--] "`
}

// remainingArgsCatcher provides a generic way to handle un-flagged arguments.
// It provides methods to examine these arguments or validate that there
// are only N such arguments present.
type remainingArgsCatcher struct {
	RemainingArgs remainingArgsField `positional-args:"yes"`
}

func (c *remainingArgsCatcher) verifyNoRemainingArgs() error {
	return c.verifyNRemainingArgs(0)
}

func (c *remainingArgsCatcher) verifyNRemainingArgs(n int) error {
	len := len(c.RemainingArgs.Rest)
	if len < n {
		return fmt.Errorf("#non-flag arguments expected: %d", n)
	}
	if len > n {
		return fmt.Errorf("unexpected arguments: %v", c.RemainingArgs.Rest[n:])
	}
	return nil
}

func (c *remainingArgsCatcher) numRemainingArgs() int {
	return len(c.RemainingArgs.Rest)
}

func (c *remainingArgsCatcher) remainingArg(i int) string {
	return c.RemainingArgs.Rest[i]
}

// requiredIDRemainingArgsCatcher provides a consistent way to process a required
// ID argument as an optional un-flagged singleton argument.
type requiredIDRemainingArgsCatcher struct {
	ID string `long:"id" description:"The identifier of the object concerned. May also be specified without the '--id' flag. Required"`
	remainingArgsCatcher
}

// verifyRequiredIDAndNoRemainingArgs verifies that the ID is set either from a flag or from
// the (only) un-flagged argument.
func (c *requiredIDRemainingArgsCatcher) verifyRequiredIDAndNoRemainingArgs() error {
	if c.ID == "" {
		if len(c.RemainingArgs.Rest) != 1 {
			return fmt.Errorf("expected --id flag or a single identifier argument")
		}
		c.ID = c.remainingArg(0)
		return nil
	}
	return c.verifyNoRemainingArgs()
}

// optionalIDRemainingArgsCatcher provides a consistent way to process an optional
// ID argument as an un-flagged singleton argument.
type optionalIDRemainingArgsCatcher struct {
	ID string `long:"id" description:"The identifier of the object concerned. May also be specified without the '--id' flag. Optional"`
	remainingArgsCatcher
}

// verifyOptionalIDAndNoRemainingArgs verifies that the ID is either set from a flag
// or from an un-flagged argument. The ID may be unset.
func (c *optionalIDRemainingArgsCatcher) verifyOptionalIDAndNoRemainingArgs() error {
	if c.ID == "" {
		if len(c.RemainingArgs.Rest) > 0 {
			if err := c.verifyNRemainingArgs(1); err != nil {
				return err
			}
			c.ID = c.remainingArg(0)
			return nil
		}
	}
	return c.verifyNoRemainingArgs()
}

// optionalNidsRemainingArgsCatcher provides a way to process an optional list of Node object identifiers
// either with flags or with un-flagged arguments.
type optionalNidsRemainingArgsCatcher struct {
	Nids []string `long:"id" description:"A node object identifier. May also be specified without the '--id' flag. Repeat to specify multiple. Optional"`
	remainingArgsCatcher
}

func (c *optionalNidsRemainingArgsCatcher) verifyOptionalNidsAndNoRemainingArgs() error {
	if len(c.Nids) == 0 {
		c.Nids = c.RemainingArgs.Rest
		return nil
	}
	return c.verifyNoRemainingArgs()
}

func sizeToString(size int64) string {
	return util.SizeBytesToString(size)
}

// Support for Aggregation headers
type agField struct {
	fieldName string // internal name
	sumOk     bool
}

type agFieldMap map[string]agField

func (agm agFieldMap) validateSumFields(inF []string) ([]string, error) {
	outF := make([]string, 0, len(inF))
	for _, f := range inF {
		af, ok := agm[f]
		if !ok || !af.sumOk {
			return nil, fmt.Errorf("Unsupported field for sum: %s", f)
		}
		outF = append(outF, af.fieldName)
	}
	return outF, nil
}

var agHeaders = []string{"Field", "Operation", "Value"}

type agResultField struct {
	FieldName string `json:"fieldName" yaml:"fieldName"`
	Operation string `json:"operation" yaml:"operation"`
	Value     string `json:"value" yaml:"value"`
}
type agResult struct {
	NumberObjects int64           `json:"numberObjects" yaml:"numberObjects"`
	Aggregations  []agResultField `json:"aggregations" yaml:"aggregations"`
}

// EmitAggregation prints aggregation headers in the requested output format
func (appCtx *AppCtx) EmitAggregation(agm agFieldMap, res interface{}) error {
	pV := reflect.Indirect(reflect.ValueOf(res))
	pFV := pV.FieldByName("TotalCount")
	totalCount := pFV.Int()
	pFV = pV.FieldByName("Aggregations")
	aggregations := pFV.Interface().([]string)
	rMap := make(map[string]string)
	for k, agf := range agm {
		rMap[agf.fieldName] = k
	}
	recs := make([][]string, 0, len(aggregations)+1)
	agResF := make([]agResultField, 0, len(aggregations))
	recs = append(recs, []string{"", "count", fmt.Sprintf("%d", totalCount)})
	for _, ag := range aggregations {
		p := strings.Split(ag, ":")
		n := rMap[p[0]]
		recs = append(recs, []string{n, p[1], p[2]})
		agResF = append(agResF, agResultField{p[0], p[1], p[2]})
	}
	agRes := agResult{
		NumberObjects: totalCount,
		Aggregations:  agResF,
	}
	switch appCtx.OutputFormat {
	case "json":
		return appCtx.EmitJSON(agRes)
	case "yaml":
		return appCtx.EmitYAML(agRes)
	}
	return appCtx.EmitTable(agHeaders, recs, nil)
}

// parseColumns parses a list of column names.
// - it validates names in a case insensitive manner.  All column names are
//   assumed to be distinct in a case insensitive manner.
// - specified column names replace the default unless the default is implied (see below)
//   in which case the specified columns are added in their relative position to the
//   default columns
// - it accepts "+name" or "-name" as adjustments to the default columns which are
//   assumed to be appended at the end.
// - it accepts '*' as the position of the default columns
// - it skips empty column specifications (i.e. a comma without a leading token)
// - leading and trailing space in a column name is trimmed for matching purposes
func (appCtx *AppCtx) parseColumns(colString string, validCols []string, defaultCols []string) ([]string, error) {
	lcCols := map[string]string{}
	for _, c := range validCols {
		lc := strings.ToLower(c)
		lc = strings.TrimSpace(lc)
		lcCols[lc] = c
	}
	mustAddDefaults := false
	beforeDefault := []string{}
	afterDefault := []string{}
	newCols := &beforeDefault
	removeDefault := map[string]struct{}{}
	seenStar := false
	for _, token := range strings.Split(colString, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if token == "*" {
			if seenStar {
				return nil, fmt.Errorf("duplicate use of *")
			}
			seenStar = true
			newCols = &afterDefault
			mustAddDefaults = true
			continue
		}
		mustRemove := false
		if strings.HasPrefix(token, "+") {
			mustAddDefaults = true
			token = strings.TrimPrefix(token, "+")
		} else if strings.HasPrefix(token, "-") {
			mustAddDefaults = true
			token = strings.TrimPrefix(token, "-")
			mustRemove = true
		}
		if c, ok := lcCols[strings.ToLower(token)]; ok {
			if mustRemove {
				removeDefault[c] = struct{}{}
			} else {
				*newCols = append(*newCols, c)
			}
		} else {
			return nil, fmt.Errorf("invalid column \"%s\"", token)
		}
	}
	cols := beforeDefault
	if mustAddDefaults {
		for _, c := range defaultCols {
			if _, has := removeDefault[c]; has {
				continue
			}
			if !util.Contains(cols, c) {
				cols = append(cols, c)
			}
		}
	}
	for _, c := range afterDefault {
		if !util.Contains(cols, c) {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		cols = defaultCols
	}
	return cols, nil
}

type showColsCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`

	columns map[string]string
}

// Execute provides a common implementation to output table columns
func (c *showColsCmd) Execute(args []string) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(c.columns)
	case "yaml":
		return appCtx.EmitYAML(c.columns)
	}
	cols := make([]string, 0, len(c.columns))
	for k := range c.columns {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	rows := make([][]string, len(cols))
	dLen := 0
	for i, col := range cols {
		row := make([]string, 2)
		row[0] = col
		d := c.columns[col]
		row[1] = d
		rows[i] = row
		if len(d) > dLen {
			dLen = len(d)
		}
	}
	cT := func(t *tablewriter.Table) {
		t.SetColWidth(dLen)
	}
	appCtx.EmitTable([]string{"Column Name", "Description"}, rows, cT)
	return nil
}

// ClearTerminal will clear the screen
func (appCtx *AppCtx) ClearTerminal() {
	// TBD: will not work for Windows
	cmd := appCtx.osExec.OSCommand("clear")
	cmd.Stdout = os.Stdout
	appCtx.osExec.OSCommandRun(cmd)
}

// OSExecutor is the abstraction of the os/exec Cmd support used
type OSExecutor interface {
	OSCommand(name string, arg ...string) *exec.Cmd
	OSCommandRun(c *exec.Cmd) error
}

// OSCommand creates an exec.Cmd
func (appCtx *AppCtx) OSCommand(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

// OSCommandRun invokes the cmd.Run method
func (appCtx *AppCtx) OSCommandRun(c *exec.Cmd) error {
	return c.Run()
}

func convertAttrs(attrs map[string]string) map[string]models.ValueType {
	cAttrs := make(map[string]models.ValueType, len(attrs))
	for a, v := range attrs {
		// assume Kind is STRING unless the attribute name includes the substring "secret"
		if ok, _ := regexp.MatchString("secret", a); ok {
			cAttrs[a] = models.ValueType{Kind: "SECRET", Value: v}
		} else {
			cAttrs[a] = models.ValueType{Kind: "STRING", Value: v}
		}
	}
	return cAttrs
}

// paranoidPasswordPrompt interactively prompts twice for a password; the values must patch.
// The prompt may include upper case characters, but they are converted to lower case in the "Retype %s:" prompt
func paranoidPasswordPrompt(prompt string) (string, error) {
	fmt.Fprintf(outputWriter, "%s: ", prompt)
	bytePassword, err := passwordHook(int(syscall.Stdin))
	fmt.Fprintln(outputWriter)
	if err != nil {
		return "", err
	}
	pw := strings.TrimSpace(string(bytePassword))
	fmt.Fprintf(outputWriter, "Retype %s: ", strings.ToLower(prompt))
	bytePassword, err = passwordHook(int(syscall.Stdin))
	fmt.Fprintln(outputWriter)
	if err != nil {
		return "", err
	}
	if pw != strings.TrimSpace(string(bytePassword)) {
		return "", fmt.Errorf("passwords do not match")
	}
	return pw, nil
}
