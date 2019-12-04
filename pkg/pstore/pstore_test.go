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


package pstore

import (
	"context"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/csp/azure"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewController(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	tl.Logger().Info("case: empty arguments")
	ca := &ControllerArgs{}
	assert.False(ca.Validate())
	ops, err := NewController(ca)
	assert.Error(err)
	assert.Equal(ErrInvalidArguments, err)
	assert.Nil(ops)

	tl.Logger().Info("case: minimal arguments")
	ca.Log = tl.Logger()
	assert.True(ca.Validate())
	ops, err = NewController(ca)
	assert.NoError(err)
	assert.NotNil(ops)
	c, ok := ops.(*Controller)
	assert.True(ok)
	assert.Equal(ca, &c.ControllerArgs)
	assert.NotNil(c.rei)
	assert.False(c.rei.Enabled)

	tl.Logger().Info("case: all arguments")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	ca.NuvoAPI = nvAPI
	ca.DebugREI = true
	assert.True(ca.Validate())
	ops, err = NewController(ca)
	assert.NoError(err)
	assert.NotNil(ops)
	c, ok = ops.(*Controller)
	assert.True(ok)
	assert.Equal(ca, &c.ControllerArgs)
	assert.NotNil(c.rei)
	assert.True(c.rei.Enabled)
}

func TestError(t *testing.T) {
	assert := assert.New(t)

	var err error

	err = NewFatalError("fatal-error")
	assert.Equal("fatal-error", err.Error())
	ed, ok := err.(*errorDesc)
	assert.True(ok)
	assert.False(ed.IsRetryable())

	err = NewRetryableError("retryable-error")
	assert.Equal("retryable-error", err.Error())
	ed, ok = err.(*errorDesc)
	assert.True(ok)
	assert.True(ed.IsRetryable())
}

func TestPSDValidate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	tl.Logger().Info("case: invalid domain type")
	psd := &ProtectionStoreDescriptor{}
	assert.False(psd.Validate())

	awsFailureTCs := []map[string]models.ValueType{
		map[string]models.ValueType{},
		map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
		},
		map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
		},
		map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
			aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
		},
	}
	for i, tc := range awsFailureTCs {
		psd.CspDomainType = aws.CSPDomainType
		psd.CspDomainAttributes = tc
		assert.False(psd.Validate(), "case: AWS invalid attrs [%d]", i)
	}

	tl.Logger().Info("case: AWS ok")
	psd = &ProtectionStoreDescriptor{
		CspDomainType: aws.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
			aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
			aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
		},
	}
	assert.True(psd.Validate())

	tl.Logger().Info("case: Azure TBD")
	psd = &ProtectionStoreDescriptor{
		CspDomainType:       azure.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{},
	}
	assert.True(psd.Validate())

	tl.Logger().Info("case: GC")
	psd = &ProtectionStoreDescriptor{
		CspDomainType: gc.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			gc.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			gc.AttrCred:             models.ValueType{Kind: "SECRET", Value: "cred"},
		},
	}
	assert.True(psd.Validate())

	tl.Logger().Info("case: GC fail")
	psd = &ProtectionStoreDescriptor{
		CspDomainType: gc.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			gc.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
		},
	}
	assert.False(psd.Validate())
}

func TestPSDInitialize(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// unknown
	t.Log("unknown domain type")
	psd := &ProtectionStoreDescriptor{}
	psd.Initialize(&models.CSPDomain{})
	assert.Len(psd.CspDomainAttributes, 0)
	assert.Nil(psd.CspDomainAttributes)
	assert.Empty(psd.CspDomainType)

	// AWS
	t.Log("AWS domain type")
	awsCSP := &models.CSPDomain{}
	awsCSP.CspDomainType = models.CspDomainTypeMutable(aws.CSPDomainType)
	da := make(map[string]models.ValueType)
	da[aws.AttrPStoreBucketName] = models.ValueType{Value: "bucket", Kind: common.ValueTypeString}
	da[aws.AttrRegion] = models.ValueType{Value: "region", Kind: common.ValueTypeString}
	da[aws.AttrAccessKeyID] = models.ValueType{Value: "access-key", Kind: common.ValueTypeString}
	da[aws.AttrSecretAccessKey] = models.ValueType{Value: "secret-access-key", Kind: common.ValueTypeSecret}
	da["other"] = models.ValueType{Value: "foo", Kind: common.ValueTypeDuration}
	awsCSP.CspDomainAttributes = da
	psd = &ProtectionStoreDescriptor{}
	psd.Initialize(awsCSP)
	assert.EqualValues(awsCSP.CspDomainType, psd.CspDomainType)
	assert.Len(psd.CspDomainAttributes, 4)
	assert.Equal(models.ValueType{Kind: "STRING", Value: "access-key"}, psd.CspDomainAttributes[aws.AttrAccessKeyID])
	assert.Equal(models.ValueType{Kind: "SECRET", Value: "secret-access-key"}, psd.CspDomainAttributes[aws.AttrSecretAccessKey])
	assert.Equal(models.ValueType{Kind: "STRING", Value: "bucket"}, psd.CspDomainAttributes[aws.AttrPStoreBucketName])
	assert.Equal(models.ValueType{Kind: "STRING", Value: "region"}, psd.CspDomainAttributes[aws.AttrRegion])
}

func TestPSDValidateForSnapshotCatalogEntry(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	tl.Logger().Info("case: invalid domain type")
	psd := &ProtectionStoreDescriptor{}
	assert.False(psd.ValidateForSnapshotCatalogEntry())

	awsFailureTCs := []map[string]models.ValueType{
		map[string]models.ValueType{},
		map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
		},
		map[string]models.ValueType{
			aws.AttrRegion: models.ValueType{Kind: "STRING", Value: "rg"},
		},
		map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
			aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
		},
		map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
			aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
		},
	}
	for i, tc := range awsFailureTCs {
		psd.CspDomainType = aws.CSPDomainType
		psd.CspDomainAttributes = tc
		assert.False(psd.ValidateForSnapshotCatalogEntry(), "case: AWS invalid attrs [%d]", i)
	}

	psd = &ProtectionStoreDescriptor{
		CspDomainType: aws.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
		},
	}
	assert.True(psd.ValidateForSnapshotCatalogEntry())
}

func TestPSDInitializeForSnapshotCatalogEntry(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// unknown
	t.Log("unknown domain type")
	psd := &ProtectionStoreDescriptor{}
	psd.InitializeForSnapshotCatalogEntry(&models.CSPDomain{})
	assert.Len(psd.CspDomainAttributes, 0)
	assert.Nil(psd.CspDomainAttributes)
	assert.Empty(psd.CspDomainType)

	// AWS
	t.Log("AWS domain type")
	awsCSP := &models.CSPDomain{}
	awsCSP.CspDomainType = models.CspDomainTypeMutable(aws.CSPDomainType)
	da := make(map[string]models.ValueType)
	da[aws.AttrPStoreBucketName] = models.ValueType{Value: "bucket", Kind: common.ValueTypeString}
	da[aws.AttrRegion] = models.ValueType{Value: "region", Kind: common.ValueTypeString}
	da[aws.AttrAccessKeyID] = models.ValueType{Value: "access-key", Kind: common.ValueTypeString}
	da[aws.AttrSecretAccessKey] = models.ValueType{Value: "secret-access-key", Kind: common.ValueTypeSecret}
	awsCSP.CspDomainAttributes = da
	psd = &ProtectionStoreDescriptor{}
	psd.InitializeForSnapshotCatalogEntry(awsCSP)
	assert.EqualValues(awsCSP.CspDomainType, psd.CspDomainType)
	assert.Len(psd.CspDomainAttributes, 2)
	assert.Equal(models.ValueType{Kind: "STRING", Value: "bucket"}, psd.CspDomainAttributes[aws.AttrPStoreBucketName])
	assert.Equal(models.ValueType{Kind: "STRING", Value: "region"}, psd.CspDomainAttributes[aws.AttrRegion])
}

func TestSnapshotCatalogEntryValidate(t *testing.T) {
	assert := assert.New(t)

	psd := ProtectionStoreDescriptor{
		CspDomainType: aws.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
		},
	}

	t.Log("case: SCEValidate success")
	sce := &SnapshotCatalogEntry{
		SnapIdentifier:       "snapId",
		SnapTime:             time.Now(),
		SizeBytes:            10,
		AccountID:            "accountID",
		AccountName:          "accountName",
		VolumeSeriesID:       "vsID",
		VolumeSeriesName:     "vsName",
		ProtectionDomainID:   "pdID",
		ProtectionDomainName: "pdName",
		EncryptionAlgorithm:  "NONE",
		ProtectionStores:     []ProtectionStoreDescriptor{psd},
		ConsistencyGroupID:   "cgID",
		ConsistencyGroupName: "cgName",
	}
	assert.True(sce.Validate())

	// SEC failure cases (inverse order)
	tcs := []string{
		"psd-entry",
		"ConsistencyGroupName", "ConsistencyGroupID",
		"ProtectionStores", "EncryptionAlgorithm",
		"ProtectionDomainName", "ProtectionDomainID",
		"VolumeSeriesName", "VolumeSeriesID",
		"AccountName", "AccountID",
		"SizeBytes", "SnapTime", "SnapIdentifier",
	}
	for _, f := range tcs {
		switch f {
		case "psd-entry":
			sce.ProtectionStores[0].CspDomainType = ""
			assert.False(sce.ProtectionStores[0].ValidateForSnapshotCatalogEntry())
		case "ConsistencyGroupName":
			sce.ConsistencyGroupName = ""
		case "ConsistencyGroupID":
			sce.ConsistencyGroupID = ""
		case "ProtectionStores":
			sce.ProtectionStores = nil
		case "EncryptionAlgorithm":
			sce.EncryptionAlgorithm = ""
		case "ProtectionDomainName":
			sce.ProtectionDomainName = ""
		case "ProtectionDomainID":
			sce.ProtectionDomainID = ""
		case "VolumeSeriesName":
			sce.VolumeSeriesName = ""
		case "VolumeSeriesID":
			sce.VolumeSeriesID = ""
		case "AccountName":
			sce.AccountName = ""
		case "AccountID":
			sce.AccountID = ""
		case "SizeBytes":
			sce.SizeBytes = -1
		case "SnapTime":
			sce.SnapTime = time.Time{}
		case "SnapIdentifier":
			sce.SnapIdentifier = ""
		}
		assert.False(sce.Validate(), "case: SCE failure", f)
	}
}

func sceSourceData() (*SnapshotCatalogEntrySourceObjects, *SnapshotCatalogEntry) {
	now := time.Now()
	mNow := strfmt.DateTime(now)
	expSCE := &SnapshotCatalogEntry{
		SnapIdentifier:       "snapIdentifier",
		SnapTime:             time.Time(mNow), // loses some precision
		SizeBytes:            1000,
		AccountID:            "accountId",
		AccountName:          "accountName",
		TenantAccountName:    "tenantAccountName",
		VolumeSeriesID:       "vsId",
		VolumeSeriesName:     "vsName",
		ProtectionDomainID:   "pdId",
		ProtectionDomainName: "pdName",
		EncryptionAlgorithm:  "ea",
		ProtectionStores: []ProtectionStoreDescriptor{
			ProtectionStoreDescriptor{
				CspDomainType: aws.CSPDomainType,
				CspDomainAttributes: map[string]models.ValueType{
					aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bucket"},
					aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "region"},
				},
			},
		},
		ConsistencyGroupID:   "cgId",
		ConsistencyGroupName: "cgName",
		VolumeSeriesTags:     []string{"vsTag1", "vsTag2"},
		SnapshotTags:         []string{"snapTag1"},
	}

	snapObj := &models.Snapshot{}
	snapObj.Meta = &models.ObjMeta{ID: "snapId"}
	snapObj.SnapIdentifier = "snapIdentifier"
	snapObj.SnapTime = mNow
	snapObj.SizeBytes = 1000
	snapObj.Tags = []string{"snapTag1"}

	aObj := &models.Account{}
	aObj.Meta = &models.ObjMeta{ID: "accountId"}
	aObj.Name = "accountName"

	taObj := &models.Account{}
	taObj.Meta = &models.ObjMeta{ID: "tenantAccountId"}
	taObj.Name = "tenantAccountName"

	vsObj := &models.VolumeSeries{}
	vsObj.Meta = &models.ObjMeta{ID: "vsId"}
	vsObj.Name = "vsName"
	vsObj.Tags = []string{"vsTag1", "vsTag2"}

	pdObj := &models.ProtectionDomain{}
	pdObj.Meta = &models.ObjMeta{ID: "pdId"}
	pdObj.Name = "pdName"
	pdObj.EncryptionAlgorithm = "ea"
	pdObj.EncryptionPassphrase = &models.ValueType{Kind: common.ValueTypeSecret, Value: "secret"}

	// Any domain type will do here
	cspObj := &models.CSPDomain{}
	cspObj.Meta = &models.ObjMeta{ID: "domId"}
	cspObj.CspDomainType = models.CspDomainTypeMutable(aws.CSPDomainType)
	da := make(map[string]models.ValueType)
	da[aws.AttrPStoreBucketName] = models.ValueType{Value: "bucket", Kind: common.ValueTypeString}
	da[aws.AttrRegion] = models.ValueType{Value: "region", Kind: common.ValueTypeString}
	da[aws.AttrAccessKeyID] = models.ValueType{Value: "access-key", Kind: common.ValueTypeString}
	da[aws.AttrSecretAccessKey] = models.ValueType{Value: "secret-access-key", Kind: common.ValueTypeSecret}
	cspObj.CspDomainAttributes = da

	cgObj := &models.ConsistencyGroup{}
	cgObj.Meta = &models.ObjMeta{ID: "cgId"}
	cgObj.Name = "cgName"

	sceSrc := &SnapshotCatalogEntrySourceObjects{
		Snapshot:         snapObj,
		Account:          aObj,
		TenantAccount:    taObj,
		VolumeSeries:     vsObj,
		ProtectionDomain: pdObj,
		ProtectionStores: []*models.CSPDomain{cspObj},
		ConsistencyGroup: cgObj,
	}
	return sceSrc, expSCE
}

func TestSnapshotCatalogEntryInitialize(t *testing.T) {
	assert := assert.New(t)

	sceSrc, expSCE := sceSourceData()
	sce := &SnapshotCatalogEntry{}
	sce.Initialize(sceSrc)
	assert.Equal(expSCE, sce)
}

type fakeInternalOps struct {
	InRRpath string
	InRRres  interface{}
	RetRRerr error

	InSCid   string
	InSCargs *endpoint.CopyArgs
	InSCres  interface{}
	RetSCerr error
}

func (c *fakeInternalOps) ReadResult(resFileName string, res interface{}) error {
	c.InRRpath = resFileName
	c.InRRres = res
	return c.RetRRerr
}
func (c *fakeInternalOps) SpawnCopy(ctx context.Context, ID string, args *endpoint.CopyArgs, res interface{}) error {
	c.InSCid = ID
	c.InSCargs = args
	c.InSCres = res
	if c.RetSCerr != nil {
		switch v := res.(type) {
		case *SnapshotBackupResult:
			v.Error.Description = c.RetSCerr.Error()
		case *SnapshotRestoreResult:
			v.Error.Description = c.RetSCerr.Error()
		}
	}
	return c.RetSCerr
}
