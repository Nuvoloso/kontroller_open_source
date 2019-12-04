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


package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	fps "github.com/Nuvoloso/kontroller/pkg/pstore/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	now := time.Now()
	obj := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			AccountID:          "account1",
			ConsistencyGroupID: "cg1",
			PitIdentifier:      "pit1",
			SizeBytes:          12345,
			SnapIdentifier:     "HEAD",
			SnapTime:           strfmt.DateTime(now),
			TenantAccountID:    "tid1",
			VolumeSeriesID:     "vs1",
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
			Locations: map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
				"csp-2": {CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "csp-2"},
				"csp-3": {CreationTime: strfmt.DateTime(now.Add(-3 * time.Hour)), CspDomainID: "csp-3"},
			},
			Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			SystemTags: models.ObjTags{"stag1", "stag2"},
			Tags:       models.ObjTags{"tag1", "tag2"},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}

	t.Log("case: SystemManagementCap capability fails")
	ai := &auth.Info{}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.snapshotFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "account1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.snapshotFetchFilter(ai, obj))

	t.Log("case: CSPDomainUsageCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "account1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.snapshotFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.snapshotFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.snapshotFetchFilter(ai, obj))

	t.Log("case: owner account")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "account1"
	assert.NoError(hc.snapshotFetchFilter(ai, obj))

	t.Log("case: not owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: false}
	ai.AccountID = "account1"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.snapshotFetchFilter(ai, obj))
}

func TestSnapshotCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fPSO := &fps.Controller{}
	app.PSO = fPSO
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fops := &fakeOps{}
	hc.ops = fops
	now := time.Now()
	ai := &auth.Info{}
	params := ops.SnapshotCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload: &models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				PitIdentifier:      "pit1",
				ProtectionDomainID: "snap-pd-1",
				SnapIdentifier:     "SNAP-1",
				SnapTime:           strfmt.DateTime(now),
				VolumeSeriesID:     "vs1",
			},
			SnapshotMutable: models.SnapshotMutable{
				DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
				Locations: map[string]models.SnapshotLocation{
					"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
				},
				Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
				SystemTags: models.ObjTags{"stag1", "stag2"},
				Tags:       models.ObjTags{"snapTag1", "snapTag2"},
			},
		},
	}
	ctx := params.HTTPRequest.Context()
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("vs1"),
			},
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       "account1",
			TenantAccountID: "tenacc1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:               "vsName",
				ConsistencyGroupID: "cg1",
				SizeBytes:          swag.Int64(util.BytesInMiB),
				ServicePlanID:      "spID",
				Tags:               models.ObjTags{"vsTag1", "vsTag2"},
			},
		},
	}
	sObj := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
			AccountID:          models.ObjID(vsObj.AccountID),
			ConsistencyGroupID: models.ObjID(vsObj.ConsistencyGroupID),
			PitIdentifier:      "pit1",
			ProtectionDomainID: "pd-1",
			SizeBytes:          swag.Int64Value(vsObj.SizeBytes),
			SnapIdentifier:     "SNAP-1",
			SnapTime:           strfmt.DateTime(now),
			TenantAccountID:    models.ObjID(vsObj.TenantAccountID),
			VolumeSeriesID:     "vs1",
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
			Locations: map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
			},
			Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			SystemTags: models.ObjTags{"stag1", "stag2"},
			Tags:       models.ObjTags{"snapTag1", "snapTag2"},
		},
	}
	cspObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("csp-1"),
			},
			AccountID:     "account1",
			CspDomainType: aws.CSPDomainType,
		},
		CSPDomainMutable: models.CSPDomainMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"account1"},
			Name:               "cspDomain",
		},
	}
	da := make(map[string]models.ValueType)
	da[aws.AttrPStoreBucketName] = models.ValueType{Value: "bucket", Kind: common.ValueTypeString}
	da[aws.AttrRegion] = models.ValueType{Value: "region", Kind: common.ValueTypeString}
	da[aws.AttrAccessKeyID] = models.ValueType{Value: "access-key", Kind: common.ValueTypeString}
	da[aws.AttrSecretAccessKey] = models.ValueType{Value: "secret-access-key", Kind: common.ValueTypeSecret}
	cspObj.CspDomainAttributes = da
	cgObj := &models.ConsistencyGroup{}
	cgObj.Meta = &models.ObjMeta{ID: "cg1"}
	cgObj.Name = "cgName"
	snapPdObj := &models.ProtectionDomain{}
	snapPdObj.Meta = &models.ObjMeta{ID: "snap-pd-1"}
	snapPdObj.Name = "snapPD"
	snapPdObj.EncryptionAlgorithm = "snapEA"
	snapPdObj.EncryptionPassphrase = &models.ValueType{Kind: common.ValueTypeSecret, Value: "snapSecret"}
	catCspObj := &models.CSPDomain{}
	catCspObj.Meta = &models.ObjMeta{ID: "cat-csp-1"}
	catCspObj.CspDomainType = models.CspDomainTypeMutable(aws.CSPDomainType)
	da = make(map[string]models.ValueType)
	da[aws.AttrPStoreBucketName] = models.ValueType{Value: "cat-bucket", Kind: common.ValueTypeString}
	da[aws.AttrRegion] = models.ValueType{Value: "cat-region", Kind: common.ValueTypeString}
	da[aws.AttrAccessKeyID] = models.ValueType{Value: "cat-access-key", Kind: common.ValueTypeString}
	da[aws.AttrSecretAccessKey] = models.ValueType{Value: "cat-secret-access-key", Kind: common.ValueTypeSecret}
	catCspObj.CspDomainAttributes = da
	catPdObj := &models.ProtectionDomain{}
	catPdObj.Meta = &models.ObjMeta{ID: "cat-pd-1"}
	catPdObj.Name = "catPD"
	catPdObj.EncryptionAlgorithm = "catEA"
	catPdObj.EncryptionPassphrase = &models.ValueType{Kind: common.ValueTypeSecret, Value: "catSecret"}
	taObj := &models.Account{}
	taObj.Meta = &models.ObjMeta{ID: "tenacc1"}
	taObj.Name = "tennantAccount"
	aObj := &models.Account{}
	aObj.Meta = &models.ObjMeta{ID: "account1"}
	aObj.Name = "account"
	aObj.TenantAccountID = models.ObjIDMutable(taObj.Meta.ID)
	aObj.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		CspDomainID:        "cat-csp-1",
		ProtectionDomainID: "cat-pd-1",
	}

	var ret middleware.Responder

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: success")
	intAi := &auth.Info{}
	// test with non-internal though in reality calls are internal
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = string(vsObj.AccountID)
	fops = &fakeOps{}
	hc.ops = fops
	fops.InAccountFetchAiMulti = []*auth.Info{}
	fops.InAccountFetchMulti = []string{}
	fops.RetAccountFetchMulti = []*models.Account{aObj, taObj}
	fops.RetConsistencyGroupFetchObj = cgObj
	fops.InCspDomainFetchAiMulti = []*auth.Info{}
	fops.InCspDomainFetchMulti = []string{}
	fops.RetCspDomainFetchMulti = []*models.CSPDomain{cspObj, catCspObj}
	fops.InProtectionDomainFetchAiMulti = []*auth.Info{}
	fops.InProtectionDomainFetchMulti = []string{}
	fops.RetProtectionDomainFetchMulti = []*models.ProtectionDomain{snapPdObj, catPdObj}
	oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
	oS := mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Create(ctx, params.Payload).Return(sObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	ret = hc.snapshotCreate(params)
	assert.NotNil(ret)
	_, ok := ret.(*ops.SnapshotCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.EqualValues("tenacc1", params.Payload.TenantAccountID)
	// Account
	assert.Len(fops.InAccountFetchAiMulti, 2)
	assert.Equal(ai, fops.InAccountFetchAiMulti[0]) // invoker auth
	assert.Equal(ai, fops.InAccountFetchAiMulti[1]) // invoker auth
	assert.Len(fops.InAccountFetchMulti, 2)
	assert.EqualValues(aObj.Meta.ID, fops.InAccountFetchMulti[0])
	assert.EqualValues(taObj.Meta.ID, fops.InAccountFetchMulti[1])
	assert.Contains(params.Payload.AccountID, fops.InAccountFetchMulti[0])
	assert.Contains(params.Payload.TenantAccountID, fops.InAccountFetchMulti[1])
	// Dom
	assert.Len(fops.InCspDomainFetchAiMulti, 2)
	assert.Equal(ai, fops.InCspDomainFetchAiMulti[0])    // invoker auth
	assert.Equal(intAi, fops.InCspDomainFetchAiMulti[1]) // internal auth
	assert.Len(fops.InCspDomainFetchMulti, 2)
	assert.EqualValues(cspObj.Meta.ID, fops.InCspDomainFetchMulti[0])
	assert.EqualValues(catCspObj.Meta.ID, fops.InCspDomainFetchMulti[1])
	assert.Contains(params.Payload.Locations, fops.InCspDomainFetchMulti[0])
	// PD
	assert.Len(fops.InProtectionDomainFetchAiMulti, 2)
	assert.Equal(ai, fops.InProtectionDomainFetchAiMulti[0])    // internal auth
	assert.Equal(intAi, fops.InProtectionDomainFetchAiMulti[1]) // internal auth
	assert.Len(fops.InProtectionDomainFetchMulti, 2)
	assert.EqualValues(snapPdObj.Meta.ID, fops.InProtectionDomainFetchMulti[0])
	assert.EqualValues(catPdObj.Meta.ID, fops.InProtectionDomainFetchMulti[1])
	assert.EqualValues(params.Payload.ProtectionDomainID, fops.InProtectionDomainFetchMulti[0])
	// CG
	assert.EqualValues(cgObj.Meta.ID, fops.InConsistencyGroupFetchID)
	assert.Equal(ai, fops.InConsistencyGroupFetchAi)
	// PSO
	assert.NotNil(fPSO.InSCatUpArg)
	expUA := &pstore.SnapshotCatalogUpsertArgs{
		Entry: pstore.SnapshotCatalogEntry{
			SnapIdentifier:       params.Payload.SnapIdentifier,
			SnapTime:             time.Time(params.Payload.SnapTime),
			SizeBytes:            params.Payload.SizeBytes,
			AccountID:            string(aObj.Meta.ID),
			AccountName:          string(aObj.Name),
			TenantAccountName:    string(taObj.Name),
			VolumeSeriesID:       string(vsObj.Meta.ID),
			VolumeSeriesName:     string(vsObj.Name),
			ProtectionDomainID:   string(snapPdObj.Meta.ID),
			ProtectionDomainName: string(snapPdObj.Name),
			EncryptionAlgorithm:  snapPdObj.EncryptionAlgorithm,
			ProtectionStores: []pstore.ProtectionStoreDescriptor{
				pstore.ProtectionStoreDescriptor{
					CspDomainType: string(cspObj.CspDomainType),
					CspDomainAttributes: map[string]models.ValueType{
						aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bucket"},
						aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "region"},
					},
				},
			},
			ConsistencyGroupID:   string(cgObj.Meta.ID),
			ConsistencyGroupName: string(cgObj.Name),
			VolumeSeriesTags:     vsObj.Tags,
			SnapshotTags:         params.Payload.Tags,
		},
		PStore: &pstore.ProtectionStoreDescriptor{
			CspDomainType:       string(cspObj.CspDomainType),
			CspDomainAttributes: catCspObj.CspDomainAttributes,
		},
		EncryptionAlgorithm: catPdObj.EncryptionAlgorithm,
		Passphrase:          catPdObj.EncryptionPassphrase.Value,
		ProtectionDomainID:  string(catPdObj.Meta.ID),
	}
	assert.Equal(expUA, fPSO.InSCatUpArg)
	assert.True(fPSO.InSCatUpArg.Validate()) // is valid
	assert.Equal(sObj, evM.InACScope)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: relevant VS obj params used instead of input params if such provided")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = string(vsObj.AccountID)
	fops = &fakeOps{}
	hc.ops = fops
	fops.InAccountFetchAiMulti = []*auth.Info{}
	fops.InAccountFetchMulti = []string{}
	fops.RetAccountFetchMulti = []*models.Account{aObj, taObj}
	fops.RetConsistencyGroupFetchObj = cgObj
	fops.InCspDomainFetchAiMulti = []*auth.Info{}
	fops.InCspDomainFetchMulti = []string{}
	fops.RetCspDomainFetchMulti = []*models.CSPDomain{cspObj, catCspObj}
	fops.InProtectionDomainFetchAiMulti = []*auth.Info{}
	fops.InProtectionDomainFetchMulti = []string{}
	fops.RetProtectionDomainFetchMulti = []*models.ProtectionDomain{snapPdObj, catPdObj}
	mockCtrl = gomock.NewController(t)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Create(ctx, params.Payload).Return(sObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	inParams := ops.SnapshotCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload: &models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				PitIdentifier:      "pit1",
				ProtectionDomainID: "snap-pd-1",
				SnapIdentifier:     "SNAP-1",
				SnapTime:           strfmt.DateTime(now),
				VolumeSeriesID:     "vs1",
				SizeBytes:          12345,                    // ignored
				ConsistencyGroupID: "some_cg",                // ignored
				AccountID:          "some_account_id",        // ignored
				TenantAccountID:    "some_tenant_account_id", // ignored
			},
			SnapshotMutable: models.SnapshotMutable{
				DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
				Locations: map[string]models.SnapshotLocation{
					"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
				},
				Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
				SystemTags: models.ObjTags{"stag1", "stag2"},
				Tags:       models.ObjTags{"snapTag1", "snapTag2"},
			},
		},
	}
	inParamsOriginal := ops.SnapshotCreateParams{}
	testutils.Clone(inParams, &inParamsOriginal)
	ret = hc.snapshotCreate(inParams)
	assert.NotNil(ret)
	resSnapObj, ok := ret.(*ops.SnapshotCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.NotEqual(inParamsOriginal.Payload.SizeBytes, resSnapObj.Payload.SizeBytes)
	assert.Equal(*vsObj.SizeBytes, resSnapObj.Payload.SizeBytes)
	assert.NotEqual(inParamsOriginal.Payload.ConsistencyGroupID, resSnapObj.Payload.ConsistencyGroupID)
	assert.Equal(models.ObjID(vsObj.ConsistencyGroupID), resSnapObj.Payload.ConsistencyGroupID)
	assert.NotEqual(inParamsOriginal.Payload.TenantAccountID, resSnapObj.Payload.TenantAccountID)
	assert.Equal(models.ObjID(vsObj.TenantAccountID), resSnapObj.Payload.TenantAccountID)
	assert.NotEqual(inParamsOriginal.Payload.AccountID, resSnapObj.Payload.AccountID)
	assert.Equal(models.ObjID(vsObj.AccountID), resSnapObj.Payload.AccountID)
	tl.Flush()

	t.Log("case: auth internal error")
	fops.RetProtectionDomainFetchObj = &models.ProtectionDomain{}
	fops.RetProtectionDomainFetchErr = nil
	params.HTTPRequest = &http.Request{}
	ret = hc.snapshotCreate(params)
	assert.NotNil(ret)
	mE, ok := ret.(*ops.SnapshotCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	invalidParamsError := &centrald.Error{M: com.ErrorMissing + ": PitIdentifier, SnapIdentifier, VolumeSeriesID, Locations and ProtectionDomainID are required", C: 400}

	for tc := 0; tc <= 19; tc++ {
		mockCtrl.Finish()
		params := ops.SnapshotCreateParams{
			HTTPRequest: requestWithAuthContext(ai),
			Payload: &models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					PitIdentifier:      "pit1",
					ProtectionDomainID: "pd-1",
					SnapIdentifier:     "HEAD",
					SnapTime:           strfmt.DateTime(now),
					VolumeSeriesID:     "vs1",
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
					},
					Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
					SystemTags: models.ObjTags{"stag1", "stag2"},
					Tags:       models.ObjTags{"tag1", "tag2"},
				},
			},
		}
		ctx := params.HTTPRequest.Context()
		fPSO = &fps.Controller{}
		app.PSO = fPSO
		fops := &fakeOps{}
		hc.ops = fops
		fops.InAccountFetchAiMulti = []*auth.Info{}
		fops.InAccountFetchMulti = []string{}
		fops.RetAccountFetchMulti = []*models.Account{aObj, taObj}
		fops.RetConsistencyGroupFetchObj = cgObj
		fops.InCspDomainFetchAiMulti = []*auth.Info{}
		fops.InCspDomainFetchMulti = []string{}
		fops.RetCspDomainFetchMulti = []*models.CSPDomain{cspObj, catCspObj}
		fops.InProtectionDomainFetchAiMulti = []*auth.Info{}
		fops.InProtectionDomainFetchMulti = []string{}
		fops.RetProtectionDomainFetchMulti = []*models.ProtectionDomain{snapPdObj, catPdObj}
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		expectedErr := centrald.ErrorMissing
		errPat := ".*"
		lockObtained := true
		switch tc {
		case 0:
			t.Log("case: no VolumeSeries found")
			expectedErr = &centrald.Error{M: com.ErrorMissing + ": invalid VolumeSeriesID", C: 400}
			oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 1:
			t.Log("case: create fails")
			expectedErr = centrald.ErrorExists
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			oS = mock.NewMockSnapshotOps(mockCtrl)
			oS.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
			mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		case 2:
			t.Log("case: unauthorized")
			expectedErr = centrald.ErrorUnauthorizedOrForbidden
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: false}
			ai.AccountID = "account1"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 3:
			t.Log("case: invalid cspDomainId")
			expectedErr = &centrald.Error{M: com.ErrorMissing + ": invalid cspDomainId", C: 400}
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
			ai.AccountID = "account1"
			fops.RetCspDomainFetchErr = centrald.ErrorNotFound
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 4:
			t.Log("case: invalid cspDomainId account")
			expectedErr = centrald.ErrorUnauthorizedOrForbidden
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
			var newCspObj *models.CSPDomain
			testutils.Clone(cspObj, &newCspObj)
			newCspObj.AuthorizedAccounts = []models.ObjIDMutable{"wrong_account"}
			fops.RetCspDomainFetchMulti = []*models.CSPDomain{newCspObj}
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 5:
			t.Log("case: DeleteAfterTime is in the past")
			expectedErr = &centrald.Error{M: com.ErrorMissing + ": DeleteAfterTime is in the past", C: 400}
			params.Payload.DeleteAfterTime = strfmt.DateTime(time.Now().Add(-time.Hour))
			lockObtained = false
		case 6:
			t.Log("case: no PitIdentifier")
			expectedErr = invalidParamsError
			params.Payload.PitIdentifier = ""
			lockObtained = false
		case 7:
			t.Log("case: no SnapIdentifier")
			expectedErr = invalidParamsError
			params.Payload.SnapIdentifier = ""
			lockObtained = false
		case 8:
			t.Log("case: no VolumeSeriesID")
			expectedErr = invalidParamsError
			params.Payload.VolumeSeriesID = ""
			lockObtained = false
		case 9:
			t.Log("case: no Location")
			expectedErr = invalidParamsError
			params.Payload.Locations = map[string]models.SnapshotLocation{}
			lockObtained = false
		case 10:
			t.Log("case: no ProtectionDomainID")
			expectedErr = invalidParamsError
			params.Payload.ProtectionDomainID = ""
			lockObtained = false
		case 11:
			t.Log("case: nil payload")
			expectedErr = invalidParamsError
			params.Payload = nil
			lockObtained = false
		case 12:
			t.Log("case: invalid protectionDomainId")
			expectedErr = centrald.ErrorMissing
			fops.RetProtectionDomainFetchObj = nil
			fops.RetProtectionDomainFetchErr = centrald.ErrorNotFound
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 13:
			t.Log("case: upsert fails")
			ai.RoleObj = nil // use internal
			fPSO.RetSCatUpErr = fmt.Errorf("upsert-error")
			expectedErr = centrald.ErrorInternalError
			errPat = "snapshot catalog upsert: upsert-error"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 14:
			t.Log("case: cat-pd fetch fails")
			ai.RoleObj = nil // use internal
			fops.RetProtectionDomainFetchMulti[1] = nil
			expectedErr = centrald.ErrorInternalError
			errPat = "load catalog protection domain object.*fake-pd-missing"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 15:
			t.Log("case: cat-ps fetch fails")
			ai.RoleObj = nil // use internal
			fops.RetCspDomainFetchMulti[1] = nil
			expectedErr = centrald.ErrorInternalError
			errPat = "load catalog CSP domain object.*fake-dom-missing"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 16:
			t.Log("case: cg fetch fails")
			ai.RoleObj = nil // use internal
			fops.RetConsistencyGroupFetchObj = nil
			fops.RetConsistencyGroupFetchErr = fmt.Errorf("cg-error")
			expectedErr = centrald.ErrorInternalError
			errPat = "load consistency group object.*cg-error"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 17:
			t.Log("case: tenant account fetch fails")
			ai.RoleObj = nil // use internal
			fops.RetAccountFetchMulti[1] = nil
			expectedErr = centrald.ErrorInternalError
			errPat = "load tenant account object.*fake-account-missing"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 18:
			t.Log("case: account fetch fails")
			ai.RoleObj = nil // use internal
			fops.RetAccountFetchMulti = nil
			fops.RetAccountFetchErr = fmt.Errorf("account-fetch-error")
			fops.RetAccountFetchObj = nil
			expectedErr = centrald.ErrorInternalError
			errPat = "load account object.*account-fetch-error"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		case 19:
			t.Log("case: invalid SnapshotCatalogPolicy for account")
			ai.RoleObj = nil // use internal
			fops.RetAccountFetchMulti[0].SnapshotCatalogPolicy.CspDomainID = ""
			errPat = "invalid snapshotCatalogPolicy"
			oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS).MinTimes(1)
		default:
			assert.False(true, "case %d", tc)
			continue
		}
		hc.DS = mds
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		ret = hc.snapshotCreate(params)
		assert.NotNil(ret)
		mE, ok := ret.(*ops.SnapshotCreateDefault)
		assert.True(ok)
		if lockObtained {
			assert.Equal(cntRLock+1, hc.cntRLock)
			assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		}
		assert.Equal(expectedErr.C, int(mE.Payload.Code))
		assert.Regexp("^"+expectedErr.M, *mE.Payload.Message)
		assert.Regexp(errPat, *mE.Payload.Message)
		tl.Flush()
	}
}

func TestSnapshotDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	now := time.Now()
	ai := &auth.Info{}
	params := ops.SnapshotDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
			VolumeSeriesID: "vs1",
			SnapIdentifier: "snapID",
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
			Locations: map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
			},
			Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			SystemTags: models.ObjTags{"stag1", "stag2"},
			Tags:       models.ObjTags{"tag1", "tag2"},
		},
	}
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("vs1"),
			},
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       "account1",
			TenantAccountID: "tenacc1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "cg1",
				SizeBytes:          swag.Int64(util.BytesInMiB),
				ServicePlanID:      "spID",
			},
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				Mounts: []*models.Mount{},
			},
		},
	}
	vsMounts := []*models.Mount{
		{SnapIdentifier: "snapID", MountedNodeID: "node1", MountMode: "READ_WRITE", MountState: "MOUNTING"},
		{SnapIdentifier: "HEAD", MountedNodeID: "node1", MountedNodeDevice: "dev", MountMode: "READ_WRITE", MountState: "MOUNTED"},
	}

	var ret middleware.Responder
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: success")
	oS := mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Delete(ctx, params.ID).Return(nil)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, string(obj.VolumeSeriesID)).Return(vsObj, nil)
	oVSR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	vsrLParams := volume_series_request.VolumeSeriesRequestListParams{
		SnapshotID:   swag.String(params.ID),
		IsTerminated: swag.Bool(false),
	}
	oVSR.EXPECT().Count(ctx, vsrLParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	hc.DS = mds
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.snapshotDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.SnapshotDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues(string(obj.Meta.ID), evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	for tc := 0; tc <= 5; tc++ {
		mockCtrl.Finish()
		ai.RoleObj = nil
		ai.AccountID = ""
		vsObj.Mounts = nil
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		oS = mock.NewMockSnapshotOps(mockCtrl)
		oVSR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		expectedCntLock := cntLock
		expectedCntUnlock := cntUnlock
		expectedErr := centrald.ErrorNotFound
		switch tc {
		case 0:
			t.Log("case: fetch failure")
			expectedCntLock--
			expectedCntUnlock--

			oS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
		case 1:
			t.Log("case: delete failure")
			expectedErr = centrald.ErrorDbError

			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oS.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
			oVS.EXPECT().Fetch(ctx, string(obj.VolumeSeriesID)).Return(vsObj, nil)
			oVSR.EXPECT().Count(ctx, vsrLParams, uint(1)).Return(0, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
		case 2:
			t.Log("case: failure to fetch volume")
			expectedErr = &centrald.Error{M: com.ErrorMissing + ": invalid VolumeSeriesID", C: 400}

			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oVS.EXPECT().Fetch(ctx, string(obj.VolumeSeriesID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
		case 3:
			t.Log("case: removing mounted snapshot")
			expectedErr = &centrald.Error{M: "Snapshot is still mounted", C: centrald.ErrorUnauthorizedOrForbidden.C}

			vsObj.Mounts = vsMounts
			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oVS.EXPECT().Fetch(ctx, string(obj.VolumeSeriesID)).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
		case 4:
			t.Log("case: unauthorized")
			expectedErr = centrald.ErrorUnauthorizedOrForbidden

			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: false}
			ai.AccountID = "account1"
			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		case 5:
			t.Log("case: in use by VSR")
			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oVS.EXPECT().Fetch(ctx, string(obj.VolumeSeriesID)).Return(vsObj, nil)
			oVSR.EXPECT().Count(ctx, vsrLParams, uint(1)).Return(1, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
			expectedErr = hc.eRequestInConflict("snapshot in use").(*centrald.Error)
		default:
			assert.False(true)
		}
		mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.snapshotDelete(params) })
		assert.NotNil(ret)
		mE, ok := ret.(*ops.SnapshotDeleteDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mE.Payload.Code))
		assert.Equal(expectedErr.M, *mE.Payload.Message)
		assert.Equal(expectedCntLock+1, hc.cntLock)
		assert.Equal(expectedCntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.snapshotDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.SnapshotDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
}

func TestSnapshotFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	now := time.Now()
	ai := &auth.Info{}
	params := ops.SnapshotFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	csp1Loc := models.SnapshotLocation{CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"}
	obj := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
			ConsistencyGroupID: "cg-id",
			SnapIdentifier:     "snap-identifier",
			VolumeSeriesID:     "vs-1",
			PitIdentifier:      "pit-identifier",
			SizeBytes:          1,
			SnapTime:           strfmt.DateTime(now.Add(-24 * time.Hour)),
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
			Locations: map[string]models.SnapshotLocation{
				"csp-1": csp1Loc,
			},
			Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			SystemTags: models.ObjTags{"stag1", "stag2"},
			Tags:       models.ObjTags{"tag1", "tag2"},
		},
	}
	var ret middleware.Responder
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: success")
	oS := mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.snapshotFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.SnapshotFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	for tc := 0; tc <= 2; tc++ {
		mockCtrl.Finish()
		ai.RoleObj = nil
		ai.AccountID = ""
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		oS = mock.NewMockSnapshotOps(mockCtrl)
		expectedErr := centrald.ErrorNotFound
		switch tc {
		case 0:
			t.Log("case: fetch failure")
			expectedErr = centrald.ErrorNotFound
			oS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		case 1:
			t.Log("case: unauthorized")
			expectedErr = centrald.ErrorUnauthorizedOrForbidden
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: false}
			ai.AccountID = "account1"
			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		case 2:
			t.Log("case: auth internal error")
			expectedErr = centrald.ErrorInternalError
			params.HTTPRequest = &http.Request{}
		default:
			assert.False(true)
		}
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.snapshotFetch(params) })
		assert.NotNil(ret)
		mE, ok := ret.(*ops.SnapshotFetchDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mE.Payload.Code))
		assert.Equal(expectedErr.M, *mE.Payload.Message)
		tl.Flush()
	}

	t.Log("case: success (intSnapshotFetchData)")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
	hc.DS = mds
	sd, err := hc.intSnapshotFetchData(ctx, ai, params.ID)
	assert.NoError(err)
	expSD := &models.SnapshotData{
		ConsistencyGroupID: models.ObjIDMutable(obj.ConsistencyGroupID),
		DeleteAfterTime:    obj.DeleteAfterTime,
		Locations:          []*models.SnapshotLocation{&csp1Loc},
		PitIdentifier:      obj.PitIdentifier,
		SizeBytes:          swag.Int64(obj.SizeBytes),
		SnapIdentifier:     obj.SnapIdentifier,
		SnapTime:           obj.SnapTime,
		VolumeSeriesID:     obj.VolumeSeriesID,
	}
	assert.Equal(expSD, sd)
	tl.Flush()

	t.Log("case: failure (intSnapshotFetchData)")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(nil, fmt.Errorf("fetch-error"))
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
	hc.DS = mds
	sd, err = hc.intSnapshotFetchData(ctx, ai, params.ID)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(sd)
	tl.Flush()
}

func TestSnapshotList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fops := &fakeOps{}
	hc.ops = fops
	ai := &auth.Info{}
	params := ops.SnapshotListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.Snapshot{
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
	}
	var ret middleware.Responder
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: success")
	params.SortAsc, params.SortDesc = []string{"meta.version"}, []string{"snapTime"}
	oS := mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.SnapshotListOK)
	assert.True(ok)
	assert.Len(objects, int(mR.TotalCount))
	assert.Equal(1, fops.CntConstrainEOQueryAccounts)
	assert.Nil(fops.InConstrainEOQueryAccountsAID)
	assert.Nil(fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: bad sort keys")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.SortAsc, params.SortDesc = []string{"badkey"}, []string{"badkey2"}
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.SnapshotListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mD.Payload.Message)
	assert.Equal(2, fops.CntConstrainEOQueryAccounts)
	params.SortAsc, params.SortDesc = []string{}, []string{}
	tl.Flush()

	t.Log("case: list failure")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SnapshotListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(3, fops.CntConstrainEOQueryAccounts)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SnapshotListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	assert.Equal(3, fops.CntConstrainEOQueryAccounts)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: constrainEitherOrQueryAccounts changes accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("aid1"), swag.String("tid1")
	cParams := params
	cParams.AccountID, cParams.TenantAccountID = fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mR, ok = ret.(*ops.SnapshotListOK)
	assert.True(ok)
	assert.Equal(4, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts nil accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = nil, nil
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mR, ok = ret.(*ops.SnapshotListOK)
	assert.True(ok)
	assert.Equal(5, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts error")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SnapshotListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(6, fops.CntConstrainEOQueryAccounts)
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = nil
	ai.RoleObj = nil
	tl.Flush()

	t.Log("case: CountOnly")
	params.CountOnly = swag.Bool(true)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Count(ctx, params, uint(0)).Return(len(objects), nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mR, ok = ret.(*ops.SnapshotListOK)
	assert.True(ok)
	assert.Len(objects, int(mR.TotalCount))
	assert.Equal(7, fops.CntConstrainEOQueryAccounts)

	t.Log("case: CountOnly fails")
	params.CountOnly = swag.Bool(true)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Count(ctx, params, uint(0)).Return(0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.snapshotList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SnapshotListDefault)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(8, fops.CntConstrainEOQueryAccounts)
}

func TestSnapshotUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	fops := &fakeOps{}
	hc.ops = fops
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.snapshotMutableNameMap() })
	// validate some embedded properties
	assert.Equal("tags", nMap.jName("Tags"))
	assert.Equal("deleteAfterTime", nMap.jName("DeleteAfterTime"))
	assert.Equal("messages", nMap.jName("Messages"))

	// parse params
	objM := &models.SnapshotMutable{}
	params := ops.SnapshotUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("DeleteAfterTime")},
		Payload:     objM,
	}
	ctx := params.HTTPRequest.Context()
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	var ua *centrald.UpdateArgs
	var err error
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua).Matcher()

	obj := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: models.ObjVersion(*params.Version)},
		},
	}
	obj.AccountID = "account1"

	cspObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("csp-1"),
			},
			AccountID:     "account1",
			CspDomainType: app.SupportedCspDomainTypes()[0],
		},
		CSPDomainMutable: models.CSPDomainMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"account1"},
			Name:               "cspDomain",
		},
	}

	now := time.Now()
	var ret middleware.Responder
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// success
	for tc := 0; tc <= 1; tc++ {
		mockCtrl.Finish()
		cntRLock := hc.cntRLock
		cntRUnlock := hc.cntRUnlock
		mockCtrl = gomock.NewController(t)
		mds := mock.NewMockDataStore(mockCtrl)
		oS := mock.NewMockSnapshotOps(mockCtrl)
		switch tc {
		case 0:
			t.Log("case: success")
		case 1:
			t.Log("case: success with no version")
			params.Version = nil
		default:
			assert.False(true)
		}
		oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		oS.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
		mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.snapshotUpdate(params) })
		assert.NotNil(ret)
		mO, ok := ret.(*ops.SnapshotUpdateOK)
		assert.True(ok)
		assert.Equal(cntRLock+1, hc.cntRLock)
		assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		assert.Equal(obj, mO.Payload)
		tl.Flush()
	}

	defaultLocations := map[string]models.SnapshotLocation{
		"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
		"csp-2": {CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "csp-2"},
	}

	// failures
	for tc := 0; tc <= 15; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds := mock.NewMockDataStore(mockCtrl)
		oS := mock.NewMockSnapshotOps(mockCtrl)
		fops = &fakeOps{}
		hc.ops = fops

		// reset params
		params.Version = swag.Int32(8)
		params.Append = []string{}
		params.Set = []string{}
		params.Remove = []string{}

		updateUA := false
		needCspDom := false
		needSnapObj := false
		expectedErr := centrald.ErrorDbError

		switch tc {
		case 0:
			t.Log("case: fetch failure")
			params.Set = []string{nMap.jName("DeleteAfterTime")}
			oS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
			mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		case 1:
			t.Log("case: update failure")
			params.Set = []string{nMap.jName("DeleteAfterTime")}
			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oS.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
			mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		case 2:
			t.Log("case: no change")
			expectedErr = centrald.ErrorUpdateInvalidRequest
		case 3:
			t.Log("case: invalid version")
			expectedErr = centrald.ErrorIDVerNotFound

			params.Set = []string{nMap.jName("DeleteAfterTime")}
			params.Version = swag.Int32(1)
			needSnapObj = true
		case 4:
			t.Log("case: remove all locations")
			expectedErr = &centrald.Error{M: com.ErrorUpdateInvalidRequest + ": on remove there should be at least one location left", C: 400}

			obj.Locations = map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
			}
			params.Remove = []string{nMap.jName("Locations")}
			params.Payload.Locations = obj.Locations
			needSnapObj = true
		case 5:
			t.Log("case: remove more locations than exist")
			expectedErr = &centrald.Error{M: com.ErrorUpdateInvalidRequest + ": on remove there should be at least one location left", C: 400}

			obj.Locations = defaultLocations
			// params.Set = []sting{}
			params.Remove = []string{nMap.jName("Locations")}
			params.Payload.Locations = map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
				"csp-2": {CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "csp-2"},
				"csp-3": {CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "csp-3"},
			}
			needSnapObj = true
		case 6:
			t.Log("case: update failure on remove locations")
			obj.Locations = defaultLocations
			params.Remove = []string{nMap.jName("Locations")}
			params.Payload.Locations = map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now.Add(-1 * time.Hour)), CspDomainID: "csp-1"},
			}
			updateUA = true
		case 7:
			t.Log("case: update failure on set location with time update")
			obj.Locations = defaultLocations
			params.Set = []string{"locations.csp-1"}
			params.Payload.Locations = map[string]models.SnapshotLocation{
				"csp-1": models.SnapshotLocation{CreationTime: strfmt.DateTime(now.Add(-1 * time.Hour)), CspDomainID: "csp-1"},
			}
			updateUA = true
			needCspDom = true
		case 8:
			t.Log("case: failure to set location")
			expectedErr = &centrald.Error{M: com.ErrorMissing + ": invalid cspDomainId", C: 400}

			obj.Locations = defaultLocations
			params.Set = []string{"locations.csp-1"}
			params.Payload.Locations = map[string]models.SnapshotLocation{
				"csp-1": models.SnapshotLocation{CreationTime: strfmt.DateTime(now.Add(-1 * time.Hour)), CspDomainID: "csp-2"},
			}
			needSnapObj = true
		case 9:
			t.Log("case: failure to append location")
			obj.Locations = map[string]models.SnapshotLocation{
				"csp-3": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-3"},
				"csp-2": {CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "csp-2"},
			}
			params.Append = []string{nMap.jName("Locations")}
			params.Payload.Locations = map[string]models.SnapshotLocation{
				"csp-1": models.SnapshotLocation{CreationTime: strfmt.DateTime(now.Add(-1 * time.Hour)), CspDomainID: "csp-1"},
			}
			updateUA = true
			needCspDom = true
		case 10:
			t.Log("case: DeleteAfterTime is in the past")
			expectedErr = &centrald.Error{M: com.ErrorMissing + ": DeleteAfterTime is in the past", C: 400}

			params.Set = []string{nMap.jName("DeleteAfterTime")}
			params.Payload.DeleteAfterTime = strfmt.DateTime(now.Add(-time.Hour))
		case 11:
			t.Log("case: modify tags")
			expectedErr = centrald.ErrorUnauthorizedOrForbidden

			params.Set = []string{nMap.jName("Tags")}
			params.Payload.Tags = models.ObjTags{"tag1", "tag2"}
			obj.AccountID = "wrong_account"
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
			ai.AccountID = "account1"
			needSnapObj = true
		case 12:
			t.Log("case: unauthorized Locations update")
			expectedErr = centrald.ErrorUnauthorizedOrForbidden

			obj.AccountID = "account1"
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: false}
			ai.AccountID = "account1"
			params.Set = []string{nMap.jName("Locations")}
			params.Payload.Locations = map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
			}
			needSnapObj = true
		case 13:
			t.Log("case: unauthorized CspDomain account for Locations update")
			expectedErr = &centrald.Error{M: com.ErrorUnauthorizedOrForbidden + ": CSP domain is not available to the account", C: 403}

			ai.RoleObj = nil
			obj.AccountID = "account1"
			params.Set = []string{nMap.jName("Locations")}
			params.Payload.Locations = map[string]models.SnapshotLocation{
				"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
			}
			cspObj.AuthorizedAccounts = []models.ObjIDMutable{"wrong_account"}
			needCspDom = true
			needSnapObj = true
		case 14:
			t.Log("case: no payload")
			expectedErr = centrald.ErrorUpdateInvalidRequest

			params.Payload = nil
		case 15:
			t.Log("case: auth internal error")
			expectedErr = centrald.ErrorInternalError

			params.HTTPRequest = &http.Request{}
		default:
			assert.False(true)
		}
		if updateUA {
			uP[centrald.UpdateSet] = params.Set
			uP[centrald.UpdateRemove] = params.Remove
			uP[centrald.UpdateAppend] = params.Append
			assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
			assert.NoError(err)
			assert.NotNil(ua)
			uaM2 := updateArgsMatcher(t, ua).Matcher()
			oS.EXPECT().Update(ctx, uaM2, params.Payload).Return(nil, centrald.ErrorDbError)
		}
		if updateUA || needSnapObj {
			oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsSnapshot().Return(oS).MinTimes(1)
		}
		if needCspDom {
			fops.RetCspDomainFetchObj = cspObj
		}
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.snapshotUpdate(params) })
		assert.NotNil(ret)
		mE, ok := ret.(*ops.SnapshotUpdateDefault)
		if tc != 15 {
			assert.True(ok)
		}
		assert.Equal(expectedErr.C, int(mE.Payload.Code))
		assert.Equal(expectedErr.M, *mE.Payload.Message)
		tl.Flush()
	}
}
