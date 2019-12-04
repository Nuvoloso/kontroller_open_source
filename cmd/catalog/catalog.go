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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/jessevdk/go-flags"
	logging "github.com/op/go-logging"
)

var outputWriter io.Writer

func init() {
	outputWriter = os.Stdout
}

func createEndpoint(ca *configArgsStruct) (endpoint.EndPoint, error) {
	ea := endpoint.Arg{
		Purpose: "Manipulation",
		Type:    "S3",
	}

	ea.Args.S3.BucketName = ca.BucketName
	ea.Args.S3.Region = ca.Region
	ea.Args.S3.AccessKeyID = ca.AccessKeyID
	ea.Args.S3.SecretAccessKey = ca.SecretAccessKey
	ea.Args.S3.Domain = ca.DomainID
	ea.Args.S3.PassPhrase = ca.PassPhrase

	ep, err := endpoint.SetupEndpoint(&ea, nil, 0)
	if err != nil {
		return nil, errors.New("End point setup failed" + err.Error())
	}
	return ep, nil
}

const (
	hAccount       = "Account"
	hVolumeSeries  = "VolumeSeries"
	hCG            = "CGroup"
	hTime          = "Time"
	hSnapID        = "SnapID"
	hSize          = "VolSize"
	hVolSeriesTags = "VolSeriesTags"
	hSnapshotTags  = "SnapshotTags"
)

type configArgsStruct struct {
	BucketName      string `short:"b" long:"bucket" description:"bucket name" required:"true"`
	SecretAccessKey string `long:"secret-access-key" description:"AWS secret access key" required:"true"`
	AccessKeyID     string `long:"secret-key-id" description:"AWS secret key ID" required:"true"`
	DomainID        string `short:"d" long:"domain-id" description:"Catalog Protection Store Domain ID" required:"true"`
	Region          string `short:"r" long:"region" description:"AWS region" required:"true"`
	PassPhrase      string `short:"p" long:"pass-phrase" description:"Catalog pass phrase" required:"true"`
}

var configArgs configArgsStruct

type getSnapCmdStruct struct {
	SnapID   string `short:"s" long:"snapshot" description:"snapshot for which to retreive information" required:"true"`
	FileName string `short:"f" long:"file" description:"name of the file to place the snapshot information"`
}

var getSnapCmd getSnapCmdStruct

func (c *getSnapCmdStruct) Execute(args []string) error {
	getArgs := &pstore.SnapshotCatalogGetArgs{
		PStore: &pstore.ProtectionStoreDescriptor{
			CspDomainType: aws.CSPDomainType,
			CspDomainAttributes: map[string]models.ValueType{
				aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: configArgs.BucketName},
				aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: configArgs.Region},
				aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: configArgs.AccessKeyID},
				aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: configArgs.SecretAccessKey},
			},
		},
		EncryptionAlgorithm: common.EncryptionAES256,
		Passphrase:          configArgs.PassPhrase,
		ProtectionDomainID:  configArgs.DomainID,
		SnapID:              getSnapCmd.SnapID,
	}

	cArgs := &pstore.ControllerArgs{Log: logging.MustGetLogger("catalog")}
	cntrl, err := pstore.NewController(cArgs)
	if err != nil {
		return errors.New("Error: NewController: " + err.Error())
	}

	si, err := cntrl.SnapshotCatalogGet(context.Background(), getArgs)
	if err != nil {
		return errors.New("Error: catalog list: " + err.Error())
	}

	if getSnapCmd.FileName == "" {
		getSnapCmd.FileName = getSnapCmd.SnapID
	}
	file, err := os.Create(getSnapCmd.FileName)
	if err != nil {
		return errors.New("Error creating file " + getSnapCmd.FileName + " " + err.Error())
	}
	defer file.Close()

	b2, err := json.MarshalIndent(&si, "", "  ")
	if err != nil {
		return errors.New("Error writing file " + getSnapCmd.FileName + " " + err.Error())
	}
	buf := bytes.NewBuffer(b2)
	io.Copy(file, buf)

	return nil
}

type listSnapsCmdStruct struct {
	OutputFormat string `short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
	Emitter
}

var listSnapsCmd listSnapsCmdStruct

type snapData struct {
	Account      string `json:"account"`
	VolumeSeries string `json:"volumeSeries"`
	CG           string `json:"consistencyGroup"`
	Time         string `json:"time"`
	SnapID       string `json:"snapID"`
	Size         string `json:"size"`
	VSTags       string `json:"VSTags"`
	SnapTags     string `json:"SnapTags"`
}

// snapHeaders record keys/headers and their description
var snapHeaders = map[string]string{
	hAccount:       "account",
	hVolumeSeries:  "volume series",
	hCG:            "consistency group",
	hTime:          "snapshot time",
	hSnapID:        "snapshot ID",
	hSize:          "volume size",
	hVolSeriesTags: "volume series tags",
	hSnapshotTags:  "snapshot tags",
}

func (c *listSnapsCmdStruct) makeRecord(o *snapData) map[string]string {
	return map[string]string{
		hAccount:       o.Account,
		hVolumeSeries:  o.VolumeSeries,
		hCG:            o.CG,
		hTime:          o.Time,
		hSnapID:        o.SnapID,
		hSize:          o.Size,
		hVolSeriesTags: o.VSTags,
		hSnapshotTags:  o.SnapTags,
	}
}

func (c *listSnapsCmdStruct) emit(data []*snapData) error {
	switch c.OutputFormat {
	case "json":
		return c.EmitJSON(data)
	case "yaml":
		return c.EmitYAML(data)
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
	return c.EmitTable(c.tableCols, rows, nil)
}

func (c *listSnapsCmdStruct) Execute(args []string) error {
	c.Emitter = &StdoutEmitter{}

	c.tableCols = []string{hAccount, hVolumeSeries, hVolSeriesTags, hCG, hSnapID, hSnapshotTags, hTime, hSize}

	listArgs := &pstore.SnapshotCatalogListArgs{
		PStore: &pstore.ProtectionStoreDescriptor{
			CspDomainType: aws.CSPDomainType,
			CspDomainAttributes: map[string]models.ValueType{
				aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: configArgs.BucketName},
				aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: configArgs.Region},
				aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: configArgs.AccessKeyID},
				aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: configArgs.SecretAccessKey},
			},
		},
		EncryptionAlgorithm: common.EncryptionAES256,
		Passphrase:          configArgs.PassPhrase,
		ProtectionDomainID:  configArgs.DomainID,
	}

	ctx := context.Background()

	cntlr := &pstore.Controller{}
	iter, err := cntlr.SnapshotCatalogList(ctx, listArgs)
	if err != nil {
		return errors.New("Error: catalog list: " + err.Error())
	}

	res := []*snapData{}

	for si, err := iter.Next(ctx); err == nil && si != nil; si, err = iter.Next(ctx) {
		sizeStr := fmt.Sprintf("%3.1f", float32(si.SizeBytes)/(1024*1024*1024))

		var vsTags string
		if len(si.VolumeSeriesTags) == 0 {
			vsTags = "None"
		} else {
			vsTags = strings.Join(si.VolumeSeriesTags, " ")
		}
		var snapTags string
		if len(si.VolumeSeriesTags) == 0 {
			snapTags = "None"
		} else {
			snapTags = strings.Join(si.SnapshotTags, " ")
		}

		row := &snapData{
			Account:      si.AccountName,
			VolumeSeries: si.VolumeSeriesName,
			CG:           si.ConsistencyGroupName,
			Time:         si.SnapTime.Format(time.RFC3339),
			SnapID:       si.SnapIdentifier,
			Size:         sizeStr,
			VSTags:       vsTags,
			SnapTags:     snapTags,
		}
		res = append(res, row)
	}

	if err == nil {
		c.emit(res)
	}

	return err
}

func main() {
	var parser = flags.NewParser(&configArgs, flags.Default)

	parser.AddCommand("list-snapshots",
		"List snapshots from the catalog",
		"List the snapshots contained in the catalog.",
		&listSnapsCmd)
	parser.AddCommand("get-snapshot-metadata",
		"Get snapshot information",
		"Get information regarding a particular snapshot.",
		&getSnapCmd)

	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
