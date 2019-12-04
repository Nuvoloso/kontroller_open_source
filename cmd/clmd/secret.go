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
	"regexp"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
)

func init() {
	initSecret()
}

func initSecret() {
	cmd, _ := parser.AddCommand("secret", "Secret commands", "Secret Subcommands", &secretCmd{})
	cmd.AddCommand("fetch", "Fetch a Secret", "Fetch a particular secret in a namespace", &secretFetchCmd{})
	cmd.AddCommand("fetchmv", "Fetch a multi value Secret", "Fetch a particular secret in a namespace", &secretFetchMVCmd{})
	cmd.AddCommand("create", "Create a Secret", "Create a secret in a namespace", &secretCreateCmd{})
	cmd.AddCommand("createmv", "Create a multi value Secret", "Create a secret in a namespace", &secretCreateMVCmd{})
	cmd.AddCommand("format", "Format a Secret", "Create a secret in a namespace", &secretFormatCmd{})
	cmd.AddCommand("formatmv", "Format a multivalue Secret", "Format a secret in a namespace", &secretFormatMVCmd{})
}

type secretCmd struct {
	tableCols []string
}

const (
	hSName      = "Secret Name"
	hSNamespace = "Secret Namespace"
	hSData      = "Secret Data"
)

var secretHeaders = map[string]string{
	hSName:      "secret name",
	hSNamespace: "secret namespace",
	hSData:      "secret data",
}

var secretDefaultHeaders = []string{hSName, hSNamespace, hSData}

func (c *secretCmd) makeRecord(o *cluster.SecretObj) map[string]string {
	return map[string]string{
		hSName:      o.Name,
		hSNamespace: o.Namespace,
		hSData:      o.Data,
	}
}

func (c *secretCmd) makeRecordMV(o *cluster.SecretObjMV) map[string]string {
	bytes, _ := json.Marshal(o.Data)
	return map[string]string{
		hSName:      o.Name,
		hSNamespace: o.Namespace,
		hSData:      string(bytes),
	}
}

func (c *secretCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = secretDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := secretHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *secretCmd) Emit(data []*cluster.SecretObj) error {
	switch appCtx.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	rows := make([][]string, len(data))
	for i, o := range data {
		rec := c.makeRecord(o)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows[i] = row
	}
	return appCtx.EmitTable(c.tableCols, rows, nil)
}

func (c *secretCmd) EmitMV(data []*cluster.SecretObjMV) error {
	switch appCtx.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	rows := make([][]string, len(data))
	for i, o := range data {
		rec := c.makeRecordMV(o)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows[i] = row
	}
	return appCtx.EmitTable(c.tableCols, rows, nil)
}

type secretFetchCmd struct {
	Name      string `short:"s" long:"name" description:"Specify the Secret name" required:"yes"`
	Namespace string `short:"n" long:"namespace" description:"Specify the Namespace of the Secret" required:"yes"`
	Columns   string `long:"columns" description:"Comma separated list of column names"`
	secretCmd
}

func (c *secretFetchCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	sfa := &cluster.SecretFetchArgs{
		Name:      c.Name,
		Namespace: c.Namespace,
	}
	res, err := appCtx.K8sClient.SecretFetch(appCtx.ctx, sfa)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.SecretObj{res})
}

type secretFetchMVCmd struct {
	Name      string `short:"s" long:"name" description:"Specify the Secret name" required:"yes"`
	Namespace string `short:"n" long:"namespace" description:"Specify the Namespace of the Secret" required:"yes"`
	Columns   string `long:"columns" description:"Comma separated list of column names"`
	secretCmd
}

func (c *secretFetchMVCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	sfa := &cluster.SecretFetchArgs{
		Name:      c.Name,
		Namespace: c.Namespace,
	}
	res, err := appCtx.K8sClient.SecretFetchMV(appCtx.ctx, sfa)
	if err != nil {
		return err
	}
	return c.EmitMV([]*cluster.SecretObjMV{res})
}

type secretCreateCmd struct {
	Name      string `short:"s" long:"name" description:"Specify the Secret name" required:"yes"`
	Namespace string `short:"n" long:"namespace" description:"Specify the Namespace of the Secret" required:"yes"`
	Data      string `short:"d" long:"data" description:"Secret data specified as a string" required:"yes"`
	Columns   string `long:"columns" description:"Comma separated list of column names"`
	secretCmd
}

func (c *secretCreateCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	sca := &cluster.SecretCreateArgs{
		Name:      c.Name,
		Namespace: c.Namespace,
		Data:      c.Data,
	}
	res, err := appCtx.K8sClient.SecretCreate(appCtx.ctx, sca)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.SecretObj{res})
}

type secretCreateMVCmd struct {
	Name      string            `short:"s" long:"name" description:"Specify the Secret name" required:"yes"`
	Namespace string            `short:"n" long:"namespace" description:"Specify the Namespace of the Secret" required:"yes"`
	Data      map[string]string `short:"d" long:"data" description:"Secret data specified as a string. Repeat as necessary" required:"yes"`
	Columns   string            `long:"columns" description:"Comma separated list of column names"`
	secretCmd
}

func (c *secretCreateMVCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	sca := &cluster.SecretCreateArgsMV{
		Name:      c.Name,
		Namespace: c.Namespace,
		Data:      c.Data,
	}
	res, err := appCtx.K8sClient.SecretCreateMV(appCtx.ctx, sca)
	if err != nil {
		return err
	}
	return c.EmitMV([]*cluster.SecretObjMV{res})
}

type secretFormatCmd struct {
	Name      string `short:"s" long:"name" description:"Specify the Secret name" required:"yes"`
	Namespace string `short:"n" long:"namespace" description:"Specify the Namespace of the Secret" required:"yes"`
	Data      string `short:"d" long:"data" description:"Secret data specified as a string" required:"yes"`
	Columns   string `long:"columns" description:"Comma separated list of column names"`
	secretCmd
}

func (c *secretFormatCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	sca := &cluster.SecretCreateArgs{
		Name:      c.Name,
		Namespace: c.Namespace,
		Data:      c.Data,
	}
	res, err := appCtx.K8sClient.SecretFormat(appCtx.ctx, sca)
	if err != nil {
		return err
	}
	fmt.Println(res)
	return nil
}

type secretFormatMVCmd struct {
	Name      string            `short:"s" long:"name" description:"Specify the Secret name" required:"yes"`
	Namespace string            `short:"n" long:"namespace" description:"Specify the Namespace of the Secret" required:"yes"`
	Data      map[string]string `short:"d" long:"data" description:"Secret data specified as a string. Repeat as necessary" required:"yes"`
	Columns   string            `long:"columns" description:"Comma separated list of column names"`
	secretCmd
}

func (c *secretFormatMVCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	sca := &cluster.SecretCreateArgsMV{
		Name:      c.Name,
		Namespace: c.Namespace,
		Data:      c.Data,
	}
	res, err := appCtx.K8sClient.SecretFormatMV(appCtx.ctx, sca)
	if err != nil {
		return err
	}
	fmt.Println(res)
	return nil
}
