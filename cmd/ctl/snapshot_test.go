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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotMakeRecord(t *testing.T) {
	assert := assert.New(t)

	dat := "2019-10-29T15:54:51Z"
	datV, err := time.Parse(time.RFC3339, dat)
	assert.NoError(err)
	datDt := strfmt.DateTime(datV)
	ts1 := "2018-10-29T15:54:51Z"
	tv1, err := time.Parse(time.RFC3339, ts1)
	assert.NoError(err)
	dt1 := strfmt.DateTime(tv1)
	ts2 := "2018-10-28T21:54:51Z"
	tv2, err := time.Parse(time.RFC3339, ts2)
	assert.NoError(err)
	dt2 := strfmt.DateTime(tv2)

	sc := &snapshotCmd{}
	accountsCache := map[string]ctxIDName{
		"accountID": {"id2", "System"},
		"id2":       {"", "nuvoloso"},
	}
	cspDomainCache := map[string]string{
		"csp-1": "domainName1",
		"csp-2": "domainName2",
	}
	volumeSeriesCache := map[string]ctxIDName{
		"vs1": {"accountID", "MyVolSeries"},
		"vs2": {"accountID", "OtherVolSeries"},
	}

	o := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
			AccountID:          "accountID",
			ConsistencyGroupID: "cg1",
			PitIdentifier:      "pit1",
			SizeBytes:          12345,
			SnapIdentifier:     "HEAD",
			SnapTime:           dt1,
			TenantAccountID:    "id2",
			VolumeSeriesID:     "vs1",
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: datDt,
			Locations: map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: dt1, CspDomainID: "csp-1"},
				"csp-2": {CreationTime: dt2, CspDomainID: "csp-2"},
			},
			Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			SystemTags: models.ObjTags{"stag1", "stag2"},
			Tags:       models.ObjTags{"tag1", "tag2"},
		},
	}
	for tc := 0; tc <= 3; tc++ {
		sc.accounts = accountsCache
		sc.cspDomains = cspDomainCache
		sc.volumeSeries = volumeSeriesCache

		volumeSeries := "MyVolSeries"
		locations := "domainName1, domainName2"
		switch tc {
		case 0:
			t.Log("case: all caches available")
		case 1:
			t.Log("case: no accounts cache")
			sc.accounts = map[string]ctxIDName{}
		case 2:
			t.Log("case: no csp domain cache, fall back on the ID")
			sc.cspDomains = map[string]string{}
			locations = "csp-1, csp-2"
		case 3:
			t.Log("case: no VS cache, fall back on the ID")
			sc.volumeSeries = map[string]ctxIDName{}
			volumeSeries = string(o.VolumeSeriesID)
		}
		rec := sc.makeRecord(o)
		t.Log(rec)

		if tc == 0 {
			assert.Equal("nuvoloso", rec[hTenant])
			assert.Equal("System", rec[hAccount])
		}
		if tc == 1 {
			assert.Equal("id2", rec[hTenant])
			assert.Equal("accountID", rec[hAccount])
		}
		assert.NotNil(rec)
		assert.Len(rec, len(snapshotHeaders))
		assert.Equal("objectID", rec[hID])
		assert.Equal("1", rec[hVersion])
		assert.Equal("pit1", rec[hPitIdentifier])
		assert.Equal("HEAD", rec[hSnapIdentifier])
		assert.Equal(ts1, rec[hSnapTime])
		assert.Equal(volumeSeries, rec[hVolumeSeries])
		assert.Equal("cg1", rec[hConsistencyGroup])
		assert.Equal("12345B", rec[hSizeBytes])
		assert.Equal(dat, rec[hDeleteAfterTime])
		assert.Equal("stag1, stag2", rec[hSystemTags])
		assert.Equal("tag1, tag2", rec[hTags])
		assert.Equal(locations, rec[hLocations])
	}
}

func TestSnapshotList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	res := &snapshot.SnapshotListOK{
		Payload: []*models.Snapshot{
			&models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id1",
						Version: 1,
					},
					AccountID:          "accountID",
					ConsistencyGroupID: "c-g-1",
					PitIdentifier:      "pit1",
					SizeBytes:          12345,
					SnapIdentifier:     "HEAD",
					SnapTime:           strfmt.DateTime(now),
					TenantAccountID:    "id2",
					VolumeSeriesID:     "vs1",
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"CSP-DOMAIN-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "CSP-DOMAIN-1"},
					},
					Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg1"}},
					SystemTags: models.ObjTags{"stag13", "stag32"},
					Tags:       models.ObjTags{"tag13", "tag32"},
				},
			},
			&models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id2",
						Version: 1,
					},
					AccountID:          "accountID",
					ConsistencyGroupID: "c-g-1",
					PitIdentifier:      "pit1",
					SizeBytes:          12345,
					SnapIdentifier:     "HEAD",
					SnapTime:           strfmt.DateTime(now.Add(-1 * time.Hour)),
					TenantAccountID:    "id2",
					VolumeSeriesID:     "vs1",
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(120 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"CSP-DOMAIN-1": {CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "CSP-DOMAIN-1"},
					},
					Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg2"}},
					SystemTags: models.ObjTags{"stag21", "stag22"},
					Tags:       models.ObjTags{"tag21", "tag22"},
				},
			},
		},
	}

	dat := "2019-10-29T15:54:51Z"
	datV, err := time.Parse(time.RFC3339, dat)
	assert.NoError(err)
	datDt := strfmt.DateTime(datV)
	tsBefore := "2018-10-29T15:54:51Z"
	tvBefore, err := time.Parse(time.RFC3339, tsBefore)
	assert.NoError(err)
	dtBefore := strfmt.DateTime(tvBefore)
	tsAfter := "2018-10-28T21:54:51Z"
	tvAfter, err := time.Parse(time.RFC3339, tsAfter)
	assert.NoError(err)
	dtAfter := strfmt.DateTime(tvAfter)

	// successful list
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	var mAPI *mockmgmtclient.MockAPI
	for tc := 0; tc <= 8; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		params := snapshot.NewSnapshotListParams()
		fw := &fakeWatcher{t: t}
		expWArgs := &models.CrudWatcherCreateArgs{}
		appCtx.watcher = nil
		fos := &fakeOSExecutor{}
		appCtx.osExec = nil
		if tc > 1 && tc < 5 {
			params.AccountID = swag.String("accountID")
			params.ConsistencyGroupID = swag.String("c-g-1")
			params.CspDomainID = swag.String("CSP-DOMAIN-1")
			params.SnapIdentifier = swag.String("HEAD")
			params.SystemTags = []string{"stag11"}
			params.Tags = []string{"tag12"}
		}

		var cmdArgs []string
		var m *mockmgmtclient.SnapshotMatcher
		switch tc {
		case 0:
			t.Log("case: list, all defaults")
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadAccounts, loadCGs, loadVSsWithFetch)
			m = mockmgmtclient.NewSnapshotMatcher(t, params)

			cmdArgs = []string{"snap", "list"}
		case 1:
			t.Log("case: list, yaml")
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			m = mockmgmtclient.NewSnapshotMatcher(t, params)

			cmdArgs = []string{"snap", "list", "-o", "yaml"}
		case 2:
			t.Log("case: list, json, most options, no time options")
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadDomains, loadCGs, loadVSsWithList)
			params.VolumeSeriesID = swag.String("vs1")
			m = mockmgmtclient.NewSnapshotMatcher(t, params)

			cmdArgs = []string{"snapshots", "list", "-A", "System", "--owned-only", "--consistency-group", "CG", "-D", "domainName",
				"-S", "HEAD", "--system-tag", "stag11", "-t", "tag12", "-V", "MyVolSeries", "-o", "json"}
		case 3:
			t.Log("case: list, yaml, remaining options with absolute time")
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadCGs)

			params.CountOnly = swag.Bool(true)
			params.DeleteAfterTimeLE = &datDt
			params.SnapTimeGE = &dtAfter
			params.SnapTimeLE = &dtBefore
			params.PitIdentifiers = []string{"pit1", "pit2"}
			m = mockmgmtclient.NewSnapshotMatcher(t, params)

			cmdArgs = []string{"snapshots", "list", "-A", "System", "--owned-only", "--consistency-group", "CG", "-D", "domainName",
				"-S", "HEAD", "--system-tag", "stag11", "-t", "tag12",
				"-P", "pit1", "--pit-id", "pit2",
				"--snap-time-ge", tsAfter, "--snap-time-le", tsBefore, "--delete-after-time-le", dat, "--count-only", "-o", "yaml"}
		case 4:
			t.Log("case: list, relative time options, list options")
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadCGs)

			params.CountOnly = swag.Bool(true)
			stGe := strfmt.DateTime(now.Add(-2 * 24 * time.Hour))
			stLe := strfmt.DateTime(now.Add(-1 * 24 * time.Hour))
			datLe := strfmt.DateTime(now.Add(90 * 24 * time.Hour))
			limit := int32(10)
			skip := int32(5)
			params.SnapTimeGE = &stGe
			params.SnapTimeLE = &stLe
			params.DeleteAfterTimeLE = &datLe
			params.Skip = &skip
			params.Limit = &limit
			params.SortAsc = []string{"key1"}
			params.SortDesc = []string{"key2"}
			m = mockmgmtclient.NewSnapshotMatcher(t, params)
			m.DSnapTimeGE = 2 * 24 * time.Hour
			m.DSnapTimeLE = 1 * 24 * time.Hour
			m.DDeleteAfterTimeLE = 90 * 24 * time.Hour

			cmdArgs = []string{"snapshots", "list", "-A", "System", "--owned-only", "--consistency-group", "CG", "-D", "domainName",
				"-S", "HEAD", "--system-tag", "stag11", "-t", "tag12", "--sort-asc", "key1", "--sort-desc", "key2", "--skip", "5", "--limit", "10",
				"--snap-time-ge", "2d", "--snap-time-le", "1d", "--delete-after-time-le", "90d", "--count-only", "-o", "yaml"}
		case 5:
			t.Log("case: list with columns")
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadAccounts, loadCGs, loadVSsWithFetch)
			m = mockmgmtclient.NewSnapshotMatcher(t, params)

			cmdArgs = []string{"snapshots", "list", "--columns", "Account,SnapIdentifier,VolumeSeries"}
		case 6:
			t.Log("case: list with follow")
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadCGs, loadVSsWithList)
			params.VolumeSeriesID = swag.String("vs1")
			params.AccountID = swag.String("accountID")
			params.ConsistencyGroupID = swag.String("c-g-1")
			params.CspDomainID = swag.String("CSP-DOMAIN-1")
			params.SnapIdentifier = swag.String("HEAD")
			stGe := strfmt.DateTime(now.Add(-20 * time.Minute))
			params.SnapTimeGE = &stGe
			m = mockmgmtclient.NewSnapshotMatcher(t, params)
			m.DSnapTimeGE = 20 * time.Minute
			cmdArgs = []string{"snapshots", "list", "-f", "-A", "System", "--owned-only", "--consistency-group", "CG", "-D", "domainName",
				"-V", "MyVolSeries", "-S", "HEAD", "-R", "20m", "-o", "yaml"}
			appCtx.watcher = fw
			fw.CallCBCount = 1
			appCtx.osExec = fos
			expWArgs = &models.CrudWatcherCreateArgs{
				Matchers: []*models.CrudMatcher{
					&models.CrudMatcher{
						URIPattern:   "/snapshots/?",
						ScopePattern: ".*consistencyGroupID:c-g-1.*locations:.*CSP-DOMAIN-1.*snapIdentifier:HEAD.*volumeSeriesId:vs1",
					},
				},
			}
		case 7:
			t.Log("case: list with protection domain name")
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains)
			cmdArgs = []string{"snapshots", "list", "--protection-domain", "PD1", "-o", "yaml"}
			params.ProtectionDomainID = swag.String("PD-1")
			m = mockmgmtclient.NewSnapshotMatcher(t, params)
		case 8:
			t.Log("case: list with protection domain id")
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			cmdArgs = []string{"snapshots", "list", "--protection-domain-id", "PD-1", "-o", "yaml"}
			params.ProtectionDomainID = swag.String("PD-1")
			m = mockmgmtclient.NewSnapshotMatcher(t, params)
		}
		cOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)
		mAPI.EXPECT().Snapshot().Return(cOps)
		cOps.EXPECT().SnapshotList(m).Return(res, nil)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initSnapshot()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if tc == 0 {
			assert.Len(te.tableHeaders, len(snapshotDefaultHeaders))
			assert.Len(te.tableData, len(res.Payload))
		} else if tc == 5 {
			assert.Len(te.tableHeaders, 3)
			assert.Len(te.tableData, len(res.Payload))
		} else if tc == 2 {
			assert.NotNil(te.jsonData)
			assert.EqualValues(res.Payload, te.jsonData)
		} else {
			assert.NotNil(te.yamlData)
			assert.EqualValues(res.Payload, te.yamlData)
		}
		if tc == 6 {
			assert.NotNil(fw.InArgs)
			assert.Equal(expWArgs, fw.InArgs)
			assert.NotNil(fw.InCB)
			assert.Equal(1, fw.NumCalls)
			assert.NoError(fw.CBErr)
		}
	}

	// errors
	for tc := 0; tc <= 10; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		if tc < 8 {
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		} else if tc == 8 {
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs)
		} else if tc == 10 {
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains)
		} else {
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
		}

		var cmdArgs []string
		var expectedErr error
		errPat := ""
		switch tc {
		case 0:
			t.Log("case: list with invalid columns")
			errPat = "invalid column"

			cmdArgs = []string{"snapshots", "list", "--columns", "BAD_ONE,SnapIdentifier,VolumeSeries"}
		case 1:
			t.Log("case: list, API failure model error")
			apiErr := &snapshot.SnapshotListDefault{
				Payload: &models.Error{
					Code:    400,
					Message: swag.String("api error"),
				},
			}
			expectedErr = fmt.Errorf(*apiErr.Payload.Message)

			params := snapshot.NewSnapshotListParams()
			m := mockmgmtclient.NewSnapshotMatcher(t, params)
			cOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)
			mAPI.EXPECT().Snapshot().Return(cOps)
			cOps.EXPECT().SnapshotList(m).Return(nil, apiErr)

			cmdArgs = []string{"snap", "list"}
		case 2:
			t.Log("case: list, API failure arbitrary error")
			expectedErr = fmt.Errorf("OTHER ERROR")

			params := snapshot.NewSnapshotListParams()
			m := mockmgmtclient.NewSnapshotMatcher(t, params)
			cOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)
			mAPI.EXPECT().Snapshot().Return(cOps)
			cOps.EXPECT().SnapshotList(m).Return(nil, expectedErr)

			cmdArgs = []string{"snap", "list"}
		case 3:
			t.Log("case: consistency group list error")
			expectedErr = fmt.Errorf("cgListErr")

			cgm := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupListParams())
			cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
			mAPI.EXPECT().ConsistencyGroup().Return(cgOps)
			cgOps.EXPECT().ConsistencyGroupList(cgm).Return(nil, expectedErr)

			cmdArgs = []string{"snap", "list", "--consistency-group", "group"}
		case 4:
			t.Log("case: cspdomain list error")
			expectedErr = fmt.Errorf("cspDomainListError")

			dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
			dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
			mAPI.EXPECT().CspDomain().Return(dOps)
			dOps.EXPECT().CspDomainList(dm).Return(nil, expectedErr)

			cmdArgs = []string{"snap", "list", "-D", "domain"}
		case 5:
			t.Log("case: volume series list error")
			expectedErr = fmt.Errorf("vsListError")

			vsm := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
			vsm.ListParam.Name = swag.String("vs")
			vsm.ListParam.AccountID = swag.String("accountID")
			vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
			mAPI.EXPECT().VolumeSeries().Return(vsOps)
			vsOps.EXPECT().VolumeSeriesList(vsm).Return(nil, expectedErr)

			cmdArgs = []string{"snap", "list", "-V", "vs"}
		case 6:
			t.Log("case: volume series not found")
			errPat = "VolumeSeries.*not found for account.*"

			vsm := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
			vsm.ListParam.Name = swag.String("vs")
			vsm.ListParam.AccountID = swag.String("accountID")
			vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
			mAPI.EXPECT().VolumeSeries().Return(vsOps)
			vsOps.EXPECT().VolumeSeriesList(vsm).Return(&volume_series.VolumeSeriesListOK{}, nil)

			cmdArgs = []string{"snap", "list", "-V", "vs"}
		case 7:
			t.Log("case: init context failure")
			expectedErr = fmt.Errorf("ctx error")

			appCtx.Account, appCtx.AccountID = "", ""
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)

			cmdArgs = []string{"snapshots", "list", "-A", "System"}
		case 8:
			t.Log("case: consistency group not found")
			errPat = "consistency group.*not found"

			cmdArgs = []string{"snapshots", "list", "-A", "System", "--consistency-group", "foo"}
		case 9:
			t.Log("case: CSP domain not found")
			errPat = "CSP domain.*not found"

			cmdArgs = []string{"snapshots", "list", "-D", "foo"}
		case 10:
			t.Log("case: PD name not found")
			cmdArgs = []string{"snapshots", "list", "--protection-domain", "bad-pd"}
			errPat = "protection domain.*not found"
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initSnapshot()
		err := parseAndRun(cmdArgs)
		assert.NotNil(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		} else {
			assert.Equal(expectedErr, err)
		}
	}
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err = parseAndRun([]string{"snapshots", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestSnapshotFetch(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	snapToFetch := "id1"
	fParams := snapshot.NewSnapshotFetchParams()
	fParams.ID = snapToFetch

	now := time.Now()
	res := &snapshot.SnapshotFetchOK{
		Payload: &models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID:      "id1",
					Version: 1,
				},
				AccountID:          "accountID",
				ConsistencyGroupID: "c-g-1",
				PitIdentifier:      "pit1",
				SizeBytes:          12345,
				SnapIdentifier:     "HEAD",
				SnapTime:           strfmt.DateTime(now),
				TenantAccountID:    "id2",
				VolumeSeriesID:     "vs1",
			},
			SnapshotMutable: models.SnapshotMutable{
				DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
				Locations: map[string]models.SnapshotLocation{
					"CSP-DOMAIN-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "CSP-DOMAIN-1"},
				},
				Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg1"}},
				SystemTags: models.ObjTags{"stag13", "stag32"},
				Tags:       models.ObjTags{"tag13", "tag32"},
			},
		},
	}

	t.Log("case: fetch with success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadAccounts, loadCGs, loadVSsWithFetch)
	m := mockmgmtclient.NewSnapshotMatcher(t, fParams)
	cOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(cOps)
	cOps.EXPECT().SnapshotFetch(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err := parseAndRun([]string{"snap", "get", snapToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(snapshotDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	mockCtrl.Finish()
	t.Log("case: fetch, API model error")
	apiErr := &snapshot.SnapshotFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(cOps).MinTimes(1)
	cOps.EXPECT().SnapshotFetch(fParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err = parseAndRun([]string{"snap", "get", "--id", snapToFetch})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	mockCtrl.Finish()
	t.Log("case: fetch with invalid columns")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err = parseAndRun([]string{"snapshots", "get", "--id", snapToFetch, "--columns", "BAD_ONE,SnapIdentifier,VolumeSeriesID"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	mockCtrl.Finish()
	t.Log("init context failure")
	appCtx.Account, appCtx.AccountID = "System", ""
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err = parseAndRun([]string{"snap", "get", "-A", "System", "--id", snapToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err = parseAndRun([]string{"snap", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestSnapshotModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	dat := "2019-10-29T15:54:51Z"
	datV, err := time.Parse(time.RFC3339, dat)
	assert.NoError(err)
	datDt := strfmt.DateTime(datV)

	uDat := "2019-11-29T15:54:51Z"
	uDatV, err := time.Parse(time.RFC3339, uDat)
	assert.NoError(err)
	uDatDt := strfmt.DateTime(uDatV)

	now := time.Now()

	payload := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID:      "id1",
				Version: 1,
			},
			AccountID:          "accountID",
			ConsistencyGroupID: "c-g-1",
			PitIdentifier:      "pit1",
			SizeBytes:          12345,
			SnapIdentifier:     "HEAD",
			SnapTime:           strfmt.DateTime(now),
			TenantAccountID:    "id2",
			VolumeSeriesID:     "vs1",
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: datDt,
			Locations: map[string]models.SnapshotLocation{
				"CSP-DOMAIN-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "CSP-DOMAIN-1"},
			},
			Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg1"}},
			SystemTags: models.ObjTags{"stag11", "stag12"},
			Tags:       models.ObjTags{"tag11", "tag12"},
		},
	}
	listPayload := []*models.Snapshot{}
	listPayload = append(listPayload, payload)
	lRes := &snapshot.SnapshotListOK{Payload: listPayload}
	fRes := &snapshot.SnapshotFetchOK{Payload: payload}

	lParams := snapshot.NewSnapshotListParams()
	lParams.ConsistencyGroupID = swag.String("c-g-1")
	lParams.SnapIdentifier = swag.String("HEAD")
	lParams.VolumeSeriesID = swag.String("vs1")
	m := mockmgmtclient.NewSnapshotMatcher(t, lParams)

	fParams := snapshot.NewSnapshotFetchParams()
	fParams.ID = string(payload.Meta.ID)
	fm := mockmgmtclient.NewSnapshotMatcher(t, fParams)

	// successful updates
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	var mAPI *mockmgmtclient.MockAPI
	for tc := 0; tc <= 4; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		sOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)

		var resPayload *models.Snapshot
		if tc != 4 {
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs, loadVSsWithList)
			sOps.EXPECT().SnapshotList(m).Return(lRes, nil)
			resPayload = lRes.Payload[0]
		} else {
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
			sOps.EXPECT().SnapshotFetch(fm).Return(fRes, nil)
			resPayload = fRes.Payload
		}
		uParams := snapshot.NewSnapshotUpdateParams()
		uParams.ID = string(resPayload.Meta.ID)

		mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)

		var cmdArgs []string
		switch tc {
		case 0:
			t.Log("case: successful update for DeleteAfterTime and tags with SET")
			uParams.Set = []string{"deleteAfterTime", "tags"}
			uParams.Payload = &models.SnapshotMutable{
				DeleteAfterTime: uDatDt,
				Tags:            models.ObjTags{"tag1", "tag2"},
			}
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"--delete-after-time", uDat, "-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-o", "json"}
		case 1:
			t.Log("case: successful update for tags with APPEND")
			uParams.Append = []string{"tags"}
			uParams.Payload = &models.SnapshotMutable{
				Tags: models.ObjTags{"tag1", "tag2"},
			}
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"-t", "tag1", "-t", "tag2", "--tag-action", "APPEND", "-o", "json"}
		case 2:
			t.Log("case: successful update for tags with REMOVE")
			uParams.Remove = []string{"tags"}
			uParams.Payload = &models.SnapshotMutable{
				Tags: models.ObjTags{"tag1", "tag2"},
			}
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-o", "json"}
		case 3:
			t.Log("case: update with empty SET")
			uParams.Set = []string{"tags"}
			uParams.Payload = &models.SnapshotMutable{}
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"--tag-action", "SET", "-o", "json"}
		case 4:
			t.Log("case: update with empty SET using object ID")
			uParams.Set = []string{"tags"}
			uParams.Payload = &models.SnapshotMutable{}
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--id", "id1", "--tag-action", "SET", "-o", "json"}
		}
		um := mockmgmtclient.NewSnapshotMatcher(t, uParams)

		uRet := snapshot.NewSnapshotUpdateOK()
		uRet.Payload = resPayload
		sOps.EXPECT().SnapshotUpdate(um).Return(uRet, nil)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initSnapshot()
		err = parseAndRun(cmdArgs)
		assert.Nil(err)
		assert.NotNil(te.jsonData)
		assert.EqualValues([]*models.Snapshot{uRet.Payload}, te.jsonData)
		appCtx.Account, appCtx.AccountID = "", ""
	}

	// errors
	apiUpdateErr := &snapshot.SnapshotUpdateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("update api error"),
		},
	}
	apiFetchErr := &snapshot.SnapshotFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("fetch api error"),
		},
	}
	uParams := snapshot.NewSnapshotUpdateParams()
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Set = []string{"tags"}
	uParams.Payload = &models.SnapshotMutable{}
	um := mockmgmtclient.NewSnapshotMatcher(t, uParams)

	for tc := 0; tc <= 6; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		sOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)

		var cmdArgs []string
		var expectedErr error
		switch tc {
		case 0:
			t.Log("case: modify, API failure model error")
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs, loadVSsWithList)
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"--tag-action", "SET", "-o", "json"}

			mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
			sOps.EXPECT().SnapshotList(m).Return(lRes, nil)
			sOps.EXPECT().SnapshotUpdate(um).Return(nil, apiUpdateErr)
		case 1:
			t.Log("case: modify, API failure arbitrary error")
			expectedErr = fmt.Errorf("OTHER ERROR")

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs, loadVSsWithList)
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"--tag-action", "SET", "-o", "json"}

			mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
			sOps.EXPECT().SnapshotList(m).Return(lRes, nil).MinTimes(1)
			sOps.EXPECT().SnapshotUpdate(um).Return(nil, expectedErr).MinTimes(1)
		case 2:
			t.Log("case: modify, consistency group not found")
			expectedErr = fmt.Errorf("consistency group.*not found")

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs, loadVSsWithList)
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "foo", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"--tag-action", "SET", "-o", "json"}
		case 3:
			t.Log("case: modify, volume series cache load failure")
			expectedErr = fmt.Errorf("vsListError")

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"-S", "HEAD", "--volume-series", "MyVolSeries",
				"--tag-action", "SET", "-o", "json"}

			vsm := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
			vsm.ListParam.Name = swag.String("MyVolSeries")
			vsm.ListParam.AccountID = swag.String("accountID")
			vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
			mAPI.EXPECT().VolumeSeries().Return(vsOps)
			vsOps.EXPECT().VolumeSeriesList(vsm).Return(nil, expectedErr).MinTimes(1)
		case 4:
			t.Log("case: modify, snapshot list failure")
			expectedErr = fmt.Errorf("LIST ERROR")

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs, loadVSsWithList)
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries",
				"--tag-action", "SET", "-o", "json"}

			mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
			sOps.EXPECT().SnapshotList(m).Return(nil, expectedErr).MinTimes(1)
		case 5:
			t.Log("case: modify, snapshot fetch failure")
			expectedErr = fmt.Errorf("FETCH ERROR")

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--id", "id1", "--tag-action", "SET", "-o", "json"}

			mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
			sOps.EXPECT().SnapshotFetch(fm).Return(nil, expectedErr).MinTimes(1)
		case 6:
			t.Log("case: modify, snapshot fetch failure with API error")
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--id", "id1", "--tag-action", "SET", "-o", "json"}

			mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
			sOps.EXPECT().SnapshotFetch(fm).Return(nil, apiFetchErr).MinTimes(1)
		}
		uRet := snapshot.NewSnapshotUpdateOK()
		uRet.Payload = lRes.Payload[0]
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initSnapshot()
		err = parseAndRun(cmdArgs)
		assert.NotNil(err)
		if tc == 0 {
			assert.Equal(*apiUpdateErr.Payload.Message, err.Error())
		} else if tc == 6 {
			assert.Equal(*apiFetchErr.Payload.Message, err.Error())
		} else {
			assert.Regexp(expectedErr.Error(), err.Error())
		}
		appCtx.Account, appCtx.AccountID = "", ""
	}

	mockCtrl.Finish()
	t.Log("case: init context failure")
	appCtx.Account, appCtx.AccountID = "System", ""
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err = parseAndRun([]string{"snap", "modify", "-A", "System",
		"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: other error cases")
	for tc := 0; tc <= 3; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)

		var cmdArgs []string
		var expectedErrPat string
		switch tc {
		case 0:
			t.Log("case: no modifications")
			expectedErrPat = "No modifications"

			cmdArgs = []string{"sn", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries", "-o", "json"}

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
		case 1:
			t.Log("case: missing parameters to find snapshot to modify")
			expectedErrPat = "Either Snapshot object ID or the pair of SnapIdentifier and VolumeSeries should be provided"

			cmdArgs = []string{"sn", "modify", "-A", "System", "--tag-action", "SET", "-o", "json"}

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
		case 2:
			t.Log("case: invalid columns")
			expectedErrPat = "invalid column"

			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "MyVolSeries", "--columns", "SnapIdentifier,BAD_ONE"}

			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		case 3:
			t.Log("case: bad VolumeSeries")
			expectedErrPat = "VolumeSeries.*not found"

			cmdArgs = []string{"snap", "modify", "-A", "System",
				"--consistency-group", "CG", "-S", "HEAD", "--volume-series", "foo", "--tag-action", "SET", "-o", "json"}

			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
			vsm := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
			vsm.ListParam.Name = swag.String("foo")
			vsm.ListParam.AccountID = swag.String("accountID")
			vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
			mAPI.EXPECT().VolumeSeries().Return(vsOps)
			vsOps.EXPECT().VolumeSeriesList(vsm).Return(&volume_series.VolumeSeriesListOK{}, nil).MinTimes(1)
		}
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initSnapshot()
		err = parseAndRun(cmdArgs)
		assert.NotNil(err)
		assert.Regexp(expectedErrPat, err.Error())
		appCtx.Account, appCtx.AccountID = "", ""
	}

	t.Log("id + extra")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSnapshot()
	err = parseAndRun([]string{"snap", "modify", "--id", "id1", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
