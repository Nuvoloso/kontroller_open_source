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

	vs "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
)

type cacheHelper struct {
	accounts           map[string]ctxIDName       // id to tenantID,name map
	accountObjs        map[string]*models.Account // id to object
	applicationGroups  map[string]ctxIDName       // id to accountID,name map
	consistencyGroups  map[string]ctxIDName       // id to accountID,name map
	cspDomains         map[string]string          // id to name map
	clusters           map[string]ctxIDName       // id to domainID,clusterName map
	nodes              map[string]ctxIDName       // id to domainID,NodeName map
	roles              map[string]string          // id to name map
	servicePlans       map[string]string          // id to name map
	users              map[string]string          // id to authIdentifier map
	volumeSeries       map[string]ctxIDName       // id to accountID,name map
	skipNodeCache      bool
	cspCredentials     map[string]string                   // id to name map
	cspDomainAttrKinds map[string]string                   // cspDomain attribute to kind
	protectionDomains  map[string]*models.ProtectionDomain // id to obj
}

type ctxIDName struct {
	id   string
	name string
}

func (c *cacheHelper) loadDCNCaches() error {
	var err error
	if err = c.cacheCSPDomains(); err == nil {
		if err = c.cacheClusters(); err == nil {
			err = c.cacheNodes()
		}
	}
	return err
}

func (c *cacheHelper) cacheAccounts() error {
	if c.accounts != nil {
		return nil
	}
	accountCmd := &accountCmd{}
	accounts, err := accountCmd.list(nil)
	if err != nil {
		return err
	}
	c.accounts = make(map[string]ctxIDName)
	c.accountObjs = make(map[string]*models.Account)
	for _, o := range accounts {
		c.accounts[string(o.Meta.ID)] = ctxIDName{id: string(o.TenantAccountID), name: string(o.Name)}
		c.accountObjs[string(o.Meta.ID)] = o
	}
	return nil
}

// validateAccount will validate the account in the context of the appCtx.Account and return its ID.
// accountUsage differentiates account usage in error messages, e.g. "authorized".
func (c *cacheHelper) validateAccount(name, accountUsage string) (string, error) {
	if err := c.cacheAccounts(); err != nil {
		return "", err
	}
	contextAccountID := ""
	if appCtx.Account != common.SystemAccount {
		// SystemAccount is top level and is not used in tenant account lookups
		contextAccountID = appCtx.AccountID
	}
	for id, a := range c.accounts {
		if a.name == name && a.id == contextAccountID {
			return id, nil
		}
	}
	if accountUsage != "" {
		accountUsage = accountUsage + " "
	}
	return "", fmt.Errorf("%saccount '%s' not found", accountUsage, name)
}

func (c *cacheHelper) cacheApplicationGroups() error {
	if c.applicationGroups != nil {
		return nil
	}
	cmd := &applicationGroupCmd{}
	applicationGroups, err := cmd.list(nil)
	if err != nil {
		return err
	}
	c.applicationGroups = make(map[string]ctxIDName)
	for _, o := range applicationGroups {
		c.applicationGroups[string(o.Meta.ID)] = ctxIDName{id: string(o.AccountID), name: string(o.Name)}
	}
	return nil
}

func (c *cacheHelper) cacheCSPDomains() error {
	if c.cspDomains != nil {
		return nil
	}
	cspCmd := &cspDomainCmd{}
	cspDomains, err := cspCmd.list(nil)
	if err != nil {
		return err
	}
	c.cspDomains = make(map[string]string)
	for _, o := range cspDomains {
		c.cspDomains[string(o.Meta.ID)] = string(o.Name)
	}
	return nil
}

func (c *cacheHelper) cacheClusters() error {
	if c.clusters != nil {
		return nil
	}
	cmd := &clusterCmd{}
	clusters, err := cmd.list(nil)
	if err != nil {
		return err
	}
	c.clusters = make(map[string]ctxIDName)
	for _, o := range clusters {
		c.clusters[string(o.Meta.ID)] = ctxIDName{id: string(o.CspDomainID), name: string(o.Name)}
	}
	return nil
}

func (c *cacheHelper) cacheConsistencyGroups() error {
	if c.consistencyGroups != nil {
		return nil
	}
	cmd := &consistencyGroupCmd{}
	consistencyGroups, err := cmd.list(nil)
	if err != nil {
		return err
	}
	c.consistencyGroups = make(map[string]ctxIDName)
	for _, o := range consistencyGroups {
		c.consistencyGroups[string(o.Meta.ID)] = ctxIDName{id: string(o.AccountID), name: string(o.Name)}
	}
	return nil
}

func (c *cacheHelper) cacheNodes() error {
	if c.nodes != nil || c.skipNodeCache {
		return nil
	}
	cmd := &nodeCmd{}
	nodes, err := cmd.list(nil)
	if err != nil {
		return err
	}
	c.nodes = make(map[string]ctxIDName)
	for _, o := range nodes {
		c.nodes[string(o.Meta.ID)] = ctxIDName{id: string(o.ClusterID), name: string(o.Name)}
	}
	return nil
}

func (c *cacheHelper) cacheProtectionDomains() error {
	if c.protectionDomains != nil {
		return nil
	}
	pdCmd := &protectionDomainCmd{}
	protectionDomains, err := pdCmd.list(nil)
	if err != nil {
		return err
	}
	c.protectionDomains = make(map[string]*models.ProtectionDomain)
	for _, o := range protectionDomains {
		c.protectionDomains[string(o.Meta.ID)] = o
	}
	return nil
}

func (c *cacheHelper) cacheRoles() error {
	if c.roles != nil {
		return nil
	}
	roleCmd := &roleCmd{}
	roles, err := roleCmd.list(nil)
	if err != nil {
		return err
	}
	c.roles = make(map[string]string)
	for _, o := range roles {
		c.roles[string(o.Meta.ID)] = string(o.Name)
	}
	return nil
}

func (c *cacheHelper) cacheServicePlans() error {
	if c.servicePlans != nil {
		return nil
	}
	cmd := &servicePlanCmd{}
	servicePlans, err := cmd.list(nil)
	if err != nil {
		return err
	}
	c.servicePlans = make(map[string]string)
	for _, o := range servicePlans {
		c.servicePlans[string(o.Meta.ID)] = string(o.Name)
	}
	return nil
}

func (c *cacheHelper) cacheUsers() error {
	if c.users != nil {
		return nil
	}
	userCmd := &userCmd{}
	users, err := userCmd.list(nil)
	if err != nil {
		return err
	}
	c.users = make(map[string]string)
	for _, o := range users {
		c.users[string(o.Meta.ID)] = string(o.AuthIdentifier)
	}
	return nil
}

// if VS object is provided (by its id, name and account) then it should just be added to the cache, otherwise load all existing VolumeSeries to cache
func (c *cacheHelper) cacheVolumeSeries(vsID, vsName, vsAccountID string) error {
	if c.volumeSeries == nil {
		c.volumeSeries = make(map[string]ctxIDName)
	}
	if vsID != "" {
		if _, ok := c.volumeSeries[vsID]; ok { // already present in the cache?
			return nil
		}
		if vsName != "" && vsAccountID != "" {
			if _, ok := c.volumeSeries[vsID]; !ok { // already present in the cache?
				c.volumeSeries[vsID] = ctxIDName{id: vsAccountID, name: vsName}
			}
		} else { // fetch VS by its ID
			vsObj, err := appCtx.API.VolumeSeries().VolumeSeriesFetch(vs.NewVolumeSeriesFetchParams().WithID(vsID))
			if err != nil {
				if e, ok := err.(*vs.VolumeSeriesFetchDefault); ok && e.Payload.Message != nil {
					return fmt.Errorf("%s", *e.Payload.Message)
				}
				return err
			}
			c.volumeSeries[vsID] = ctxIDName{id: string(vsObj.Payload.AccountID), name: string(vsObj.Payload.Name)}
		}
		return nil
	}
	vsCmd := &volumeSeriesCmd{}
	volumeSeries, err := vsCmd.list(nil)
	if err != nil {
		return err
	}
	for _, o := range volumeSeries {
		c.volumeSeries[string(o.Meta.ID)] = ctxIDName{id: string(o.AccountID), name: string(o.Name)}
	}
	return nil
}

func (c *cacheHelper) cacheCspCredentials(credID, credName string) error {
	if c.cspCredentials == nil {
		c.cspCredentials = make(map[string]string)
	}
	if credID != "" && credName != "" {
		c.cspCredentials[credID] = string(credName)
		return nil
	}
	cmd := &cspCredentialCmd{}
	creds, err := cmd.list(nil)
	if err != nil {
		return err
	}
	c.cspCredentials = make(map[string]string)
	for _, o := range creds {
		c.cspCredentials[string(o.Meta.ID)] = string(o.Name)
	}
	return nil
}

func (c *cacheHelper) cacheCspDomainAttrKinds(attrKinds map[string]string) error {
	if c.cspDomainAttrKinds == nil {
		c.cspDomainAttrKinds = make(map[string]string, len(attrKinds))
	}
	c.cspDomainAttrKinds = attrKinds
	return nil
}

// validateApplicationConsistencyGroupNames validates application and consistency group names for the context account, loads the caches and returns their IDs.
func (c *cacheHelper) validateApplicationConsistencyGroupNames(agNames []string, cgName string) ([]models.ObjIDMutable, string, error) {
	agIds, cgID := []models.ObjIDMutable{}, ""
	if cgName != "" && appCtx.Account == "" {
		return agIds, cgID, fmt.Errorf("consistency-group requires account")
	}
	if len(agNames) > 0 && appCtx.Account == "" {
		return agIds, cgID, fmt.Errorf("application-group requires account")
	}
	var err error
	if cgName != "" {
		if err = c.cacheConsistencyGroups(); err != nil {
			return agIds, cgID, err
		}
		for id, cn := range c.consistencyGroups {
			if cn.id == appCtx.AccountID && cn.name == cgName {
				cgID = id
			}
		}
		if cgID == "" {
			return agIds, cgID, fmt.Errorf("consistency group '%s' not found for account '%s'", cgName, appCtx.Account)
		}
	}
	if len(agNames) > 0 {
		if err = c.cacheApplicationGroups(); err != nil {
			return agIds, cgID, err
		}
	}
nextName:
	for _, name := range agNames {
		for id, cn := range c.applicationGroups {
			if cn.id == appCtx.AccountID && cn.name == name {
				agIds = append(agIds, models.ObjIDMutable(id))
				continue nextName
			}
		}
		return agIds, cgID, fmt.Errorf("application group '%s' not found for account '%s'", name, appCtx.Account)
	}
	return agIds, cgID, nil
}

// validateDomainClusterNodeNames validates domain, cluster and node names, loads the caches and returns their IDs
func (c *cacheHelper) validateDomainClusterNodeNames(domainName, clusterName, nodeName string) (string, string, string, error) {
	var domainID, clusterID, nodeID string
	if nodeName != "" && clusterName == "" {
		return "", "", "", fmt.Errorf("node-name requires cluster-name and domain")
	}
	if clusterName != "" && domainName == "" {
		return "", "", "", fmt.Errorf("cluster-name requires domain")
	}
	if err := c.loadDCNCaches(); err != nil {
		if domainName == "" && clusterName == "" && nodeName == "" {
			return domainID, clusterID, nodeID, nil
		}
		return "", "", "", err
	}
	if domainName != "" {
		for id, n := range c.cspDomains {
			if n == domainName {
				domainID = id
			}
		}
		if domainID == "" {
			return "", "", "", fmt.Errorf("csp domain '%s' not found", domainName)
		}
	}
	if clusterName != "" {
		for id, cn := range c.clusters {
			if cn.id == string(domainID) && cn.name == clusterName {
				clusterID = id
			}
		}
		if clusterID == "" {
			return "", "", "", fmt.Errorf("cluster '%s' not found in domain '%s'", clusterName, domainName)
		}
	}
	if nodeName != "" {
		for id, cn := range c.nodes {
			if cn.id == clusterID && cn.name == nodeName {
				nodeID = id
			}
		}
		if nodeID == "" {
			return "", "", "", fmt.Errorf("node '%s' not found in cluster '%s'", nodeName, clusterName)
		}
	}
	return domainID, clusterID, nodeID, nil
}

func (c *cacheHelper) lookupProtectionDomainByName(pdName string) (*models.ProtectionDomain, error) {
	for _, o := range c.protectionDomains {
		if string(o.Name) == pdName {
			return o, nil
		}
	}
	return nil, fmt.Errorf("protection domain '%s' not found", pdName)
}

func (c *cacheHelper) lookupProtectionDomainByID(pdID string) (*models.ProtectionDomain, error) {
	if o, found := c.protectionDomains[pdID]; found {
		return o, nil
	}
	return nil, fmt.Errorf("protection domain with ID [%s] not found", pdID)
}

func (c *cacheHelper) validateProtectionDomainName(pdName string) (string, error) {
	var o *models.ProtectionDomain
	var err error
	if err = c.cacheProtectionDomains(); err == nil {
		if o, err = c.lookupProtectionDomainByName(pdName); err == nil {
			return string(o.Meta.ID), nil
		}
	}
	return "", err
}

func (c *cacheHelper) validateDomainName(dName string) (string, error) {
	var err error
	if err = c.cacheCSPDomains(); err == nil {
		for id, n := range c.cspDomains {
			if n == dName {
				return id, nil
			}
		}
		err = fmt.Errorf("csp domain '%s' not found", dName)
	}
	return "", err
}
