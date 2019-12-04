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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestStorageFormulaListCorrectness(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	types := app.SupportedStorageFormulas()

	// validate that there are Storage Formulas
	assert.NotNil(types)
	assert.NotEmpty(types)

	// validate all Storage Formulas have unique names
	names := map[models.StorageFormulaName]struct{}{}
	for _, formula := range types {
		_, exists := names[formula.Name]
		assert.False(exists, "not unique: "+string(formula.Name))
		names[formula.Name] = struct{}{}
	}

	// validate all Storage Formulas add up to 100%, each has at most 400% overhead
	for _, formula := range types {
		var formulaPercent int32
		for name, formulaComponent := range formula.StorageComponent {
			assert.NotNil(app.GetCspStorageType(models.CspStorageType(name)))
			assert.True(*formulaComponent.Percentage >= 0)
			formulaPercent += *formulaComponent.Percentage
			overhead := swag.Int32Value(formulaComponent.Overhead)
			assert.True(overhead >= 0 && overhead <= 400)
		}
		assert.True(formulaPercent == 100)
	}

	// validate all Cache Formulas add up to 100% or less, no overhead
	for _, formula := range types {
		var formulaPercent int32
		for name, formulaComponent := range formula.CacheComponent {
			assert.NotNil(app.GetCspStorageType(models.CspStorageType(name)))
			assert.True(*formulaComponent.Percentage >= 0)
			formulaPercent += *formulaComponent.Percentage
			assert.Zero(swag.Int32Value(formulaComponent.Overhead))
		}
		assert.True(formulaPercent <= 100)
	}

	// validate types in ValidateCacheFormula
	assert.False(app.ValidateStorageFormula(""))
	for _, formula := range types {
		assert.True(formula.Name != "")
		assert.True(app.ValidateStorageFormula(formula.Name))
		assert.False(app.ValidateStorageFormula(formula.Name + "x"))
	}

	// validate IoProfile values are set
	for _, formula := range types {
		assert.True(formula.IoProfile.IoPattern.Name != "" && formula.IoProfile.ReadWriteMix.Name != "")
		assert.True(*formula.IoProfile.IoPattern.MinSizeBytesAvg >= 0 && *formula.IoProfile.IoPattern.MaxSizeBytesAvg > 0)
		assert.True(*formula.IoProfile.ReadWriteMix.MinReadPercent >= 0 && *formula.IoProfile.ReadWriteMix.MaxReadPercent <= 100)
	}
}

func TestStorageFormulaSelectForServicePlan(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})
	servicePlanID := models.ObjIDMutable("servicePlanID")
	params := service_plan.ServicePlanFetchParams{ID: string(servicePlanID)}
	types := app.SupportedStorageFormulas()

	// validate that we can return a formula properly
	// Create a service plan's SLOs to match a storage formula
	sp := &models.ServicePlan{}
	sp.IoProfile = types[1].IoProfile
	sp.Slos = models.SloListMutable{
		"Response Time Average": {
			ValueType:                 models.ValueType{Kind: types[1].SscList["Response Time Average"].Kind, Value: types[1].SscList["Response Time Average"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Response Time Maximum": {
			ValueType:                 models.ValueType{Kind: types[1].SscList["Response Time Maximum"].Kind, Value: types[1].SscList["Response Time Maximum"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Availability": {
			ValueType:                 models.ValueType{Kind: types[1].SscList["Availability"].Kind, Value: types[1].SscList["Availability"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := NewMockServicePlanOps(mockCtrl)
	oA.EXPECT().Fetch(nil, params.ID).Return(sp, nil)
	mds := NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oA)
	t.Log("case: succeed")
	app.DS = mds
	formulas, err := app.StorageFormulasForServicePlan(nil, servicePlanID, nil)
	assert.NoError(err)
	assert.NotNil(formulas)
	tl.Flush()

	// repeat without fetch
	formulas, err = app.StorageFormulasForServicePlan(nil, "", sp)
	assert.NoError(err)
	assert.NotNil(formulas)
	tl.Flush()

	// handle errors
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oA = NewMockServicePlanOps(mockCtrl)
	oA.EXPECT().Fetch(nil, params.ID).Return(nil, ErrorNotFound)
	mds = NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oA)
	t.Log("case: fail")
	app.DS = mds
	formulas, err = app.StorageFormulasForServicePlan(nil, servicePlanID, nil)
	assert.Error(err)
	assert.Nil(formulas)
	tl.Flush()
}

func TestStorageFormulaSelectByAttr(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})
	types := app.SupportedStorageFormulas()

	// validate that we return no formulas for a ServicePlan with no match - response time average
	t.Log("case: fail, no service plan for the response time average")
	ioProfile := models.IoProfile{
		IoPattern: &models.IoPattern{
			Name:            types[0].IoProfile.IoPattern.Name,
			MinSizeBytesAvg: types[0].IoProfile.IoPattern.MinSizeBytesAvg,
			MaxSizeBytesAvg: types[0].IoProfile.IoPattern.MaxSizeBytesAvg,
		},
		ReadWriteMix: &models.ReadWriteMix{
			Name:           types[0].IoProfile.ReadWriteMix.Name,
			MinReadPercent: types[0].IoProfile.ReadWriteMix.MinReadPercent,
			MaxReadPercent: types[0].IoProfile.ReadWriteMix.MaxReadPercent,
		},
	}

	slos := models.SloListMutable{
		"Response Time Average": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Average"].Kind, Value: "1us"},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Response Time Maximum": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Maximum"].Kind, Value: types[0].SscList["Response Time Maximum"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Availability": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Availability"].Kind, Value: types[0].SscList["Availability"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
	}
	formulas := app.StorageFormulaForAttrs(&ioProfile, slos)
	assert.Nil(formulas)
	tl.Flush()

	// validate that we return no formulas for a ServicePlan with no match - response time maximum
	t.Log("case: fail, no service plan for the response time maximum")
	ioProfile = *types[0].IoProfile
	slos = models.SloListMutable{
		"Response Time Average": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Average"].Kind, Value: types[0].SscList["Response Time Average"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Response Time Maximum": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Maximum"].Kind, Value: "1us"},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Availability": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Availability"].Kind, Value: types[0].SscList["Availability"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
	}
	formulas = app.StorageFormulaForAttrs(&ioProfile, slos)
	assert.Nil(formulas)
	tl.Flush()

	// validate that we return no formulas for a ServicePlan with no match - availability
	t.Log("case: fail, no service plan for the availability")
	ioProfile = models.IoProfile{
		IoPattern: &models.IoPattern{
			Name:            types[0].IoProfile.IoPattern.Name,
			MinSizeBytesAvg: types[0].IoProfile.IoPattern.MinSizeBytesAvg,
			MaxSizeBytesAvg: types[0].IoProfile.IoPattern.MaxSizeBytesAvg,
		},
		ReadWriteMix: &models.ReadWriteMix{
			Name:           types[0].IoProfile.ReadWriteMix.Name,
			MinReadPercent: types[0].IoProfile.ReadWriteMix.MinReadPercent,
			MaxReadPercent: types[0].IoProfile.ReadWriteMix.MaxReadPercent,
		},
	}
	slos = models.SloListMutable{
		"Response Time Average": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Average"].Kind, Value: types[0].SscList["Response Time Average"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Response Time Maximum": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Maximum"].Kind, Value: types[0].SscList["Response Time Maximum"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Availability": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Availability"].Kind, Value: "100%"},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
	}
	formulas = app.StorageFormulaForAttrs(&ioProfile, slos)
	assert.Nil(formulas)
	tl.Flush()

	// validate that we return no formulas for a ServicePlan with no match - Kind Mismatch
	t.Log("case: fail, no service plan for the availability")
	ioProfile = models.IoProfile{
		IoPattern: &models.IoPattern{
			Name:            types[0].IoProfile.IoPattern.Name,
			MinSizeBytesAvg: types[0].IoProfile.IoPattern.MinSizeBytesAvg,
			MaxSizeBytesAvg: types[0].IoProfile.IoPattern.MaxSizeBytesAvg,
		},
		ReadWriteMix: &models.ReadWriteMix{
			Name:           types[0].IoProfile.ReadWriteMix.Name,
			MinReadPercent: types[0].IoProfile.ReadWriteMix.MinReadPercent,
			MaxReadPercent: types[0].IoProfile.ReadWriteMix.MaxReadPercent,
		},
	}
	slos = models.SloListMutable{
		"Response Time Average": {
			ValueType:                 models.ValueType{Kind: "MAGIC", Value: types[0].SscList["Response Time Average"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Response Time Maximum": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Maximum"].Kind, Value: types[0].SscList["Response Time Maximum"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Availability": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Availability"].Kind, Value: types[0].SscList["Availability"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
	}
	formulas = app.StorageFormulaForAttrs(&ioProfile, slos)
	assert.Nil(formulas)
	tl.Flush()

	// validate that we return no formulas for a ServicePlan with no match - IO Profile
	t.Log("case: fail, no service plan for the IO Profile")
	ioProfile = models.IoProfile{
		IoPattern: &models.IoPattern{
			Name:            "Plaid",
			MinSizeBytesAvg: types[0].IoProfile.IoPattern.MinSizeBytesAvg,
			MaxSizeBytesAvg: types[0].IoProfile.IoPattern.MaxSizeBytesAvg,
		},
		ReadWriteMix: &models.ReadWriteMix{
			Name:           "VideoGames",
			MinReadPercent: types[0].IoProfile.ReadWriteMix.MinReadPercent,
			MaxReadPercent: types[0].IoProfile.ReadWriteMix.MaxReadPercent,
		},
	}
	slos = models.SloListMutable{
		"Response Time Average": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Average"].Kind, Value: types[0].SscList["Response Time Average"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Response Time Maximum": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Response Time Maximum"].Kind, Value: types[0].SscList["Response Time Maximum"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Availability": {
			ValueType:                 models.ValueType{Kind: types[0].SscList["Availability"].Kind, Value: types[0].SscList["Availability"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
	}
	formulas = app.StorageFormulaForAttrs(&ioProfile, slos)
	assert.Nil(formulas)
	tl.Flush()

	// validate that we return formulas for a matching ServicePlan
	t.Log("case: succeed, find service plan for the IO Profile")
	ioProfile = models.IoProfile{
		IoPattern: &models.IoPattern{
			Name:            types[1].IoProfile.IoPattern.Name,
			MinSizeBytesAvg: types[1].IoProfile.IoPattern.MinSizeBytesAvg,
			MaxSizeBytesAvg: types[1].IoProfile.IoPattern.MaxSizeBytesAvg,
		},
		ReadWriteMix: &models.ReadWriteMix{
			Name:           types[1].IoProfile.ReadWriteMix.Name,
			MinReadPercent: types[1].IoProfile.ReadWriteMix.MinReadPercent,
			MaxReadPercent: types[1].IoProfile.ReadWriteMix.MaxReadPercent,
		},
	}
	slos = models.SloListMutable{
		"Response Time Average": {
			ValueType:                 models.ValueType{Kind: types[1].SscList["Response Time Average"].Kind, Value: types[1].SscList["Response Time Average"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Response Time Maximum": {
			ValueType:                 models.ValueType{Kind: types[1].SscList["Response Time Maximum"].Kind, Value: types[1].SscList["Response Time Maximum"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
		"Availability": {
			ValueType:                 models.ValueType{Kind: types[1].SscList["Availability"].Kind, Value: types[1].SscList["Availability"].Value},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
		},
	}
	formulas = app.StorageFormulaForAttrs(&ioProfile, slos)
	assert.NotNil(formulas)
	tl.Flush()
	ioP2 := cloneIoProfile(&ioProfile)
	formulas = app.StorageFormulaForAttrs(ioP2, slos)
	assert.NotNil(formulas)
	tl.Flush()
}

func cloneIoProfile(s *models.IoProfile) *models.IoProfile {
	var iop models.IoPattern
	testutils.Clone(s.IoPattern, &iop)
	var rwm models.ReadWriteMix
	testutils.Clone(s.ReadWriteMix, &rwm)
	n := &models.IoProfile{
		IoPattern:    &iop,
		ReadWriteMix: &rwm,
	}
	return n
}

func TestSelectFormulaForDomain(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	domain := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta:          &models.ObjMeta{ID: "DOM-1"},
			CspDomainType: "AWS",
		},
	}
	falseSFs := []*models.StorageFormula{
		&models.StorageFormula{
			Description: "50ms, Streaming, Write Mostly",
			Name:        "50ms-strm-wm",
			IoProfile: &models.IoProfile{
				IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
				ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
			},
			SscList: models.SscListMutable{
				"Response Time Average": {Kind: "DURATION", Value: "100ms"},
				"Response Time Maximum": {Kind: "DURATION", Value: "100ms"},
				"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
			},
			StorageComponent: map[string]models.StorageFormulaTypeElement{
				"Amazon gp2": {Percentage: swag.Int32(10)},
				"Amazon st1": {Percentage: swag.Int32(90)},
			},
			CacheComponent: map[string]models.StorageFormulaTypeElement{
				"Amazon HDD": {Percentage: swag.Int32(10)},
			},
			StorageLayout: "standalone",
			CspDomainType: "notAWS",
		},
	}

	// failure cases
	errTcs := []struct {
		cspDomain *models.CSPDomain
		formulas  []*models.StorageFormula
		reExpErr  string
	}{
		{nil, nil, "invalid arguments"},
		{nil, nil, "invalid arguments"},
		{domain, nil, "invalid arguments"},
		{domain, []*models.StorageFormula{}, "invalid arguments"},
	}
	for i, tc := range errTcs {
		f, err := app.SelectFormulaForDomain(tc.cspDomain, tc.formulas)
		assert.Errorf(err, "tc%d", i)
		assert.Regexp(tc.reExpErr, err, "tc%d", i)
		assert.Nil(f, "tc%d", i)
	}

	// success cases
	f, err := app.SelectFormulaForDomain(domain, app.SupportedStorageFormulas())
	assert.NoError(err)
	assert.NotNil(f)

	// failure no domaintype matches
	f, err = app.SelectFormulaForDomain(domain, falseSFs)
	assert.Error(err)
	assert.Regexp("No formulas available for given criteria", err)
	assert.Nil(f)
}

func TestCreateCapacityReservationPlan(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	// Use convenient parcel sizes
	oneGiB := int64(units.GiB)
	app.cspSTCache = []*models.CSPStorageType{
		&models.CSPStorageType{Name: "Amazon gp2", ParcelSizeBytes: swag.Int64(oneGiB)},
		&models.CSPStorageType{Name: "Amazon st1", ParcelSizeBytes: swag.Int64(oneGiB)},
	}

	sfStandaloneNoOverhead := &models.StorageFormula{
		Description: "50ms, Streaming, Write Mostly",
		Name:        "50ms-strm-wm",
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Amazon gp2": {Percentage: swag.Int32(10), Overhead: swag.Int32(0)},
			"Amazon st1": {Percentage: swag.Int32(90)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"Amazon HDD": {Percentage: swag.Int32(10)},
		},
		StorageLayout: "standalone", // TBD: mirror support
	}
	var sfStandaloneWithOverhead *models.StorageFormula
	testutils.Clone(sfStandaloneNoOverhead, &sfStandaloneWithOverhead)
	sc, found := sfStandaloneWithOverhead.StorageComponent["Amazon gp2"]
	assert.True(found)
	sc.Overhead = swag.Int32(50)
	sfStandaloneWithOverhead.StorageComponent["Amazon gp2"] = sc

	// standalone test without overhead
	args := &CreateCapacityReservationPlanArgs{
		SizeBytes: 10 * oneGiB,
		SF:        sfStandaloneNoOverhead,
	}
	crp := app.CreateCapacityReservationPlan(args)
	assert.NotNil(crp)
	assert.Len(crp.StorageTypeReservations, 2)
	str, ok := crp.StorageTypeReservations["Amazon gp2"]
	assert.True(ok)
	assert.Equal(oneGiB, swag.Int64Value(str.SizeBytes))
	assert.EqualValues(1, str.NumMirrors)
	str, ok = crp.StorageTypeReservations["Amazon st1"]
	assert.True(ok)
	assert.Equal(9*oneGiB, swag.Int64Value(str.SizeBytes))
	assert.EqualValues(1, str.NumMirrors)

	// standalone test with overhead
	args = &CreateCapacityReservationPlanArgs{
		SizeBytes: 10 * oneGiB,
		SF:        sfStandaloneWithOverhead,
	}
	crp = app.CreateCapacityReservationPlan(args)
	assert.NotNil(crp)
	assert.Len(crp.StorageTypeReservations, 2)
	str, ok = crp.StorageTypeReservations["Amazon gp2"]
	assert.True(ok)
	assert.Equal(2*oneGiB, swag.Int64Value(str.SizeBytes)) // rolled up to next parcel
	assert.EqualValues(1, str.NumMirrors)
	str, ok = crp.StorageTypeReservations["Amazon st1"]
	assert.True(ok)
	assert.Equal(9*oneGiB, swag.Int64Value(str.SizeBytes))
	assert.EqualValues(1, str.NumMirrors)

}

func TestStorageFormulaComputeCost(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := AppInit(&AppArgs{Log: tl.Logger()})

	// Use convenient parcel sizes
	parcelSizeB := int64(1 * units.MiB)
	app.cspSTCache = []*models.CSPStorageType{
		&models.CSPStorageType{Name: "Storage Type 1", ParcelSizeBytes: swag.Int64(parcelSizeB)},
		&models.CSPStorageType{Name: "Storage Type 2", ParcelSizeBytes: swag.Int64(parcelSizeB)},
	}

	// test without overhead
	sfStandaloneNoOverhead := &models.StorageFormula{
		Description: "Arbitrary formula",
		Name:        "F1",
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Storage Type 1": {Percentage: swag.Int32(25)},
			"Storage Type 2": {Percentage: swag.Int32(75)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"Cache Type 1": {Percentage: swag.Int32(25)},
		},
		StorageLayout: "mirrored", // TBD: not yet supported
	}
	storageCosts := map[string]models.StorageCost{
		"Storage Type 1": {
			CostPerGiB: 0.1,
		},
		"Storage Type 2": {
			CostPerGiB: 1,
		},
		"Cache Type 1": {
			CostPerGiB: 10,
		},
	}
	expSPC := &models.ServicePlanCost{
		CostBreakdown: map[string]models.StorageCostFraction{
			"Storage Type 1": {
				ContributedBytes: int64(units.GiB / 4), // 0.25 GiB
				CostContributed:  0.025,
				CostPerGiB:       0.1,
			},
			"Storage Type 2": {
				ContributedBytes: int64(units.GiB * 3 / 4), // 0.75 GiB
				CostContributed:  0.75,
				CostPerGiB:       1,
			},
			"Cache Type 1": {
				ContributedBytes: int64(units.GiB / 4), // 0.25 GiB
				CostContributed:  2.5,
				CostPerGiB:       10,
			},
		},
		CostPerGiB:             3.275,
		CostPerGiBWithoutCache: 0.775,
		StorageFormula:         sfStandaloneNoOverhead,
	}
	spc := app.StorageFormulaComputeCost(sfStandaloneNoOverhead, storageCosts)
	t.Log(spc.CostBreakdown)
	assert.Equal(expSPC, spc)

	// test with overhead
	var sfStandaloneWithOverhead *models.StorageFormula
	testutils.Clone(sfStandaloneNoOverhead, &sfStandaloneWithOverhead)
	sc, found := sfStandaloneWithOverhead.StorageComponent["Storage Type 1"]
	assert.True(found)
	sc.Overhead = swag.Int32(100) // double!
	sfStandaloneWithOverhead.StorageComponent["Storage Type 1"] = sc
	expSPC = &models.ServicePlanCost{
		CostBreakdown: map[string]models.StorageCostFraction{
			"Storage Type 1": {
				ContributedBytes: int64(units.GiB / 2), // 0.5 GiB
				CostContributed:  0.05,
				CostPerGiB:       0.1,
			},
			"Storage Type 2": {
				ContributedBytes: int64(units.GiB * 3 / 4), // 0.75 GiB
				CostContributed:  0.75,
				CostPerGiB:       1,
			},
			"Cache Type 1": {
				ContributedBytes: int64(units.GiB / 4), // 0.25 GiB
				CostContributed:  2.5,
				CostPerGiB:       10,
			},
		},
		CostPerGiB:             3.3,
		CostPerGiBWithoutCache: 0.8,
		StorageFormula:         sfStandaloneWithOverhead,
	}
	spc = app.StorageFormulaComputeCost(sfStandaloneWithOverhead, storageCosts)
	t.Log(spc.CostBreakdown)
	assert.Equal(expSPC, spc)
}

func TestSelectVolumeProvisioners(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	// test existing functionality
	sp1 := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{ID: "SP-1"},
		},
	}
	sp2 := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{ID: "SP-2"},
		},
	}
	sp3 := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{ID: "SP-3"},
		},
	}
	args := &SelectVolumeProvisionersArgs{
		SPs:   []*models.Pool{sp1, sp2, sp3},
		NumSP: 2,
	}
	sps, err := app.SelectVolumeProvisioners(args)
	assert.NoError(err)
	assert.NotNil(sps)
	assert.Len(sps, 2)
	assert.Equal(sp1, sps[0])
	assert.Equal(sp2, sps[1])

	args.NumSP = 4
	sps, err = app.SelectVolumeProvisioners(args)
	assert.Error(err)
	assert.Regexp("insufficient number", err)
	assert.Nil(sps)
}

type fakeSFC struct {
	// StorageFormulasForServicePlan
	InSFfSPid   models.ObjIDMutable
	InSFfSPobj  *models.ServicePlan
	RetSFfSPObj []*models.StorageFormula
	RetSFfSPErr error

	// SelectFormulaForDomain
	InSFfDo    *models.CSPDomain
	InSFfDf    []*models.StorageFormula
	RetSFfDf   *models.StorageFormula
	RetSFfDErr error

	// StorageFormulaComputeCost
	InSFCCf *models.StorageFormula
	InSFCCc map[string]models.StorageCost
	RetSFCC *models.ServicePlanCost
}

var _ = StorageFormulaComputer(&fakeSFC{})

func (sfc *fakeSFC) SupportedStorageFormulas() []*models.StorageFormula {
	return nil
}
func (sfc *fakeSFC) FindStorageFormula(formulaName models.StorageFormulaName) *models.StorageFormula {
	return nil
}
func (sfc *fakeSFC) ValidateStorageFormula(formulaName models.StorageFormulaName) bool {
	return true
}
func (sfc *fakeSFC) StorageFormulaForAttrs(ioProfile *models.IoProfile, slos models.SloListMutable) []*models.StorageFormula {
	return nil
}
func (sfc *fakeSFC) CreateCapacityReservationPlan(args *CreateCapacityReservationPlanArgs) *models.CapacityReservationPlan {
	return nil
}
func (sfc *fakeSFC) SelectVolumeProvisioners(args *SelectVolumeProvisionersArgs) ([]*models.Pool, error) {
	return nil, nil
}
func (sfc *fakeSFC) StorageFormulasForServicePlan(ctx context.Context, spID models.ObjIDMutable, spObj *models.ServicePlan) ([]*models.StorageFormula, error) {
	sfc.InSFfSPid = spID
	sfc.InSFfSPobj = spObj
	return sfc.RetSFfSPObj, sfc.RetSFfSPErr
}
func (sfc *fakeSFC) SelectFormulaForDomain(cspDomain *models.CSPDomain, formulas []*models.StorageFormula) (*models.StorageFormula, error) {
	sfc.InSFfDo = cspDomain
	sfc.InSFfDf = formulas
	return sfc.RetSFfDf, sfc.RetSFfDErr
}
func (sfc *fakeSFC) StorageFormulaComputeCost(sf *models.StorageFormula, storageCost map[string]models.StorageCost) *models.ServicePlanCost {
	sfc.InSFCCf = sf
	sfc.InSFCCc = storageCost
	return sfc.RetSFCC
}
