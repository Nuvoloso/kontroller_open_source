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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_storage_type"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

// Matcher for CSPStorageType
type mockCspStorageTypeMatchCtx int

const (
	mockCspStorageTypeListParam mockCspStorageTypeMatchCtx = iota
)

type mockCspStorageTypeMatcher struct {
	t         *testing.T
	ctx       mockCspStorageTypeMatchCtx
	listParam *csp_storage_type.CspStorageTypeListParams
}

func newCspStorageTypeMatcher(t *testing.T, ctx mockCspStorageTypeMatchCtx) *mockCspStorageTypeMatcher {
	return &mockCspStorageTypeMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockCspStorageTypeMatcher) Matcher() gomock.Matcher {
	return o
}

// Matches is from gomock.Matcher
func (o *mockCspStorageTypeMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch o.ctx {
	case mockCspStorageTypeListParam:
		p := x.(*csp_storage_type.CspStorageTypeListParams)
		return assert.EqualValues(o.listParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *mockCspStorageTypeMatcher) String() string {
	switch o.ctx {
	case mockCspStorageTypeListParam:
		return "matches on list"
	}
	return "unknown context"
}

func TestCspStorageTypeMakeRecord(t *testing.T) {
	assert := assert.New(t)

	ac := &cspStorageTypeCmd{}
	o := &models.CSPStorageType{
		Name:                             "Amazon gp2",
		Description:                      "EBS General Purpose SSD (gp2) Volume",
		CspDomainType:                    "AWS",
		MaxAllocationSizeBytes:           swag.Int64(17592186044416),
		MinAllocationSizeBytes:           swag.Int64(1073741824),
		AccessibilityScope:               "CSPDOMAIN",
		PreferredAllocationSizeBytes:     swag.Int64(2147483648),
		PreferredAllocationUnitSizeBytes: swag.Int64(1073741824),
		ParcelSizeBytes:                  swag.Int64(1073741824),
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(0),
			Throughput: swag.Int64(94 * int64(units.MB)),
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(cspStorageTypeHeaders))
	assert.Equal("Amazon gp2", rec[hName])
	assert.Equal("EBS General Purpose SSD (gp2) Volume", rec[hDescription])
	assert.EqualValues("AWS", o.CspDomainType)
	assert.EqualValues(17592186044416, swag.Int64Value(o.MaxAllocationSizeBytes))
	assert.EqualValues(1073741824, swag.Int64Value(o.MinAllocationSizeBytes))
	assert.EqualValues(2147483648, swag.Int64Value(o.PreferredAllocationSizeBytes))
	assert.EqualValues(1073741824, swag.Int64Value(o.PreferredAllocationUnitSizeBytes))
	assert.EqualValues(1073741824, swag.Int64Value(o.ParcelSizeBytes))
	assert.Equal("94MB/s/GiB", rec[hProvisioningUnit])
}

func TestCspStorageTypeList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	res := &csp_storage_type.CspStorageTypeListOK{
		Payload: []*models.CSPStorageType{
			&models.CSPStorageType{
				Name:                             "Amazon gp2",
				Description:                      "EBS General Purpose SSD (gp2) Volume",
				CspDomainType:                    "AWS",
				MaxAllocationSizeBytes:           swag.Int64(17592186044416),
				MinAllocationSizeBytes:           swag.Int64(1073741824),
				AccessibilityScope:               "CSPDOMAIN",
				PreferredAllocationSizeBytes:     swag.Int64(int64(500 * units.Gibibyte)),
				PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.Gibibyte)),
				ParcelSizeBytes:                  swag.Int64(int64(units.Gibibyte)),
				ProvisioningUnit: &models.ProvisioningUnit{
					IOPS:       swag.Int64(3),
					Throughput: swag.Int64(0),
				},
			},
			&models.CSPStorageType{
				Name:                             "Amazon SSD",
				Description:                      "Amazon SSD Instance Storage",
				CspDomainType:                    "AWS",
				MaxAllocationSizeBytes:           swag.Int64(858993459200),
				MinAllocationSizeBytes:           swag.Int64(1073741824),
				AccessibilityScope:               "NODE",
				PreferredAllocationSizeBytes:     swag.Int64(int64(800 * units.Gibibyte)),
				PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.Gibibyte)),
				ParcelSizeBytes:                  swag.Int64(int64(units.Gibibyte)),
				ProvisioningUnit: &models.ProvisioningUnit{
					IOPS:       swag.Int64(137),
					Throughput: swag.Int64(0),
				},
			},
		},
	}

	// list, with type
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockCSPStorageTypeClient(mockCtrl)
	mAPI.EXPECT().CspStorageType().Return(cOps)
	m := newCspStorageTypeMatcher(t, mockCspStorageTypeListParam)
	m.listParam = csp_storage_type.NewCspStorageTypeListParams()
	m.listParam.CspDomainType = swag.String("AWS")
	cOps.EXPECT().CspStorageTypeList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err := parseAndRun([]string{"csp-storage-types", "list", "-T", "AWS"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspStorageTypeDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list GCP, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPStorageTypeClient(mockCtrl)
	mAPI.EXPECT().CspStorageType().Return(cOps)
	m = newCspStorageTypeMatcher(t, mockCspStorageTypeListParam)
	m.listParam = csp_storage_type.NewCspStorageTypeListParams()
	m.listParam.CspDomainType = swag.String("GCP")
	cOps.EXPECT().CspStorageTypeList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err = parseAndRun([]string{"csp-storage-type", "list", "--domain-type", "GCP", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPStorageTypeClient(mockCtrl)
	mAPI.EXPECT().CspStorageType().Return(cOps)
	m = newCspStorageTypeMatcher(t, mockCspStorageTypeListParam)
	m.listParam = csp_storage_type.NewCspStorageTypeListParams()
	cOps.EXPECT().CspStorageTypeList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err = parseAndRun([]string{"storage-type", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPStorageTypeClient(mockCtrl)
	mAPI.EXPECT().CspStorageType().Return(cOps)
	cOps.EXPECT().CspStorageTypeList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err = parseAndRun([]string{"storage-types", "list", "--columns", "Name,Scope"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err = parseAndRun([]string{"csp-storage-type", "list", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPStorageTypeClient(mockCtrl)
	mAPI.EXPECT().CspStorageType().Return(cOps)
	apiErr := &csp_storage_type.CspStorageTypeListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().CspStorageTypeList(m.Matcher()).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err = parseAndRun([]string{"storage-type", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPStorageTypeClient(mockCtrl)
	mAPI.EXPECT().CspStorageType().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().CspStorageTypeList(m.Matcher()).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err = parseAndRun([]string{"storage-type", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspStorageType()
	err = parseAndRun([]string{"csp-storage-type", "list", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
