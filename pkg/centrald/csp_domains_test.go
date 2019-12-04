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


package centrald

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	_ "github.com/Nuvoloso/kontroller/pkg/csp/aws"   // for TestCspStorageTypes
	_ "github.com/Nuvoloso/kontroller/pkg/csp/azure" // for TestCspStorageTypes
	_ "github.com/Nuvoloso/kontroller/pkg/csp/gc"    // for TestCspStorageTypes
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestCSPDomainType(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	// Domain type tests
	cspDomainTypes := app.SupportedCspDomainTypes()
	awsDom := models.CspDomainTypeMutable("AWS")
	assert.Contains(cspDomainTypes, awsDom)
	ok := app.ValidateCspDomainType("foobar-bad-type")
	assert.False(ok)

	// Insert a fake domain type for the UT
	dt := models.CspDomainTypeMutable("--UT-FAKE-DT--")
	assert.Nil(app.CSP(dt)) // side-effect is to make the cache map if necessary

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP
	fakeDomAttrMD := map[string]models.AttributeDescriptor{
		"required_string":  {Kind: "STRING"},
		"required_int":     {Kind: "INT"},
		"optional_string":  {Kind: "STRING", Optional: true},
		"optional_int":     {Kind: "INT", Optional: true},
		"immutable_string": {Kind: "STRING", Immutable: true},
	}
	cSP.EXPECT().DomainAttributes().Return(fakeDomAttrMD).Times(2)

	// build attrs from the desc
	dad := app.DomainAttrDescForType(dt)
	assert.NotEmpty(dad)

	t.Log("case: Success")
	attrs := make(map[string]models.ValueType, len(dad))
	numRequired := 0
	var optAttr string // any
	for da, desc := range dad {
		if !desc.Optional {
			numRequired++
		} else {
			optAttr = da
			continue
		}
		vt := models.ValueType{Kind: desc.Kind}
		switch desc.Kind {
		case "INT":
			vt.Value = "5"
		case "STRING":
			vt.Value = "string"
		default:
			assert.True(false)
		}
		attrs[da] = vt
	}
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CSP-DOMAIN-1",
				Version: 1,
			},
			CspDomainType:       dt,
			CspDomainAttributes: attrs,
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "MyCSPDomain",
		},
	}
	cspCredAttrs := map[string]models.ValueType{
		"k1": models.ValueType{Kind: "STRING", Value: "v1"},
		"k2": models.ValueType{Kind: "SECRET", Value: "v2"},
	}
	// let SanitizedAttributes modify one of the optional attributes
	sanAttrs := map[string]models.ValueType{
		"immutable_string": models.ValueType{Kind: "STRING", Value: "string"},
		"k1":               models.ValueType{Kind: "STRING", Value: "v1"},
		"k2":               models.ValueType{Kind: "SECRET", Value: "v2"},
		"required_int":     models.ValueType{Kind: "INT", Value: "5"},
		"required_string":  models.ValueType{Kind: "STRING", Value: "string"},
		"optional_int":     models.ValueType{Kind: "INT", Value: "111"},
	}
	assert.NotEqual(dObj.CspDomainAttributes["optional_int"].Value, sanAttrs["optional_int"].Value)
	domObj := &models.CSPDomain{}
	*domObj = *dObj
	domObj.CspDomainAttributes = sanAttrs
	ctx := context.Background()
	mockCtx := gomock.Any()
	cSP.EXPECT().SanitizedAttributes(gomock.Any()).Return(sanAttrs, nil)
	cDC := mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(domObj).Return(cDC, nil)
	cDC.EXPECT().SetTimeout(0)
	cDC.EXPECT().Validate(mockCtx).Return(nil)
	cDC.EXPECT().CreateProtectionStore(mockCtx).Return(attrs, nil)
	t.Log(attrs)
	assert.NoError(app.ValidateAndInitCspDomain(ctx, dObj, cspCredAttrs))
	assert.Equal(dObj.CspDomainAttributes["optional_int"].Value, sanAttrs["optional_int"].Value)
	tl.Flush()

	// Sanitize fails
	t.Log("case: Sanitize fails")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	sanitizeError := fmt.Errorf("unsanitary")
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP
	app.cspClientCache = nil
	t.Log(attrs)
	t.Log(dObj.CspDomainAttributes)
	cSP.EXPECT().DomainAttributes().Return(fakeDomAttrMD).MinTimes(1)
	cSP.EXPECT().SanitizedAttributes(dObj.CspDomainAttributes).Return(nil, sanitizeError)
	assert.Error(app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{}))
	tl.Flush()

	// DomainClient fails
	t.Log("case: DomainClient fails")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP
	app.cspClientCache = nil
	t.Log(attrs)
	cSP.EXPECT().DomainAttributes().Return(fakeDomAttrMD).MinTimes(1)
	cSP.EXPECT().SanitizedAttributes(dObj.CspDomainAttributes).Return(attrs, nil)
	cSP.EXPECT().Client(dObj).Return(nil, fmt.Errorf("Client fail"))
	assert.Error(app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{}))
	tl.Flush()

	// Validate fails
	t.Log("case: Validate fails")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP
	app.cspClientCache = nil
	cSP.EXPECT().DomainAttributes().Return(fakeDomAttrMD).MinTimes(1)
	cSP.EXPECT().SanitizedAttributes(dObj.CspDomainAttributes).Return(attrs, nil)
	cDC = mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(cDC, nil)
	cDC.EXPECT().SetTimeout(0)
	cDC.EXPECT().Validate(mockCtx).Return(fmt.Errorf("Validate fail"))
	assert.Error(app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{}))
	tl.Flush()

	// Create Persistent Store fails
	t.Log("case: Create Persistent Store fails")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP
	app.cspClientCache = nil
	cSP.EXPECT().DomainAttributes().Return(fakeDomAttrMD).MinTimes(1)
	cSP.EXPECT().SanitizedAttributes(dObj.CspDomainAttributes).Return(attrs, nil)
	cDC = mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(cDC, nil)
	cDC.EXPECT().SetTimeout(0)
	cDC.EXPECT().Validate(mockCtx).Return(nil)
	cDC.EXPECT().CreateProtectionStore(mockCtx).Return(nil, fmt.Errorf("p store fail"))
	assert.Error(app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{}))
	tl.Flush()

	// optional attr not present
	t.Log("case: optional attr not present")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP
	app.cspClientCache = nil
	cSP.EXPECT().DomainAttributes().Return(fakeDomAttrMD).MinTimes(1)
	cSP.EXPECT().SanitizedAttributes(dObj.CspDomainAttributes).Return(attrs, nil)
	cDC = mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(cDC, nil)
	cDC.EXPECT().SetTimeout(0)
	cDC.EXPECT().Validate(mockCtx).Return(nil)
	cDC.EXPECT().CreateProtectionStore(mockCtx).Return(attrs, nil)
	delete(attrs, optAttr)
	t.Log(attrs)
	assert.NoError(app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{}))
	tl.Flush()

	// no attrs when some are required
	t.Log("case: no attrs when some are required")
	assert.True(numRequired > 0)
	dObj.CspDomainAttributes = nil
	err := app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{})
	assert.Error(err)
	assert.Regexp("is required", err)
	dObj.CspDomainAttributes = make(map[string]models.ValueType, 0)
	err = app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{})
	assert.Error(err)
	assert.Regexp("is required", err)
	tl.Flush()

	// invalid type
	t.Log("case: invalid type")
	dObj.CspDomainType = dt + "BAD"
	err = app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{})
	assert.Error(err)
	assert.Regexp("supported cspDomainType values", err)
	tl.Flush()

	// unexpected attribute
	t.Log("case: unexpected attribute")
	aName := "unexpected attribute"
	attrs[aName] = models.ValueType{Kind: "STRING", Value: "invalid"}
	t.Log(attrs)
	dObj.CspDomainType = dt
	dObj.CspDomainAttributes = attrs
	err = app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{})
	assert.Error(err)
	assert.Regexp("invalid .* attribute "+aName, err)
	delete(attrs, aName)
	tl.Flush()

	// change kind
	t.Log("case: change kind")
	for da, desc := range dad {
		vt := attrs[da] // or panic
		switch desc.Kind {
		case "STRING":
			vt.Kind = "INT"
		case "INT":
			vt.Kind = "STRING"
		default:
			assert.True(false)
		}
		attrs[da] = vt
		break
	}
	t.Log(attrs)
	err = app.ValidateAndInitCspDomain(ctx, dObj, map[string]models.ValueType{})
	assert.Error(err)
	assert.Regexp("Kind should be", err)
	tl.Flush()

	// value type check during validate
	t.Log("case: value type check during validate")
	attrs = make(map[string]models.ValueType, len(dad))
	for da, desc := range dad {
		vt := models.ValueType{Kind: desc.Kind}
		switch desc.Kind {
		case "INT":
			vt.Value = "not an int"
		case "STRING":
			vt.Value = "string"
		default:
			assert.True(false)
		}
		attrs[da] = vt
	}
	t.Log(attrs)
	dObj.CspDomainAttributes = attrs
	err = app.ValidateAndInitCspDomain(ctx, dObj, nil)
	assert.Error(err)
	assert.Regexp(" expected .* but got", err)
	tl.Flush()

	// lookup a real csp domain
	t.Log("case: lookup a real csp domain and credential")
	realCSP := app.CSP(awsDom)
	assert.NotNil(realCSP)
	realAttrs := realCSP.DomainAttributes()
	ctxAttrs := app.DomainAttrDescForType(awsDom)
	assert.Equal(realAttrs, ctxAttrs)

	// immutable attribute is modified
	t.Log("case: immutable attribute is modified")
	oldAttr := map[string]models.ValueType{
		"aws_region":                       models.ValueType{Kind: "STRING", Value: "old_region"}, // immutable
		"aws_availability_zone":            models.ValueType{Kind: "STRING", Value: "zone"},
		"aws_protection_store_bucket_name": models.ValueType{Kind: "STRING", Value: "ps"},
	}
	newAttr := map[string]models.ValueType{
		"aws_region":                       models.ValueType{Kind: "STRING", Value: "new_region"}, // immutable
		"aws_availability_zone":            models.ValueType{Kind: "STRING", Value: "zone"},
		"aws_protection_store_bucket_name": models.ValueType{Kind: "STRING", Value: "ps"},
	}
	ctxAttrs = app.DomainAttrDescForType(awsDom)
	assert.Error(app.validateBasicAttributes(awsDom, ctxAttrs, oldAttr, newAttr))
	tl.Flush()

	assert.Panics(func() { app.DomainAttrDescForType("foobar-bad-type") })
	assert.Panics(func() { app.CredentialAttrDescForType("foobar-bad-type") })
}

func TestCspStorageTypes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	types := app.SupportedCspStorageTypes()
	assert.NotNil(types)
	assert.NotEmpty(types)

	names := map[models.CspStorageType]struct{}{}
	// this test needs to be updated each time an new CSP type is added
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.Len(cspDomainTypes, 3)
	for _, t := range types {
		assert.Contains(cspDomainTypes, models.CspDomainTypeMutable(t.CspDomainType))
		assert.True(app.ValidateCspDomainType(models.CspDomainTypeMutable(t.CspDomainType)))
		ret := app.GetCspStorageType(t.Name)
		assert.NotNil(ret)
		assert.Equal(*ret, *t)
		assert.Nil(app.GetCspStorageType(t.Name + "x"))

		assert.True(swag.Int64Value(t.MinAllocationSizeBytes) <= swag.Int64Value(t.MaxAllocationSizeBytes))
		assert.NotZero(swag.Int64Value(t.MaxAllocationSizeBytes))

		// all names must be unique
		_, exists := names[t.Name]
		assert.False(exists, "not unique: "+string(t.Name))
		names[t.Name] = struct{}{}
	}
	assert.Nil(app.GetCspStorageType(""))
}

func TestValidateCspDomainAndStorageTypes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := models.CspDomainTypeMutable("AWS")
	assert.Contains(cspDomainTypes, dt)

	assert.Nil(app.cspSTCache)
	cspStorageTypes := app.SupportedCspStorageTypes()
	assert.NotNil(cspStorageTypes)
	assert.NotNil(app.cspSTCache)
	assert.NotEmpty(cspStorageTypes)
	assert.Equal(app.cspSTCache, cspStorageTypes)
	var st models.CspStorageType
	for _, t := range cspStorageTypes {
		if t.CspDomainType == models.CspDomainType(dt) {
			st = t.Name
			break
		}
	}
	assert.NotEmpty(st)
	assert.NoError(app.ValidateCspDomainAndStorageTypes(dt, st))
	assert.Error(app.ValidateCspDomainAndStorageTypes("cloud", st))
	assert.Error(app.ValidateCspDomainAndStorageTypes(dt, st+"xxx"))
	// Insert a fake domain type for the UT
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	dtF := models.CspDomainTypeMutable("--UT-FAKE-DT--")
	assert.NotNil(app.cspCache)
	app.cspCache[dtF] = cSP
	assert.Error(app.ValidateCspDomainAndStorageTypes(dtF, st))
}

func TestGetMCDeployment(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{
		DS:  ds,
		Log: tl.Logger(),
		Server: &restapi.Server{
			TLSCACertificate: "./csp_domains_test.go", // he, he
		},
		AgentdCertPath:   "./csp_domains_test.go",
		AgentdKeyPath:    "./csp_domains_test.go",
		ClusterdCertPath: "./csp_domains_test.go",
		ClusterdKeyPath:  "./csp_domains_test.go",
		ClusterDeployTag: "test-tag",
		DriverType:       "flex",
		ImagePath:        "image-path",
	})
	assert.NotEmpty(app.ImagePath)

	data, err := ioutil.ReadFile(string(app.AgentdCertPath))
	assert.Nil(err)
	b64data := base64.StdEncoding.EncodeToString(data)

	dt := models.CspDomainTypeMutable("--UT-FAKE-DT--")
	mcArgs := &cluster.MCDeploymentArgs{
		CSPDomainID:    "CSP-DOMAIN-1",
		CSPDomainType:  string(dt),
		ManagementHost: "management.host",
		CACert:         b64data,
		AgentdCert:     b64data,
		AgentdKey:      b64data,
		ClusterdCert:   b64data,
		ClusterdKey:    b64data,
		DriverType:     app.DriverType,
		ImageTag:       app.ClusterDeployTag,
		ImagePath:      app.ImagePath,
	}
	args := &cluster.MCDeploymentArgs{}
	args.CSPDomainID = mcArgs.CSPDomainID
	args.CSPDomainType = mcArgs.CSPDomainType
	args.ManagementHost = mcArgs.ManagementHost

	// error case: invalid domain
	dep, err := app.GetMCDeployment(args)
	assert.NotNil(err)
	assert.Nil(dep)
	assert.Regexp("invalid cspDomainType", err)

	// success case (internal)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP // insert the fake domain type for the UT
	mMCD := mockcluster.NewMockMCDeployer(mockCtrl)
	app.MCDeployer = mMCD
	retDep := &cluster.Deployment{
		Format:     "yaml",
		Deployment: "dep",
	}
	mMCD.EXPECT().GetMCDeployment(mcArgs).Return(retDep, nil)
	dep, err = app.GetMCDeployment(args)
	assert.Nil(err)
	assert.NotNil(dep)
	assert.Equal(retDep, dep)
	mockCtrl.Finish()

	// success case (external)
	app.ImagePullSecretPath = "./csp_domains_test.go"
	mcArgs.ImagePullSecret = b64data
	mcArgs.ImagePath = app.ImagePath
	args = &cluster.MCDeploymentArgs{}
	args.CSPDomainID = mcArgs.CSPDomainID
	args.CSPDomainType = mcArgs.CSPDomainType
	args.ManagementHost = mcArgs.ManagementHost

	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.cspCache[dt] = cSP // insert the fake domain type for the UT
	mMCD = mockcluster.NewMockMCDeployer(mockCtrl)
	app.MCDeployer = mMCD
	retDep = &cluster.Deployment{
		Format:     "yaml",
		Deployment: "dep",
	}
	mMCD.EXPECT().GetMCDeployment(mcArgs).Return(retDep, nil)
	dep, err = app.GetMCDeployment(args)
	assert.Nil(err)
	assert.NotNil(dep)
	assert.Equal(retDep, dep)

	// error cases: cert file not specified
	for i := 0; i <= 4; i++ {
		var sp *flags.Filename
		switch i {
		case 0:
			sp = &app.Server.TLSCACertificate
		case 1:
			sp = &app.AgentdCertPath
		case 2:
			sp = &app.AgentdKeyPath
		case 3:
			sp = &app.ClusterdCertPath
		case 4:
			sp = &app.ClusterdKeyPath
		}
		s := *sp
		*sp = ""
		dep, err = app.GetMCDeployment(args)
		assert.NotNil(err)
		assert.Regexp("file not specified", err)
		assert.Nil(dep)
		*sp = s
	}

	// error case: invalid file
	s := app.AgentdCertPath
	app.AgentdCertPath = "./foobar"
	args.ImageTag = "override" // verify that ImageTag overrides app.ClusterDeployTag when specified
	dep, err = app.GetMCDeployment(args)
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err)
	assert.Nil(dep)
	assert.Equal("override", args.ImageTag)
	app.AgentdCertPath = s
}

func TestGetDomainClient(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger(), ClientTimeout: 99})

	// Insert a fake provider for the UT
	dt := models.CspDomainTypeMutable("--UT-FAKE-DT--")
	assert.Nil(app.CSP(dt)) // side-effect is to make the cache map if necessary

	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CSP-DOMAIN-1",
				Version: 1,
			},
			CspDomainType: dt,
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "MyCSPDomain",
		},
	}
	dObj.CspDomainAttributes = map[string]models.ValueType{
		"ca1": models.ValueType{Kind: "STRING", Value: "old_v1"},
		"ca2": models.ValueType{Kind: "SECRET", Value: "old_v2"},
	}

	// success case, not in cache
	dcr, ok := app.cspClientCache[dObj.Meta.ID]
	assert.False(ok)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cDC := mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(cDC, nil)
	app.cspCache[dt] = cSP
	cDC.EXPECT().SetTimeout(99)
	cl, err := app.GetDomainClient(dObj)
	assert.Nil(err)
	assert.NotNil(cl)
	dcr, ok = app.cspClientCache[dObj.Meta.ID]
	assert.True(ok)
	assert.Equal(dObj.Meta.Version, dcr.version)
	assert.Equal(cDC, dcr.client)
	assert.Equal(cDC, cl)

	// success case, cache hit
	cl, err = app.GetDomainClient(dObj)
	assert.Nil(err)
	assert.NotNil(cl)
	dcr, ok = app.cspClientCache[dObj.Meta.ID]
	assert.True(ok)
	assert.Equal(dObj.Meta.Version, dcr.version)
	assert.Equal(cDC, dcr.client)
	assert.Equal(cDC, cl)

	// Change the version - should re-cache
	prevVersion := dObj.Meta.Version
	dObj.Meta.Version++
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cDC = mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(cDC, nil)
	app.cspCache[dt] = cSP
	cDC.EXPECT().SetTimeout(99)
	cl, err = app.GetDomainClient(dObj)
	assert.Nil(err)
	assert.NotNil(cl)
	dcr, ok = app.cspClientCache[dObj.Meta.ID]
	assert.True(ok)
	assert.Equal(dObj.Meta.Version, dcr.version)
	assert.NotEqual(prevVersion, dcr.version)
	assert.Equal(cDC, dcr.client)
	assert.Equal(cDC, cl)

	// Change the attributes - should re-cache
	dObj.Meta.Version = prevVersion // restore
	dObj.CspDomainAttributes = map[string]models.ValueType{
		"ca1": models.ValueType{Kind: "STRING", Value: "new_v1"},
		"ca2": models.ValueType{Kind: "SECRET", Value: "new_v2"},
	}
	prevAttrValSHA := dcr.attrValSHA
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cDC = mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(cDC, nil)
	app.cspCache[dt] = cSP
	cDC.EXPECT().SetTimeout(99)
	cl, err = app.GetDomainClient(dObj)
	assert.Nil(err)
	assert.NotNil(cl)
	dcr, ok = app.cspClientCache[dObj.Meta.ID]
	assert.True(ok)
	assert.Equal(cDC, dcr.client)
	assert.Equal(cDC, cl)
	assert.NotEqual(prevAttrValSHA, dcr.attrValSHA)

	// No meta-data: no caching
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	meta := dObj.Meta
	oLen := len(app.cspClientCache)
	dObj.Meta = nil
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cDC = mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(cDC, nil)
	app.cspCache[dt] = cSP
	cDC.EXPECT().SetTimeout(99)
	cl, err = app.GetDomainClient(dObj)
	assert.Nil(err)
	assert.NotNil(cl)
	assert.Equal(oLen, len(app.cspClientCache))
	dObj.Meta = meta

	// Error when creating the client
	dObj.Meta.Version++
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().Client(dObj).Return(nil, fmt.Errorf("client error"))
	app.cspCache[dt] = cSP
	cl, err = app.GetDomainClient(dObj)
	assert.Error(err)
	assert.Regexp("client error", err)
	assert.Nil(cl)

	// Error with invalid domain type
	dObj.CspDomainType = "FAKE DOMAIN TYPE"
	cl, err = app.GetDomainClient(dObj)
	assert.Error(err)
	assert.Regexp("invalid CSP domain type", err)
	assert.Nil(cl)
}

func TestDomainServicePlanCost(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := AppInit(&AppArgs{Log: tl.Logger()})

	fSFC := &fakeSFC{}
	app.SFC = fSFC

	// Insert a fake provider for the UT
	dt := models.CspDomainTypeMutable("--UT-FAKE-DT--")
	assert.Nil(app.CSP(dt)) // side-effect is to make the cache map if necessary

	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CSP-DOMAIN-1",
				Version: 1,
			},
			CspDomainType: dt,
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "MyCSPDomain",
		},
	}
	spObj := &models.ServicePlan{}

	t.Log("case: fail on StorageFormulasForServicePlan")
	fSFC.RetSFfSPErr = fmt.Errorf("sfsp-error")
	spc, err := app.DomainServicePlanCost(nil, dObj, spObj)
	assert.Error(err)
	assert.Regexp("sfsp-error", err)
	assert.Nil(spc)
	assert.EqualValues("", fSFC.InSFfSPid)
	assert.Equal(spObj, fSFC.InSFfSPobj)

	fSFC.RetSFfSPErr = nil
	fSFC.RetSFfSPObj = []*models.StorageFormula{}

	t.Log("case: fail on SelectFormulaForDomain")
	fSFC.RetSFfDErr = fmt.Errorf("sffd-error")
	spc, err = app.DomainServicePlanCost(nil, dObj, spObj)
	assert.Error(err)
	assert.Regexp("sffd-error", err)
	assert.Nil(spc)
	assert.Equal(dObj, fSFC.InSFfDo)
	assert.Equal(fSFC.RetSFfSPObj, fSFC.InSFfDf)

	fSFC.RetSFfDErr = nil
	fSFC.RetSFfDf = &models.StorageFormula{}

	t.Log("case: no storage costs in domain")
	dObj.StorageCosts = nil
	fSFC.RetSFCC = &models.ServicePlanCost{}
	fSFC.InSFCCf = nil
	fSFC.InSFCCc = nil
	spc, err = app.DomainServicePlanCost(nil, dObj, spObj)
	assert.NoError(err)
	assert.NotNil(spc)
	assert.NotNil(fSFC.InSFCCc)
	assert.Equal(fSFC.RetSFfDf, fSFC.InSFCCf)
	assert.Equal(fSFC.RetSFCC, spc)

	t.Log("case: storage costs in domain")
	dObj.StorageCosts = map[string]models.StorageCost{}
	fSFC.InSFCCc = nil
	spc, err = app.DomainServicePlanCost(nil, dObj, spObj)
	assert.NoError(err)
	assert.NotNil(spc)
	assert.EqualValues(dObj.StorageCosts, fSFC.InSFCCc)
}

func TestValidateCspCredential(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger(), ClientTimeout: 99})

	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := models.CspDomainTypeMutable("AWS")
	assert.Contains(cspDomainTypes, dt)

	expCredsAttrs := map[string]models.AttributeDescriptor{
		"aws_secret_access_key": models.AttributeDescriptor{
			Description: "An AWS account secret key", Immutable: false, Kind: "SECRET", Optional: false,
		},
		"aws_access_key_id": models.AttributeDescriptor{
			Description: "An AWS account access key", Immutable: false, Kind: "STRING", Optional: false,
		},
	}
	ctxAttrs := app.CredentialAttrDescForType("AWS")
	assert.Equal(expCredsAttrs, ctxAttrs)

	// ValidateCspCredentialAttributes
	// all required attributes are present
	attr1 := map[string]models.ValueType{
		"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "key"},
		"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
	}
	assert.NoError(app.validateBasicAttributes(dt, ctxAttrs, nil, attr1))
	assert.NoError(app.ValidateCspCredentialAttributes(dt, nil, attr1))

	// required attribute is missing
	attr2 := map[string]models.ValueType{
		"aws_availability_zone": models.ValueType{Kind: "STRING", Value: "zone"},
		"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
	}
	assert.Error(app.validateBasicAttributes(dt, ctxAttrs, nil, attr2))
	assert.Error(app.ValidateCspCredentialAttributes(dt, nil, attr2))

	// unnecessary attributes are present
	attr3 := map[string]models.ValueType{
		"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "key"},
		"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
		"aws_availability_zone": models.ValueType{Kind: "STRING", Value: "zone"},
	}
	assert.Error(app.validateBasicAttributes(dt, ctxAttrs, nil, attr3))
	assert.Error(app.ValidateCspCredentialAttributes(dt, nil, attr3))

	credsAttributes := map[string]models.AttributeDescriptor{
		"aws_access_key_id":     models.AttributeDescriptor{Kind: "STRING", Description: "An AWS account access key"},
		"aws_secret_access_key": models.AttributeDescriptor{Kind: "SECRET", Description: "An AWS account secret key"},
	}

	// ValidateCspCredential
	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().CredentialAttributes().Return(credsAttributes).Times(1)
	cSP.EXPECT().ValidateCredential(models.CspDomainTypeMutable(dt), attr1).Return(nil)
	app.cspCache[dt] = cSP
	err := app.ValidateCspCredential(dt, nil, attr1)
	assert.Nil(err)

	// error on credential validation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().CredentialAttributes().Return(credsAttributes).Times(1)
	cSP.EXPECT().ValidateCredential(dt, attr1).Return(fmt.Errorf("invalid credential"))
	app.cspCache[dt] = cSP
	err = app.ValidateCspCredential(dt, nil, attr1)
	assert.Error(err)
	assert.Regexp("invalid credential", err)

	// error on ValidateCspCredentialAttributes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().CredentialAttributes().Return(credsAttributes).Times(1)
	app.cspCache[dt] = cSP
	err = app.ValidateCspCredential(dt, nil, attr2)
	assert.Error(err)
	assert.Regexp("cspDomainType AWS attribute aws_access_key_id \\(STRING\\) is required", err)
}
