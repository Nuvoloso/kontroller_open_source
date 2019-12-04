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
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
	units "github.com/docker/go-units"
)

func init() {
	initPV()
}

func initPV() {
	cmd, _ := parser.AddCommand("persistentvolume", "Persistent Volume commands", "Persistent Volume Subcommands", &pvCmd{})
	cmd.Aliases = []string{"pv"}
	cmd.AddCommand("list", "List Persistent Volumes", "List all persistent volumes in the cluster", &pvListCmd{})
	cmd.AddCommand("create", "Create a Persistent Volume", "Create a persistent volume to be used in the cluster", &pvCreateCmd{})
	cmd.AddCommand("delete", "Delete a Persistent Volume", "Delete a persistent volume from a cluster", &pvDeleteCmd{})
	cmd.AddCommand("watch", "Watch for changes to Persistent Volumes", "Watch for changes to Persistent Volumes in the cluster", &pvWatchCmd{})
}

type pvCmd struct {
	tableCols []string
}

const (
	hPVName    = "PV Name"
	hPVUid     = "PV uid"
	hPVStorage = "PV Storage"
)

var pvHeaders = map[string]string{
	hPVName:    "pv name",
	hPVUid:     "pv uid",
	hPVStorage: "pv storage",
}

var pvDefaultHeaders = []string{hPVName, hPVUid, hPVStorage}

func (c *pvCmd) makeRecord(o *cluster.PersistentVolumeObj) map[string]string {
	return map[string]string{
		hPVName:    o.Name,
		hPVUid:     string(o.UID),
		hPVStorage: string(o.Capacity),
	}
}

func (c *pvCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = pvDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := pvHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *pvCmd) Emit(data []*cluster.PersistentVolumeObj) error {
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

type pvListCmd struct {
	Columns string `long:"columns" description:"Comma separated list of column names"`
	pvCmd
}

func (c *pvListCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	res, err := appCtx.K8sClient.PersistentVolumeList(appCtx.ctx)
	if err != nil {
		return err
	}
	return c.Emit(res)
}

type pvCreateCmd struct {
	VolumeID   string `short:"v" long:"volume-id" description:"Specify the Persistent Volume ID" required:"yes"`
	Size       int64  `short:"s" long:"size" description:"Specify the size in GiB." required:"yes"`
	AccountID  string `short:"A" long:"accountid" description:"Specify the accountID" required:"yes"`
	FsType     string `short:"F" long:"fstype" description:"Specify the FS Type" required:"no" default:"ext4"`
	SystemID   string `short:"S" long:"systemid" description:"Specify the SystemID" required:"yes"`
	DriverType string `short:"D" long:"drivertype" description:"Specify the DriverType" required:"no" default:"FLEX"`
	Columns    string `long:"columns" description:"Comma separated list of column names"`
	pvCmd
}

func (c *pvCreateCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	pvca := &cluster.PersistentVolumeCreateArgs{
		VolumeID:   c.VolumeID,
		SizeBytes:  c.Size * int64(units.GiB),
		AccountID:  c.AccountID,
		FsType:     c.FsType,
		SystemID:   c.SystemID,
		DriverType: c.DriverType,
	}
	res, err := appCtx.K8sClient.PersistentVolumeCreate(appCtx.ctx, pvca)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.PersistentVolumeObj{res})
}

type pvDeleteCmd struct {
	VolumeID string `short:"v" long:"volume-id" description:"Specify the Persistent Volume ID" required:"yes"`
	Columns  string `long:"columns" description:"Comma separated list of column names"`
	pvCmd
}

func (c *pvDeleteCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	pvda := &cluster.PersistentVolumeDeleteArgs{
		VolumeID: c.VolumeID,
	}
	res, err := appCtx.K8sClient.PersistentVolumeDelete(appCtx.ctx, pvda)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.PersistentVolumeObj{res})
}

type pvWatchCmd struct {
	pvCmd
}

func (c *pvWatchCmd) Execute(args []string) error {
	wargs := &cluster.PVWatcherArgs{
		Timeout: 1 * time.Minute,
		Ops: &watchOps{
			c: c,
		},
	}
	watcher, pvList, err := appCtx.K8sClient.CreatePublishedPersistentVolumeWatcher(appCtx.ctx, wargs)
	if err != nil {
		return err
	}
	if err := c.InitialPVList(pvList); err != nil {
		return err
	}
	if err := watcher.Start(appCtx.ctx); err != nil {
		return err
	}
	x := make(chan os.Signal)
	signal.Notify(x, os.Interrupt)
	<-x
	fmt.Println("interrupted")
	watcher.Stop()
	return nil
}

type watchOps struct {
	c *pvWatchCmd
}

func (c *pvWatchCmd) InitialPVList(pvList []*cluster.PersistentVolumeObj) error {
	for _, pv := range pvList {
		fmt.Println(pv.Name)
	}
	return nil
}

func (w *watchOps) PVDeleted(pv *cluster.PersistentVolumeObj) error {
	fmt.Println(pv.Name)
	return nil
}
