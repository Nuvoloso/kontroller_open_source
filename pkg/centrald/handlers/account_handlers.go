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
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// register handlers for Account
func (c *HandlerComp) accountRegisterHandlers() {
	c.app.API.AccountAccountCreateHandler = ops.AccountCreateHandlerFunc(c.accountCreate)
	c.app.API.AccountAccountDeleteHandler = ops.AccountDeleteHandlerFunc(c.accountDelete)
	c.app.API.AccountAccountFetchHandler = ops.AccountFetchHandlerFunc(c.accountFetch)
	c.app.API.AccountAccountListHandler = ops.AccountListHandlerFunc(c.accountList)
	c.app.API.AccountAccountProtectionDomainClearHandler = ops.AccountProtectionDomainClearHandlerFunc(c.accountProtectionDomainClear)
	c.app.API.AccountAccountProtectionDomainSetHandler = ops.AccountProtectionDomainSetHandlerFunc(c.accountProtectionDomainSet)
	c.app.API.AccountAccountSecretResetHandler = ops.AccountSecretResetHandlerFunc(c.accountSecretReset)
	c.app.API.AccountAccountSecretRetrieveHandler = ops.AccountSecretRetrieveHandlerFunc(c.accountSecretRetrieve)
	c.app.API.AccountAccountUpdateHandler = ops.AccountUpdateHandlerFunc(c.accountUpdate)
}

var nmAccountMutable JSONToAttrNameMap

func (c *HandlerComp) accountMutableNameMap() JSONToAttrNameMap {
	if nmAccountMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmAccountMutable == nil {
			nmAccountMutable = c.makeJSONToAttrNameMap(M.AccountMutable{})
		}
	}
	return nmAccountMutable
}

// accountFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// If they do, the properties of the object are filtered based on capabilities.
func (c *HandlerComp) accountFetchFilter(ai *auth.Info, obj *M.Account) error {
	filterRoles := false
	err := ai.CapOK(centrald.AccountFetchAllRolesCap, M.ObjIDMutable(obj.Meta.ID))
	if err != nil {
		if err = ai.CapOK(centrald.AccountFetchOwnRoleCap, M.ObjIDMutable(obj.Meta.ID)); err == nil {
			filterRoles = true
		} else if (ai.RoleObj.Capabilities[centrald.AccountFetchAllRolesCap] || ai.RoleObj.Capabilities[centrald.AccountFetchOwnRoleCap]) && ai.TenantAccountID == string(obj.Meta.ID) {
			// special case: subordinate can view its tenant, TBD may need additional field filtering
			err = nil
			obj.UserRoles = map[string]M.AuthRole{}
		}
	}
	if err != nil {
		if obj.TenantAccountID == "" {
			err = ai.CapOK(centrald.ManageSpecialAccountsCap)
		} else {
			err = ai.CapOK(centrald.ManageNormalAccountsCap, obj.TenantAccountID)
		}
		if err != nil && ai.AccountID == "" {
			// special case: when no account is specified, allow fetching of any account where the user has an enabled role and fetch capability
			if role, exists := obj.UserRoles[ai.UserID]; exists && !role.Disabled {
				if roleObj, e2 := c.DS.OpsRole().Fetch(string(role.RoleID)); e2 == nil {
					if roleObj.Capabilities[centrald.AccountFetchAllRolesCap] {
						filterRoles = false
						err = nil
					} else if roleObj.Capabilities[centrald.AccountFetchOwnRoleCap] {
						filterRoles = true
						err = nil
					}
				}
			}
		}
		if err != nil {
			return err
		}
	}
	if filterRoles {
		role := obj.UserRoles[ai.UserID]
		obj.UserRoles = map[string]M.AuthRole{ai.UserID: role}
	}
	return nil
}

// accountApplyInheritedProperties silently inserts the default snapshot and VSR management policies values from global System object (no update in the DB).
func (c *HandlerComp) accountApplyInheritedProperties(ctx context.Context, obj *M.Account) error {
	inheritSMP := false
	inheritSCP := false
	inheritVMP := false
	if obj.SnapshotManagementPolicy == nil || obj.SnapshotManagementPolicy.Inherited {
		inheritSMP = true
	}
	if obj.SnapshotCatalogPolicy == nil || obj.SnapshotCatalogPolicy.Inherited {
		inheritSCP = true
	}
	if obj.VsrManagementPolicy == nil || obj.VsrManagementPolicy.Inherited {
		inheritVMP = true
	}
	var err error
	if inheritSCP {
		obj.SnapshotCatalogPolicy = &M.SnapshotCatalogPolicy{}

		// The specification of SnapshotCatalogPolicy in the System object has scope over all Account objects in the system unless a narrower scope is present,
		// e.g. first search the Account object identified by the tenantAccountId property,
		// if SnapshotCatalogPolicy is not set there search the System object.
		if obj.TenantAccountID != "" {
			taObj, err := c.DS.OpsAccount().Fetch(ctx, string(obj.TenantAccountID))
			if err == nil && taObj.SnapshotCatalogPolicy != nil {
				inheritSCP = false // no need to fetch System object
				*obj.SnapshotCatalogPolicy = *taObj.SnapshotCatalogPolicy
				obj.SnapshotCatalogPolicy.Inherited = true
			}
		}
	}
	var sysObj *M.System
	if inheritSMP || inheritVMP || inheritSCP {
		sysObj, err = c.DS.OpsSystem().Fetch()
		if err != nil {
			return err
		}
	}
	if inheritSMP {
		obj.SnapshotManagementPolicy = &M.SnapshotManagementPolicy{}
		*obj.SnapshotManagementPolicy = *sysObj.SnapshotManagementPolicy
		obj.SnapshotManagementPolicy.Inherited = true
	}
	if inheritVMP {
		obj.VsrManagementPolicy = &M.VsrManagementPolicy{}
		*obj.VsrManagementPolicy = *sysObj.VsrManagementPolicy
		obj.VsrManagementPolicy.Inherited = true
	}
	if inheritSCP {
		*obj.SnapshotCatalogPolicy = *sysObj.SnapshotCatalogPolicy
		obj.SnapshotCatalogPolicy.Inherited = true
	}
	return nil
}

// validateAccountSnapshotCatalogPolicy is a helper to validate if modifications to existing snapshot catalog policy are allowed.
// When the *inherited* field value for the policy gets set to "true" *all* other policy property fields will be ignored.
func (c *HandlerComp) validateAccountSnapshotCatalogPolicy(ctx context.Context, ai *auth.Info, obj *M.Account, uSCP *M.SnapshotCatalogPolicy) error {
	if uSCP == nil {
		return c.eUpdateInvalidMsg("snapshotCatalogPolicy not specified")
	}
	if uSCP.Inherited {
		return nil // don't check further
	} else if uSCP.CspDomainID == "" || uSCP.ProtectionDomainID == "" {
		return c.eUpdateInvalidMsg("CspDomainID and/or ProtectionDomainID for non-inherited snapshotCatalogPolicy not specified")
	}
	// The Account that defines this policy must be authorized to use the protection store represented by the CSPDomain object specified by the cspDomainId field,
	// and must own the ProtectionDomain object specified by the protectionDomainId field
	if _, err := c.ops.intProtectionDomainFetch(ctx, ai, string(uSCP.ProtectionDomainID)); err != nil { // checks ai.account == pd.account
		return err
	}
	dObj, err := c.ops.intCspDomainFetch(ctx, ai, string(uSCP.CspDomainID))
	if err != nil {
		return err
	}
	if !(dObj.AccountID == M.ObjIDMutable(obj.Meta.ID) || util.Contains(dObj.AuthorizedAccounts, M.ObjIDMutable(obj.Meta.ID))) {
		return c.eUpdateInvalidMsg("account is not authorized to use CSPDomain %s", uSCP.CspDomainID)
	}
	return nil
}

type accountUserMap map[string]*M.User

// validateUserRoles is a helper to validate that User objects exist and that the roles are appropriate for the account.
// System account constraints are checked in the datastore. Assumes that on update, only called for Append and Set cases, not Remove.
// the userCache may be updated as a side effect.
// Caller must hold c.RLock
func (c *HandlerComp) validateUserRoles(ctx context.Context, attr *centrald.UpdateAttr, obj *M.Account, uObj *M.AccountMutable, userCache accountUserMap) error {
	errF := c.eMissingMsg
	roles := obj.UserRoles
	action := centrald.UpdateActionArgs{FromBody: true}
	if uObj != nil {
		errF = c.eUpdateInvalidMsg
		roles = uObj.UserRoles
		action = attr.Actions[centrald.UpdateAppend]
		if !action.FromBody {
			action = attr.Actions[centrald.UpdateSet]
		}
	}
	for k, v := range roles {
		if _, exists := action.Fields[k]; action.FromBody || exists {
			if _, exists = userCache[k]; !exists {
				if userObj, err := c.DS.OpsUser().Fetch(ctx, k); err == nil {
					userCache[k] = userObj
				} else {
					if err == centrald.ErrorNotFound {
						err = errF("invalid userId: %s", k)
					}
					return err
				}
			}
			if !util.Contains(obj.AccountRoles, M.ObjIDMutable(v.RoleID)) {
				return errF("invalid user roleId for account: %s", v.RoleID)
			}
		}
	}
	return nil
}

// accountUserRolesAuditMsg is a helper to generate a message to be included in the audit log about the changes to the userRoles.
// Assumes validation and update/create were already performed. On creation, oObj must be nil.
// Caller must hold c.RLock
func (c *HandlerComp) accountUserRolesAuditMsg(oObj *M.Account, uObj *M.Account, userCache accountUserMap) string {
	// when there are multiple account roles, flag users with admin role in the summary message (all admins have AccountUpdate capability)
	adminRoles := map[M.ObjID]struct{}{}
	if len(uObj.AccountRoles) > 1 {
		roleList, err := c.DS.OpsRole().List(role.RoleListParams{})
		if err != nil { // unlikely, just warn
			c.Log.Warningf("Account [%s] failed to list roles: %s", uObj.Meta.ID, err.Error())
			roleList = nil
		}
		for _, role := range roleList {
			if ok, _ := role.Capabilities[centrald.AccountUpdateCap]; ok {
				adminRoles[role.Meta.ID] = struct{}{}
			}
		}
	}
	type uList struct { // list in a struct so we can get a pointer to one of several and modify the list
		l []string
	}
	addList := &uList{}
	remList := &uList{}
	enaList := &uList{}
	disList := &uList{}
	setList := &uList{}
	for k, v := range uObj.UserRoles {
		modList := setList
		if v.Disabled {
			modList = disList
		}
		if oObj != nil {
			if oldRole, exists := oObj.UserRoles[k]; exists {
				if oldRole.RoleID != v.RoleID {
					if modList != disList {
						modList = addList // added to new role
					}
				} else if oldRole.Disabled == v.Disabled {
					continue // no change for this user
				} else if !v.Disabled {
					modList = enaList
				}
			} else {
				modList = addList // new user
			}
		}
		admin := ""
		if _, exists := adminRoles[M.ObjID(v.RoleID)]; exists {
			admin = "(A)"
		}
		userObj, _ := userCache[k] // cache must be fully populated
		modList.l = append(modList.l, fmt.Sprintf("%s%s", userObj.AuthIdentifier, admin))
	}
	if oObj != nil {
		// find any users that were removed
		for k, v := range oObj.UserRoles {
			if _, exists := uObj.UserRoles[k]; !exists {
				admin := ""
				if _, exists := adminRoles[M.ObjID(v.RoleID)]; exists {
					admin = "(A)"
				}
				userObj, _ := userCache[k] // cache must be fully populated
				remList.l = append(remList.l, fmt.Sprintf("%s%s", userObj.AuthIdentifier, admin))
			}
		}
	}
	retList := []string{}
	if len(setList.l) > 0 {
		sort.Strings(setList.l)
		retList = append(retList, strings.Join(setList.l, ", "))
	}
	if len(addList.l) > 0 {
		sort.Strings(addList.l)
		retList = append(retList, fmt.Sprintf("added: %s", strings.Join(addList.l, ", ")))
	}
	if len(remList.l) > 0 {
		sort.Strings(remList.l)
		retList = append(retList, fmt.Sprintf("removed: %s", strings.Join(remList.l, ", ")))
	}
	if len(enaList.l) > 0 {
		sort.Strings(enaList.l)
		retList = append(retList, fmt.Sprintf("enabled: %s", strings.Join(enaList.l, ", ")))
	}
	if len(disList.l) > 0 {
		sort.Strings(disList.l)
		retList = append(retList, fmt.Sprintf("disabled: %s", strings.Join(disList.l, ", ")))
	}
	ret := ""
	if len(retList) > 0 {
		ret = fmt.Sprintf("userRoles[%s]", strings.Join(retList, "; "))
	}
	return ret
}

func (c *HandlerComp) validateAccountSecretScope(scope string) bool {
	switch scope {
	case common.AccountSecretScopeGlobal:
	case common.AccountSecretScopeCspDomain:
	case common.AccountSecretScopeCluster:
	default:
		return false
	}
	return true
}

func accountSecretMapValuesByScope(clID, domID string) map[string]string {
	return map[string]string{
		common.AccountSecretScopeGlobal:    ",",
		common.AccountSecretScopeCspDomain: "," + domID,
		common.AccountSecretScopeCluster:   clID + "," + domID,
	}
}

func accountSecretMapValueParse(sv string) (string, string) {
	parts := strings.SplitN(sv, ",", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

// accountSecretMatch searches for the secret in the object.
func (c *HandlerComp) accountSecretMatch(obj *M.Account, secret, clID, domID string) bool {
	if obj.Secrets != nil {
		svm := accountSecretMapValuesByScope(clID, domID)
		svC := svm[common.AccountSecretScopeCluster]
		svD := svm[common.AccountSecretScopeCspDomain]
		svG := svm[common.AccountSecretScopeGlobal]
		if val, ok := obj.Secrets[secret]; ok && (val == svC || val == svD || val == svG) {
			return true // ordered CLUSTER, DOMAIN, or GLOBAL match
		}
	}
	return false
}

// accountSecretDriverOps is an interface with callbacks used by the accountSecretDriver
type accountSecretDriverOps interface {
	// PreUpdateCheck is invoked to view the account object fetched by the driver,
	// perform authorization checks or load additional objects as necessary.
	// It can return an error to force termination.
	PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *M.Account) error
	// UpdateSecrets is invoked to modify the account secrets. It returns true if modified.
	UpdateSecrets(aObj *M.Account) bool
	// Object returns an operation specific pointer to support unit testing.
	Object() interface{}
}

// accountSecretDriver encapsulates the common functionality needed to update the Secrets map.
//
// The invoker must hold the existential lock (c.mux); internally c.accountMux will be acquired
// to serialize against any concurrent Account object modification.  This is necessary because while
// the driver uses a versioned update there is no retry in case of a version mismatch error.
func (c *HandlerComp) accountSecretDriver(ctx context.Context, ai *auth.Info, id string, op accountSecretDriverOps) error {
	c.accountMux.Lock()
	defer c.accountMux.Unlock()
	aObj, err := c.ops.intAccountFetch(ctx, ai, id)
	if err != nil {
		return err
	}
	if aObj.Secrets == nil {
		aObj.Secrets = map[string]string{}
	}
	if err = op.PreUpdateCheck(ctx, ai, aObj); err != nil {
		return err
	}
	if op.UpdateSecrets(aObj) { // returns true if update needed
		version := int32(aObj.Meta.Version)
		params := ops.NewAccountUpdateParams()
		params.ID = string(aObj.Meta.ID)
		params.Version = swag.Int32(version)
		params.Set = []string{"secrets"}
		params.Payload = &M.AccountMutable{
			Secrets: aObj.Secrets,
		}
		ai = &auth.Info{} // internal access needed to modify secret map
		req := &http.Request{}
		params.HTTPRequest = req.WithContext(ctx)
		// passing aObj to update indicates that all locks already obtained before call
		if _, err := c.ops.intAccountUpdate(params, ai, aObj); err != nil {
			return err
		}
	}
	return nil
}

// accountSecretRetriever supports secret creation and retrieval.
// It satisfies the accountSecretDriverOps interface.
type accountSecretRetriever struct {
	c           *HandlerComp
	clusterID   string
	clusterObj  *M.Cluster
	secretKey   string
	secretValue string
	scope       string
}

func (h *accountSecretRetriever) Object() interface{} {
	return h
}

func (h *accountSecretRetriever) PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *M.Account) error {
	if h.clusterObj == nil {
		c := h.c
		var err error
		h.clusterObj, err = c.ops.intClusterFetch(ctx, ai, h.clusterID)
		if err != nil {
			return c.eMissingMsg("clusterId")
		}
	}
	if h.c.accountProtectionDomainForProtectionStore(aObj, h.clusterObj.CspDomainID) == "" {
		return h.c.eInvalidState("no protection domain for cluster protection store")
	}
	return nil
}

func (h *accountSecretRetriever) UpdateSecrets(aObj *M.Account) bool {
	svm := accountSecretMapValuesByScope(string(h.clusterObj.Meta.ID), string(h.clusterObj.CspDomainID))
	svC := svm[common.AccountSecretScopeCluster]
	svD := svm[common.AccountSecretScopeCspDomain]
	svG := svm[common.AccountSecretScopeGlobal]
	findBySecretValue := func(desiredValue string) (string, bool) {
		for key, value := range aObj.Secrets {
			if desiredValue == value {
				return key, true
			}
		}
		return "", false
	}
	for _, sv := range []string{svC, svD, svG} { // scope ordered searches
		if sec, found := findBySecretValue(sv); found {
			h.secretKey = sec
			return false
		}
	}
	// Create a new secret
	// - Key is the secret. This makes account lookup by secret efficient.
	// - Value is "[clusterID],[cspDomainID]" to support scope validation on reset.
	h.secretKey = uuidGenerator()
	h.scope = h.clusterObj.ClusterUsagePolicy.AccountSecretScope
	h.secretValue = svm[h.scope]
	aObj.Secrets[h.secretKey] = h.secretValue
	return true
}

// closure to support resetting secrets
type accountSecretResetter struct {
	c          *HandlerComp
	scope      string
	recursive  bool
	clID       string
	domID      string
	aObj       *M.Account
	numDeleted int
}

func (h *accountSecretResetter) Object() interface{} {
	return h
}

func (h *accountSecretResetter) PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *M.Account) error {
	c := h.c
	var err error
	if err = c.accountUpdateAuthCheck(ctx, ai, aObj); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.AccountUpdateAction, aObj.Meta.ID, aObj.Name, "", true, "Unauthorized attempt to reset secrets")
		return err
	}
	// ensure that the scope object properties are present; no need to actually validate the value
	switch h.scope {
	case common.AccountSecretScopeCspDomain:
		if h.domID == "" {
			return c.eMissingMsg("cspDomainId")
		}
	case common.AccountSecretScopeCluster:
		if h.clID == "" {
			return c.eMissingMsg("clusterId")
		}
	}
	return nil
}

func (h *accountSecretResetter) UpdateSecrets(aObj *M.Account) bool {
	h.aObj = aObj
	svm := accountSecretMapValuesByScope(h.clID, h.domID)
	svG := svm[common.AccountSecretScopeGlobal]
	var matcher func(v, cv, dv string) bool
	switch {
	case h.scope == common.AccountSecretScopeGlobal && h.recursive:
		h.numDeleted = len(aObj.Secrets)
		aObj.Secrets = nil // reset all
		return true
	case h.scope == common.AccountSecretScopeGlobal:
		matcher = func(v, cv, dv string) bool { return v == svG }
	case h.scope == common.AccountSecretScopeCspDomain && h.recursive:
		matcher = func(v, cv, dv string) bool { return dv == h.domID }
	case h.scope == common.AccountSecretScopeCspDomain:
		matcher = func(v, cv, dv string) bool { return dv == h.domID && cv == "" } // exact match
	case h.scope == common.AccountSecretScopeCluster:
		matcher = func(v, cv, dv string) bool { return cv == h.clID }
	}
	keysToDelete := []string{}
	for k, v := range aObj.Secrets {
		cv, dv := accountSecretMapValueParse(v)
		if matcher(v, cv, dv) {
			keysToDelete = append(keysToDelete, k)
		}
	}
	h.numDeleted = len(keysToDelete)
	if h.numDeleted > 0 {
		for _, k := range keysToDelete {
			delete(aObj.Secrets, k)
		}
		return true
	}
	return false
}

func (c *HandlerComp) accountReferencesProtectionDomain(aObj *M.Account, pdID string) bool {
	for _, id := range aObj.ProtectionDomains {
		if string(id) == pdID {
			return true
		}
	}
	return false
}

func (c *HandlerComp) accountProtectionDomainForProtectionStore(aObj *M.Account, cspID M.ObjIDMutable) M.ObjIDMutable {
	var retID M.ObjIDMutable
	if len(aObj.ProtectionDomains) > 0 {
		if pdID, found := aObj.ProtectionDomains[string(cspID)]; found {
			retID = pdID
		} else {
			retID = aObj.ProtectionDomains[common.ProtectionStoreDefaultKey]
		}
	}
	return retID
}

type accountProtectionDomainsDriverOps interface {
	// PreUpdateCheck is invoked to view the account object fetched by the driver,
	// perform additional authorization checks or load additional objects as necessary.
	// It can return an error to force termination.
	PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *M.Account) error
	// UpdateProtectionDomains is invoked to modify the account protectionDomains property. It returns true if modified.
	UpdateProtectionDomains(aObj *M.Account) bool
}

var errAccountProtectionDriverNoChange = fmt.Errorf("no-change-in-protection-domain")

func (c *HandlerComp) accountProtectionDomainsDriver(ctx context.Context, ai *auth.Info, accountID string, version int32, op accountProtectionDomainsDriverOps) (*M.Account, error) {
	c.RLock()
	defer c.RUnlock()
	c.accountMux.Lock()
	defer c.accountMux.Unlock()
	var err error
	if err = c.app.AuditLog.Ready(); err != nil {
		return nil, err
	}
	aObj, err := c.ops.intAccountFetch(ctx, ai, accountID)
	if err != nil {
		return nil, err
	}
	if version > 0 && int32(aObj.Meta.Version) != version {
		return nil, centrald.ErrorIDVerNotFound
	}
	if aObj.ProtectionDomains == nil {
		aObj.ProtectionDomains = make(map[string]M.ObjIDMutable)
	}
	if err = c.accountUpdateAuthCheck(ctx, ai, aObj); err == nil {
		if err = ai.CapOK(centrald.ProtectionDomainManagementCap); err == nil { // preempt based on cap only
			err = op.PreUpdateCheck(ctx, ai, aObj)
		}
	}
	if err != nil {
		if e, ok := err.(*centrald.Error); ok && strings.HasPrefix(e.M, common.ErrorUnauthorizedOrForbidden) {
			c.app.AuditLog.Post(ctx, ai, centrald.AccountUpdateAction, aObj.Meta.ID, aObj.Name, "", true, "Unauthorized protection domain update")
		}
		return nil, err
	}
	if !op.UpdateProtectionDomains(aObj) {
		return aObj, errAccountProtectionDriverNoChange
	}
	// now update the account using internal access
	ai = &auth.Info{}
	uParams := ops.NewAccountUpdateParams()
	uParams.ID = string(aObj.Meta.ID)
	uParams.Version = swag.Int32(int32(aObj.Meta.Version))
	uParams.Set = []string{"protectionDomains"}
	uParams.Payload = &M.AccountMutable{ProtectionDomains: aObj.ProtectionDomains}
	req := &http.Request{}
	uParams.HTTPRequest = req.WithContext(ctx)
	// passing aObj to update indicates that all locks already obtained before call
	return c.ops.intAccountUpdate(uParams, ai, aObj)
}

type accountPDSetter struct {
	c        *HandlerComp
	pdID     string
	cspID    string
	key      string
	aObj     *M.Account
	cspObj   *M.CSPDomain
	pdObj    *M.ProtectionDomain
	auditMsg string
}

func (h *accountPDSetter) PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *M.Account) error {
	h.aObj = aObj
	pdObj, err := h.c.ops.intProtectionDomainFetch(ctx, ai, h.pdID) // checks ai.account == pd.account
	if err != nil {
		return err
	}
	h.pdObj = pdObj
	if aObj.Meta.ID != M.ObjID(pdObj.AccountID) {
		h.auditMsg = "Protection domain used in wrong account"
		return h.c.eUnauthorizedOrForbidden("invalid protectionDomainId usage")
	}
	if h.cspID != "" {
		cspObj, err := h.c.ops.intCspDomainFetch(ctx, ai, h.cspID)
		if err != nil {
			if err == centrald.ErrorUnauthorizedOrForbidden {
				return h.c.eUnauthorizedOrForbidden("invalid cspDomainId")
			}
			return h.c.eInvalidData("invalid cspDomainId: %s", err.Error())
		}
		h.cspObj = cspObj
	}
	return nil
}

func (h *accountPDSetter) UpdateProtectionDomains(aObj *M.Account) bool {
	h.key = common.ProtectionStoreDefaultKey
	h.auditMsg = "Set as DEFAULT"
	if h.cspID != "" {
		h.key = h.cspID
		h.auditMsg = fmt.Sprintf("Set for domain '%s'", h.cspObj.Name)
	}
	value := M.ObjIDMutable(h.pdID)
	if v, found := aObj.ProtectionDomains[h.key]; found && v == value {
		return false
	}
	aObj.ProtectionDomains[h.key] = value
	return true
}

type accountPDClearer struct {
	c        *HandlerComp
	cspID    string
	key      string
	aObj     *M.Account
	pdID     string
	pdObj    *M.ProtectionDomain
	auditMsg string
}

func (h *accountPDClearer) PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *M.Account) error {
	h.aObj = aObj
	// check if key in map
	key := common.ProtectionStoreDefaultKey
	if h.cspID != "" {
		key = h.cspID
	}
	pdID, inMap := aObj.ProtectionDomains[key]
	if !inMap {
		return nil
	}
	h.key = key
	h.pdID = string(pdID)
	// load the PD (for audit/auth)
	pdObj, err := h.c.ops.intProtectionDomainFetch(ctx, ai, h.pdID)
	if err != nil {
		return err
	}
	h.pdObj = pdObj
	// find all authorized CSPDomains
	authorizedDoms, err := h.findAuthorizedCspDomains(ctx)
	if err != nil {
		return err
	}
	// construct the audit message
	if h.cspID == "" { // default cleared
		h.auditMsg = "DEFAULT unset"
	} else {
		h.auditMsg = fmt.Sprintf("Unset for domain [%s]", h.cspID)
		for _, dObj := range authorizedDoms {
			if string(dObj.Meta.ID) == h.cspID {
				h.auditMsg = fmt.Sprintf("Unset for domain '%s'", dObj.Name)
			}
		}
	}
	// check for bound volumes if necessary
	domsToCheck := []string{}
	if h.cspID == "" { // default cleared
		// authorized cspDomains not in the map must not have bound volumes
		for _, dObj := range authorizedDoms {
			if _, inMap = aObj.ProtectionDomains[string(dObj.Meta.ID)]; !inMap {
				domsToCheck = append(domsToCheck, string(dObj.Meta.ID))
			}
		}
	} else { // explicit dom cleared
		if _, hasDefault := aObj.ProtectionDomains[common.ProtectionStoreDefaultKey]; !hasDefault {
			domsToCheck = append(domsToCheck, h.cspID)
		}
	}
	for _, domID := range domsToCheck {
		hasBound, err := h.checkForVsBoundToDomain(ctx, domID)
		if err != nil {
			return err
		}
		if hasBound {
			return h.c.eRequestInConflict("volumeSeries are impacted by this operation")
		}
	}
	return nil
}

// list of all CSPDomain objects that have this account authorized; no post-processing done as only ids required
func (h *accountPDClearer) findAuthorizedCspDomains(ctx context.Context) ([]*M.CSPDomain, error) {
	cspLP := csp_domain.CspDomainListParams{}
	cspLP.AuthorizedAccountID = swag.String(string(h.aObj.Meta.ID))
	return h.c.DS.OpsCspDomain().List(ctx, cspLP)
}

func (h *accountPDClearer) checkForVsBoundToDomain(ctx context.Context, domID string) (bool, error) {
	vsLP := volume_series.VolumeSeriesListParams{}
	vsLP.AccountID = swag.String(string(h.aObj.Meta.ID))
	vsLP.BoundCspDomainID = swag.String(domID)
	num, err := h.c.DS.OpsVolumeSeries().Count(ctx, vsLP, 1)
	h.c.Log.Debugf("Account[%s] domain [%s] bound query result: %d %v", h.aObj.Meta.ID, domID, num, err)
	return num > 0, err
}

func (h *accountPDClearer) UpdateProtectionDomains(aObj *M.Account) bool {
	if h.key == "" {
		return false
	}
	delete(aObj.ProtectionDomains, h.key)
	return true
}

// Handlers

// verify either tenant or System account have the SnapshotCatalogPolicy set
func (c *HandlerComp) checkForAvailableSnapshotCatalogPolicy(ctx context.Context, tenantAccountID string) bool {
	if tenantAccountID != "" {
		taObj, err := c.DS.OpsAccount().Fetch(ctx, tenantAccountID)
		if err == nil && taObj.SnapshotCatalogPolicy != nil {
			return true
		}
	}
	sysObj, err := c.DS.OpsSystem().Fetch()
	if err == nil && sysObj.SnapshotCatalogPolicy != nil && sysObj.SnapshotCatalogPolicy.CspDomainID != "" && sysObj.SnapshotCatalogPolicy.ProtectionDomainID != "" {
		return true
	}
	return false
}

func (c *HandlerComp) accountCreate(params ops.AccountCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.Name == "" || strings.Contains(string(params.Payload.Name), "/") {
		err := c.eMissingMsg("name")
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.Name == centrald.SystemAccount {
		// Disallow accounts of this name, even if specified with a TenantAccountID
		err := c.eMissingMsg("reserved name: %s", centrald.SystemAccount)
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Lock()
	defer c.Unlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.TenantAccountID != "" {
		err := ai.CapOK(centrald.ManageNormalAccountsCap, params.Payload.TenantAccountID)
		if err == nil {
			var tenant *M.Account
			if tenant, err = c.DS.OpsAccount().Fetch(ctx, string(params.Payload.TenantAccountID)); err == nil && tenant.TenantAccountID != "" {
				err = c.eMissingMsg("tenantAccountId")
			}
		} else {
			c.app.AuditLog.Post(ctx, ai, centrald.AccountCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid tenantAccountId")
			}
			return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	} else if err := ai.CapOK(centrald.ManageSpecialAccountsCap); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.AccountCreateAction, "", params.Payload.Name, "", true, "Create tenant unauthorized")
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if len(params.Payload.AccountRoles) == 0 {
		// for now roles are fixed: fill in the expected values
		roles, err := c.DS.OpsRole().List(role.RoleListParams{})
		if err != nil {
			return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		for _, r := range roles {
			if params.Payload.TenantAccountID == "" {
				if r.Name == centrald.TenantAdminRole {
					params.Payload.AccountRoles = []M.ObjIDMutable{M.ObjIDMutable(r.Meta.ID)}
					break
				}
			} else {
				if r.Name == centrald.AccountAdminRole || r.Name == centrald.AccountUserRole {
					params.Payload.AccountRoles = append(params.Payload.AccountRoles, M.ObjIDMutable(r.Meta.ID))
				}
			}
		}
	} else {
		err := c.eMissingMsg("accountRoles should be empty")
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	userCache := accountUserMap{}
	if err := c.validateUserRoles(ctx, nil, params.Payload, nil, userCache); err != nil {
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.SnapshotCatalogPolicy != nil {
		err := c.eMissingMsg("snapshotCatalogPolicy should be empty")
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsAccount().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewAccountCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	msg := "Created"
	if details := c.accountUserRolesAuditMsg(nil, obj, userCache); details != "" {
		msg += " with " + details
	}
	c.app.AuditLog.Post(ctx, ai, centrald.AccountCreateAction, obj.Meta.ID, obj.Name, "", false, msg)
	c.Log.Infof("Account %s created [%s]", obj.Name, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	err = c.accountApplyInheritedProperties(ctx, obj)
	if err != nil {
		c.Log.Warningf("Failed to retrieve inherited property: %s", err.Error())
	}
	return ops.NewAccountCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) accountDelete(params ops.AccountDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.Account
	if obj, err = c.DS.OpsAccount().Fetch(ctx, params.ID); err == nil {
		if string(obj.Name) == centrald.SystemAccount {
			c.Log.Warning("Attempt to delete the '%s' account", centrald.SystemAccount)
			err = centrald.ErrorUnauthorizedOrForbidden
		} else {
			c.Lock()
			defer c.Unlock()
			if err = c.app.AuditLog.Ready(); err == nil {
				if obj.TenantAccountID == "" {
					err = ai.CapOK(centrald.ManageSpecialAccountsCap)
				} else {
					err = ai.CapOK(centrald.ManageNormalAccountsCap, obj.TenantAccountID)
				}
				if err == nil {
					vrlParams := volume_series_request.VolumeSeriesRequestListParams{AccountID: &params.ID, IsTerminated: swag.Bool(false)}
					var n int
					// TBD update to check for AGs instead of VolumeSeries
					if n, err = c.DS.OpsVolumeSeries().Count(ctx, volume_series.VolumeSeriesListParams{AccountID: &params.ID}, 1); err == nil {
						if n != 0 {
							err = &centrald.Error{M: "volume series are still associated with the account", C: centrald.ErrorExists.C}
						} else if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, vrlParams, 1); err == nil {
							if n != 0 {
								err = &centrald.Error{M: "active volume series requests are still associated with the account", C: centrald.ErrorExists.C}
							} else if n, err = c.DS.OpsCspDomain().Count(ctx, csp_domain.CspDomainListParams{AccountID: &params.ID}, 1); err == nil {
								// CSP Domain check covers all objects that require a CSP Domain: cluster, node, pool, SPA, storage, SR
								if n != 0 {
									err = &centrald.Error{M: "CSP domains are still associated with the account", C: centrald.ErrorExists.C}
								} else if n, err = c.DS.OpsAccount().Count(ctx, ops.AccountListParams{TenantAccountID: &params.ID}, 1); err == nil {
									if n != 0 {
										err = &centrald.Error{M: "accounts are still associated with the tenant account", C: centrald.ErrorExists.C}
									} else if err = c.DS.OpsServicePlan().RemoveAccount(ctx, params.ID); err == nil {
										if err = c.DS.OpsAccount().Delete(ctx, params.ID); err == nil {
											c.app.AuditLog.Post(ctx, ai, centrald.AccountDeleteAction, obj.Meta.ID, obj.Name, "", false, "Deleted")
											c.Log.Infof("Account %s deleted [%s]", obj.Name, obj.Meta.ID)
											c.setDefaultObjectScope(params.HTTPRequest, obj)
											return ops.NewAccountDeleteNoContent()
										}
									}
								}
							}
						}
					}
				} else {
					c.app.AuditLog.Post(ctx, ai, centrald.AccountDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
				}
			}
		}
	}
	return ops.NewAccountDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) accountFetch(params ops.AccountFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intAccountFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewAccountFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj.Secrets = nil // never send over the wire
	return ops.NewAccountFetchOK().WithPayload(obj)
}

// intAccountFetch is the internal sharable part of accountFetch; protect appropriately!
func (c *HandlerComp) intAccountFetch(ctx context.Context, ai *auth.Info, id string) (*M.Account, error) {
	obj, err := c.DS.OpsAccount().Fetch(ctx, id)
	if err == nil {
		if err = c.accountFetchFilter(ai, obj); err == nil {
			err = c.accountApplyInheritedProperties(ctx, obj)
		}
	}
	return obj, err
}

func (c *HandlerComp) accountList(params ops.AccountListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	secretLookup := false
	if params.AccountSecret != nil && *params.AccountSecret != "" {
		if err = ai.InternalOK(); err != nil {
			return ops.NewAccountListDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.ClusterID == nil || *params.ClusterID == "" || params.CspDomainID == nil || *params.CspDomainID == "" {
			err := c.eMissingMsg("accountSecret requires clusterId and cspDomainId")
			return ops.NewAccountListDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		secretLookup = true
	}
	list, err := c.DS.OpsAccount().List(ctx, params)
	if err != nil {
		return ops.NewAccountListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := make([]*M.Account, 0, len(list))
	for _, obj := range list {
		if secretLookup && !c.accountSecretMatch(obj, *params.AccountSecret, *params.ClusterID, *params.CspDomainID) {
			continue
		}
		if c.accountFetchFilter(ai, obj) == nil {
			if c.accountApplyInheritedProperties(ctx, obj) == nil {
				obj.Secrets = nil // never send over the wire
				ret = append(ret, obj)
			}
		}
	}
	return ops.NewAccountListOK().WithPayload(ret)
}

func (c *HandlerComp) accountUpdate(params ops.AccountUpdateParams) middleware.Responder {
	obj, err := c.intAccountUpdate(params, nil, nil)
	if err != nil {
		return ops.NewAccountUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewAccountUpdateOK().WithPayload(obj)
}

// intAccountUpdate is the body of the update handler that can be used in a different context.
// It will obtain the needed locks (mux.RLock, accountMux.Lock) if oObj is nil - see accountSecretDriver for lock usage.
func (c *HandlerComp) intAccountUpdate(params ops.AccountUpdateParams, ai *auth.Info, oObj *M.Account) (*M.Account, error) {
	ctx := params.HTTPRequest.Context()
	var err error
	if ai == nil {
		ai, err = c.GetAuthInfo(params.HTTPRequest)
		if err != nil {
			return nil, err
		}
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.accountMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return nil, err
	}
	if params.Payload == nil {
		return nil, c.eUpdateInvalidMsg("missing payload")
	}
	if oObj == nil {
		c.RLock()
		defer c.RUnlock()
		c.accountMux.Lock()
		defer c.accountMux.Unlock()
		oObj, err = c.DS.OpsAccount().Fetch(ctx, params.ID)
		if err != nil {
			return nil, err
		}
	}
	if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		return nil, centrald.ErrorIDVerNotFound
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return nil, err
	}
	attrs := []string{}
	if ua.IsModified("Name") && oObj.Name != params.Payload.Name {
		if params.Payload.Name == "" {
			return nil, c.eUpdateInvalidMsg("non-empty name is required")
		} else if strings.Contains(string(params.Payload.Name), "/") {
			return nil, c.eUpdateInvalidMsg("'/' not allowed in name")
		} else if params.Payload.Name == centrald.SystemAccount {
			return nil, c.eUpdateInvalidMsg("reserved name: %s", centrald.SystemAccount)
		}
		attrs = append(attrs, "name")
	}
	if ua.IsModified("Messages") || ua.IsModified("Secrets") || ua.IsModified("ProtectionDomains") {
		if err = ai.InternalOK(); err != nil {
			return nil, err
		}
	} else if err = c.accountUpdateAuthCheck(ctx, ai, oObj); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.AccountUpdateAction, oObj.Meta.ID, oObj.Name, "", true, "Update unauthorized")
		return nil, err
	}
	userCache := accountUserMap{}
	if attr := ua.FindUpdateAttr("UserRoles"); attr != nil && attr.IsModified() {
		// prime the userCache with the users in the original object
		for k := range oObj.UserRoles {
			if userObj, err := c.DS.OpsUser().Fetch(ctx, k); err == nil {
				userCache[k] = userObj
			} else {
				return nil, err
			}
		}
		if !attr.Actions[centrald.UpdateRemove].FromBody {
			if err = c.validateUserRoles(ctx, attr, oObj, params.Payload, userCache); err != nil {
				return nil, err
			}
		}
	}
	if ua.IsModified("SnapshotManagementPolicy") {
		if err = c.validateSnapshotManagementPolicy(params.Payload.SnapshotManagementPolicy); err != nil {
			return nil, err
		}
	}
	if ua.IsModified("SnapshotCatalogPolicy") {
		// policy can only be cleared (or inherited) if there exists an effective policy either in the tenant account or in the System object
		if params.Payload.SnapshotCatalogPolicy == nil || params.Payload.SnapshotCatalogPolicy.Inherited {
			if !c.checkForAvailableSnapshotCatalogPolicy(ctx, string(oObj.TenantAccountID)) {
				err = c.eUpdateInvalidMsg("removal of snapshotCatalogPolicy is not allowed if there is no effective policy inherited")
			}
		} else {
			err = c.validateAccountSnapshotCatalogPolicy(ctx, ai, oObj, params.Payload.SnapshotCatalogPolicy)
		}
		if err != nil {
			return nil, err
		}
	}
	if ua.IsModified("VsrManagementPolicy") {
		if err = c.validateVSRManagementPolicy(params.Payload.VsrManagementPolicy); err != nil {
			return nil, err
		}
	}
	obj, err := c.DS.OpsAccount().Update(ctx, ua, params.Payload)
	if err != nil {
		return nil, err
	}
	c.accountApplyInheritedProperties(ctx, obj) // ignore error
	obj.Secrets = nil                           // never send over the wire
	if ua.IsModified("Disabled") && oObj.Disabled != params.Payload.Disabled {
		if params.Payload.Disabled {
			attrs = append(attrs, "disabled")
		} else {
			attrs = append(attrs, "enabled")
		}
	}
	if ua.IsModified("UserRoles") {
		if details := c.accountUserRolesAuditMsg(oObj, obj, userCache); details != "" {
			attrs = append(attrs, details)
		}
	}
	if len(attrs) > 0 {
		msg := fmt.Sprintf("Updated %s", strings.Join(attrs, ", "))
		c.app.AuditLog.Post(ctx, ai, centrald.AccountUpdateAction, M.ObjID(params.ID), obj.Name, "", false, msg)
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return obj, nil
}

// accountUpdateAuthCheck is the auth check done for Account updates
func (c *HandlerComp) accountUpdateAuthCheck(ctx context.Context, ai *auth.Info, oObj *M.Account) error {
	var err error
	if err = ai.CapOK(centrald.AccountUpdateCap, M.ObjIDMutable(oObj.Meta.ID)); err != nil {
		if oObj.TenantAccountID == "" {
			err = ai.CapOK(centrald.ManageSpecialAccountsCap)
		} else {
			err = ai.CapOK(centrald.ManageNormalAccountsCap, oObj.TenantAccountID)
		}
		if err != nil {
			// special case: allow update of any account where the user has an enabled role and update capability
			if role, exists := oObj.UserRoles[ai.UserID]; exists && !role.Disabled {
				if roleObj, e2 := c.DS.OpsRole().Fetch(string(role.RoleID)); e2 == nil && roleObj.Capabilities[centrald.AccountUpdateCap] {
					return nil
				}
			}
		}
	}
	return err
}

func (c *HandlerComp) accountSecretRetrieve(params ops.AccountSecretRetrieveParams) middleware.Responder {
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountSecretRetrieveDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.InternalOK(); err != nil {
		return ops.NewAccountSecretRetrieveDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.ClusterID == "" {
		err = c.eMissingMsg("clusterId")
		return ops.NewAccountSecretRetrieveDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	vt, err := c.intAccountSecretRetrieve(params.HTTPRequest.Context(), ai, params.ID, params.ClusterID, nil)
	if err != nil {
		return ops.NewAccountSecretRetrieveDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewAccountSecretRetrieveOK().WithPayload(vt)
}

// intAccountSecretRetrieve is the internal sharable part of accountSecretRetrieve; protect with c.mux
func (c *HandlerComp) intAccountSecretRetrieve(ctx context.Context, ai *auth.Info, aID string, clID string, clObj *M.Cluster) (*M.ValueType, error) {
	h := &accountSecretRetriever{
		c:          c,
		clusterID:  clID,
		clusterObj: clObj, // may be nil at this time
	}
	err := c.ops.accountSecretDriver(ctx, ai, aID, h)
	if err != nil {
		return nil, err
	}
	// TBD: Audit required?
	if h.scope != "" && h.secretValue != "" {
		c.Log.Debugf("Account [%s]: new secret for cluster %s [%s, %s]", aID, clID, h.scope, h.secretValue)
	} else {
		c.Log.Debugf("Account [%s]: found secret for cluster %s", aID, clID)
	}
	return &M.ValueType{Kind: common.ValueTypeSecret, Value: h.secretKey}, nil
}

func (c *HandlerComp) accountSecretReset(params ops.AccountSecretResetParams) middleware.Responder {
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountSecretResetDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !c.validateAccountSecretScope(params.AccountSecretScope) {
		err = c.eUpdateInvalidMsg("invalid accountSecretScope")
		return ops.NewAccountSecretResetDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	h := &accountSecretResetter{
		c:         c,
		scope:     params.AccountSecretScope,
		recursive: swag.BoolValue(params.Recursive),
		clID:      swag.StringValue(params.ClusterID),
		domID:     swag.StringValue(params.CspDomainID),
	}
	c.RLock()
	defer c.RUnlock()
	err = c.accountSecretDriver(params.HTTPRequest.Context(), ai, params.ID, h)
	if err != nil {
		return ops.NewAccountSecretResetDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	msg := fmt.Sprintf("Secrets reset [Scope=%s, #=%d]", h.scope, h.numDeleted)
	c.Log.Debugf("Account [%s]: %s", params.ID, msg)
	c.app.AuditLog.Post(params.HTTPRequest.Context(), ai, centrald.AccountUpdateAction, M.ObjID(params.ID), h.aObj.Name, "", false, msg)
	return ops.NewAccountSecretResetNoContent()
}

func (c *HandlerComp) accountProtectionDomainSet(params ops.AccountProtectionDomainSetParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountProtectionDomainSetDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	h := &accountPDSetter{
		c:     c,
		pdID:  params.ProtectionDomainID,
		cspID: swag.StringValue(params.CspDomainID),
	}
	o, err := c.accountProtectionDomainsDriver(ctx, ai, params.ID, swag.Int32Value(params.Version), h)
	if err != nil && err != errAccountProtectionDriverNoChange {
		if h.pdObj != nil && h.auditMsg != "" && h.aObj != nil {
			h.c.app.AuditLog.Post(ctx, ai, centrald.ProtectionDomainSetAction, M.ObjID(h.pdID), h.pdObj.Name, M.ObjIDMutable(h.cspID), true, h.auditMsg)
		}
		return ops.NewAccountProtectionDomainSetDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err == nil {
		h.c.app.AuditLog.Post(ctx, ai, centrald.ProtectionDomainSetAction, M.ObjID(h.pdID), h.pdObj.Name, M.ObjIDMutable(h.cspID), false, h.auditMsg)
	}
	return ops.NewAccountProtectionDomainSetOK().WithPayload(o)
}

func (c *HandlerComp) accountProtectionDomainClear(params ops.AccountProtectionDomainClearParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewAccountProtectionDomainClearDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	h := &accountPDClearer{
		c:     c,
		cspID: swag.StringValue(params.CspDomainID),
	}
	o, err := c.accountProtectionDomainsDriver(ctx, ai, params.ID, swag.Int32Value(params.Version), h)
	if err != nil && err != errAccountProtectionDriverNoChange {
		return ops.NewAccountProtectionDomainClearDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err == nil {
		h.c.app.AuditLog.Post(ctx, ai, centrald.ProtectionDomainClearAction, M.ObjID(h.pdID), h.pdObj.Name, M.ObjIDMutable(h.cspID), false, h.auditMsg)
	}
	return ops.NewAccountProtectionDomainClearOK().WithPayload(o)
}
