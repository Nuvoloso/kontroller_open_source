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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	spa "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

func (c *HandlerComp) eCode(err interface{}) int {
	if e, ok := err.(*centrald.Error); ok {
		return e.C
	}
	return http.StatusInternalServerError
}

func (c *HandlerComp) eError(err interface{}) *models.Error {
	var s string
	if e, ok := err.(error); ok {
		s = e.Error()
	} else {
		s = fmt.Sprintf("%s", err)
	}
	return &models.Error{Code: int32(c.eCode(err)), Message: swag.String(s)}
}

// eExists produces and logs an extended error message
func (c *HandlerComp) eExists(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorExists.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorExists.C}
}

// eInternalError produces and logs an extended error message
func (c *HandlerComp) eInternalError(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorInternalError.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorInternalError.C}
}

// eInvalidData produces and logs an extended error message
func (c *HandlerComp) eInvalidData(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorInvalidData.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorInvalidData.C}
}

// eInvalidData produces and logs an extended error message
func (c *HandlerComp) eInvalidState(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorInvalidState.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorInvalidState.C}
}

// eMissingMsg produces and logs an extended error message
func (c *HandlerComp) eMissingMsg(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorMissing.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorMissing.C}
}

// eErrorNotFound produces and logs an extended error message
func (c *HandlerComp) eErrorNotFound(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorNotFound.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorNotFound.C}
}

// eUpdateInvalidMsgMsg produces and logs an extended error message
func (c *HandlerComp) eUpdateInvalidMsg(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorUpdateInvalidRequest.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorUpdateInvalidRequest.C}
}

// eRequestInConflict produces and logs an extended error message
func (c *HandlerComp) eRequestInConflict(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorRequestInConflict.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorRequestInConflict.C}
}

// eUnauthorizedOrForbidden produces and logs an extended error message
func (c *HandlerComp) eUnauthorizedOrForbidden(format string, args ...interface{}) error {
	msg := fmt.Sprintf(centrald.ErrorUnauthorizedOrForbidden.M+": "+format, args...)
	c.Log.Error(msg)
	return &centrald.Error{M: msg, C: centrald.ErrorUnauthorizedOrForbidden.C}
}

// JSONToAttrNameMap contains a mapping of the JSON tag name to the Field name of a structure
type JSONToAttrNameMap map[string]reflect.StructField

// isArray returns true if jName is considered to be an array (directly or indirectly)
func (jnm JSONToAttrNameMap) isArray(jName string) (bool, reflect.Type) {
	sf := jnm[jName] // or panic
	t := sf.Type
	k := t.Kind()
	if k == reflect.Ptr {
		t = sf.Type.Elem()
		k = t.Kind()
	}
	return k == reflect.Array || k == reflect.Slice, t
}

// isStruct returns true if jName is considered to be a struct (directly or indirectly)
func (jnm JSONToAttrNameMap) isStruct(jName string) (bool, reflect.Type) {
	sf := jnm[jName] // or panic
	t := sf.Type
	k := t.Kind()
	if k == reflect.Ptr {
		t = sf.Type.Elem()
		k = t.Kind()
	}
	return k == reflect.Struct, t
}

// isStringTMap returns true if jName is considered to be a map[string]T (directly or indirectly)
func (jnm JSONToAttrNameMap) isStringTMap(jName string) (bool, reflect.Type) {
	sf := jnm[jName] // or panic
	t := sf.Type
	k := t.Kind()
	if k == reflect.Ptr {
		t = sf.Type.Elem()
		k = t.Kind()
	}
	return k == reflect.Map && t.Key().Kind() == reflect.String, t
}

// isMap returns true if jName is considered to be a map
func (jnm JSONToAttrNameMap) isMap(jName string) (bool, reflect.Type) {
	sf := jnm[jName] // or panic
	t := sf.Type
	k := t.Kind()
	if k == reflect.Ptr {
		t = sf.Type.Elem()
		k = t.Kind()
	}
	return k == reflect.Map, t
}

// jName returns the JSON name for a given field name (inverse lookup)
func (jnm JSONToAttrNameMap) jName(fName string) string {
	for jn, sf := range jnm {
		if sf.Name == fName {
			return jn
		}
	}
	panic("unable to find jName for " + fName)
}

var acceptableSortKinds = []reflect.Kind{reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
	reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.Bool}

func (jnm JSONToAttrNameMap) isSortable(jName string) bool {
	sf := jnm[jName] // or panic
	t := sf.Type
	var timeVar strfmt.DateTime
	if t == reflect.TypeOf(timeVar) || t == reflect.TypeOf(&timeVar) {
		return true
	}
	tKind := t.Kind()
	if tKind == reflect.Ptr {
		tKind = t.Elem().Kind()
	}
	return util.Contains(acceptableSortKinds, tKind)
}

func findJSONFields(t reflect.Type, nMap JSONToAttrNameMap) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type.Kind() == reflect.Struct && f.Anonymous {
			findJSONFields(f.Type, nMap) // recurse, assuming no inline
		} else {
			if jt, ok := f.Tag.Lookup("json"); ok {
				el := strings.Split(jt, ",")
				if el[0] != "" {
					nMap[el[0]] = f
				}
			}
		}
	}
}

// create a map of JSON tag name to attribute StructField from a reflect.Type
func (c *HandlerComp) makeJSONToAttrNameMapFromType(t reflect.Type) JSONToAttrNameMap {
	nMap := make(JSONToAttrNameMap)
	findJSONFields(t, nMap)
	return nMap
}

// create a map of JSON tag name to attribute StructField from an object (struct assumed)
func (c *HandlerComp) makeJSONToAttrNameMap(obj interface{}) JSONToAttrNameMap {
	return c.makeJSONToAttrNameMapFromType(reflect.TypeOf(obj))
}

// getStructNameMap returns the name map of the given struct type (assumed), using a cache for efficiency
// in case of named types
func (c *HandlerComp) getStructNameMap(sT reflect.Type) JSONToAttrNameMap {
	n := sT.Name()
	if n == "" { // anonymous
		return c.makeJSONToAttrNameMapFromType(sT)
	}
	k := sT.PkgPath() + "/" + n
	c.cacheMux.Lock()
	defer c.cacheMux.Unlock()
	if fMap, ok := c.structNameCache[k]; ok {
		return fMap
	}
	sTMap := c.makeJSONToAttrNameMapFromType(sT)
	if _, ok := c.structNameCache[k]; !ok {
		c.structNameCache[k] = sTMap
	}
	return sTMap
}

// makeStdUpdateArgs returns an UpdateArgs structure initialized from common update related query parameters:
//  - id
//  - version (optional)
//  - params [][]string with UpdateAction major index
// It requires a mapping of external (JSON tag) names to its StructField.
// It will return an error if there are no changes requested and also the generated ua (for testing)
func (c *HandlerComp) makeStdUpdateArgs(nMap JSONToAttrNameMap, id string, version *int32, params [centrald.NumActionTypes][]string) (*centrald.UpdateArgs, error) {
	// initialize the return structure
	ver := int32(0)
	if version != nil {
		ver = *version
	}
	ua := &centrald.UpdateArgs{
		ID:         id,
		Version:    ver,
		Attributes: make([]centrald.UpdateAttr, len(nMap)),
	}
	i := 0
	for _, sf := range nMap {
		ua.Attributes[i].Name = sf.Name
		for j := range ua.Attributes[i].Actions {
			aa := &ua.Attributes[i].Actions[j]
			aa.Fields = make(map[string]struct{})
			aa.Indexes = make(map[int]struct{})
		}
		i++
	}
	// upAFor(name) returns the ua.Attribute structure for a field name
	upAFor := func(fName string) *centrald.UpdateAttr {
		if a := ua.FindUpdateAttr(fName); a != nil {
			return a
		}
		panic("Could not find upA for " + fName)
	}
	actionNames := map[centrald.UpdateAction]string{
		centrald.UpdateRemove: "remove",
		centrald.UpdateAppend: "append",
		centrald.UpdateSet:    "set",
	}
	var actionName string // current action name
	// parseAttrName returns (name, extension, error)
	parseAttrName := func(actionName, name string) (string, string, error) {
		parts := strings.Split(name, ".")
		switch len(parts) {
		case 1:
			return parts[0], "", nil
		case 2:
			return parts[0], parts[1], nil
		}
		return "", "", c.eUpdateInvalidMsg("%s='%s': invalid name format", actionName, name)
	}
	// parse params
	var sf reflect.StructField
	for i, list := range params {
		action := centrald.UpdateAction(i)
		actionName := actionNames[action]
		for _, an := range list {
			// parse
			n, x, err := parseAttrName(actionName, an)
			if err != nil {
				return nil, err
			}
			var ok bool
			if sf, ok = nMap[n]; !ok {
				return nil, c.eUpdateInvalidMsg("%s='%s': invalid name", actionName, an)
			}
			upA := upAFor(sf.Name)
			upAA := &upA.Actions[action]
			var isArray, isStruct, isStringTMap bool
			var fT reflect.Type
			isArray, _ = nMap.isArray(n)
			var idx int
			if isArray {
				if x != "" {
					if idx, err = strconv.Atoi(x); err != nil || idx < 0 {
						return nil, c.eUpdateInvalidMsg("%s='%s': invalid index", actionName, an)
					}
				}
			} else {
				isStruct, fT = nMap.isStruct(n)
				if !isStruct {
					isStringTMap, _ = nMap.isStringTMap(n)
					if !isStringTMap && x != "" {
						return nil, c.eUpdateInvalidMsg("%s='%s': subfield not supported for datatype", actionName, an)
					}
				} else if x != "" {
					fMap := c.getStructNameMap(fT)
					fSF, ok := fMap[x]
					if !ok {
						return nil, c.eUpdateInvalidMsg("%s='%s': invalid field", actionName, an)
					}
					x = fSF.Name
				}
			}
			// local validation
			if action == centrald.UpdateAppend || action == centrald.UpdateRemove {
				if isArray {
					if x != "" {
						return nil, c.eUpdateInvalidMsg("%s='%s': index not permitted", actionName, an)
					}
					// apply to body
					upAA.FromBody = true
				} else if isStringTMap {
					if x != "" {
						return nil, c.eUpdateInvalidMsg("%s='%s': field not permitted", actionName, an)
					}
					// apply to body
					upAA.FromBody = true
				} else {
					return nil, c.eUpdateInvalidMsg("%s='%s': invalid datatype", actionName, an)
				}
			} else {
				if x != "" {
					if isArray {
						// array element[idx]
						upAA.Indexes[idx] = struct{}{}
					} else if isStringTMap || isStruct {
						// struct or map field x
						upAA.Fields[x] = struct{}{}
					}
				} else {
					// apply to body
					upAA.FromBody = true
				}
			}
		}
	}
	// global validation and check for changes
	for _, upA := range ua.Attributes {
		nonSetChanges := false
		removeChanges := false
		for i, upAA := range upA.Actions {
			action := centrald.UpdateAction(i)
			actionName = actionNames[action]
			if upAA.IsModified() {
				ua.HasChanges = true
				if action != centrald.UpdateSet {
					nonSetChanges = true
				}
				if action == centrald.UpdateRemove {
					removeChanges = true
				}
			}
			if upAA.FromBody && (len(upAA.Fields) > 0 || len(upAA.Indexes) > 0) {
				return nil, c.eUpdateInvalidMsg("%s='%s': both 'name' and 'name.' forms present", actionName, nMap.jName(upA.Name))
			}
		}
		// if set=name[.x] then error if any other action on name
		upAA := upA.Actions[centrald.UpdateSet]
		if upAA.IsModified() && nonSetChanges {
			return nil, c.eUpdateInvalidMsg("set='%s': conflicts with remove or append", nMap.jName(upA.Name))
		}
		// if append=name then error if set in remove
		upAA = upA.Actions[centrald.UpdateAppend]
		if upAA.IsModified() && removeChanges {
			return nil, c.eUpdateInvalidMsg("append='%s': conflicts with remove", nMap.jName(upA.Name))
		}
	}
	if !ua.HasChanges {
		return ua, centrald.ErrorUpdateInvalidRequest
	}
	return ua, nil
}

// mergeAttributes merges updates into original based on the UpdateAttr, returning the merged attributes
// The original attributes are returned if either the updates or the UpdateAttr are nil
// An error can be returned if the merge fails (e.g. a set field operation but no such attribute exists in the updates)
func (c *HandlerComp) mergeAttributes(a *centrald.UpdateAttr, jName string, original, updates map[string]models.ValueType) (map[string]models.ValueType, error) {
	if a == nil || updates == nil {
		return original, nil
	}
	attrs := make(map[string]models.ValueType)
	for k, v := range original {
		attrs[k] = v
	}
	setAction := a.Actions[centrald.UpdateSet]
	if setAction.FromBody {
		// just use updated attributes
		attrs = updates
	} else if len(setAction.Fields) > 0 {
		// merge only specified map entries
		for k := range setAction.Fields {
			if v, exists := updates[k]; exists {
				attrs[k] = v
			} else {
				return nil, c.eUpdateInvalidMsg("set='%s.%s': key not found", jName, k)
			}
		}
	} else if a.Actions[centrald.UpdateAppend].FromBody {
		// merge all map entries found in the params
		for k, v := range updates {
			attrs[k] = v
		}
	} else if a.Actions[centrald.UpdateRemove].FromBody {
		// remove all map entries found in the params
		for k := range updates {
			delete(attrs, k)
		}
	}
	return attrs, nil
}

const (
	stgCostChgFmt = "Storage [%s] cost changed from [%g] to [%g]"
	stgCostSetFmt = "Storage [%s] cost set to [%g]"
)

// mergeStorageCosts merges updates into original based on the UpdateAttr, returning the merged attributes
// The original attributes are returned if either the updates or the UpdateAttr are nil
// An error can be returned if the merge fails (e.g. a set field operation but no such attribute exists in the updates)
func (c *HandlerComp) mergeStorageCosts(a *centrald.UpdateAttr, jName string, original, updates map[string]models.StorageCost) (map[string]models.StorageCost, []string, error) {
	auditMsgs := []string{}
	if a == nil || updates == nil {
		return original, auditMsgs, nil
	}
	setAction := a.Actions[centrald.UpdateSet]
	merged := make(map[string]models.StorageCost)
	if !setAction.FromBody {
		for k, v := range original {
			merged[k] = v
		}
	}
	storageCostChanged := func(st string, nv models.StorageCost) {
		var msg string
		if pv, found := original[st]; found {
			msg = fmt.Sprintf(stgCostChgFmt, st, pv.CostPerGiB, nv.CostPerGiB)
		} else {
			msg = fmt.Sprintf(stgCostSetFmt, st, nv.CostPerGiB)
		}
		auditMsgs = append(auditMsgs, msg)
		merged[st] = nv
	}
	storageCostCleared := func(st string) {
		delete(merged, st)
		if pv, found := original[st]; found { // audit only if present
			auditMsgs = append(auditMsgs, fmt.Sprintf(stgCostChgFmt, st, pv.CostPerGiB, float64(0.0)))
		}
	}
	if setAction.FromBody {
		// just use updated attributes
		for st, nv := range updates {
			storageCostChanged(st, nv)
		}
	} else if len(setAction.Fields) > 0 {
		// merge only specified map entries
		for k := range setAction.Fields {
			if v, exists := updates[k]; exists {
				storageCostChanged(k, v)
			} else {
				return nil, nil, fmt.Errorf("storageCosts: set='%s.%s': key not found", jName, k)
			}
		}
	} else if a.Actions[centrald.UpdateAppend].FromBody {
		// merge all map entries found in the params
		for k, v := range updates {
			storageCostChanged(k, v)
		}
	} else if a.Actions[centrald.UpdateRemove].FromBody {
		// remove all map entries found in the params
		for k := range updates {
			storageCostCleared(k)
		}
	}
	return merged, auditMsgs, nil
}

// validateStorageCosts checks the validity of a storageCost map for a given type of CSP
func (c *HandlerComp) validateStorageCosts(costs map[string]models.StorageCost, cspDomainType string) error {
	for k, v := range costs {
		if stObj := c.app.AppCSP.GetCspStorageType(models.CspStorageType(k)); stObj == nil || string(stObj.CspDomainType) != cspDomainType {
			return fmt.Errorf("storageCosts: invalid storage type %s", k)
		}
		if v.CostPerGiB < 0.0 {
			return fmt.Errorf("storageCosts: invalid cost for storage type %s", k)
		}
	}
	return nil
}

// validateApplicationGroupIds performs existence and authorization for a list of application groups
// returning their corresponding names on success.
// Caller must hold c.RLock
func (c *HandlerComp) validateApplicationGroupIds(ctx context.Context, accountID models.ObjIDMutable, agIDs []models.ObjIDMutable) ([]string, error) {
	uniqueIds := map[models.ObjIDMutable]struct{}{}
	names := make([]string, len(agIDs))
	for i, id := range agIDs {
		if _, exists := uniqueIds[id]; exists {
			return nil, c.eMissingMsg("applicationGroupIds must be unique: %s", id)
		}
		uniqueIds[id] = struct{}{}
		if agObj, err := c.DS.OpsApplicationGroup().Fetch(ctx, string(id)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid applicationGroupId: %s", id)
			}
			return nil, err
		} else if agObj.AccountID != accountID {
			return nil, &centrald.Error{M: fmt.Sprintf("applicationGroup %s is not available to the account", id), C: centrald.ErrorUnauthorizedOrForbidden.C}
		} else {
			names[i] = string(agObj.Name)
		}
	}
	return names, nil
}

// authAccountValidator interface
type authAccountValidator interface {
	validateAuthorizedAccountsUpdate(ctx context.Context, ai *auth.Info, action centrald.AuditAction, objID string, objName models.ObjName, a *centrald.UpdateAttr, old, upd []models.ObjIDMutable) (string, error)
}

// validateAuthorizedAccountsUpdate determines if an authorized account list update is allowed. A summary message is returned on success.
// Caller must hold c.RLock
func (c *HandlerComp) validateAuthorizedAccountsUpdate(ctx context.Context, ai *auth.Info, action centrald.AuditAction, objID string, objName models.ObjName, a *centrald.UpdateAttr, old, upd []models.ObjIDMutable) (string, error) {
	toAdd := map[models.ObjIDMutable]struct{}{}
	toRemove := map[models.ObjIDMutable]struct{}{}
	inOld := map[models.ObjIDMutable]struct{}{}
	for _, id := range old {
		inOld[id] = struct{}{}
	}
	if a.Actions[centrald.UpdateAppend].FromBody {
		for _, id := range upd {
			if _, exists := inOld[id]; !exists {
				toAdd[id] = struct{}{}
			}
		}
	} else if a.Actions[centrald.UpdateRemove].FromBody {
		for _, id := range upd {
			if _, exists := inOld[id]; exists {
				toRemove[id] = struct{}{}
			}
		}
	} else {
		// 2 passes to determine which accounts are added and removed from the original list.
		// Because the replacement list can be sparse, build up the full new list during the 1st pass
		action := a.Actions[centrald.UpdateSet]
		inNew := map[models.ObjIDMutable]struct{}{}
		for i, id := range upd {
			if _, exists := action.Indexes[i]; action.FromBody || exists {
				inNew[id] = struct{}{}
				if _, exists = inOld[id]; !exists {
					toAdd[id] = struct{}{}
				}
			} else if i < len(old) {
				inNew[old[i]] = struct{}{}
			}
		}
		for _, id := range old {
			if _, exists := inNew[id]; !exists {
				toRemove[id] = struct{}{}
			}
		}
	}
	names := []string{}
	for id := range toAdd {
		aObj, err := c.DS.OpsAccount().Fetch(ctx, string(id))
		if err == nil {
			// tenant admin can authorize/remove their own account or any subordinate account
			if err = ai.CapOK(centrald.CSPDomainManagementCap, id, aObj.TenantAccountID); err != nil {
				c.app.AuditLog.Post(ctx, ai, action, models.ObjID(objID), objName, "", true, "Update unauthorized")
			}
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eUpdateInvalidMsg("invalid accounts: %s", id)
			}
			return "", err
		}
		names = append(names, string(aObj.Name))
	}
	var b bytes.Buffer
	if len(names) > 0 {
		sort.Strings(names)
		fmt.Fprintf(&b, "added[%s]", strings.Join(names, ", "))
	}
	names = []string{}
	for id := range toRemove {
		aObj, err := c.DS.OpsAccount().Fetch(ctx, string(id))
		if err == nil {
			// tenant admin can authorize/remove their own account or any subordinate account
			if err = ai.CapOK(centrald.CSPDomainManagementCap, id, aObj.TenantAccountID); err != nil {
				c.app.AuditLog.Post(ctx, ai, action, models.ObjID(objID), objName, "", true, "Update unauthorized")
			}
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				continue
			}
			return "", err
		}
		spaParams := spa.ServicePlanAllocationListParams{AuthorizedAccountID: swag.String(string(id))}
		switch action {
		case centrald.ClusterUpdateAction:
			spaParams.ClusterID = swag.String(objID)
		case centrald.CspDomainUpdateAction:
			spaParams.CspDomainID = swag.String(objID)
		case centrald.ServicePlanUpdateAction:
			spaParams.ServicePlanID = swag.String(objID)
		default: // future proof
			panic(fmt.Sprintf("unexpected action %d for validateAuthorizedAccountsUpdate", action))
		}
		if n, err := c.DS.OpsServicePlanAllocation().Count(ctx, spaParams, 1); err == nil {
			if n != 0 {
				err = c.eUpdateInvalidMsg("account %s[%s] being used by at least one servicePlanAllocation", aObj.Name, id)
				return "", err
			}
		} else {
			return "", err
		}
		names = append(names, string(aObj.Name))
	}
	if len(names) > 0 {
		if b.Len() > 0 {
			b.WriteRune(' ')
		}
		sort.Strings(names)
		fmt.Fprintf(&b, "removed[%s]", strings.Join(names, ", "))
	}
	return b.String(), nil
}

// addNewObjIDToScopeMap updates the scope map with objectId if version 1
func (c *HandlerComp) addNewObjIDToScopeMap(objP interface{}, m crude.ScopeMap) {
	oV := reflect.ValueOf(objP)
	if oV.Kind() == reflect.Ptr {
		oV = reflect.Indirect(oV)
	}
	metaV := oV.FieldByName("Meta")
	if metaV.Kind() == reflect.Ptr {
		metaV = reflect.Indirect(metaV)
	}
	ver := metaV.FieldByName("Version").Int()
	if ver == 1 {
		m[crude.ScopeMetaID] = metaV.FieldByName("ID").String()
	}
}

// setDefaultObjectScope is a generic object scope context helper to be used in handlers that do not require custom behavior
func (c *HandlerComp) setDefaultObjectScope(r *http.Request, objP interface{}) {
	m := make(map[string]string)
	c.addNewObjIDToScopeMap(objP, m)
	c.app.CrudeOps.SetScope(r, m, objP)
}

// makeAggregationResult converts an Aggregation list into the form expected by the REST API
func (c *HandlerComp) makeAggregationResult(ag []*centrald.Aggregation) []string {
	list := make([]string, len(ag))
	for i, a := range ag {
		list[i] = fmt.Sprintf("%s:%s:%d", a.FieldPath, a.Type, a.Value)
	}
	return list
}

var validVolumeDataRetentionValues = []string{common.VolumeDataRetentionOnDeleteDelete, common.VolumeDataRetentionOnDeleteRetain}

func (c *HandlerComp) validateVolumeDataRetentionOnDelete(value string, permitEmpty bool) bool {
	if value == "" && permitEmpty {
		return true
	}
	return util.Contains(validVolumeDataRetentionValues, value)
}

var validAccountSecretScopesByObj = map[string][]string{
	common.AccountSecretScopeGlobal:    []string{common.AccountSecretScopeGlobal, common.AccountSecretScopeCspDomain, common.AccountSecretScopeCluster},
	common.AccountSecretScopeCspDomain: []string{common.AccountSecretScopeCspDomain, common.AccountSecretScopeCluster},
	common.AccountSecretScopeCluster:   []string{common.AccountSecretScopeCluster},
}

func (c *HandlerComp) validateClusterUsagePolicy(cup *models.ClusterUsagePolicy, objScope string) error {
	if cup == nil {
		return c.eUpdateInvalidMsg("clusterUsagePolicy not specified")
	}
	if cup.Inherited == true {
		return nil // don't check further
	}
	if !c.validateAccountSecretScope(cup.AccountSecretScope) {
		return c.eUpdateInvalidMsg("clusterUsagePolicy: accountSecretScope invalid")
	}
	if !util.Contains(validAccountSecretScopesByObj[objScope], cup.AccountSecretScope) {
		return c.eUpdateInvalidMsg("clusterUsagePolicy: accountSecretScope invalid for object")
	}
	if !c.validateVolumeDataRetentionOnDelete(cup.VolumeDataRetentionOnDelete, false) {
		return c.eUpdateInvalidMsg("clusterUsagePolicy: volumeDataRetentionOnDelete invalid")
	}
	return nil
}

// validateSnapshotManagementPolicy is a helper to validate if modifications to existing snapshot management policy are allowed.
// When the *inherited* field value for the policy gets set to "true" *all* other policy property fields will be ignored.
func (c *HandlerComp) validateSnapshotManagementPolicy(uSMP *models.SnapshotManagementPolicy) error {
	if uSMP == nil {
		return c.eUpdateInvalidMsg("snapshotManagementPolicy not specified")
	}
	if uSMP.Inherited {
		return nil // don't check further
	}
	if swag.Int32Value(uSMP.RetentionDurationSeconds) == 0 && !uSMP.NoDelete {
		return c.eUpdateInvalidMsg("snapshotManagementPolicy: proper RetentionDurationSeconds value should be provided if 'NoDelete' flag is false")
	}
	if !c.validateVolumeDataRetentionOnDelete(uSMP.VolumeDataRetentionOnDelete, true) {
		return c.eUpdateInvalidMsg("snapshotManagementPolicy: invalid volumeDataRetentionOnDelete")
	}
	return nil
}

// validateVSRManagementPolicy is a helper to validate if modifications to existing VSR management policy are allowed.
// When the *inherited* field value for the policy gets set to "true" *all* other policy property fields will be ignored.
func (c *HandlerComp) validateVSRManagementPolicy(uVMP *models.VsrManagementPolicy) error {
	if uVMP == nil {
		return c.eUpdateInvalidMsg("vsrManagementPolicy not specified")
	}
	if uVMP.Inherited {
		return nil // don't check further
	}
	if swag.Int32Value(uVMP.RetentionDurationSeconds) == 0 && !uVMP.NoDelete {
		err := c.eUpdateInvalidMsg("vsrManagementPolicy: proper RetentionDurationSeconds value should be provided if 'NoDelete' flag is false")
		return err
	}
	return nil
}

var validClusterStates = []string{
	common.ClusterStateDeployable,
	common.ClusterStateManaged,
	common.ClusterStateTimedOut,
	common.ClusterStateResetting,
	common.ClusterStateTearDown,
}

// validateClusterState checks the value of the cluster state
func (c *HandlerComp) validateClusterState(state string) bool {
	return util.Contains(validClusterStates, state)
}

var validNodeStates = []string{
	common.NodeStateManaged,
	common.NodeStateTimedOut,
	common.NodeStateTearDown,
}

// validateNodeState checks the value of the node state
func (c *HandlerComp) validateNodeState(state string) bool {
	return util.Contains(validNodeStates, state)
}

func (c *HandlerComp) verifySortKeys(nMap JSONToAttrNameMap, ascKeys []string, dscKeys []string) error {
	parseAttrName := func(name string) (string, string, error) {
		parts := strings.Split(name, ".")
		switch len(parts) {
		case 1:
			return parts[0], "", nil
		case 2:
			return parts[0], parts[1], nil
		}
		return "", "", c.eInvalidData("'%s': invalid name format", name)
	}
	keys := append(ascKeys, dscKeys...)
	for _, key := range keys {
		n, x, err := parseAttrName(key)
		if err != nil {
			return err
		}
		var ok bool
		if _, ok = nMap[n]; !ok {
			return c.eInvalidData("'%s': invalid name", key)
		}
		var isArray, isStruct, isMap bool
		var fT reflect.Type
		isArray, _ = nMap.isArray(n)
		isMap, fT = nMap.isMap(n)
		if isArray || isMap {
			return c.eInvalidData("'%s': sort on arrays or maps unsupported", key)
		}
		isStruct, fT = nMap.isStruct(n)
		if isStruct {
			if x != "" {
				fMap := c.getStructNameMap(fT)
				_, ok := fMap[x]
				if !ok || !fMap.isSortable(x) {
					return c.eInvalidData("'%s': invalid/unsortable field", key)
				}
			} else if !nMap.isSortable(n) {
				return c.eInvalidData("'%s': unsortable field", key)
			}
		} else if !nMap.isSortable(n) {
			return c.eInvalidData("'%s': unsortable field", key)
		}
	}
	return nil
}

func (c *HandlerComp) nodeIsActive(nObj *models.Node) error {
	var err error
	// allow TIMED_OUT because this can be temporary, also service states other than READY can be temporary
	if nObj.State == common.NodeStateTearDown {
		err = c.eInvalidState("invalid node state (%s)", nObj.State)
	}
	return err
}
