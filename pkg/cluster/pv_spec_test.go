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


package cluster

import (
	"context"
	"reflect"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestPVSpecTemplateProcessor(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	savedpvTProc := pvTProc
	savedpvSpecTemplateDir := pvSpecTemplateDir
	defer func() {
		pvTProc = savedpvTProc
		pvSpecTemplateDir = savedpvSpecTemplateDir
	}()

	// point to the local test data
	pvSpecTemplateDir = "./testdata"
	pvTProc = nil
	ctx := context.Background()
	tProc, err := getPVTemplateProcessor(ctx, tl.Logger())
	assert.Nil(err)
	assert.NotNil(tProc)
	assert.NotNil(pvTProc)
	assert.Equal(pvTProc, tProc)

	tp, ok := tProc.(*pvSpecTemplate)
	assert.True(ok)
	assert.NotNil(tp)

	expNames := []string{"pv.yaml.tmpl", "pv-v2.yaml.tmpl", "pv-missingkey.yaml.tmpl", "pv-badcall.yaml.tmpl"}
	foundNames := []string{}
	for _, p := range tp.tt.Templates() {
		n := p.Name()
		tl.Logger().Info("TemplateFile:", n)
		foundNames = append(foundNames, n)
	}
	assert.Len(foundNames, len(expNames))
	for _, n := range foundNames {
		assert.Contains(expNames, n)
	}
	assert.NotContains(foundNames, "invalid.yaml.tmpl")

	tA := &PVSpecArgs{
		AccountID:      "account1",
		VolumeID:       "volume1",
		SystemID:       "system1",
		Capacity:       "10Gi",
		ClusterVersion: "v1",
		FsType:         "ext4",
	}
	tmpl := tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("pv.yaml.tmpl", tmpl.Name())
	pv, err := tProc.ExecuteTemplate(ctx, tA)
	assert.Nil(err)
	assert.NotNil(pv)
	assert.Equal("yaml", pv.Format)
	assert.Regexp("File=pv.yaml", pv.PvSpec)
	assert.Regexp("accountid="+tA.AccountID, pv.PvSpec)
	assert.Regexp("clusterversion="+tA.ClusterVersion, pv.PvSpec)
	assert.Regexp("volumeid="+tA.VolumeID, pv.PvSpec)
	assert.Regexp("systemid="+tA.SystemID, pv.PvSpec)
	assert.Regexp("fstype="+tA.FsType, pv.PvSpec)
	assert.Regexp("capacity="+tA.Capacity, pv.PvSpec)

	tA.ClusterVersion = "v2"
	tmpl = tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("pv-v2.yaml.tmpl", tmpl.Name())
	pv, err = tProc.ExecuteTemplate(ctx, tA)
	assert.Nil(err)
	assert.NotNil(pv)
	assert.Equal("yaml", pv.Format)
	assert.Regexp("File=pv-v2.yaml", pv.PvSpec)
	assert.Regexp("accountid="+tA.AccountID, pv.PvSpec)
	assert.Regexp("clusterversion="+tA.ClusterVersion, pv.PvSpec)
	assert.Regexp("volumeid="+tA.VolumeID, pv.PvSpec)
	assert.Regexp("systemid="+tA.SystemID, pv.PvSpec)
	assert.Regexp("fstype="+tA.FsType, pv.PvSpec)
	assert.Regexp("capacity="+tA.Capacity, pv.PvSpec)

	// error case: fail because a value is not specified
	assert.Equal(6, reflect.TypeOf(*tA).NumField())
	for i := 0; i <= 5; i++ {
		var sp *string
		var ip *int
		var sv string
		var iv int
		switch i {
		case 0:
			sp = &tA.AccountID
		case 1:
			sp = &tA.ClusterVersion
		case 2:
			sp = &tA.VolumeID
		case 3:
			sp = &tA.SystemID
		case 4:
			sp = &tA.FsType
		case 5:
			sp = &tA.Capacity
		}
		if sp != nil {
			sv = *sp
			*sp = ""
		}
		pv, err = tProc.ExecuteTemplate(ctx, tA)
		assert.NotNil(err, "case: %d", i)
		assert.Nil(pv)
		assert.Regexp("invalid arguments", err.Error())
		if sp != nil {
			*sp = sv
		}
		if ip != nil {
			*ip = iv
		}
		tl.Flush()
	}

	// error case: execute failure because of bad function
	tA.ClusterVersion = "badcall"
	tmpl = tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("pv-badcall.yaml.tmpl", tmpl.Name())
	pv, err = tProc.ExecuteTemplate(ctx, tA)
	assert.NotNil(err)
	assert.Nil(pv)
	assert.Regexp("MissingFunc", err.Error())

	// error case: execute failure because of missing key
	tA.ClusterVersion = "missingkey"
	tmpl = tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("pv-missingkey.yaml.tmpl", tmpl.Name())
	pv, err = tProc.ExecuteTemplate(ctx, tA)
	assert.NotNil(err)
	assert.Nil(pv)
	assert.Regexp("MissingKey", err.Error())

	// error case: no fallback template
	pvSpecTemplateDir = "./testdata/badmcdir"
	pvTProc = nil
	tProc, err = getPVTemplateProcessor(ctx, tl.Logger())
	assert.Nil(err)
	assert.NotNil(tProc)
	assert.NotNil(pvTProc)
	assert.Equal(pvTProc, tProc)
	tp, ok = tProc.(*pvSpecTemplate)
	assert.True(ok)
	assert.NotNil(tp)

	expNames = []string{"pv-nosuchcsp.yaml.tmpl"}
	foundNames = []string{}
	for _, p := range tp.tt.Templates() {
		n := p.Name()
		tl.Logger().Info("TemplateFile:", n)
		foundNames = append(foundNames, n)
	}
	assert.Len(foundNames, len(expNames))
	for _, n := range foundNames {
		assert.Contains(expNames, n)
	}

	tA = &PVSpecArgs{
		AccountID:      "account1",
		VolumeID:       "volume1",
		SystemID:       "system1",
		Capacity:       "10Gi",
		ClusterVersion: PVSpecDefaultVersion,
		FsType:         PVSpecDefaultFsType,
	}

	tmpl = tp.findTemplate(tA)
	assert.Nil(tmpl)
	pv, err = tProc.ExecuteTemplate(ctx, tA)
	assert.NotNil(err)
	assert.Nil(pv)
	assert.Regexp("could not find appropriate deployment template for.*v1", err.Error())

	// point to the real deployment template
	pvSpecTemplateDir = "../../deploy/centrald/lib/deploy"
	pvTProc = nil
	tProc, err = getPVTemplateProcessor(ctx, tl.Logger())
	assert.Nil(err)
	assert.NotNil(tProc)
	assert.NotNil(pvTProc)
	assert.Equal(pvTProc, tProc)
	tp, ok = tProc.(*pvSpecTemplate)
	assert.True(ok)
	assert.NotNil(tp)

	tA = &PVSpecArgs{
		AccountID:      "account1",
		VolumeID:       "volume1",
		SystemID:       "system1",
		Capacity:       "10Gi",
		ClusterVersion: PVSpecDefaultVersion,
		FsType:         PVSpecDefaultFsType,
	}
	tmpl = tp.findTemplate(tA)
	assert.NotNil(tmpl)
	pv, err = tProc.ExecuteTemplate(ctx, tA)
	assert.Nil(err)
	assert.NotNil(pv)
	tl.Logger().Info(pv.PvSpec)
	assert.Equal("yaml", pv.Format)
	assert.NotRegexp("{{.AccountID}}", pv.PvSpec)
	assert.Regexp("account1", pv.PvSpec)
	assert.NotRegexp("{{.ClusterVersion}}", pv.PvSpec)
	assert.Regexp(PVSpecDefaultVersion, pv.PvSpec)
	assert.NotRegexp("{{.VolumeID}}", pv.PvSpec)
	assert.Regexp("volume1", pv.PvSpec)
	assert.NotRegexp("{{.SystemID}}", pv.PvSpec)
	assert.Regexp("system1", pv.PvSpec)
	assert.NotRegexp("{{.FsType}}", pv.PvSpec)
	assert.Regexp(PVSpecDefaultFsType, pv.PvSpec)
	assert.NotRegexp("{{.Capacity}}", pv.PvSpec)
	assert.Regexp("10Gi", pv.PvSpec)
}
