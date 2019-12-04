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
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	clusterOps "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appAuth "github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
)

// HandlerComp is used to manage this handler component
type HandlerComp struct {
	app                   *centrald.AppCtx
	Log                   *logging.Logger
	DS                    centrald.DataStore
	mux                   sync.RWMutex // exposed through the centrald.CrudHelper interface and is used to prevent deletion beneath a concurrent operation (caution: does not prevent concurrent modification)
	accountMux            sync.Mutex   // - nested beneath mux.RLock(); serializes concurrent Account object modifications
	clusterMux            sync.Mutex   // - nested beneath mux.RLock(); serializes concurrent Cluster object modifications
	nodeMux               sync.Mutex   // exposed through the centrald.CrudHelper interface and is used to serialize concurrent Node object modifications by clusterHeartbeatTask or handlers
	cacheMux              sync.Mutex   // protects internal caches; can be used in a closure independent of mux
	cntLock, cntRLock     uint32
	cntUnlock, cntRUnlock uint32
	structNameCache       map[string]JSONToAttrNameMap
	authAccountValidator  authAccountValidator
	spaValidator          SpaValidator
	clusterClientMap      map[string]cluster.Client
	ops                   internalOps
}

type internalOps interface {
	accountReferencesProtectionDomain(aObj *M.Account, pdID string) bool
	accountSecretDriver(ctx context.Context, ai *appAuth.Info, id string, op accountSecretDriverOps) error
	constrainBothQueryAccounts(ai *appAuth.Info, accountID *string, accountCap string, tenantAccountID *string, tenantAccountCap string) (*string, *string, bool)
	constrainEitherOrQueryAccounts(ctx context.Context, ai *appAuth.Info, accountID *string, accountCap string, tenantAccountID *string, tenantAccountCap string) (*string, *string, error)
	intAccountFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.Account, error)
	intAccountSecretRetrieve(ctx context.Context, ai *appAuth.Info, aID string, clID string, clObj *M.Cluster) (*M.ValueType, error)
	intAccountUpdate(params account.AccountUpdateParams, ai *appAuth.Info, oObj *M.Account) (*M.Account, error)
	intClusterFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.Cluster, error)
	intClusterUpdate(params clusterOps.ClusterUpdateParams, ai *appAuth.Info, oObj *M.Cluster) (*M.Cluster, error)
	intConsistencyGroupFetch(ctx context.Context, ai *appAuth.Info, id string, aObj *M.Account) (*M.ConsistencyGroup, error)
	intCspCredentialFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.CSPCredential, error)
	intCspDomainFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.CSPDomain, error)
	intProtectionDomainFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.ProtectionDomain, error)
	intServicePlanAllocationFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.ServicePlanAllocation, error)
	intServicePlanFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.ServicePlan, error)
	intSnapshotFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.Snapshot, error)
	intSnapshotFetchData(ctx context.Context, ai *appAuth.Info, id string) (*M.SnapshotData, error)
	intSystemHostnameFetch(ctx context.Context) (string, error)
	intSystemFetch() (*M.System, error)
}

func newHandlerComp() *HandlerComp {
	hc := &HandlerComp{}
	hc.structNameCache = make(map[string]JSONToAttrNameMap)
	hc.authAccountValidator = hc
	hc.spaValidator = hc
	hc.ops = hc // self reference
	return hc
}

func init() {
	centrald.AppRegisterComponent(newHandlerComp())
}

// Init registers handlers for this component
func (c *HandlerComp) Init(app *centrald.AppCtx) {
	c.app = app
	c.Log = app.Log
	c.DS = app.DS

	app.CrudHelpers = c // provide the centrald.AppCrudHelpers interface

	c.accountRegisterHandlers()
	c.applicationGroupRegisterHandlers()
	c.clusterRegisterHandlers()
	c.consistencyGroupRegisterHandlers()
	c.cspCredentialRegisterHandlers()
	c.cspDomainRegisterHandlers()
	c.cspStorageTypesRegisterHandlers()
	c.debugRegisterHandlers()
	c.nodeRegisterHandlers()
	c.poolRegisterHandlers()
	c.protectionDomainRegisterHandlers()
	c.roleRegisterHandlers()
	c.servicePlanAllocationRegisterHandlers()
	c.servicePlanRegisterHandlers()
	c.sloRegisterHandlers()
	c.snapshotRegisterHandlers()
	c.storageRegisterHandlers()
	c.storageFormulaRegisterHandlers()
	c.storageRequestRegisterHandlers()
	c.systemRegisterHandlers()
	c.userRegisterHandlers()
	c.volumeSeriesRegisterHandlers()
	c.volumeSeriesRequestRegisterHandlers()
	c.app.CrudeOps.RegisterHandlers(c.app.API, c)
	c.app.MetricMover.RegisterHandlers(c.app.API, c)
	c.app.TaskScheduler.RegisterHandlers(c.app.API, c)
}

// Start starts this component
func (c *HandlerComp) Start() {
	c.Log.Info("Starting HandlerComponent")
}

// Stop terminates this component
func (c *HandlerComp) Stop() {
	c.Log.Info("Stopped HandlerComponent")
}

// Lock obtains a write lock
func (c *HandlerComp) Lock() {
	c.mux.Lock()
	c.cntLock++
}

// Unlock releases a write lock
func (c *HandlerComp) Unlock() {
	c.cntUnlock++
	c.mux.Unlock()
}

// RLock obtains a read lock
func (c *HandlerComp) RLock() {
	c.mux.RLock()
	atomic.AddUint32(&c.cntRLock, 1)
}

// RUnlock releases a read lock
func (c *HandlerComp) RUnlock() {
	atomic.AddUint32(&c.cntRUnlock, 1)
	c.mux.RUnlock()
}

// NodeLock obtains a write lock on a Node object
func (c *HandlerComp) NodeLock() {
	c.nodeMux.Lock()
}

// NodeUnlock releases a write lock on a Node object
func (c *HandlerComp) NodeUnlock() {
	c.nodeMux.Unlock()
}

// ClusterLock obtains a write lock on a Cluster object
func (c *HandlerComp) ClusterLock() {
	c.clusterMux.Lock()
}

// ClusterUnlock releases a write lock on a Cluster object
func (c *HandlerComp) ClusterUnlock() {
	c.clusterMux.Unlock()
}

// GetAuthInfo gets the auth.Info from the request context
func (c *HandlerComp) GetAuthInfo(req *http.Request) (*appAuth.Info, error) {
	if cnv := req.Context().Value(appAuth.InfoKey{}); cnv != nil {
		return cnv.(*appAuth.Info), nil
	}
	return nil, centrald.ErrorInternalError
}

// crude.AccessControl functions

// GetAuth returns the auth.Subject for the current request, or a model Error on failure.
func (c *HandlerComp) GetAuth(req *http.Request) (auth.Subject, *M.Error) {
	ai, err := c.GetAuthInfo(req)
	if err != nil {
		return nil, c.eError(err)
	}
	return ai, nil
}

// EventAllowed checks if the creator of the watcher could fetch the object that caused an event.
// It is called when an event is dispatched to each watcher.
func (c *HandlerComp) EventAllowed(s auth.Subject, ev *crude.CrudEvent) bool {
	var ai *appAuth.Info
	if s == nil {
		// watcher created within centrald, eg vreq component, via direct call to Watch() has nil appAuth.Info
		ai = &appAuth.Info{}
	} else {
		ok := false
		if ai, ok = s.(*appAuth.Info); !ok {
			c.Log.Errorf("wrong Subject[%s]", reflect.TypeOf(s))
			return false
		}
	}
	if ev == nil {
		return false
	}
	switch mObj := ev.AccessControlScope.(type) {
	case *M.Account:
		return c.accountFetchFilter(ai, mObj) == nil
	case *M.ApplicationGroup:
		return c.applicationGroupFetchFilter(ai, mObj) == nil
	case *M.Cluster:
		return c.clusterFetchFilter(ai, mObj) == nil
	case *M.ConsistencyGroup:
		return c.consistencyGroupFetchFilter(ai, mObj) == nil
	case *M.CSPCredential:
		return c.cspCredentialFetchFilter(ai, mObj) == nil
	case *M.CSPDomain:
		return c.cspDomainFetchFilter(ai, mObj) == nil
	case *NodeAndCluster:
		return c.nodeFetchFilter(ai, mObj.Node, mObj.Cluster) == nil
	case *M.Pool:
		return c.poolFetchFilter(ai, mObj) == nil
	case *M.ProtectionDomain:
		return c.protectionDomainFetchFilter(ai, mObj) == nil
	case *M.ServicePlanAllocation:
		return c.servicePlanAllocationFetchFilter(ai, mObj) == nil
	case *M.ServicePlan:
		return c.servicePlanFetchFilter(ai, mObj) == nil
	case *M.Snapshot:
		return c.snapshotFetchFilter(ai, mObj) == nil
	case *M.Storage:
		return c.storageFetchFilter(ai, mObj) == nil
	case *M.StorageRequest:
		return c.storageRequestFetchFilter(ai, mObj) == nil
	case *M.System:
		// no restrictions on fetch
	case *M.User:
		return c.userFetchFilter(ai, mObj) == nil
	case *M.VolumeSeries:
		return c.volumeSeriesFetchFilter(ai, mObj) == nil
	case *M.VolumeSeriesRequest:
		return c.volumeSeriesRequestFetchFilter(ai, mObj) == nil
	default:
		// no restrictions on tasks; most generated in-process with no http context
		if strings.HasPrefix(ev.TrimmedURI, "/tasks") {
			return true
		}
		// no restrictions here on debug
		if strings.HasPrefix(ev.TrimmedURI, "/debug") {
			return true
		}
		// POST /metrics/... and POST /audit-log-records cause events but they have no AccessControlScope and are not desired
		if ev.Method != "POST" || (!strings.HasPrefix(ev.TrimmedURI, "/metrics/") && !strings.HasPrefix(ev.TrimmedURI, "/audit-log-records")) {
			// no events are expected for CSPStorageType, Role, SLO or StorageFormula
			c.Log.Errorf("unsupported AccessControlScope[%s] URI[%s]", reflect.TypeOf(ev.AccessControlScope), ev.TrimmedURI)
		}
		return false
	}
	return true
}

// TaskViewOK is part of the housekeeping.AccessControl interface
func (c *HandlerComp) TaskViewOK(s auth.Subject, op string) bool {
	ai, ok := s.(*appAuth.Info)
	if !ok {
		c.Log.Errorf("TaskAccessControl: Subject[%s]", reflect.TypeOf(s))
		return false
	}
	if ai.CapOK(centrald.SystemManagementCap) != nil {
		return false
	}
	return true
}

// TaskCreateOK is part of the housekeeping.AccessControl interface
func (c *HandlerComp) TaskCreateOK(s auth.Subject, op string) bool {
	return c.TaskViewOK(s, op)
}

// TaskCancelOK is part of the housekeeping.AccessControl interface
func (c *HandlerComp) TaskCancelOK(s auth.Subject, op string) bool {
	return c.TaskViewOK(s, op)
}

// Indirection for UUID generation to support UT
type uuidGen func() string

var uuidGenerator uuidGen = func() string { return uuid.NewV4().String() }

// constrainBothQueryAccounts returns accountID and tenantAccountID query constraints for an "and both" owner/authorized query given the authInfo,
// the original corresponding objectList query parameters and capabilities required for each account.
// When returned skip is true no query should be performed.
//
// Use in an objectList function like (the specific capabilities depend on the type of object):
// params.AuthorizedAccountID, params.AccountID, skip = c.constrainBothQueryAccounts(ai, params.AuthorizedAccountID, centrald.CSPDomainUsageCap, params.AccountID, centrald.CSPDomainManagementCap)
func (c *HandlerComp) constrainBothQueryAccounts(ai *appAuth.Info, accountID *string, accountCap string, tenantAccountID *string, tenantAccountCap string) (*string, *string, bool) {
	if !ai.Internal() {
		if ai.CapOK(tenantAccountCap) != nil && ai.CapOK(accountCap) != nil {
			// has neither capability, skip query
			return nil, nil, true
		}
		isTenant := ai.TenantAccountID == "" && ai.CapOK(tenantAccountCap) == nil
		if aID := swag.StringValue(accountID); aID == "" {
			if tID := swag.StringValue(tenantAccountID); tID == "" {
				// neither set, set one filter. Tenant can see anything they own, subordinates can only see objects where they are authorized
				if isTenant {
					tenantAccountID = swag.String(ai.AccountID)
				} else {
					accountID = swag.String(ai.AccountID)
				}
			} else if !isTenant {
				// Only AccountID set. Non-tenant can only list objects with their AuthorizedAccountID. Add filter for the "and both" query
				accountID = swag.String(ai.AccountID)
			} else if tID != ai.AccountID {
				// tenant but wrong owner accountID, skip query
				return nil, nil, true
			}
		} else if aID != ai.AccountID {
			// AuthorizedAccountID set and it's not a match. Tenant can query for specific subordinate so add that to the "and both" filter, otherwise skip query
			if tID := swag.StringValue(tenantAccountID); isTenant && (tID == "" || tID == ai.AccountID) {
				tenantAccountID = swag.String(ai.AccountID)
			} else {
				return nil, nil, true
			}
		}
	}
	return accountID, tenantAccountID, false
}

// constrainEitherOrQueryAccounts returns either/or accountID and tenantAccountID query constraints given the authInfo, the
// original corresponding objectList query parameters and capabilities required for each account.
// When both returned accounts are nil and the caller is not internal, all accounts are filtered and no query should be performed.
//
// Use in an objectList function like (the specific capabilities depend on the type of object):
// params.accountID, params.tenantAccountID, err = c.constrainEitherOrQueryAccounts(ai, params.accountID, centrald.VolumeSeriesOwnerCap, params.tenantAccountID, centrald.VolumeSeriesFetchCap)
func (c *HandlerComp) constrainEitherOrQueryAccounts(ctx context.Context, ai *appAuth.Info, accountID *string, accountCap string, tenantAccountID *string, tenantAccountCap string) (*string, *string, error) {
	if !ai.Internal() {
		tenantAccountIDReset := false
		// if no account filters were specified, add filter on both if caller is a tenant admin, remove filter if wrong tenant admin
		if tID := swag.StringValue(tenantAccountID); tID == "" {
			if swag.StringValue(accountID) == "" && ai.CapOK(tenantAccountCap) == nil {
				tenantAccountID = swag.String(ai.AccountID)
				accountID = swag.String(ai.AccountID)
			} else {
				tenantAccountID = nil
			}
		} else if ai.CapOK(tenantAccountCap, M.ObjIDMutable(tID)) != nil {
			tenantAccountID = nil
			tenantAccountIDReset = true
		}
		if aID := swag.StringValue(accountID); aID == "" {
			// if caller is not a tenant admin, filter on their own account or nothing
			if ai.CapOK(accountCap) == nil && ai.CapOK(tenantAccountCap) != nil && !tenantAccountIDReset {
				accountID = swag.String(ai.AccountID)
			} else {
				accountID = nil
			}
		} else if ai.CapOK(accountCap, M.ObjIDMutable(aID)) != nil {
			// not owner, but tenant admin can also query for objects owned by any specific subordinate
			aObj, err := c.DS.OpsAccount().Fetch(ctx, aID)
			if err != nil || aObj.TenantAccountID == "" || ai.CapOK(tenantAccountCap, aObj.TenantAccountID) != nil {
				if err != nil && err != centrald.ErrorNotFound {
					return nil, nil, err
				}
				// specified AccountID is not in the scope of the caller
				accountID = nil
			}
		}
	}
	return accountID, tenantAccountID, nil
}

func (c *HandlerComp) lookupClusterClient(clusterType string) cluster.Client {
	c.cacheMux.Lock()
	defer c.cacheMux.Unlock()
	if c.clusterClientMap == nil {
		c.clusterClientMap = make(map[string]cluster.Client)
	}
	if cl, ok := c.clusterClientMap[clusterType]; ok {
		return cl
	}
	clClient, err := cluster.NewClient(clusterType)
	if err == nil {
		c.clusterClientMap[clusterType] = clClient
		return clClient
	}
	return nil
}

// MakeStdUpdateArgs is the centrald.CrudHelpers version of makeStdUpdateArgs.
// Pass a model object as the first argument to indicate the type of object involved.
func (c *HandlerComp) MakeStdUpdateArgs(mObj interface{}, id string, version *int32, params [centrald.NumActionTypes][]string) (*centrald.UpdateArgs, error) {
	var nMap JSONToAttrNameMap
	switch mObj.(type) {
	case *M.Cluster:
		nMap = c.clusterMutableNameMap()
	case *M.Node:
		nMap = c.nodeMutableNameMap()
	default:
		return nil, fmt.Errorf("object not externalized")
	}
	return c.makeStdUpdateArgs(nMap, id, version, params)
}
