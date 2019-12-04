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
	"fmt"
	"reflect"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	units "github.com/docker/go-units"
	"github.com/go-openapi/swag"
)

// StorageFormulaComputer offers an abstraction over StorageFormula methods
type StorageFormulaComputer interface {
	SupportedStorageFormulas() []*models.StorageFormula
	FindStorageFormula(formulaName models.StorageFormulaName) *models.StorageFormula
	ValidateStorageFormula(formulaName models.StorageFormulaName) bool
	StorageFormulaForAttrs(ioProfile *models.IoProfile, slos models.SloListMutable) []*models.StorageFormula
	StorageFormulasForServicePlan(ctx context.Context, servicePlanID models.ObjIDMutable, spObj *models.ServicePlan) ([]*models.StorageFormula, error)
	StorageFormulaComputeCost(sf *models.StorageFormula, storageCost map[string]models.StorageCost) *models.ServicePlanCost

	// Policy related methods
	SelectFormulaForDomain(cspDomain *models.CSPDomain, formulas []*models.StorageFormula) (*models.StorageFormula, error)
	CreateCapacityReservationPlan(args *CreateCapacityReservationPlanArgs) *models.CapacityReservationPlan
	SelectVolumeProvisioners(args *SelectVolumeProvisionersArgs) ([]*models.Pool, error)
}

// SupportedStorageFormulas returns the storage formulas of all domain types
func (app *AppCtx) SupportedStorageFormulas() []*models.StorageFormula {
	sfs := []*models.StorageFormula{}
	// TBD: cache the storage types
	for _, cst := range app.SupportedCspDomainTypes() {
		csp := app.CSP(cst)
		sfs = append(sfs, csp.SupportedStorageFormulas()...)
	}
	return sfs
}

// FindStorageFormula looks up a storage formula
func (app *AppCtx) FindStorageFormula(formulaName models.StorageFormulaName) *models.StorageFormula {
	for _, f := range app.SupportedStorageFormulas() {
		if f.Name == formulaName {
			return f
		}
	}
	return nil
}

// ValidateStorageFormula verifies a storage formula name
func (app *AppCtx) ValidateStorageFormula(formulaName models.StorageFormulaName) bool {
	if app.FindStorageFormula(formulaName) != nil {
		return true
	}
	return false
}

//StorageFormulaForAttrs returns the set of applicable formulas for a given workload description
func (app *AppCtx) StorageFormulaForAttrs(ioProfile *models.IoProfile, slos models.SloListMutable) []*models.StorageFormula {
	var formulaChoices []*models.StorageFormula

	app.Log.Debugf("** WANT: ioPattern : %s", ioProfile.IoPattern.Name)
	app.Log.Debugf("** WANT: ReadWriteMix: %s", ioProfile.ReadWriteMix.Name)
	app.Log.Debugf("** WANT: slos: %v", slos)

	// XXX - Currently look for exact matches. Over time we want
	// to look for ALL possible matches and then score/rank them somehow.
	// That way a customer can choose to "overprovision" the storage
	// if that's the only way to satisfy the request
	for _, storageFormula := range app.SupportedStorageFormulas() {
		app.Log.Debugf("** SEE: formula: %s", storageFormula.Name)
		app.Log.Debugf("**      ioProfile: %s", storageFormula.IoProfile.IoPattern.Name)
		app.Log.Debugf("**      ioProfile: %s", storageFormula.IoProfile.ReadWriteMix.Name)
		app.Log.Debugf("**      SSCList: %v", storageFormula.SscList)
		if reflect.DeepEqual(storageFormula.IoProfile.IoPattern, ioProfile.IoPattern) &&
			reflect.DeepEqual(storageFormula.IoProfile.ReadWriteMix, ioProfile.ReadWriteMix) &&
			storageFormula.SscList["Response Time Average"].Kind == slos["Response Time Average"].Kind &&
			storageFormula.SscList["Response Time Average"].Value == slos["Response Time Average"].Value &&
			storageFormula.SscList["Response Time Maximum"].Kind == slos["Response Time Maximum"].Kind &&
			storageFormula.SscList["Response Time Maximum"].Value == slos["Response Time Maximum"].Value &&
			storageFormula.SscList["Availability"].Kind == slos["Availability"].Kind &&
			storageFormula.SscList["Availability"].Value == slos["Availability"].Value {
			app.Log.Debugf("**      Yippee! Formula %s MATCHES!!!", storageFormula.Name)
			formulaChoices = append(formulaChoices, storageFormula)
		}
	}
	return formulaChoices
}

// StorageFormulasForServicePlan returns the possible StorageFormula objects for a given ServicePlan.
// The object is fetched if not provided on input.
func (app *AppCtx) StorageFormulasForServicePlan(ctx context.Context, servicePlanID models.ObjIDMutable, spObj *models.ServicePlan) ([]*models.StorageFormula, error) {
	if spObj == nil {
		var err error
		spObj, err = app.DS.OpsServicePlan().Fetch(ctx, string(servicePlanID))
		if err != nil {
			return nil, err
		}
	}
	return app.StorageFormulaForAttrs(spObj.IoProfile, spObj.Slos), nil
}

// SelectFormulaForDomain selects the best formula for a CSPDomain
// from a set of storage formulas. It returns nil if no applicable formula found.
func (app *AppCtx) SelectFormulaForDomain(cspDomain *models.CSPDomain, formulas []*models.StorageFormula) (*models.StorageFormula, error) {
	if cspDomain == nil || formulas == nil || len(formulas) == 0 {
		return nil, fmt.Errorf("invalid arguments")
	}
	// TBD: Picking first right now. Will have to pick formula more intelligently if multiple available.
	sFList := make([]*models.StorageFormula, 0)
	for _, storageFormula := range formulas {
		if string(cspDomain.CspDomainType) == string(storageFormula.CspDomainType) {
			sFList = append(sFList, storageFormula)
		}
	}
	if len(sFList) == 0 {
		return nil, fmt.Errorf("No formulas available for given criteria")
	}
	return sFList[0], nil
}

// CreateCapacityReservationPlanArgs contains arguments to CreateCapacityReservationPlan
type CreateCapacityReservationPlanArgs struct {
	SizeBytes int64                  // the amount of data being requested for the Volume
	SF        *models.StorageFormula // the formula used
}

// CreateCapacityReservationPlan creates the reservation plan to acquire a particular amount of (non-cache) storage by applying the provided formula.
func (app *AppCtx) CreateCapacityReservationPlan(args *CreateCapacityReservationPlanArgs) *models.CapacityReservationPlan {
	crp := &models.CapacityReservationPlan{
		StorageTypeReservations: map[string]models.StorageTypeReservation{},
	}
	for st, sc := range args.SF.StorageComponent {
		// find the percentage and overhead in the formula, round up to parcel size
		sz := args.SizeBytes * int64(swag.Int32Value(sc.Percentage))
		sz += sz * int64(swag.Int32Value(sc.Overhead)) / int64(100)
		sz /= 100
		cst := app.GetCspStorageType(models.CspStorageType(st))
		parcelSize := swag.Int64Value(cst.ParcelSizeBytes)
		if diff := sz % parcelSize; diff != 0 {
			sz = sz - diff + parcelSize
		}
		// TBD use layout to set numMirrors
		numMirrors := int32(1)
		crp.StorageTypeReservations[st] = models.StorageTypeReservation{
			NumMirrors: numMirrors,
			SizeBytes:  swag.Int64(sz),
		}
	}
	return crp
}

// StorageFormulaComputeCost computes the cost of a service plan from a storage cost map and storageFormula
func (app *AppCtx) StorageFormulaComputeCost(sf *models.StorageFormula, storageCost map[string]models.StorageCost) *models.ServicePlanCost {
	oneGiB := int64(units.GiB) // Cost is expressed per GiB storage.
	mult := int64(1024)        // To minimize parcel roundup error use a large
	compSize := mult * oneGiB  // multiple of a GiB then normalize.
	spc := &models.ServicePlanCost{}
	spc.StorageFormula = sf
	spc.CostBreakdown = map[string]models.StorageCostFraction{}
	addFractionalCost := func(st string, cb int64) {
		sfc := models.StorageCostFraction{
			ContributedBytes: cb / mult, // for 1 GiB ask
		}
		if cost, found := storageCost[st]; found {
			sfc.CostPerGiB = cost.CostPerGiB
		}
		sfc.CostContributed = float64(cb) * sfc.CostPerGiB / float64(compSize) // for 1 GiB ask
		spc.CostBreakdown[st] = sfc
		spc.CostPerGiB += sfc.CostContributed
	}
	// storage components have overhead and roundup - use planner
	crpArgs := &CreateCapacityReservationPlanArgs{
		SizeBytes: compSize,
		SF:        sf,
	}
	crp := app.SFC.CreateCapacityReservationPlan(crpArgs)
	for st, str := range crp.StorageTypeReservations {
		addFractionalCost(st, swag.Int64Value(str.SizeBytes))
	}
	spc.CostPerGiBWithoutCache = spc.CostPerGiB
	// consider cache components
	for st, cc := range sf.CacheComponent {
		addFractionalCost(st, compSize*int64(swag.Int32Value(cc.Percentage))/int64(100))
	}
	return spc
}

// SelectVolumeProvisionersArgs contains arguments to SelectVolumeProvisioners
type SelectVolumeProvisionersArgs struct {
	// The pools all provide storage of the same type; multiple such calls are made for a given
	// volume, one for each type of storage specified by the storage formula used.
	SPs   []*models.Pool // candidate set
	NumSP int            // desired number of pools

	// Additional contextual data are provided to aid in selection.
	// Potential reasons to reject a pool would include access control policy.
	Vol       *models.VolumeSeries   // the Volume concerned
	SF        *models.StorageFormula // the formula used
	SizeBytes int64                  // the amount of data being reserved in each SP
}

// SelectVolumeProvisioners selects a subset of Pools out of a set of potential candidates.
// Selected pools will provide reserved capacity for the volume.
func (app *AppCtx) SelectVolumeProvisioners(args *SelectVolumeProvisionersArgs) ([]*models.Pool, error) {
	// TBD. For now pick the top
	if len(args.SPs) < args.NumSP {
		return nil, fmt.Errorf("insufficient number of pools")
	}
	res := make([]*models.Pool, 0, args.NumSP)
	for i := 0; i < args.NumSP; i++ {
		res = append(res, args.SPs[i])
	}
	return res, nil
}
