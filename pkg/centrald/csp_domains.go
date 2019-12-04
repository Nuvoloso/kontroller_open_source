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
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// AppCloudServiceProvider offers an abstraction over the csp domain methods
type AppCloudServiceProvider interface {
	CSP(cspDomainType models.CspDomainTypeMutable) csp.CloudServiceProvider
	SupportedCspDomainTypes() []models.CspDomainTypeMutable
	DomainAttrDescForType(cspDomainType models.CspDomainTypeMutable) map[string]models.AttributeDescriptor
	CredentialAttrDescForType(cspDomainType models.CspDomainTypeMutable) map[string]models.AttributeDescriptor
	ValidateCspDomainType(cspDomainType models.CspDomainTypeMutable) bool
	ValidateCspCredentialAttributes(domType models.CspDomainTypeMutable, oldAttrs, newAttrs map[string]models.ValueType) error
	ValidateCspCredential(domType models.CspDomainTypeMutable, oldAttrs, newAttrs map[string]models.ValueType) error
	ValidateAndInitCspDomain(ctx context.Context, dObj *models.CSPDomain, cspCredAttrs map[string]models.ValueType) error
	GetMCDeployment(args *cluster.MCDeploymentArgs) (*cluster.Deployment, error)
	SupportedCspStorageTypes() []*models.CSPStorageType
	GetCspStorageType(name models.CspStorageType) *models.CSPStorageType
	ValidateCspDomainAndStorageTypes(cspDomainType models.CspDomainTypeMutable, cspStorageType models.CspStorageType) error
	GetDomainClient(dObj *models.CSPDomain) (csp.DomainClient, error)
	DomainServicePlanCost(ctx context.Context, cspDomain *models.CSPDomain, sp *models.ServicePlan) (*models.ServicePlanCost, error)
}

// AppCtx is an AppCloudServiceProvider
var _ = AppCloudServiceProvider(&AppCtx{})

// CSP returns a cached csp.CloudServiceProvider interface
func (app *AppCtx) CSP(cspDomainType models.CspDomainTypeMutable) csp.CloudServiceProvider {
	appMutex.Lock()
	defer appMutex.Unlock()
	return app.csp(cspDomainType)
}

// csp is the implementation of CSP within the mutex
func (app *AppCtx) csp(cspDomainType models.CspDomainTypeMutable) csp.CloudServiceProvider {
	if app.cspCache == nil {
		app.cspCache = make(map[models.CspDomainTypeMutable]csp.CloudServiceProvider)
	}
	app.Log.Debug("CSP for", cspDomainType)
	var cSP csp.CloudServiceProvider
	var ok bool
	var err error
	cSP, ok = app.cspCache[cspDomainType]
	if !ok {
		app.Log.Debug("Creating CSP for", cspDomainType)
		cSP, err = csp.NewCloudServiceProvider(cspDomainType)
		if err == nil {
			cSP.SetDebugLogger(app.Log)
			app.cspCache[cspDomainType] = cSP
		} else {
			app.Log.Warningf("Attempt to fetch CSP for %s: %s", cspDomainType, err.Error())
			cSP = nil
		}
	}
	return cSP
}

// SupportedCspDomainTypes returns a list of supported CSP domain types
func (app *AppCtx) SupportedCspDomainTypes() []models.CspDomainTypeMutable {
	return csp.SupportedCspDomainTypes()
}

// DomainAttrDescForType returns a map of property names and Kind for a supported cspDomainType
// It panics on invalid cspDomainType
func (app *AppCtx) DomainAttrDescForType(cspDomainType models.CspDomainTypeMutable) map[string]models.AttributeDescriptor {
	sp := app.CSP(cspDomainType)
	if sp == nil {
		panic("invalid cspDomainType " + cspDomainType)
	}
	return sp.DomainAttributes()
}

// TBD: to do it properly
var credentialAttributes = map[string][]string{
	"AWS": []string{"aws_access_key_id", "aws_secret_access_key"},
}

// CredentialAttrDescForType returns a map of property names and Kind for a supported cspDomainType
// It panics on invalid cspDomainType
func (app *AppCtx) CredentialAttrDescForType(cspDomainType models.CspDomainTypeMutable) map[string]models.AttributeDescriptor {
	sp := app.CSP(cspDomainType)
	if sp == nil {
		panic("invalid cspDomainType " + cspDomainType)
	}
	return sp.CredentialAttributes()
}

// ValidateCspDomainType verifies a Csp domain type
func (app *AppCtx) ValidateCspDomainType(cspDomainType models.CspDomainTypeMutable) bool {
	for _, dt := range csp.SupportedCspDomainTypes() {
		if dt == cspDomainType {
			return true
		}
	}
	return false
}

// ValidateAndInitCspDomain verifies the properties and values of CspDomainAttributes for a cspDomain object,
// validates the attributes used to connect to the CSP and creates a protection store.
func (app *AppCtx) ValidateAndInitCspDomain(ctx context.Context, dObj *models.CSPDomain, cspCredAttrs map[string]models.ValueType) error {
	cSP := app.CSP(dObj.CspDomainType)
	if cSP == nil {
		return fmt.Errorf("supported cspDomainType values are %s", app.SupportedCspDomainTypes())
	}
	// validate domain attribute only as credential attributes should have been already validated
	dad := cSP.DomainAttributes()
	err := app.validateBasicAttributes(dObj.CspDomainType, dad, nil, dObj.CspDomainAttributes)
	if err != nil {
		return err
	}
	combinedAttrs := map[string]models.ValueType{}
	for da, vt := range dObj.CspDomainAttributes {
		combinedAttrs[da] = vt
	}
	for ca, vt := range cspCredAttrs {
		combinedAttrs[ca] = vt
	}
	attrs, err := cSP.SanitizedAttributes(combinedAttrs) // Note: here the PS bucket name gets created if not specified
	if err != nil {
		return err
	}
	// update dObj optional CspDomainAttributes if necessary (e.g. in case PS bucket name was originally empty)
	for da, desc := range dad {
		if desc.Optional && attrs[da] != dObj.CspDomainAttributes[da] {
			dObj.CspDomainAttributes[da] = attrs[da]
		}
	}
	domObj := &models.CSPDomain{}
	*domObj = *dObj
	domObj.CspDomainAttributes = attrs
	cl, err := app.GetDomainClient(domObj)
	if err == nil {
		ctx, cancel := context.WithTimeout(ctx, time.Duration(app.AppArgs.CSPTimeout)*time.Second)
		defer cancel()
		if err = cl.Validate(ctx); err != nil {
			return fmt.Errorf("invalid cspDomainAttributes: %s", err.Error())
		}
		if attrs, err = cl.CreateProtectionStore(ctx); err != nil {
			return fmt.Errorf(err.Error())
		}
		dObj.CspDomainAttributes = attrs
	}
	return err
}

// ValidateCspCredentialAttributes verifies the properties and values of CSP Credential attributes.
func (app *AppCtx) ValidateCspCredentialAttributes(domType models.CspDomainTypeMutable, oldAttrs, newAttrs map[string]models.ValueType) error {
	cad := app.CredentialAttrDescForType(domType)
	err := app.validateBasicAttributes(domType, cad, oldAttrs, newAttrs)
	if err != nil {
		return err
	}
	return err
}

// ValidateCspCredential verifies CSP Credential attributes used to connect to the CSP.
func (app *AppCtx) ValidateCspCredential(domType models.CspDomainTypeMutable, oldAttrs, newAttrs map[string]models.ValueType) error {
	if err := app.ValidateCspCredentialAttributes(domType, oldAttrs, newAttrs); err != nil {
		return err
	}
	cSP := app.csp(domType) // no need to check for errors here as above call does all necessary validations
	if err := cSP.ValidateCredential(domType, newAttrs); err != nil {
		return err
	}
	return nil
}

// validateBasicAttributes verifies the properties and values of Attributes.
// It ensures that each kind specified is supported.
// It also ensures that if the kind is INT, the value is actually an integer.
// It checks to see if immutable attributes are being modified on update.
func (app *AppCtx) validateBasicAttributes(domType models.CspDomainTypeMutable, attrDescriptors map[string]models.AttributeDescriptor, oldAttrs, newAttrs map[string]models.ValueType) error {
	for da, desc := range attrDescriptors {
		a, ok := newAttrs[da]
		if !ok {
			if !desc.Optional {
				return fmt.Errorf("cspDomainType %s attribute %s (%s) is required", domType, da, desc.Kind)
			}
			continue
		}
		if a.Kind != desc.Kind {
			return fmt.Errorf("cspDomainType %s attribute %s Kind should be %s", domType, da, desc.Kind)
		}
		if err := app.ValidateValueType(a); err != nil {
			return fmt.Errorf("cspDomainType %s attribute %s %s", domType, da, err.Error())
		}
		if oldAttrs != nil && desc.Immutable && a.Value != oldAttrs[da].Value {
			return fmt.Errorf("cspDomainType %s attribute %s (%s) cannot be modified", domType, da, desc.Kind)
		}
	}
	for k := range newAttrs {
		if _, ok := attrDescriptors[k]; !ok {
			return fmt.Errorf("invalid cspDomainType %s attribute %s", domType, k)
		}
	}
	return nil
}

// GetMCDeployment returns the managed cluster deployment for a specified domain type
// It will fill in the Image path related properties
func (app *AppCtx) GetMCDeployment(args *cluster.MCDeploymentArgs) (*cluster.Deployment, error) {
	cSP := app.CSP(models.CspDomainTypeMutable(args.CSPDomainType))
	if cSP == nil {
		return nil, fmt.Errorf("invalid cspDomainType %s in CSPDomain %s", args.CSPDomainType, args.CSPDomainID)
	}
	loadAndEncode := func(n, path string) (string, error) {
		if path == "" {
			return "", fmt.Errorf("%s file not specified", n)
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%s file error: %s", n, err.Error())
		}
		return base64.StdEncoding.EncodeToString(data), nil
	}
	if args.ImageTag == "" {
		args.ImageTag = app.ClusterDeployTag
	}
	if args.DriverType == "" {
		args.DriverType = app.DriverType
	}
	args.ImagePath = app.ImagePath
	var err error
	args.CACert, err = loadAndEncode("ca cert", string(app.Server.TLSCACertificate))
	if err == nil {
		args.AgentdCert, err = loadAndEncode("agentd cert", string(app.AgentdCertPath))
		if err == nil {
			args.AgentdKey, err = loadAndEncode("agentd key", string(app.AgentdKeyPath))
			if err == nil {
				args.ClusterdCert, err = loadAndEncode("clusterd cert", string(app.ClusterdCertPath))
				if err == nil {
					args.ClusterdKey, err = loadAndEncode("clusterd key", string(app.ClusterdKeyPath))
					if err == nil && string(app.ImagePullSecretPath) != "" {
						args.ImagePullSecret, err = loadAndEncode("image pull secret", string(app.ImagePullSecretPath))
					}
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return app.MCDeployer.GetMCDeployment(args)
}

// SupportedCspStorageTypes returns a list of the currently supported CSP storage types
func (app *AppCtx) SupportedCspStorageTypes() []*models.CSPStorageType {
	appMutex.Lock()
	defer appMutex.Unlock()
	if app.cspSTCache != nil {
		return app.cspSTCache
	}
	dts := csp.SupportedCspDomainTypes()
	app.cspSTCache = make([]*models.CSPStorageType, 0)
	for _, dt := range dts {
		cSP := app.csp(dt) // note: mutex is held
		sT := cSP.SupportedCspStorageTypes()
		app.cspSTCache = append(app.cspSTCache, sT...)
	}
	return app.cspSTCache
}

// GetCspStorageType returns the named CspStorageType, or nil if no such named type exists
func (app *AppCtx) GetCspStorageType(name models.CspStorageType) *models.CSPStorageType {
	for _, cspStorageType := range app.SupportedCspStorageTypes() {
		if name == cspStorageType.Name {
			return cspStorageType
		}
	}
	return nil
}

// ValidateCspDomainAndStorageTypes validates that the cspDomainType and cspStorageType are compatible.
func (app *AppCtx) ValidateCspDomainAndStorageTypes(cspDomainType models.CspDomainTypeMutable, cspStorageType models.CspStorageType) error {
	cSP := app.CSP(cspDomainType)
	if cSP == nil {
		return fmt.Errorf("supported cspDomainType values are %s", app.SupportedCspDomainTypes())
	}
	tObj := app.GetCspStorageType(cspStorageType)
	if tObj == nil {
		return fmt.Errorf("invalid cspStorageType")
	} else if tObj.CspDomainType != models.CspDomainType(cspDomainType) {
		return fmt.Errorf("incorrect cspStorageType for CSP domain %s", string(cspDomainType))
	}
	return nil
}

type cspDomainClientRecord struct {
	version    models.ObjVersion
	client     csp.DomainClient
	attrValSHA string
}

// makeAttrValSHA creates a hash of all CSPDomain object attributes values sorted by keys
func makeAttrValSHA(dObj *models.CSPDomain) string {
	h := sha1.New()
	for _, val := range util.SortedStringKeys(dObj.CspDomainAttributes) {
		io.WriteString(h, dObj.CspDomainAttributes[val].Value)
	}
	return string(h.Sum(nil))
}

// GetDomainClient returns a cached CSPDomain client. The cached client is discarded
// if the CSPDomain object is updated.  This allows invalid credentials to be corrected.
// If the object Metadata is not present, a new client is always returned and is not cached.
func (app *AppCtx) GetDomainClient(dObj *models.CSPDomain) (csp.DomainClient, error) {
	appMutex.Lock()
	defer appMutex.Unlock()
	if app.cspClientCache == nil {
		app.cspClientCache = make(map[models.ObjID]cspDomainClientRecord)
	}
	id := models.ObjID("")
	version := models.ObjVersion(0)
	attrValSHA := makeAttrValSHA(dObj)
	if dObj.Meta != nil {
		id = dObj.Meta.ID
		version = dObj.Meta.Version
		if dcr, ok := app.cspClientCache[id]; ok && dcr.version == version && dcr.attrValSHA == attrValSHA {
			app.Log.Debugf("Found cached client for CSPDomain %s (%s, %d)", dObj.Name, id, version)
			return dcr.client, nil
		}
	}
	if cSP := app.csp(dObj.CspDomainType); cSP != nil {
		app.Log.Debugf("Fetching client for CSPDomain %s (%s, %d)", dObj.Name, id, version)
		cl, err := cSP.Client(dObj)
		if err == nil {
			if id != "" {
				app.Log.Debugf("Caching client for CSPDomain %s (%s, %d)", dObj.Name, id, version)
				dcr := cspDomainClientRecord{
					version:    version,
					client:     cl,
					attrValSHA: attrValSHA,
				}
				app.cspClientCache[id] = dcr
			}
			cl.SetTimeout(app.ClientTimeout)
			return cl, nil
		}
		app.Log.Errorf("Failed to fetch client for CSPDomain %s (%s, %d): %s", dObj.Name, id, version, err.Error())
		return nil, err
	}
	return nil, fmt.Errorf("invalid CSP domain type")
}

// DomainServicePlanCost computes the cost of a service plan in a domain
func (app *AppCtx) DomainServicePlanCost(ctx context.Context, cspDomain *models.CSPDomain, sp *models.ServicePlan) (*models.ServicePlanCost, error) {
	var sf *models.StorageFormula
	sfs, err := app.SFC.StorageFormulasForServicePlan(ctx, "", sp)
	if err == nil {
		sf, err = app.SFC.SelectFormulaForDomain(cspDomain, sfs)
	}
	if err != nil {
		return nil, err
	}
	storageCosts := cspDomain.StorageCosts
	if storageCosts == nil {
		storageCosts = map[string]models.StorageCost{}
	}
	return app.SFC.StorageFormulaComputeCost(sf, storageCosts), nil
}
