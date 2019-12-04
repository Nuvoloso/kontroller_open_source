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

	"github.com/Nuvoloso/kontroller/pkg/util"
)

func init() {
	initMD()
}

func initMD() {
	cmd, _ := parser.AddCommand("metadata", "Metadata commands", "Metadata Subcommands", &mdCmd{})
	cmd.Aliases = []string{"md"}
	cmd.AddCommand("get", "Get Metadata", "Get metadata about the cluster", &mdGetCmd{})
}

type mdCmd struct {
	tableCols []string
}

type mdGetCmd struct {
	Columns string `long:"columns" description:"Comma separated list of column names"`
	mdCmd
}

func (c *mdGetCmd) Execute(args []string) error {
	res, err := appCtx.K8sClient.MetaData(appCtx.ctx)
	if err != nil {
		return err
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		return err
	}
	names := util.SortedStringKeys(res)
	for _, n := range names {
		fmt.Printf("%s: %s\n", n, res[n])
	}
	fmt.Println()
	return nil
}
