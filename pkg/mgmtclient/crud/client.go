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


package crud

import (
	"context"
	"net/http"
	"reflect"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/metrics"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/task"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
)

// Updates identifies what properties are to be updated and how.
// Used for vanilla Update calls as well as in the Updater pattern.
// See VolumeSeriesRequestUpdater and VSRUpdaterCB for more on the Updater pattern.
type Updates struct {
	// List of fields to be updated
	Set, Append, Remove []string
	// Version to enforce if non-zero. Some objects require version to be set in which case it is taken from the associated object instead.
	Version int32
}

// Ops is an interface for client CRUD object operations
// List operations return the raw "OK" result to support aggregation queries.
type Ops interface {
	AccountFetch(ctx context.Context, objID string) (*models.Account, error)
	AccountList(ctx context.Context, lParams *account.AccountListParams) (*account.AccountListOK, error)
	ApplicationGroupCreate(ctx context.Context, o *models.ApplicationGroup) (*models.ApplicationGroup, error)
	ApplicationGroupDelete(ctx context.Context, objID string) error
	ApplicationGroupFetch(ctx context.Context, objID string) (*models.ApplicationGroup, error)
	ApplicationGroupList(ctx context.Context, lParams *application_group.ApplicationGroupListParams) (*application_group.ApplicationGroupListOK, error)
	ApplicationGroupUpdate(ctx context.Context, o *models.ApplicationGroup, items *Updates) (*models.ApplicationGroup, error)
	ApplicationGroupUpdater(ctx context.Context, oID string, modifyFn AGUpdaterCB, items *Updates) (*models.ApplicationGroup, error)
	ClusterFetch(ctx context.Context, objID string) (*models.Cluster, error)
	ClusterUpdate(ctx context.Context, o *models.Cluster, items *Updates) (*models.Cluster, error)
	ClusterUpdater(ctx context.Context, oID string, modifyFn ClusterUpdaterCB, items *Updates) (*models.Cluster, error)
	ConsistencyGroupCreate(ctx context.Context, o *models.ConsistencyGroup) (*models.ConsistencyGroup, error)
	ConsistencyGroupDelete(ctx context.Context, objID string) error
	ConsistencyGroupFetch(ctx context.Context, objID string) (*models.ConsistencyGroup, error)
	ConsistencyGroupList(ctx context.Context, lParams *consistency_group.ConsistencyGroupListParams) (*consistency_group.ConsistencyGroupListOK, error)
	ConsistencyGroupUpdate(ctx context.Context, o *models.ConsistencyGroup, items *Updates) (*models.ConsistencyGroup, error)
	ConsistencyGroupUpdater(ctx context.Context, oID string, modifyFn CGUpdaterCB, items *Updates) (*models.ConsistencyGroup, error)
	CSPDomainFetch(ctx context.Context, objID string) (*models.CSPDomain, error)
	CSPDomainList(ctx context.Context, lParams *csp_domain.CspDomainListParams) (*csp_domain.CspDomainListOK, error)
	CSPDomainUpdate(ctx context.Context, o *models.CSPDomain, items *Updates) (*models.CSPDomain, error)
	CSPDomainUpdater(ctx context.Context, oID string, modifyFn CspDomianUpdaterCB, items *Updates) (*models.CSPDomain, error)
	NodeCreate(ctx context.Context, o *models.Node) (*models.Node, error)
	NodeDelete(ctx context.Context, objID string) error
	NodeFetch(ctx context.Context, objID string) (*models.Node, error)
	NodeList(ctx context.Context, lParams *node.NodeListParams) (*node.NodeListOK, error)
	NodeUpdate(ctx context.Context, o *models.Node, items *Updates) (*models.Node, error)
	NodeUpdater(ctx context.Context, oID string, modifyFn NodeUpdaterCB, items *Updates) (*models.Node, error)
	PoolCreate(ctx context.Context, o *models.Pool) (*models.Pool, error)
	PoolDelete(ctx context.Context, objID string) error
	PoolFetch(ctx context.Context, objID string) (*models.Pool, error)
	PoolList(ctx context.Context, lParams *pool.PoolListParams) (*pool.PoolListOK, error)
	PoolUpdate(ctx context.Context, sp *models.Pool, items *Updates) (*models.Pool, error)
	PoolUpdater(ctx context.Context, oID string, modifyFn PoolUpdaterCB, items *Updates) (*models.Pool, error)
	ProtectionDomainCreate(ctx context.Context, o *models.ProtectionDomain) (*models.ProtectionDomain, error)
	ProtectionDomainFetch(ctx context.Context, objID string) (*models.ProtectionDomain, error)
	ProtectionDomainList(ctx context.Context, lParams *protection_domain.ProtectionDomainListParams) (*protection_domain.ProtectionDomainListOK, error)
	ProtectionDomainDelete(ctx context.Context, objID string) error
	ProtectionDomainUpdate(ctx context.Context, sp *models.ProtectionDomain, items *Updates) (*models.ProtectionDomain, error)
	ProtectionDomainUpdater(ctx context.Context, oID string, modifyFn ProtectionDomainUpdaterCB, items *Updates) (*models.ProtectionDomain, error)
	ServicePlanAllocationCreate(ctx context.Context, o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error)
	ServicePlanAllocationDelete(ctx context.Context, objID string) error
	ServicePlanAllocationFetch(ctx context.Context, objID string) (*models.ServicePlanAllocation, error)
	ServicePlanAllocationList(ctx context.Context, lParams *service_plan_allocation.ServicePlanAllocationListParams) (*service_plan_allocation.ServicePlanAllocationListOK, error)
	ServicePlanAllocationUpdate(ctx context.Context, o *models.ServicePlanAllocation, items *Updates) (*models.ServicePlanAllocation, error)
	ServicePlanAllocationUpdater(ctx context.Context, oID string, modifyFn SPAUpdaterCB, items *Updates) (*models.ServicePlanAllocation, error)
	ServicePlanFetch(ctx context.Context, objID string) (*models.ServicePlan, error)
	ServicePlanList(ctx context.Context, lParams *service_plan.ServicePlanListParams) (*service_plan.ServicePlanListOK, error)
	ServicePlanUpdate(ctx context.Context, sp *models.ServicePlan, items *Updates) (*models.ServicePlan, error)
	ServicePlanUpdater(ctx context.Context, oID string, modifyFn SPlanUpdaterCB, items *Updates) (*models.ServicePlan, error)
	SnapshotCreate(ctx context.Context, o *models.Snapshot) (*models.Snapshot, error)
	SnapshotList(ctx context.Context, lParams *snapshot.SnapshotListParams) (*snapshot.SnapshotListOK, error)
	SnapshotDelete(ctx context.Context, objID string) error
	StorageCreate(ctx context.Context, o *models.Storage) (*models.Storage, error)
	StorageDelete(ctx context.Context, objID string) error
	StorageFetch(ctx context.Context, objID string) (*models.Storage, error)
	StorageFormulaList(ctx context.Context, lParams *storage_formula.StorageFormulaListParams) (*storage_formula.StorageFormulaListOK, error)
	StorageIOMetricUpload(ctx context.Context, data []*models.IoMetricDatum) error
	StorageList(ctx context.Context, lParams *storage.StorageListParams) (*storage.StorageListOK, error)
	StorageRequestCreate(ctx context.Context, o *models.StorageRequest) (*models.StorageRequest, error)
	StorageRequestFetch(ctx context.Context, objID string) (*models.StorageRequest, error)
	StorageRequestList(ctx context.Context, lParams *storage_request.StorageRequestListParams) (*storage_request.StorageRequestListOK, error)
	StorageRequestUpdate(ctx context.Context, sr *models.StorageRequest, items *Updates) (*models.StorageRequest, error)
	StorageRequestUpdater(ctx context.Context, oID string, modifyFn SRUpdaterCB, items *Updates) (*models.StorageRequest, error)
	StorageUpdate(ctx context.Context, o *models.Storage, items *Updates) (*models.Storage, error)
	SystemFetch(ctx context.Context) (*models.System, error)
	SystemHostnameFetch(ctx context.Context) (string, error)
	TaskCreate(ctx context.Context, o *models.Task) (*models.Task, error)
	VolumeSeriesCreate(ctx context.Context, o *models.VolumeSeries) (*models.VolumeSeries, error)
	VolumeSeriesDelete(ctx context.Context, objID string) error
	VolumeSeriesFetch(ctx context.Context, objID string) (*models.VolumeSeries, error)
	VolumeSeriesIOMetricUpload(ctx context.Context, data []*models.IoMetricDatum) error
	VolumeSeriesList(ctx context.Context, lParams *volume_series.VolumeSeriesListParams) (*volume_series.VolumeSeriesListOK, error)
	VolumeSeriesNewID(ctx context.Context) (*models.ValueType, error)
	VolumeSeriesRequestCancel(ctx context.Context, objID string) (*models.VolumeSeriesRequest, error)
	VolumeSeriesRequestCreate(ctx context.Context, o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error)
	VolumeSeriesRequestFetch(ctx context.Context, objID string) (*models.VolumeSeriesRequest, error)
	VolumeSeriesRequestList(ctx context.Context, lParams *volume_series_request.VolumeSeriesRequestListParams) (*volume_series_request.VolumeSeriesRequestListOK, error)
	VolumeSeriesRequestUpdate(ctx context.Context, vsr *models.VolumeSeriesRequest, items *Updates) (*models.VolumeSeriesRequest, error)
	VolumeSeriesRequestUpdater(ctx context.Context, oID string, modifyFn VSRUpdaterCB, items *Updates) (*models.VolumeSeriesRequest, error)
	VolumeSeriesUpdate(ctx context.Context, o *models.VolumeSeries, items *Updates) (*models.VolumeSeries, error)
	VolumeSeriesUpdater(ctx context.Context, vsID string, modifyFn VSUpdaterCB, items *Updates) (*models.VolumeSeries, error)
}

// Error wraps a models.Error object
type Error struct {
	// Payload is a named field to disambiguate the Error type from the Error() function
	Payload models.Error
}

var serverError = &models.Error{Code: http.StatusInternalServerError, Message: swag.String("Internal Server Error")}

// NewError returns an error interface to an initialized Error object
func NewError(err *models.Error) error {
	if err == nil || err.Message == nil {
		err = serverError
	}
	return &Error{Payload: *err}
}

// Error implements the error interface over a models.Error object
func (err *Error) Error() string {
	return swag.StringValue(err.Payload.Message)
}

// IsTransient determines if the error is transient, ie. a 5xx error
func (err *Error) IsTransient() bool {
	return err.Payload.Code >= 500 && err.Payload.Code < 600
}

// NotFound indicates that the message matches a "404 object not found ..." error
func (err *Error) NotFound() bool {
	return err.Payload.Code == http.StatusNotFound && strings.HasPrefix(err.Error(), com.ErrorNotFound)
}

// InConflict indicates that the message matches a "409 conflict with ..." error
func (err *Error) InConflict() bool {
	return err.Payload.Code == http.StatusConflict && strings.HasPrefix(err.Error(), com.ErrorRequestInConflict)
}

// Exists indicates that the message matches a "409 object exists ..." error
func (err *Error) Exists() bool {
	return err.Payload.Code == http.StatusConflict && strings.HasPrefix(err.Error(), com.ErrorExists)
}

// Client supports object client operations by implementing ClientOps
type Client struct {
	ClientAPI mgmtclient.API
	Log       *logging.Logger
	updater   objUpdater
}

var _ = Ops(&Client{})

// NewClient returns a new, initialized Client
func NewClient(api mgmtclient.API, log *logging.Logger) *Client {
	oc := &Client{
		ClientAPI: api,
		Log:       log,
	}
	oc.updater = oc // self-reference
	return oc
}

// AccountFetch loads an Account object
func (c *Client) AccountFetch(ctx context.Context, objID string) (*models.Account, error) {
	params := account.NewAccountFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching Account %s", params.ID)
	sAPI := c.ClientAPI.Account()
	res, err := sAPI.AccountFetch(params)
	if err != nil {
		if e, ok := err.(*account.AccountFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch Account %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// AccountList loads a list of account objects given a set of parameters
func (c *Client) AccountList(ctx context.Context, lParams *account.AccountListParams) (*account.AccountListOK, error) {
	cAPI := c.ClientAPI.Account()
	lParams.Context = ctx
	c.Log.Debugf("Listing Account objects %v", lParams)
	lRes, err := cAPI.AccountList(lParams)
	if err != nil {
		if e, ok := err.(*account.AccountListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Account list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// ApplicationGroupCreate creates a ApplicationGroup object
func (c *Client) ApplicationGroupCreate(ctx context.Context, o *models.ApplicationGroup) (*models.ApplicationGroup, error) {
	params := application_group.NewApplicationGroupCreateParams()
	params.Payload = o
	params.Context = ctx
	c.Log.Debugf("Creating ApplicationGroup (account %s name %s)", o.AccountID, o.Name)
	sAPI := c.ClientAPI.ApplicationGroup()
	res, err := sAPI.ApplicationGroupCreate(params)
	if err != nil {
		if e, ok := err.(*application_group.ApplicationGroupCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create ApplicationGroup (account %s name %s) error: %s", o.AccountID, o.Name, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created ApplicationGroup (account %s name %s): %s", o.AccountID, o.Name, res.Payload.Meta.ID)
	return res.Payload, nil
}

// ApplicationGroupDelete destroys a ApplicationGroup object
func (c *Client) ApplicationGroupDelete(ctx context.Context, objID string) error {
	params := application_group.NewApplicationGroupDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting ApplicationGroup %s", params.ID)
	sAPI := c.ClientAPI.ApplicationGroup()
	_, err := sAPI.ApplicationGroupDelete(params)
	if err != nil {
		if e, ok := err.(*application_group.ApplicationGroupDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete ApplicationGroup %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// ApplicationGroupFetch loads a ApplicationGroup object
func (c *Client) ApplicationGroupFetch(ctx context.Context, objID string) (*models.ApplicationGroup, error) {
	params := application_group.NewApplicationGroupFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching ApplicationGroup %s", params.ID)
	sAPI := c.ClientAPI.ApplicationGroup()
	res, err := sAPI.ApplicationGroupFetch(params)
	if err != nil {
		if e, ok := err.(*application_group.ApplicationGroupFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch ApplicationGroup %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ApplicationGroupList lists application group objects
func (c *Client) ApplicationGroupList(ctx context.Context, lParams *application_group.ApplicationGroupListParams) (*application_group.ApplicationGroupListOK, error) {
	cgAPI := c.ClientAPI.ApplicationGroup()
	lParams.Context = ctx
	c.Log.Debugf("Listing ApplicationGroup objects %v", lParams)
	lRes, err := cgAPI.ApplicationGroupList(lParams)
	if err != nil {
		if e, ok := err.(*application_group.ApplicationGroupListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("ApplicationGroup list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// ApplicationGroupUpdate updates the mutable parts of a ApplicationGroup object
func (c *Client) ApplicationGroupUpdate(ctx context.Context, o *models.ApplicationGroup, items *Updates) (*models.ApplicationGroup, error) {
	params := application_group.NewApplicationGroupUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.ApplicationGroupMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating ApplicationGroup %s", params.ID)
	sAPI := c.ClientAPI.ApplicationGroup()
	res, err := sAPI.ApplicationGroupUpdate(params)
	if err != nil {
		if e, ok := err.(*application_group.ApplicationGroupUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update ApplicationGroup %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// AGUpdaterCB is the type of a function called by ApplicationGroupUpdater to apply updates to a ApplicationGroup object
type AGUpdaterCB func(o *models.ApplicationGroup) (*models.ApplicationGroup, error)

// ApplicationGroupUpdater updates a ApplicationGroup object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) ApplicationGroupUpdater(ctx context.Context, oID string, modifyFn AGUpdaterCB, items *Updates) (*models.ApplicationGroup, error) {
	ua := &updaterArgs{
		typeName: "ApplicationGroup",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.ApplicationGroup))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) { return c.ApplicationGroupFetch(ctx, oID) },
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.ApplicationGroupUpdate(ctx, o.(*models.ApplicationGroup), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.ApplicationGroup)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.ApplicationGroup), nil
}

// ClusterFetch loads a Cluster object
func (c *Client) ClusterFetch(ctx context.Context, objID string) (*models.Cluster, error) {
	params := cluster.NewClusterFetchParams()
	params.ID = objID
	params.SetContext(ctx)
	var res *cluster.ClusterFetchOK
	var err error
	clAPI := c.ClientAPI.Cluster()
	c.Log.Debugf("Fetching Cluster [%s]", objID)
	if res, err = clAPI.ClusterFetch(params); err != nil {
		if e, ok := err.(*cluster.ClusterFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch Cluster %s error: %s", objID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ClusterUpdate updates service plan objects
func (c *Client) ClusterUpdate(ctx context.Context, o *models.Cluster, items *Updates) (*models.Cluster, error) {
	params := cluster.NewClusterUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.ClusterMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating Cluster %s", params.ID)
	sAPI := c.ClientAPI.Cluster()
	res, err := sAPI.ClusterUpdate(params)
	if err != nil {
		if e, ok := err.(*cluster.ClusterUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update Cluster %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ClusterUpdaterCB is the type of a function called by ClusterUpdater to apply updates to a Cluster object
type ClusterUpdaterCB func(o *models.Cluster) (*models.Cluster, error)

// ClusterUpdater updates a Cluster object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) ClusterUpdater(ctx context.Context, oID string, modifyFn ClusterUpdaterCB, items *Updates) (*models.Cluster, error) {
	ua := &updaterArgs{
		typeName: "Cluster",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.Cluster))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.ClusterFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.ClusterUpdate(ctx, o.(*models.Cluster), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.Cluster)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.Cluster), nil
}

// ConsistencyGroupCreate creates a ConsistencyGroup object
func (c *Client) ConsistencyGroupCreate(ctx context.Context, o *models.ConsistencyGroup) (*models.ConsistencyGroup, error) {
	params := consistency_group.NewConsistencyGroupCreateParams()
	params.Payload = o
	params.Context = ctx
	c.Log.Debugf("Creating ConsistencyGroup (account %s name %s)", o.AccountID, o.Name)
	sAPI := c.ClientAPI.ConsistencyGroup()
	res, err := sAPI.ConsistencyGroupCreate(params)
	if err != nil {
		if e, ok := err.(*consistency_group.ConsistencyGroupCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create ConsistencyGroup (account %s name %s) error: %s", o.AccountID, o.Name, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created ConsistencyGroup (account %s name %s): %s", o.AccountID, o.Name, res.Payload.Meta.ID)
	return res.Payload, nil
}

// ConsistencyGroupDelete destroys a ConsistencyGroup object
func (c *Client) ConsistencyGroupDelete(ctx context.Context, objID string) error {
	params := consistency_group.NewConsistencyGroupDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting ConsistencyGroup %s", params.ID)
	sAPI := c.ClientAPI.ConsistencyGroup()
	_, err := sAPI.ConsistencyGroupDelete(params)
	if err != nil {
		if e, ok := err.(*consistency_group.ConsistencyGroupDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete ConsistencyGroup %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// ConsistencyGroupFetch loads a ConsistencyGroup object
func (c *Client) ConsistencyGroupFetch(ctx context.Context, objID string) (*models.ConsistencyGroup, error) {
	params := consistency_group.NewConsistencyGroupFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching ConsistencyGroup %s", params.ID)
	sAPI := c.ClientAPI.ConsistencyGroup()
	res, err := sAPI.ConsistencyGroupFetch(params)
	if err != nil {
		if e, ok := err.(*consistency_group.ConsistencyGroupFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch ConsistencyGroup %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ConsistencyGroupList lists consistency group objects
func (c *Client) ConsistencyGroupList(ctx context.Context, lParams *consistency_group.ConsistencyGroupListParams) (*consistency_group.ConsistencyGroupListOK, error) {
	cgAPI := c.ClientAPI.ConsistencyGroup()
	lParams.Context = ctx
	c.Log.Debugf("Listing ConsistencyGroup objects %v", lParams)
	lRes, err := cgAPI.ConsistencyGroupList(lParams)
	if err != nil {
		if e, ok := err.(*consistency_group.ConsistencyGroupListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("ConsistencyGroup list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// ConsistencyGroupUpdate updates the mutable parts of a ConsistencyGroup object
func (c *Client) ConsistencyGroupUpdate(ctx context.Context, o *models.ConsistencyGroup, items *Updates) (*models.ConsistencyGroup, error) {
	params := consistency_group.NewConsistencyGroupUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.ConsistencyGroupMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating ConsistencyGroup %s", params.ID)
	sAPI := c.ClientAPI.ConsistencyGroup()
	res, err := sAPI.ConsistencyGroupUpdate(params)
	if err != nil {
		if e, ok := err.(*consistency_group.ConsistencyGroupUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update ConsistencyGroup %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// CGUpdaterCB is the type of a function called by ConsistencyGroupUpdater to apply updates to a ConsistencyGroup object
type CGUpdaterCB func(o *models.ConsistencyGroup) (*models.ConsistencyGroup, error)

// ConsistencyGroupUpdater updates a ConsistencyGroup object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) ConsistencyGroupUpdater(ctx context.Context, oID string, modifyFn CGUpdaterCB, items *Updates) (*models.ConsistencyGroup, error) {
	ua := &updaterArgs{
		typeName: "ConsistencyGroup",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.ConsistencyGroup))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) { return c.ConsistencyGroupFetch(ctx, oID) },
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.ConsistencyGroupUpdate(ctx, o.(*models.ConsistencyGroup), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.ConsistencyGroup)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.ConsistencyGroup), nil
}

// CSPDomainFetch loads a CSPDomain object
func (c *Client) CSPDomainFetch(ctx context.Context, objID string) (*models.CSPDomain, error) {
	params := csp_domain.NewCspDomainFetchParams()
	params.ID = objID
	params.SetContext(ctx)
	var res *csp_domain.CspDomainFetchOK
	var err error
	domAPI := c.ClientAPI.CspDomain()
	c.Log.Debugf("Fetching CSPDomain [%s]", objID)
	if res, err = domAPI.CspDomainFetch(params); err != nil {
		if e, ok := err.(*csp_domain.CspDomainFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch CSPDomain %s error: %s", objID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// CSPDomainList lists domain objects
func (c *Client) CSPDomainList(ctx context.Context, lParams *csp_domain.CspDomainListParams) (*csp_domain.CspDomainListOK, error) {
	domAPI := c.ClientAPI.CspDomain()
	lParams.Context = ctx
	c.Log.Debugf("Listing CSPDomain objects %v", lParams)
	lRes, err := domAPI.CspDomainList(lParams)
	if err != nil {
		if e, ok := err.(*csp_domain.CspDomainListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("CSPDomain list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// CSPDomainUpdate updates service plan objects
func (c *Client) CSPDomainUpdate(ctx context.Context, o *models.CSPDomain, items *Updates) (*models.CSPDomain, error) {
	params := csp_domain.NewCspDomainUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.CSPDomainMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating CspDomain %s", params.ID)
	sAPI := c.ClientAPI.CspDomain()
	res, err := sAPI.CspDomainUpdate(params)
	if err != nil {
		if e, ok := err.(*csp_domain.CspDomainUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update CspDomain %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// CspDomianUpdaterCB is the type of a function called by CSPDomainUpdater to apply updates to a CspDomain object
type CspDomianUpdaterCB func(o *models.CSPDomain) (*models.CSPDomain, error)

// CSPDomainUpdater updates a CspDomain object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) CSPDomainUpdater(ctx context.Context, oID string, modifyFn CspDomianUpdaterCB, items *Updates) (*models.CSPDomain, error) {
	ua := &updaterArgs{
		typeName: "CspDomain",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.CSPDomain))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.CSPDomainFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.CSPDomainUpdate(ctx, o.(*models.CSPDomain), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.CSPDomain)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.CSPDomain), nil
}

// NodeCreate creates a Node object
func (c *Client) NodeCreate(ctx context.Context, o *models.Node) (*models.Node, error) {
	params := node.NewNodeCreateParams()
	params.Payload = o
	params.Context = ctx
	c.Log.Debugf("Creating Node (N:%s Cl:%s)", o.NodeIdentifier, o.ClusterID)
	nAPI := c.ClientAPI.Node()
	res, err := nAPI.NodeCreate(params)
	if err != nil {
		if e, ok := err.(*node.NodeCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Creating Node (N:%s Cl:%s) error: %s", o.NodeIdentifier, o.ClusterID, err.Error())
		return nil, err
	}
	c.Log.Debugf("Creating Node (N:%s Cl:%s): %s", o.NodeIdentifier, o.ClusterID, res.Payload.Meta.ID)
	return res.Payload, nil
}

// NodeDelete destroys a Node object
func (c *Client) NodeDelete(ctx context.Context, objID string) error {
	params := node.NewNodeDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting Node %s", params.ID)
	nAPI := c.ClientAPI.Node()
	_, err := nAPI.NodeDelete(params)
	if err != nil {
		if e, ok := err.(*node.NodeDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete Node %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// NodeFetch loads a Node object
func (c *Client) NodeFetch(ctx context.Context, objID string) (*models.Node, error) {
	params := node.NewNodeFetchParams()
	params.ID = objID
	params.SetContext(ctx)
	var res *node.NodeFetchOK
	var err error
	nAPI := c.ClientAPI.Node()
	c.Log.Debugf("Fetching Node [%s]", objID)
	if res, err = nAPI.NodeFetch(params); err != nil {
		if e, ok := err.(*node.NodeFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch Node %s error: %s", objID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// NodeList lists node objects
func (c *Client) NodeList(ctx context.Context, lParams *node.NodeListParams) (*node.NodeListOK, error) {
	nAPI := c.ClientAPI.Node()
	lParams.Context = ctx
	c.Log.Debugf("Listing Node objects %v", lParams)
	lRes, err := nAPI.NodeList(lParams)
	if err != nil {
		if e, ok := err.(*node.NodeListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Node list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// NodeUpdate updates the mutable part of a Node
func (c *Client) NodeUpdate(ctx context.Context, o *models.Node, items *Updates) (*models.Node, error) {
	nAPI := c.ClientAPI.Node()
	params := node.NewNodeUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.NodeMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(int32(o.Meta.Version))
	}
	params.Context = ctx
	c.Log.Debugf("Updating Node %s", o.Meta.ID)
	res, err := nAPI.NodeUpdate(params)
	if err != nil {
		if e, ok := err.(*node.NodeUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update Node %s error: %s", o.Meta.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// NodeUpdaterCB is the type of a function called by NodeUpdater to apply updates to a Node object
type NodeUpdaterCB func(o *models.Node) (*models.Node, error)

// NodeUpdater updates a Node object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) NodeUpdater(ctx context.Context, oID string, modifyFn NodeUpdaterCB, items *Updates) (*models.Node, error) {
	ua := &updaterArgs{
		typeName: "Node",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.Node))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.NodeFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.NodeUpdate(ctx, o.(*models.Node), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.Node)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.Node), nil
}

// PoolCreate creates a Pool object
func (c *Client) PoolCreate(ctx context.Context, o *models.Pool) (*models.Pool, error) {
	params := pool.NewPoolCreateParams()
	params.Payload = &models.PoolCreateArgs{
		PoolCreateOnce:    o.PoolCreateOnce,
		PoolCreateMutable: o.PoolCreateMutable,
	}
	params.Context = ctx
	c.Log.Debugf("Creating Pool (AA:%s Cl:%s T:%s)", o.AuthorizedAccountID, o.ClusterID, o.CspStorageType)
	sAPI := c.ClientAPI.Pool()
	res, err := sAPI.PoolCreate(params)
	if err != nil {
		if e, ok := err.(*pool.PoolCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create Pool (AA:%s Cl:%s T:%s) error: %s", o.AuthorizedAccountID, o.ClusterID, o.CspStorageType, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created Pool (AA:%s Cl:%s T:%s): %s", o.AuthorizedAccountID, o.ClusterID, o.CspStorageType, res.Payload.Meta.ID)
	return res.Payload, nil
}

// PoolFetch loads a Pool object
func (c *Client) PoolFetch(ctx context.Context, objID string) (*models.Pool, error) {
	params := pool.NewPoolFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching Pool %s", params.ID)
	spAPI := c.ClientAPI.Pool()
	res, err := spAPI.PoolFetch(params)
	if err != nil {
		if e, ok := err.(*pool.PoolFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch Pool %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// PoolList searches for Pool objects
func (c *Client) PoolList(ctx context.Context, lParams *pool.PoolListParams) (*pool.PoolListOK, error) {
	spAPI := c.ClientAPI.Pool()
	lParams.Context = ctx
	lRes, err := spAPI.PoolList(lParams)
	if err != nil {
		if e, ok := err.(*pool.PoolListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Pool list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// PoolDelete destroys a Pool object
func (c *Client) PoolDelete(ctx context.Context, objID string) error {
	params := pool.NewPoolDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting Pool %s", params.ID)
	sAPI := c.ClientAPI.Pool()
	_, err := sAPI.PoolDelete(params)
	if err != nil {
		if e, ok := err.(*pool.PoolDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete Pool %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// PoolUpdate updates the mutable part of a Pool
func (c *Client) PoolUpdate(ctx context.Context, sp *models.Pool, items *Updates) (*models.Pool, error) {
	spAPI := c.ClientAPI.Pool()
	params := pool.NewPoolUpdateParams()
	params.ID = string(sp.Meta.ID)
	params.Payload = &sp.PoolMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating pool %s", sp.Meta.ID)
	res, err := spAPI.PoolUpdate(params)
	if err != nil {
		if e, ok := err.(*pool.PoolUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update Pool %s error: %s", sp.Meta.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// PoolUpdaterCB is the type of a function called by PoolUpdater to apply updates to a Pool object
type PoolUpdaterCB func(o *models.Pool) (*models.Pool, error)

// PoolUpdater updates a Pool object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) PoolUpdater(ctx context.Context, oID string, modifyFn PoolUpdaterCB, items *Updates) (*models.Pool, error) {
	ua := &updaterArgs{
		typeName: "Pool",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.Pool))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.PoolFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.PoolUpdate(ctx, o.(*models.Pool), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.Pool)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.Pool), nil
}

// ProtectionDomainCreate creates a ProtectionDomain object
func (c *Client) ProtectionDomainCreate(ctx context.Context, o *models.ProtectionDomain) (*models.ProtectionDomain, error) {
	params := protection_domain.NewProtectionDomainCreateParams()
	params.Payload = &models.ProtectionDomainCreateArgs{
		ProtectionDomainCreateOnce: o.ProtectionDomainCreateOnce,
		ProtectionDomainMutable:    o.ProtectionDomainMutable,
	}
	params.Context = ctx
	c.Log.Debugf("Creating ProtectionDomain (AA:%s N:%s)", o.AccountID, o.Name)
	api := c.ClientAPI.ProtectionDomain()
	res, err := api.ProtectionDomainCreate(params)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Creating ProtectionDomain (AA:%s N:%s): error: %s", o.AccountID, o.Name, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created ProtectionDomain (AA:%s N:%s): %s", o.AccountID, o.Name, res.Payload.Meta.ID)
	return res.Payload, nil
}

// ProtectionDomainFetch loads a ProtectionDomain object
func (c *Client) ProtectionDomainFetch(ctx context.Context, objID string) (*models.ProtectionDomain, error) {
	params := protection_domain.NewProtectionDomainFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching ProtectionDomain %s", params.ID)
	api := c.ClientAPI.ProtectionDomain()
	res, err := api.ProtectionDomainFetch(params)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch ProtectionDomain %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ProtectionDomainList searches for ProtectionDomain objects
func (c *Client) ProtectionDomainList(ctx context.Context, lParams *protection_domain.ProtectionDomainListParams) (*protection_domain.ProtectionDomainListOK, error) {
	api := c.ClientAPI.ProtectionDomain()
	lParams.Context = ctx
	lRes, err := api.ProtectionDomainList(lParams)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("ProtectionDomain list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// ProtectionDomainDelete destroys a ProtectionDomain object
func (c *Client) ProtectionDomainDelete(ctx context.Context, objID string) error {
	params := protection_domain.NewProtectionDomainDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting ProtectionDomain %s", params.ID)
	api := c.ClientAPI.ProtectionDomain()
	_, err := api.ProtectionDomainDelete(params)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete ProtectionDomain %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// ProtectionDomainUpdate updates the mutable part of a ProtectionDomain
func (c *Client) ProtectionDomainUpdate(ctx context.Context, sp *models.ProtectionDomain, items *Updates) (*models.ProtectionDomain, error) {
	api := c.ClientAPI.ProtectionDomain()
	params := protection_domain.NewProtectionDomainUpdateParams()
	params.ID = string(sp.Meta.ID)
	params.Payload = &sp.ProtectionDomainMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating protection_domain %s", sp.Meta.ID)
	res, err := api.ProtectionDomainUpdate(params)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update ProtectionDomain %s error: %s", sp.Meta.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ProtectionDomainUpdaterCB is the type of a function called by ProtectionDomainUpdater to apply updates to a ProtectionDomain object
type ProtectionDomainUpdaterCB func(o *models.ProtectionDomain) (*models.ProtectionDomain, error)

// ProtectionDomainUpdater updates a ProtectionDomain object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) ProtectionDomainUpdater(ctx context.Context, oID string, modifyFn ProtectionDomainUpdaterCB, items *Updates) (*models.ProtectionDomain, error) {
	ua := &updaterArgs{
		typeName: "ProtectionDomain",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.ProtectionDomain))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.ProtectionDomainFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.ProtectionDomainUpdate(ctx, o.(*models.ProtectionDomain), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.ProtectionDomain)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.ProtectionDomain), nil
}

// ServicePlanFetch loads a Service Plan object
func (c *Client) ServicePlanFetch(ctx context.Context, objID string) (*models.ServicePlan, error) {
	params := service_plan.NewServicePlanFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching ServicePlan %s", params.ID)
	sAPI := c.ClientAPI.ServicePlan()
	res, err := sAPI.ServicePlanFetch(params)
	if err != nil {
		if e, ok := err.(*service_plan.ServicePlanFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch ServicePlan %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ServicePlanList lists service plan objects
func (c *Client) ServicePlanList(ctx context.Context, lParams *service_plan.ServicePlanListParams) (*service_plan.ServicePlanListOK, error) {
	clAPI := c.ClientAPI.ServicePlan()
	lParams.Context = ctx
	c.Log.Debugf("Listing ServicePlan objects %v", lParams)
	lRes, err := clAPI.ServicePlanList(lParams)
	if err != nil {
		if e, ok := err.(*service_plan.ServicePlanListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("ServicePlan list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// ServicePlanUpdate updates service plan objects
func (c *Client) ServicePlanUpdate(ctx context.Context, sp *models.ServicePlan, items *Updates) (*models.ServicePlan, error) {
	params := service_plan.NewServicePlanUpdateParams()
	params.ID = string(sp.Meta.ID)
	params.Payload = &sp.ServicePlanMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating ServicePlan %s", params.ID)
	sAPI := c.ClientAPI.ServicePlan()
	res, err := sAPI.ServicePlanUpdate(params)
	if err != nil {
		if e, ok := err.(*service_plan.ServicePlanUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update ServicePlan %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// SPlanUpdaterCB is the type of a function called by ServicePlanUpdater to apply updates to a ServicePlan object
type SPlanUpdaterCB func(o *models.ServicePlan) (*models.ServicePlan, error)

// ServicePlanUpdater updates a ServicePlan object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) ServicePlanUpdater(ctx context.Context, oID string, modifyFn SPlanUpdaterCB, items *Updates) (*models.ServicePlan, error) {
	ua := &updaterArgs{
		typeName: "ServicePlan",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.ServicePlan))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.ServicePlanFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.ServicePlanUpdate(ctx, o.(*models.ServicePlan), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.ServicePlan)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.ServicePlan), nil
}

// ServicePlanAllocationCreate creates a ServicePlanAllocation object
func (c *Client) ServicePlanAllocationCreate(ctx context.Context, o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
	params := service_plan_allocation.NewServicePlanAllocationCreateParams()
	params.Payload = &models.ServicePlanAllocationCreateArgs{
		ServicePlanAllocationCreateOnce:    o.ServicePlanAllocationCreateOnce,
		ServicePlanAllocationCreateMutable: o.ServicePlanAllocationCreateMutable,
	}
	params.Context = ctx
	c.Log.Debugf("Creating ServicePlanAllocation (authAccount=%s plan=%s cluster=%s size=%d)", o.AuthorizedAccountID, o.ServicePlanID, o.ClusterID, *o.TotalCapacityBytes)
	sAPI := c.ClientAPI.ServicePlanAllocation()
	res, err := sAPI.ServicePlanAllocationCreate(params)
	if err != nil {
		if e, ok := err.(*service_plan_allocation.ServicePlanAllocationCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Creating ServicePlanAllocation (authAccount=%s plan=%s cluster=%s size=%d) error: %s", o.AuthorizedAccountID, o.ServicePlanID, o.ClusterID, *o.TotalCapacityBytes, err.Error())
		return nil, err
	}
	c.Log.Debugf("Creating ServicePlanAllocation (authAccount=%s plan=%s cluster=%s size=%d): %s", o.AuthorizedAccountID, o.ServicePlanID, o.ClusterID, *o.TotalCapacityBytes, res.Payload.Meta.ID)
	return res.Payload, nil
}

// ServicePlanAllocationDelete destroys a ServicePlanAllocation object
func (c *Client) ServicePlanAllocationDelete(ctx context.Context, objID string) error {
	params := service_plan_allocation.NewServicePlanAllocationDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting ServicePlanAllocation %s", params.ID)
	sAPI := c.ClientAPI.ServicePlanAllocation()
	_, err := sAPI.ServicePlanAllocationDelete(params)
	if err != nil {
		if e, ok := err.(*service_plan_allocation.ServicePlanAllocationDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete ServicePlanAllocation %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// ServicePlanAllocationFetch loads a ServicePlanAllocation object
func (c *Client) ServicePlanAllocationFetch(ctx context.Context, objID string) (*models.ServicePlanAllocation, error) {
	params := service_plan_allocation.NewServicePlanAllocationFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching ServicePlanAllocation %s", params.ID)
	sAPI := c.ClientAPI.ServicePlanAllocation()
	res, err := sAPI.ServicePlanAllocationFetch(params)
	if err != nil {
		if e, ok := err.(*service_plan_allocation.ServicePlanAllocationFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch ServicePlanAllocation %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// ServicePlanAllocationList searches for ServicePlanAllocation objects
func (c *Client) ServicePlanAllocationList(ctx context.Context, lParams *service_plan_allocation.ServicePlanAllocationListParams) (*service_plan_allocation.ServicePlanAllocationListOK, error) {
	sAPI := c.ClientAPI.ServicePlanAllocation()
	lParams.Context = ctx
	c.Log.Debugf("Listing ServicePlanAllocation objects %v", lParams)
	lRes, err := sAPI.ServicePlanAllocationList(lParams)
	if err != nil {
		if e, ok := err.(*service_plan_allocation.ServicePlanAllocationListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("ServicePlanAllocation list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// ServicePlanAllocationUpdate updates the mutable parts of a ServicePlanAllocation object
func (c *Client) ServicePlanAllocationUpdate(ctx context.Context, o *models.ServicePlanAllocation, items *Updates) (*models.ServicePlanAllocation, error) {
	params := service_plan_allocation.NewServicePlanAllocationUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.ServicePlanAllocationMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating ServicePlanAllocation %s", params.ID)
	sAPI := c.ClientAPI.ServicePlanAllocation()
	res, err := sAPI.ServicePlanAllocationUpdate(params)
	if err != nil {
		if e, ok := err.(*service_plan_allocation.ServicePlanAllocationUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update ServicePlanAllocation %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// SPAUpdaterCB is the type of a function called by ServicePlanAllocationUpdater to apply updates to a ServicePlanAllocation object
type SPAUpdaterCB func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error)

// ServicePlanAllocationUpdater updates a ServicePlanAllocation object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) ServicePlanAllocationUpdater(ctx context.Context, oID string, modifyFn SPAUpdaterCB, items *Updates) (*models.ServicePlanAllocation, error) {
	ua := &updaterArgs{
		typeName: "ServicePlanAllocation",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.ServicePlanAllocation))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.ServicePlanAllocationFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.ServicePlanAllocationUpdate(ctx, o.(*models.ServicePlanAllocation), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.ServicePlanAllocation)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.ServicePlanAllocation), nil
}

// SnapshotCreate creates a Snapshot object
func (c *Client) SnapshotCreate(ctx context.Context, o *models.Snapshot) (*models.Snapshot, error) {
	params := snapshot.NewSnapshotCreateParams()
	params.Payload = o
	params.Context = ctx
	c.Log.Debugf("Creating Snapshot (SnapID %s, VS %s and PitUUID %s)", o.SnapIdentifier, o.VolumeSeriesID, o.PitIdentifier)
	sAPI := c.ClientAPI.Snapshot()
	res, err := sAPI.SnapshotCreate(params)
	if err != nil {
		if e, ok := err.(*snapshot.SnapshotCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create Snapshot (SnapID %s, VS %s and PitUUID %s) error: %s",
			o.SnapIdentifier, o.VolumeSeriesID, o.PitIdentifier, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created Snapshot (SnapID %s, VS %s and PitUUID %s)", o.SnapIdentifier, o.VolumeSeriesID, o.PitIdentifier)
	return res.Payload, nil
}

// SnapshotList searches for Snapshot objects
func (c *Client) SnapshotList(ctx context.Context, lParams *snapshot.SnapshotListParams) (*snapshot.SnapshotListOK, error) {
	sAPI := c.ClientAPI.Snapshot()
	lParams.Context = ctx
	c.Log.Debugf("Listing Snapshot objects %v", lParams)
	lRes, err := sAPI.SnapshotList(lParams)
	if err != nil {
		if e, ok := err.(*snapshot.SnapshotListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Snapshot list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// SnapshotDelete destroys Snapshot objects
func (c *Client) SnapshotDelete(ctx context.Context, objID string) error {
	params := snapshot.NewSnapshotDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting Snapshot %s", params.ID)
	sAPI := c.ClientAPI.Snapshot()
	_, err := sAPI.SnapshotDelete(params)
	if err != nil {
		if e, ok := err.(*snapshot.SnapshotDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete Snapshot %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// StorageCreate creates a Storage object
func (c *Client) StorageCreate(ctx context.Context, o *models.Storage) (*models.Storage, error) {
	params := storage.NewStorageCreateParams()
	params.Payload = o
	params.Context = ctx
	c.Log.Debugf("Creating Storage (pool %s size %d)", o.PoolID, *o.SizeBytes)
	sAPI := c.ClientAPI.Storage()
	res, err := sAPI.StorageCreate(params)
	if err != nil {
		if e, ok := err.(*storage.StorageCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create Storage (pool %s size %d) error: %s", o.PoolID, *o.SizeBytes, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created Storage (pool %s size %d): %s", o.PoolID, *o.SizeBytes, res.Payload.Meta.ID)
	return res.Payload, nil
}

// StorageFetch loads a Storage object
func (c *Client) StorageFetch(ctx context.Context, objID string) (*models.Storage, error) {
	params := storage.NewStorageFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching Storage %s", params.ID)
	sAPI := c.ClientAPI.Storage()
	res, err := sAPI.StorageFetch(params)
	if err != nil {
		if e, ok := err.(*storage.StorageFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch Storage %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// StorageList searches for Storage objects
func (c *Client) StorageList(ctx context.Context, lParams *storage.StorageListParams) (*storage.StorageListOK, error) {
	sAPI := c.ClientAPI.Storage()
	lParams.Context = ctx
	c.Log.Debugf("Listing Storage objects %v", lParams)
	lRes, err := sAPI.StorageList(lParams)
	if err != nil {
		if e, ok := err.(*storage.StorageListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Storage list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// StorageDelete destroys a Storage object
func (c *Client) StorageDelete(ctx context.Context, objID string) error {
	params := storage.NewStorageDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting Storage %s", params.ID)
	sAPI := c.ClientAPI.Storage()
	_, err := sAPI.StorageDelete(params)
	if err != nil {
		if e, ok := err.(*storage.StorageDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete Storage %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// StorageUpdate updates the mutable parts of a Storage object
func (c *Client) StorageUpdate(ctx context.Context, o *models.Storage, items *Updates) (*models.Storage, error) {
	params := storage.NewStorageUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.StorageMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	params.Version = int32(o.Meta.Version) // required
	params.Context = ctx
	c.Log.Debugf("Updating Storage %s", params.ID)
	sAPI := c.ClientAPI.Storage()
	res, err := sAPI.StorageUpdate(params)
	if err != nil {
		if e, ok := err.(*storage.StorageUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update Storage %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// StorageFormulaList searches for StorageFormula objects
func (c *Client) StorageFormulaList(ctx context.Context, lParams *storage_formula.StorageFormulaListParams) (*storage_formula.StorageFormulaListOK, error) {
	sAPI := c.ClientAPI.StorageFormula()
	lParams.Context = ctx
	c.Log.Debugf("Listing StorageFormula objects %v", lParams)
	lRes, err := sAPI.StorageFormulaList(lParams)
	if err != nil {
		if e, ok := err.(*storage_formula.StorageFormulaListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("StorageFormula list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// StorageIOMetricUpload transmits metric data
func (c *Client) StorageIOMetricUpload(ctx context.Context, data []*models.IoMetricDatum) error {
	params := metrics.NewStorageIOMetricUploadParams()
	params.Payload = &models.IoMetricData{
		Data: data,
	}
	params.Context = ctx
	c.Log.Debugf("StorageIOMetricUpload: %d", len(data))
	api := c.ClientAPI.Metrics()
	_, err := api.StorageIOMetricUpload(params)
	if err != nil {
		if e, ok := err.(*metrics.StorageIOMetricUploadDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("StorageIOMetricUpload error: %s", err.Error())
	}
	return err
}

// StorageRequestCreate creates a StorageRequest object
func (c *Client) StorageRequestCreate(ctx context.Context, o *models.StorageRequest) (*models.StorageRequest, error) {
	params := storage_request.NewStorageRequestCreateParams()
	params.Payload = &models.StorageRequestCreateArgs{
		StorageRequestCreateOnce:    o.StorageRequestCreateOnce,
		StorageRequestCreateMutable: o.StorageRequestCreateMutable,
	}
	params.Context = ctx
	c.Log.Debugf("Creating StorageRequest (pool %s size %d node %s)", o.PoolID, swag.Int64Value(o.MinSizeBytes), o.NodeID)
	srAPI := c.ClientAPI.StorageRequest()
	res, err := srAPI.StorageRequestCreate(params)
	if err != nil {
		if e, ok := err.(*storage_request.StorageRequestCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create StorageRequest (pool %s size %d node %s) error: %s", o.PoolID, swag.Int64Value(o.MinSizeBytes), o.NodeID, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created StorageRequest (pool %s size %d node %s): %s", o.PoolID, swag.Int64Value(o.MinSizeBytes), o.NodeID, res.Payload.Meta.ID)
	return res.Payload, nil
}

// StorageRequestList searches for StorageRequest objects
func (c *Client) StorageRequestList(ctx context.Context, lParams *storage_request.StorageRequestListParams) (*storage_request.StorageRequestListOK, error) {
	lParams.SetContext(ctx)
	c.Log.Debugf("Listing StorageRequests %+v", lParams)
	srAPI := c.ClientAPI.StorageRequest()
	res, err := srAPI.StorageRequestList(lParams)
	if err != nil {
		if e, ok := err.(*storage_request.StorageRequestListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("List StorageRequests error: %s", err.Error())
		return nil, err
	}
	c.Log.Debugf("found %d StorageRequests", len(res.Payload))
	return res, nil
}

// StorageRequestFetch loads a StorageRequest object
func (c *Client) StorageRequestFetch(ctx context.Context, objID string) (*models.StorageRequest, error) {
	params := storage_request.NewStorageRequestFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching StorageRequest %s", params.ID)
	srAPI := c.ClientAPI.StorageRequest()
	res, err := srAPI.StorageRequestFetch(params)
	if err != nil {
		if e, ok := err.(*storage_request.StorageRequestFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch StorageRequest %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// StorageRequestUpdate updates the mutable parts of the StorageRequest object
func (c *Client) StorageRequestUpdate(ctx context.Context, sr *models.StorageRequest, items *Updates) (*models.StorageRequest, error) {
	params := storage_request.NewStorageRequestUpdateParams()
	params.ID = string(sr.Meta.ID)
	params.Payload = &sr.StorageRequestMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	params.Version = int32(sr.Meta.Version) // required
	params.Context = ctx
	c.Log.Debugf("Updating StorageRequest %s", params.ID)
	srAPI := c.ClientAPI.StorageRequest()
	res, err := srAPI.StorageRequestUpdate(params)
	if err != nil {
		if e, ok := err.(*storage_request.StorageRequestUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update StorageRequest %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// SRUpdaterCB is the type of a function called by StorageRequestUpdater to apply updates to a StorageRequest object
type SRUpdaterCB func(o *models.StorageRequest) (*models.StorageRequest, error)

// StorageRequestUpdater updates a StorageRequest object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) StorageRequestUpdater(ctx context.Context, oID string, modifyFn SRUpdaterCB, items *Updates) (*models.StorageRequest, error) {
	ua := &updaterArgs{
		typeName: "StorageRequest",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.StorageRequest))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) { return c.StorageRequestFetch(ctx, oID) },
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.StorageRequestUpdate(ctx, o.(*models.StorageRequest), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.StorageRequest)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.StorageRequest), nil
}

// SystemFetch loads the system object
func (c *Client) SystemFetch(ctx context.Context) (*models.System, error) {
	params := system.NewSystemFetchParams()
	params.SetContext(ctx)
	var res *system.SystemFetchOK
	var err error
	sysAPI := c.ClientAPI.System()
	c.Log.Debugf("Fetching System")
	if res, err = sysAPI.SystemFetch(params); err != nil {
		if e, ok := err.(*system.SystemFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch System error: %s", err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// SystemHostnameFetch fetches the system hostname
func (c *Client) SystemHostnameFetch(ctx context.Context) (string, error) {
	params := system.NewSystemHostnameFetchParams()
	params.SetContext(ctx)
	var res *system.SystemHostnameFetchOK
	var err error
	sysAPI := c.ClientAPI.System()
	c.Log.Debugf("Fetching System Hostname")
	if res, err = sysAPI.SystemHostnameFetch(params); err != nil {
		if e, ok := err.(*system.SystemHostnameFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch System Hostname error: %s", err.Error())
		return "", err
	}
	return res.Payload, nil
}

// TaskCreate creates a Task object
func (c *Client) TaskCreate(ctx context.Context, o *models.Task) (*models.Task, error) {
	params := task.NewTaskCreateParams()
	params.Payload = &o.TaskCreateOnce
	params.Context = ctx
	c.Log.Debugf("Creating Task (operation %s, objectId %s)", o.Operation, o.ObjectID)
	tAPI := c.ClientAPI.Task()
	res, err := tAPI.TaskCreate(params)
	if err != nil {
		if e, ok := err.(*task.TaskCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Creating Task (operation %s, objectId %s) error: %s", o.Operation, o.ObjectID, err.Error())
		return nil, err
	}
	c.Log.Debugf("Creating Task (operation %s, objectId %s): %s", o.Operation, o.ObjectID, res.Payload.Meta.ID)
	return res.Payload, nil
}

// VolumeSeriesCreate creates a VolumeSeries object
func (c *Client) VolumeSeriesCreate(ctx context.Context, o *models.VolumeSeries) (*models.VolumeSeries, error) {
	params := volume_series.NewVolumeSeriesCreateParams()
	params.Payload = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateOnce:    o.VolumeSeriesCreateOnce,
		VolumeSeriesCreateMutable: o.VolumeSeriesCreateMutable,
	}
	params.Context = ctx
	c.Log.Debugf("Creating VolumeSeries (size %d)", *o.SizeBytes)
	sAPI := c.ClientAPI.VolumeSeries()
	res, err := sAPI.VolumeSeriesCreate(params)
	if err != nil {
		if e, ok := err.(*volume_series.VolumeSeriesCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create VolumeSeries (size %d) error: %s", *o.SizeBytes, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created VolumeSeries (size %d): %s", *o.SizeBytes, res.Payload.Meta.ID)
	return res.Payload, nil
}

// VolumeSeriesFetch loads a VolumeSeries object
func (c *Client) VolumeSeriesFetch(ctx context.Context, objID string) (*models.VolumeSeries, error) {
	params := volume_series.NewVolumeSeriesFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching VolumeSeries %s", params.ID)
	sAPI := c.ClientAPI.VolumeSeries()
	res, err := sAPI.VolumeSeriesFetch(params)
	if err != nil {
		if e, ok := err.(*volume_series.VolumeSeriesFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch VolumeSeries %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// VolumeSeriesList searches for VolumeSeries objects
func (c *Client) VolumeSeriesList(ctx context.Context, lParams *volume_series.VolumeSeriesListParams) (*volume_series.VolumeSeriesListOK, error) {
	sAPI := c.ClientAPI.VolumeSeries()
	lParams.Context = ctx
	c.Log.Debugf("Listing VolumeSeries objects %v", lParams)
	lRes, err := sAPI.VolumeSeriesList(lParams)
	if err != nil {
		if e, ok := err.(*volume_series.VolumeSeriesListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("VolumeSeries list error: %s", err.Error())
		return nil, err
	}
	return lRes, nil
}

// VolumeSeriesNewID obtains an ID for a VolumeSeries
func (c *Client) VolumeSeriesNewID(ctx context.Context) (*models.ValueType, error) {
	sAPI := c.ClientAPI.VolumeSeries()
	params := volume_series.NewVolumeSeriesNewIDParams().WithContext(ctx)
	c.Log.Debugf("Obtaining VolumeSeries ID")
	res, err := sAPI.VolumeSeriesNewID(params)
	if err != nil {
		if e, ok := err.(*volume_series.VolumeSeriesNewIDDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("VolumeSeries NewID error: %s", err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// VolumeSeriesDelete destroys a VolumeSeries object
func (c *Client) VolumeSeriesDelete(ctx context.Context, objID string) error {
	params := volume_series.NewVolumeSeriesDeleteParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Deleting VolumeSeries %s", params.ID)
	sAPI := c.ClientAPI.VolumeSeries()
	_, err := sAPI.VolumeSeriesDelete(params)
	if err != nil {
		if e, ok := err.(*volume_series.VolumeSeriesDeleteDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Delete VolumeSeries %s error: %s", params.ID, err.Error())
		return err
	}
	return nil
}

// VolumeSeriesIOMetricUpload transmits metric data
func (c *Client) VolumeSeriesIOMetricUpload(ctx context.Context, data []*models.IoMetricDatum) error {
	params := metrics.NewVolumeSeriesIOMetricUploadParams()
	params.Payload = &models.IoMetricData{
		Data: data,
	}
	params.Context = ctx
	c.Log.Debugf("VolumeSeriesIOMetricUpload: %d", len(data))
	api := c.ClientAPI.Metrics()
	_, err := api.VolumeSeriesIOMetricUpload(params)
	if err != nil {
		if e, ok := err.(*metrics.VolumeSeriesIOMetricUploadDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("VolumeSeriesIOMetricUpload error: %s", err.Error())
	}
	return err
}

// VolumeSeriesUpdate updates the mutable parts of a VolumeSeries object, always passing the version
func (c *Client) VolumeSeriesUpdate(ctx context.Context, o *models.VolumeSeries, items *Updates) (*models.VolumeSeries, error) {
	params := volume_series.NewVolumeSeriesUpdateParams()
	params.ID = string(o.Meta.ID)
	params.Payload = &o.VolumeSeriesMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	if items.Version > 0 {
		params.Version = swag.Int32(items.Version)
	}
	params.Context = ctx
	c.Log.Debugf("Updating VolumeSeries %s", params.ID)
	sAPI := c.ClientAPI.VolumeSeries()
	res, err := sAPI.VolumeSeriesUpdate(params)
	if err != nil {
		if e, ok := err.(*volume_series.VolumeSeriesUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update VolumeSeries %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// VSUpdaterCB is the type of a function called by VolumeSeriesUpdater to apply updates to a VolumeSeries object
// The callback can adjust the values of the arrays within the items structure
// if necessary, based on the state of the object.
// To do so the items structure must be in the scope of the modification function
// as it is not passed in as an argument to the callback.
// Warning: do not change the items structure pointer itself!
type VSUpdaterCB func(o *models.VolumeSeries) (*models.VolumeSeries, error)

// VolumeSeriesUpdater updates a VolumeSeries object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) VolumeSeriesUpdater(ctx context.Context, oID string, modifyFn VSUpdaterCB, items *Updates) (*models.VolumeSeries, error) {
	ua := &updaterArgs{
		typeName: "VolumeSeries",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.VolumeSeries))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) { return c.VolumeSeriesFetch(ctx, oID) },
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.VolumeSeriesUpdate(ctx, o.(*models.VolumeSeries), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.VolumeSeries)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.VolumeSeries), nil
}

// VolumeSeriesRequestCancel sets the cancelRequested property to true
func (c *Client) VolumeSeriesRequestCancel(ctx context.Context, objID string) (*models.VolumeSeriesRequest, error) {
	params := volume_series_request.NewVolumeSeriesRequestCancelParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Canceling VolumeSeriesRequest %s", params.ID)
	sAPI := c.ClientAPI.VolumeSeriesRequest()
	res, err := sAPI.VolumeSeriesRequestCancel(params)
	if err != nil {
		if e, ok := err.(*volume_series_request.VolumeSeriesRequestCancelDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Cancel VolumeSeriesRequest %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// VolumeSeriesRequestCreate creates a VolumeSeriesRequest object
func (c *Client) VolumeSeriesRequestCreate(ctx context.Context, o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
	params := volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{
		VolumeSeriesRequestCreateOnce:    o.VolumeSeriesRequestCreateOnce,
		VolumeSeriesRequestCreateMutable: o.VolumeSeriesRequestCreateMutable,
	}
	params.Context = ctx
	c.Log.Debugf("Creating VolumeSeriesRequest (operations %v node %s)", o.RequestedOperations, o.NodeID)
	vrAPI := c.ClientAPI.VolumeSeriesRequest()
	res, err := vrAPI.VolumeSeriesRequestCreate(params)
	if err != nil {
		if e, ok := err.(*volume_series_request.VolumeSeriesRequestCreateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Create VolumeSeriesRequest (operations %v node %s) error: %s", o.RequestedOperations, o.NodeID, err.Error())
		return nil, err
	}
	c.Log.Debugf("Created VolumeSeriesRequest (operations %v node %s): %s", o.RequestedOperations, o.NodeID, res.Payload.Meta.ID)
	return res.Payload, nil
}

// VolumeSeriesRequestFetch loads a VolumeSeriesRequest object
func (c *Client) VolumeSeriesRequestFetch(ctx context.Context, objID string) (*models.VolumeSeriesRequest, error) {
	params := volume_series_request.NewVolumeSeriesRequestFetchParams()
	params.ID = objID
	params.Context = ctx
	c.Log.Debugf("Fetching VolumeSeriesRequest %s", params.ID)
	sAPI := c.ClientAPI.VolumeSeriesRequest()
	res, err := sAPI.VolumeSeriesRequestFetch(params)
	if err != nil {
		if e, ok := err.(*volume_series_request.VolumeSeriesRequestFetchDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Fetch VolumeSeriesRequest %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// VolumeSeriesRequestList searches for VolumeSeriesRequest objects
func (c *Client) VolumeSeriesRequestList(ctx context.Context, lParams *volume_series_request.VolumeSeriesRequestListParams) (*volume_series_request.VolumeSeriesRequestListOK, error) {
	lParams.SetContext(ctx)
	c.Log.Debugf("Listing VolumeSeriesRequests %+v", lParams)
	api := c.ClientAPI.VolumeSeriesRequest()
	res, err := api.VolumeSeriesRequestList(lParams)
	if err != nil {
		if e, ok := err.(*volume_series_request.VolumeSeriesRequestListDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("List VolumeSeriesRequests error: %s", err.Error())
		return nil, err
	}
	c.Log.Debugf("found %d active VolumeSeriesRequests", len(res.Payload))
	return res, nil
}

// VolumeSeriesRequestUpdate updates the mutable part of a VolumeSeriesRequest
func (c *Client) VolumeSeriesRequestUpdate(ctx context.Context, vsr *models.VolumeSeriesRequest, items *Updates) (*models.VolumeSeriesRequest, error) {
	params := volume_series_request.NewVolumeSeriesRequestUpdateParams()
	params.ID = string(vsr.Meta.ID)
	params.Payload = &vsr.VolumeSeriesRequestMutable
	params.Set = items.Set
	params.Append = items.Append
	params.Remove = items.Remove
	params.Version = int32(vsr.Meta.Version) // required
	params.Context = ctx
	c.Log.Debugf("Updating VolumeSeriesRequest %s", params.ID)
	api := c.ClientAPI.VolumeSeriesRequest()
	res, err := api.VolumeSeriesRequestUpdate(params)
	if err != nil {
		if e, ok := err.(*volume_series_request.VolumeSeriesRequestUpdateDefault); ok {
			err = NewError(e.Payload)
		}
		c.Log.Errorf("Update VolumeSeriesRequest %s error: %s", params.ID, err.Error())
		return nil, err
	}
	return res.Payload, nil
}

// VSRUpdaterCB is the type of a function called by VolumeSeriesRequestUpdater to apply updates to a VolumeSeries object
type VSRUpdaterCB func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error)

// VolumeSeriesRequestUpdater updates a VolumeSeries object with retry on ErrorIDVerNotFound conflict.
// The invoker provides a modification callback function to apply modifications to the object.
// It will be invoked as follows:
// - The first call will not have an argument and should return the initial updated object if already available
// - Subsequent calls are made with a freshly loaded object and should apply the modifications to that object
func (c *Client) VolumeSeriesRequestUpdater(ctx context.Context, oID string, modifyFn VSRUpdaterCB, items *Updates) (*models.VolumeSeriesRequest, error) {
	ua := &updaterArgs{
		typeName: "VolumeSeriesRequest",
		modifyFn: func(o interface{}) (interface{}, error) {
			if o != nil && !reflect.ValueOf(o).IsNil() {
				return modifyFn(o.(*models.VolumeSeriesRequest))
			}
			return modifyFn(nil)
		},
		fetchFn: func(ctx context.Context, oID string) (interface{}, error) {
			return c.VolumeSeriesRequestFetch(ctx, oID)
		},
		updateFn: func(ctx context.Context, o interface{}, items *Updates) (interface{}, error) {
			return c.VolumeSeriesRequestUpdate(ctx, o.(*models.VolumeSeriesRequest), items)
		},
		metaFn: func(o interface{}) *models.ObjMeta { return (o.(*models.VolumeSeriesRequest)).Meta },
		oID:    oID,
		items:  items,
	}
	o, err := c.updater.objUpdate(ctx, oID, items, ua)
	if err != nil {
		return nil, err
	}
	return o.(*models.VolumeSeriesRequest), nil
}
