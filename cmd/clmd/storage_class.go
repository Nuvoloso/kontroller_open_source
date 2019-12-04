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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

func init() {
	initStorageClass()
}

func initStorageClass() {
	cmd, _ := parser.AddCommand("storageclass", "Storage Class commands", "Storage Class Subcommands", &scCmd{})
	cmd.Aliases = []string{"sc"}
	cmd.AddCommand("create", "Create a Storage Class", "Create a storage class", &scCreateCmd{})
}

type scCmd struct {
	tableCols []string
}

type scCreateCmd struct {
	Name string `short:"s" long:"name" description:"Specify the StorageClass name" required:"yes"`
	scCmd
}

func (c *scCreateCmd) Execute(args []string) error {
	sp := &models.ServicePlan{}
	sp.Name = models.ServicePlanName(c.Name)
	res, err := appCtx.K8sClient.ServicePlanPublish(appCtx.ctx, sp)
	if err != nil {
		return err
	}
	fmt.Println(res)
	return nil
}
