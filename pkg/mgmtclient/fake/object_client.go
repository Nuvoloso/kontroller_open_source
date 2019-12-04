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


package fake

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/watchers"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
)

// Client implements a fake crud.ClientOps interface
type Client struct {
	// Variable names exhibit no sane pattern; they are mnemonics from the original
	// centrald/sreq ancestor that was generalized to this package.
	// Use sane values for newly introduced variables.

	// AccountFetch
	InAccFetchID   string
	RetAccFetchObj *models.Account
	RetAccFetchErr error

	// AccountList
	InAccListParams *account.AccountListParams
	RetAccListObj   *account.AccountListOK
	RetAccListErr   error

	// ApplicationGroupCreate
	InAGCreateCtx  context.Context
	InAGCreateObj  *models.ApplicationGroup
	RetAGCreateObj *models.ApplicationGroup
	RetAGCreateErr error

	// ApplicationGroupDelete
	InAGDeleteCtx  context.Context
	InAGDeleteID   string
	RetAGDeleteErr error

	// ApplicationGroupFetch
	InAGFetchID   string
	RetAGFetchObj *models.ApplicationGroup
	RetAGFetchErr error

	// ApplicationGroupList
	InAGListParams *application_group.ApplicationGroupListParams
	RetAGListObj   *application_group.ApplicationGroupListOK
	RetAGListErr   error

	// ApplicationGroupUpdate
	InAGUpdateCtx          context.Context
	InAGUpdateItems        *crud.Updates
	InAGUpdateObj          *models.ApplicationGroup
	PassThroughAGUpdateObj bool
	RetAGUpdateObj         *models.ApplicationGroup
	RetAGUpdateErr         error

	// ApplicationGroupUpdater
	InAGUpdaterCtx        context.Context
	InAGUpdaterID         string
	InAGUpdaterItems      *crud.Updates
	ModAGUpdaterObj       *models.ApplicationGroup
	ModAGUpdaterErr       error
	FetchAGUpdaterObj     *models.ApplicationGroup
	ModAGUpdaterObj2      *models.ApplicationGroup
	RetAGUpdaterUpdateErr error
	RetAGUpdaterObj       *models.ApplicationGroup
	RetAGUpdaterErr       error

	// ClusterFetch
	InLClID   string
	CntLCl    int
	RetLClObj *models.Cluster
	RetLClErr error

	// ClusterUpdate
	InClUpdateCtx   context.Context
	InClUpdateObj   *models.Cluster
	InClUpdateItems *crud.Updates
	RetClUpdateObj  *models.Cluster
	RetClUpdateErr  error

	// ClusterUpdater
	InClUpdaterCtx        context.Context
	InClUpdaterItems      *crud.Updates
	InClUpdaterID         string
	ForceFetchClUpdater   bool
	FetchClUpdaterObj     *models.Cluster
	ModClUpdaterObj       *models.Cluster
	ModClUpdaterErr       error
	ModClUpdaterObj2      *models.Cluster
	RetClUpdaterUpdateErr error
	RetClUpdaterErr       error
	RetClUpdaterObj       *models.Cluster
	RetClUpdatedObj       *models.Cluster

	// ConsistencyGroupCreate
	InCGCreateCtx  context.Context
	InCGCreateObj  *models.ConsistencyGroup
	RetCGCreateObj *models.ConsistencyGroup
	RetCGCreateErr error

	// ConsistencyGroupDelete fakes its namesake interface method
	InCGDeleteCtx  context.Context
	InCGDeleteID   string
	RetCGDeleteErr error

	// ConsistencyGroupFetch
	InCGFetchID   string
	RetCGFetchObj *models.ConsistencyGroup
	RetCGFetchErr error

	// ConsistencyGroupList
	InCGListParams *consistency_group.ConsistencyGroupListParams
	RetCGListObj   *consistency_group.ConsistencyGroupListOK
	RetCGListErr   error

	// ConsistencyGroupUpdate
	InCGUpdateCtx          context.Context
	InCGUpdateItems        *crud.Updates
	InCGUpdateObj          *models.ConsistencyGroup
	PassThroughCGUpdateObj bool
	RetCGUpdateObj         *models.ConsistencyGroup
	RetCGUpdateErr         error

	// ConsistencyGroupUpdater
	InCGUpdaterCtx        context.Context
	InCGUpdaterID         string
	InCGUpdaterItems      *crud.Updates
	ModCGUpdaterObj       *models.ConsistencyGroup
	ModCGUpdaterErr       error
	FetchCGUpdaterObj     *models.ConsistencyGroup
	ModCGUpdaterObj2      *models.ConsistencyGroup
	RetCGUpdaterUpdateErr error
	RetCGUpdaterObj       *models.ConsistencyGroup
	RetCGUpdaterErr       error

	// CSPDomainFetch
	CntLD    int
	InLDid   string
	RetLDObj *models.CSPDomain
	RetLDErr error

	// CSPDomainUpdate
	InCdUpdateCtx   context.Context
	InCdUpdateObj   *models.CSPDomain
	InCdUpdateItems *crud.Updates
	RetCdUpdateObj  *models.CSPDomain
	RetCdUpdateErr  error

	// CSPDomainUpdater
	InCdUpdaterCtx        context.Context
	InCdUpdaterItems      *crud.Updates
	InCdUpdaterID         string
	ForceFetchCdUpdater   bool
	FetchCdUpdaterObj     *models.CSPDomain
	ModCdUpdaterObj       *models.CSPDomain
	ModCdUpdaterErr       error
	ModCdUpdaterObj2      *models.CSPDomain
	RetCdUpdaterUpdateErr error
	RetCdUpdaterErr       error
	RetCdUpdaterObj       *models.CSPDomain
	RetCdUpdatedObj       *models.CSPDomain

	// NodeCreate
	InNodeCreateCtx  context.Context
	InNodeCreateObj  *models.Node
	RetNodeCreateObj *models.Node
	RetNodeCreateErr error

	// NodeDelete
	InNodeDeleteCtx  context.Context
	InNodeDeleteObj  string
	RetNodeDeleteErr error

	// NodeFetch
	InNCtx  context.Context
	InNId   string
	RetNErr error
	RetNObj *models.Node

	// NodeList
	InLsNObj     *node.NodeListParams
	RetLsNErr    error
	RetLsNObj    *node.NodeListOK
	FutureLsNObj []*node.NodeListOK

	// NodeUpdate
	InNodeUpdateCtx          context.Context
	InNodeUpdateObj          *models.Node
	InNodeUpdateItems        *crud.Updates
	PassThroughNodeUpdateObj bool
	RetNodeUpdateObj         *models.Node
	RetNodeUpdateErr         error

	// NodeUpdater
	InNodeUpdaterCtx        context.Context
	InNodeUpdaterItems      *crud.Updates
	InNodeUpdaterID         string
	ForceFetchNodeUpdater   bool
	FetchNodeUpdaterObj     *models.Node
	ModNodeUpdaterObj       *models.Node
	ModNodeUpdaterErr       error
	ModNodeUpdaterObj2      *models.Node
	RetNodeUpdaterUpdateErr error
	RetNodeUpdaterErr       error
	RetNodeUpdaterObj       *models.Node
	RetNodeUpdatedObj       *models.Node

	// ServicePlanAllocationCreate
	InSPACreateCtx  context.Context
	InSPACreateObj  *models.ServicePlanAllocation
	RetSPACreateErr error
	RetSPACreateObj *models.ServicePlanAllocation

	// ServicePlanAllocationDelete
	InSPADeleteCtx  context.Context
	InSPADeleteID   string
	RetSPADeleteErr error

	// ServicePlanAllocationFetch
	InSPAFetchID   string
	RetSPAFetchObj *models.ServicePlanAllocation
	RetSPAFetchErr error

	// ServicePlanAllocationList
	InSPAListParams *service_plan_allocation.ServicePlanAllocationListParams
	RetSPAListOK    *service_plan_allocation.ServicePlanAllocationListOK
	RetSPAListErr   error

	// ServicePlanAllocationUpdate
	InSPAUpdateCtx   context.Context
	InSPAUpdateObj   *models.ServicePlanAllocation
	InSPAUpdateItems *crud.Updates
	RetSPAUpdateObj  *models.ServicePlanAllocation
	RetSPAUpdateErr  error

	// ServicePlanAllocationUpdater
	InSPAUpdaterCtx        context.Context
	InSPAUpdaterItems      *crud.Updates
	InSPAUpdaterID         string
	ForceFetchSPAUpdater   bool
	FetchSPAUpdaterObj     *models.ServicePlanAllocation
	ModSPAUpdaterObj       *models.ServicePlanAllocation
	ModSPAUpdaterErr       error
	ModSPAUpdaterObj2      *models.ServicePlanAllocation
	RetSPAUpdaterUpdateErr error
	RetSPAUpdaterErr       error
	RetSPAUpdaterObj       *models.ServicePlanAllocation
	RetSPAUpdatedObj       *models.ServicePlanAllocation

	// ServicePlanFetch
	InSvPFetchID   string
	RetSvPFetchObj *models.ServicePlan
	RetSvPFetchErr error

	// ServicePlanList
	InLsSvPObj  *service_plan.ServicePlanListParams
	RetLsSvPErr error
	RetLsSvPObj *service_plan.ServicePlanListOK

	// ServicePlanUpdate
	InSvPUpdateCtx   context.Context
	InSvPUpdateObj   *models.ServicePlan
	InSvPUpdateItems *crud.Updates
	RetSvPUpdateObj  *models.ServicePlan
	RetSvPUpdateErr  error

	// ServicePlanUpdater
	InSvPUpdaterCtx        context.Context
	InSvPUpdaterItems      *crud.Updates
	InSvPUpdaterID         string
	ForceFetchSvPUpdater   bool
	FetchSvPUpdaterObj     *models.ServicePlan
	ModSvPUpdaterObj       *models.ServicePlan
	ModSvPUpdaterErr       error
	ModSvPUpdaterObj2      *models.ServicePlan
	RetSvPUpdaterUpdateErr error
	RetSvPUpdaterErr       error
	RetSvPUpdaterObj       *models.ServicePlan
	RetSvPUpdatedObj       *models.ServicePlan

	// SnapshotCreate
	InCSSnapshotObj           *models.Snapshot
	RetCSSnapshotCreateObjErr error
	RetCSSnapshotObj          *models.Snapshot

	// SnapshotList
	InLsSnapshotObj  *snapshot.SnapshotListParams
	RetLsSnapshotErr error
	RetLsSnapshotOk  *snapshot.SnapshotListOK

	// SnapshotDelete
	InDSnapshotObj  string
	RetDSnapshotErr error

	// StorageCreate
	InCSobj  *models.Storage
	RetCSErr error
	RetCSObj *models.Storage

	// StorageDelete
	InDSObj  string
	RetDSErr error

	// StorageList
	InLsSObj  *storage.StorageListParams
	RetLsSErr error
	RetLsSOk  *storage.StorageListOK

	// StorageFetch
	StorageFetchID     string
	StorageFetchRetObj *models.Storage
	StorageFetchRetErr error

	// StorageIOMetricUpload
	InSIoMData []*models.IoMetricDatum
	RetSIoMErr error

	// StorageUpdate
	InUSitems        *crud.Updates
	InUSObj          *models.Storage
	RetUSErr         error
	RetUSObj         *models.Storage
	PassThroughUSObj bool

	// StorageFormulaList
	InLsSFObj  *storage_formula.StorageFormulaListParams
	RetLsSFErr error
	RetLsSFOk  *storage_formula.StorageFormulaListOK

	// PoolCreate
	InPoolCreateCtx  context.Context
	InPoolCreateObj  *models.Pool
	RetPoolCreateObj *models.Pool
	RetPoolCreateErr error

	// PoolDelete
	InPoolDeleteCtx  context.Context
	InPoolDeleteObj  string
	RetPoolDeleteErr error

	// PoolFetch
	InPoolFetchID   string
	RetPoolFetchErr error
	RetPoolFetchObj *models.Pool

	// PoolList
	InPoolListObj  *pool.PoolListParams
	RetPoolListObj *pool.PoolListOK
	RetPoolListErr error

	// PoolUpdate
	InPoolUpdateCtx   context.Context
	InPoolUpdateObj   *models.Pool
	InPoolUpdateItems *crud.Updates
	RetPoolUpdateObj  *models.Pool
	RetPoolUpdateErr  error

	// PoolUpdater
	InPoolUpdaterCtx        context.Context
	InPoolUpdaterItems      *crud.Updates
	InPoolUpdaterID         string
	ForceFetchPoolUpdater   bool
	FetchPoolUpdaterObj     *models.Pool
	ModPoolUpdaterObj       *models.Pool
	ModPoolUpdaterErr       error
	ModPoolUpdaterObj2      *models.Pool
	RetPoolUpdaterUpdateErr error
	RetPoolUpdaterErr       error
	RetPoolUpdaterObj       *models.Pool
	RetPoolUpdatedObj       *models.Pool

	// ProtectionDomainFetch
	InPDFetchID   string
	RetPDFetchErr error
	RetPDFetchObj *models.ProtectionDomain

	// StorageRequestCreate
	InSRCArgs         *models.StorageRequest
	RetSRCObj         *models.StorageRequest
	RetSRCErr         error
	PassThroughSRCObj bool

	// StorageRequestList
	InLsSRObj  *storage_request.StorageRequestListParams
	RetLsSRObj *storage_request.StorageRequestListOK
	RetLsSRErr error

	// StorageRequestFetch
	InSRFetchID   string
	RetSRFetchObj *models.StorageRequest
	RetSRFetchErr error

	// StorageRequestUpdate
	InUSRObj          *models.StorageRequest
	InUSRItems        *crud.Updates
	RetUSRErr         error
	RetUSRObj         *models.StorageRequest
	PassThroughUSRObj bool

	// StorageRequestUpdater
	SRUpdaterClone        bool
	SRUpdaterClonedObj    *models.StorageRequest
	InSRUpdaterItems      *crud.Updates
	InSRUpdaterID         string
	FetchSRUpdaterObj     *models.StorageRequest
	ModSRUpdaterObj       *models.StorageRequest
	ModSRUpdaterErr       error
	ModSRUpdaterObj2      *models.StorageRequest
	RetSRUpdaterUpdateErr error
	RetSRUpdaterErr       error
	RetSRUpdaterObj       *models.StorageRequest
	RetSRUpdatedObj       *models.StorageRequest

	// SystemFetch
	RetLSErr error
	RetLSObj *models.System

	// SystemHostnameFetch
	RetSHErr  error
	RetSHName string

	// TaskCreate
	InTaskCreateObj  *models.Task
	RetTaskCreateObj *models.Task
	RetTaskCreateErr error

	// VolumeSeriesCreate
	InCVCtx  context.Context
	InCVObj  *models.VolumeSeries
	RetCVErr error
	RetCVObj *models.VolumeSeries

	// VolumeSeriesDelete
	InDVCtx  context.Context
	InDVObj  string
	RetDVErr error

	// VolumeSeriesFetch
	InVFetchID string
	RetVObj    *models.VolumeSeries
	RetVErr    error

	// VolumeSeriesIOMetricUpload
	InVIoMData []*models.IoMetricDatum
	RetVIoMErr error

	// VolumeSeriesList
	InLsVCtx  context.Context
	InLsVObj  *volume_series.VolumeSeriesListParams
	RetLsVErr error
	RetLsVOk  *volume_series.VolumeSeriesListOK

	// VolumeSeriesNewID
	RetVsNewIDVt  *models.ValueType
	RetVsNewIDErr error

	// VolumeSeriesUpdate
	InUVCtx          context.Context
	InUVItems        *crud.Updates
	InUVObj          *models.VolumeSeries
	RetUVErr         error
	RetUVObj         *models.VolumeSeries
	PassThroughUVObj bool

	// VolumeSeriesUpdater
	InVSUpdaterCtx        context.Context
	InVSUpdaterItems      *crud.Updates
	InVSUpdaterID         string
	ForceFetchVSUpdater   bool
	FetchVSUpdaterObj     *models.VolumeSeries
	ModVSUpdaterObj       *models.VolumeSeries
	ModVSUpdaterErr       error
	ModVSUpdaterObj2      *models.VolumeSeries
	RetVSUpdaterUpdateErr error
	RetVSUpdaterErr       error
	RetVSUpdaterObj       *models.VolumeSeries

	// VolumeSeriesRequestCancel
	InVsrCancelCtx  context.Context
	InVsrCancelID   string
	RetVsrCancelObj *models.VolumeSeriesRequest
	RetVsrCancelErr error

	// VolumeSeriesRequestCreate
	InVRCCtx          context.Context
	InVRCArgs         *models.VolumeSeriesRequest
	RetVRCObj         *models.VolumeSeriesRequest
	RetVRCErr         error
	PassThroughVRCObj bool

	// VolumeSeriesRequestFetch
	InVRFetchID string
	RetVRObj    *models.VolumeSeriesRequest
	RetVRErr    error

	// VolumeSeriesRequestList
	InLsVRCtx  context.Context
	InLsVRObj  *volume_series_request.VolumeSeriesRequestListParams
	RetLsVRObj *volume_series_request.VolumeSeriesRequestListOK
	RetLsVRErr error

	// VolumeSeriesRequestUpdate
	InUVRCtx          context.Context
	InUVRObj          *models.VolumeSeriesRequest
	InUVRItems        *crud.Updates
	InUVRCount        int
	RetUVRErr         error
	RetUVRObj         *models.VolumeSeriesRequest
	PassThroughUVRObj bool

	// VolumeSeriesRequestUpdater
	InVSRUpdaterCtx        context.Context
	InVSRUpdaterItems      *crud.Updates
	InVSRUpdaterID         string
	ForceFetchVSRUpdater   bool
	FetchVSRUpdaterObj     *models.VolumeSeriesRequest
	ModVSRUpdaterObj       *models.VolumeSeriesRequest
	ModVSRUpdaterErr       error
	ModVSRUpdaterObj2      *models.VolumeSeriesRequest
	ModVSRUpdaterErr2      error
	RetVSRUpdaterUpdateErr error
	RetVSRUpdaterErr       error
	RetVSRUpdaterObj       *models.VolumeSeriesRequest
}

var _ = crud.Ops(&Client{})

// ExistsErr can be used to test Exists error cases
var ExistsErr = &crud.Error{Payload: models.Error{Code: http.StatusConflict, Message: swag.String(com.ErrorExists)}}

// NotFoundErr can be used to test NotFound error cases
var NotFoundErr = &crud.Error{Payload: models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}}

// TransientErr can be used to test transient error cases
var TransientErr = &crud.Error{Payload: models.Error{Code: http.StatusInternalServerError, Message: swag.String("transientErr")}}

// AccountFetch fakes its namesake interface method
func (fc *Client) AccountFetch(ctx context.Context, objID string) (*models.Account, error) {
	fc.InAccFetchID = objID
	return fc.RetAccFetchObj, fc.RetAccFetchErr
}

// AccountList fakes its namesake interface method
func (fc *Client) AccountList(ctx context.Context, lParams *account.AccountListParams) (*account.AccountListOK, error) {
	fc.InAccListParams = lParams
	return fc.RetAccListObj, fc.RetAccListErr
}

// ApplicationGroupCreate fakes its namesake interface method
func (fc *Client) ApplicationGroupCreate(ctx context.Context, o *models.ApplicationGroup) (*models.ApplicationGroup, error) {
	fc.InAGCreateCtx = ctx
	fc.InAGCreateObj = o
	return fc.RetAGCreateObj, fc.RetAGCreateErr
}

// ApplicationGroupDelete fakes its namesake interface method
func (fc *Client) ApplicationGroupDelete(ctx context.Context, objID string) error {
	fc.InAGDeleteCtx = ctx
	fc.InAGDeleteID = objID
	return fc.RetAGDeleteErr
}

// ApplicationGroupFetch fakes its namesake interface method
func (fc *Client) ApplicationGroupFetch(ctx context.Context, objID string) (*models.ApplicationGroup, error) {
	fc.InAGFetchID = objID
	return fc.RetAGFetchObj, fc.RetAGFetchErr
}

// ApplicationGroupList fakes its namesake interface method
func (fc *Client) ApplicationGroupList(ctx context.Context, lParams *application_group.ApplicationGroupListParams) (*application_group.ApplicationGroupListOK, error) {
	fc.InAGListParams = lParams
	return fc.RetAGListObj, fc.RetAGListErr
}

// ApplicationGroupUpdate fakes its namesake interface method
func (fc *Client) ApplicationGroupUpdate(ctx context.Context, o *models.ApplicationGroup, items *crud.Updates) (*models.ApplicationGroup, error) {
	fc.InAGUpdateCtx = ctx
	fc.InAGUpdateItems = items
	fc.InAGUpdateObj = o
	if fc.PassThroughAGUpdateObj {
		return o, nil
	}
	return fc.RetAGUpdateObj, fc.RetAGUpdateErr
}

// ApplicationGroupUpdater fakes its namesake interface method
func (fc *Client) ApplicationGroupUpdater(ctx context.Context, oID string, modifyFn crud.AGUpdaterCB, items *crud.Updates) (*models.ApplicationGroup, error) {
	fc.InAGUpdaterCtx = ctx
	fc.InAGUpdaterID = oID
	fc.InAGUpdaterItems = items
	if fc.RetAGUpdaterObj == nil && fc.RetAGUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModAGUpdaterObj = o
		fc.ModAGUpdaterErr = err
		if o == nil && err == nil {
			o = fc.FetchAGUpdaterObj
			o, err = modifyFn(o)
			fc.ModAGUpdaterObj2 = o
		} else if err == nil && fc.RetAGUpdaterUpdateErr != nil {
			return nil, fc.RetAGUpdaterUpdateErr
		}
		return o, err
	}
	return fc.RetAGUpdaterObj, fc.RetAGUpdaterErr
}

// ClusterFetch fakes its namesake interface method
func (fc *Client) ClusterFetch(ctx context.Context, objID string) (*models.Cluster, error) {
	fc.InLClID = objID
	fc.CntLCl++
	return fc.RetLClObj, fc.RetLClErr
}

// ClusterUpdate fakes its namesake interface method
func (fc *Client) ClusterUpdate(ctx context.Context, o *models.Cluster, items *crud.Updates) (*models.Cluster, error) {
	fc.InClUpdateCtx = ctx
	fc.InClUpdateObj = o
	fc.InClUpdateItems = items
	return fc.RetClUpdateObj, fc.RetClUpdateErr
}

// ClusterUpdater fakes its namesake interface method
func (fc *Client) ClusterUpdater(ctx context.Context, oID string, modifyFn crud.ClusterUpdaterCB, items *crud.Updates) (*models.Cluster, error) {
	fc.InClUpdaterCtx = ctx
	fc.InClUpdaterID = oID
	fc.InClUpdaterItems = items
	if fc.RetClUpdaterObj == nil && fc.RetClUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModClUpdaterObj = o
		fc.ModClUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchClUpdater {
			o = fc.FetchClUpdaterObj
			o, err = modifyFn(o)
			fc.ModClUpdaterObj2 = o
		} else if err == nil && fc.RetClUpdaterUpdateErr != nil {
			return nil, fc.RetClUpdaterUpdateErr
		}
		if fc.RetClUpdatedObj != nil {
			o = fc.RetClUpdatedObj
		}
		return o, err
	}
	return fc.RetClUpdaterObj, fc.RetClUpdaterErr
}

// ConsistencyGroupCreate fakes its namesake interface method
func (fc *Client) ConsistencyGroupCreate(ctx context.Context, o *models.ConsistencyGroup) (*models.ConsistencyGroup, error) {
	fc.InCGCreateCtx = ctx
	fc.InCGCreateObj = o
	return fc.RetCGCreateObj, fc.RetCGCreateErr
}

// ConsistencyGroupDelete fakes its namesake interface method
func (fc *Client) ConsistencyGroupDelete(ctx context.Context, objID string) error {
	fc.InCGDeleteCtx = ctx
	fc.InCGDeleteID = objID
	return fc.RetCGDeleteErr
}

// ConsistencyGroupFetch fakes its namesake interface method
func (fc *Client) ConsistencyGroupFetch(ctx context.Context, objID string) (*models.ConsistencyGroup, error) {
	fc.InCGFetchID = objID
	return fc.RetCGFetchObj, fc.RetCGFetchErr
}

// ConsistencyGroupList fakes its namesake interface method
func (fc *Client) ConsistencyGroupList(ctx context.Context, lParams *consistency_group.ConsistencyGroupListParams) (*consistency_group.ConsistencyGroupListOK, error) {
	fc.InCGListParams = lParams
	return fc.RetCGListObj, fc.RetCGListErr
}

// ConsistencyGroupUpdate fakes its namesake interface method
func (fc *Client) ConsistencyGroupUpdate(ctx context.Context, o *models.ConsistencyGroup, items *crud.Updates) (*models.ConsistencyGroup, error) {
	fc.InCGUpdateCtx = ctx
	fc.InCGUpdateItems = items
	fc.InCGUpdateObj = o
	if fc.PassThroughCGUpdateObj {
		return o, nil
	}
	return fc.RetCGUpdateObj, fc.RetCGUpdateErr
}

// ConsistencyGroupUpdater fakes its namesake interface method
func (fc *Client) ConsistencyGroupUpdater(ctx context.Context, oID string, modifyFn crud.CGUpdaterCB, items *crud.Updates) (*models.ConsistencyGroup, error) {
	fc.InCGUpdaterCtx = ctx
	fc.InCGUpdaterID = oID
	fc.InCGUpdaterItems = items
	if fc.RetCGUpdaterObj == nil && fc.RetCGUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModCGUpdaterObj = o
		fc.ModCGUpdaterErr = err
		if o == nil && err == nil {
			o = fc.FetchCGUpdaterObj
			o, err = modifyFn(o)
			fc.ModCGUpdaterObj2 = o
		} else if err == nil && fc.RetCGUpdaterUpdateErr != nil {
			return nil, fc.RetCGUpdaterUpdateErr
		}
		return o, err
	}
	return fc.RetCGUpdaterObj, fc.RetCGUpdaterErr
}

// CSPDomainFetch fakes its namesake interface method
func (fc *Client) CSPDomainFetch(ctx context.Context, objID string) (*models.CSPDomain, error) {
	fc.InLDid = objID
	fc.CntLD++
	return fc.RetLDObj, fc.RetLDErr
}

// CSPDomainList fakes its namesake interface method
func (fc *Client) CSPDomainList(ctx context.Context, lParams *csp_domain.CspDomainListParams) (*csp_domain.CspDomainListOK, error) {
	return nil, nil
}

// CSPDomainUpdate fakes its namesake interface method
func (fc *Client) CSPDomainUpdate(ctx context.Context, o *models.CSPDomain, items *crud.Updates) (*models.CSPDomain, error) {
	fc.InCdUpdateCtx = ctx
	fc.InCdUpdateObj = o
	fc.InCdUpdateItems = items
	return fc.RetCdUpdateObj, fc.RetCdUpdateErr
}

// CSPDomainUpdater fakes its namesake interface method
func (fc *Client) CSPDomainUpdater(ctx context.Context, oID string, modifyFn crud.CspDomianUpdaterCB, items *crud.Updates) (*models.CSPDomain, error) {
	fc.InCdUpdaterCtx = ctx
	fc.InCdUpdaterID = oID
	fc.InCdUpdaterItems = items
	if fc.RetCdUpdaterObj == nil && fc.RetCdUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModCdUpdaterObj = o
		fc.ModCdUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchCdUpdater {
			o = fc.FetchCdUpdaterObj
			o, err = modifyFn(o)
			fc.ModCdUpdaterObj2 = o
		} else if err == nil && fc.RetCdUpdaterUpdateErr != nil {
			return nil, fc.RetCdUpdaterUpdateErr
		}
		if fc.RetCdUpdatedObj != nil {
			o = fc.RetCdUpdatedObj
		}
		return o, err
	}
	return fc.RetCdUpdaterObj, fc.RetCdUpdaterErr
}

// NodeCreate fakes its namesake interface method
func (fc *Client) NodeCreate(ctx context.Context, o *models.Node) (*models.Node, error) {
	fc.InNodeCreateCtx = ctx
	fc.InNodeCreateObj = o
	return fc.RetNodeCreateObj, fc.RetNodeCreateErr
}

// NodeDelete fakes its namesake interface method
func (fc *Client) NodeDelete(ctx context.Context, objID string) error {
	fc.InNodeDeleteCtx = ctx
	fc.InNodeDeleteObj = objID
	return fc.RetNodeDeleteErr
}

// NodeFetch fakes its namesake interface method
func (fc *Client) NodeFetch(ctx context.Context, objID string) (*models.Node, error) {
	fc.InNCtx = ctx
	fc.InNId = objID
	return fc.RetNObj, fc.RetNErr
}

// NodeList fakes its namesake interface method
func (fc *Client) NodeList(ctx context.Context, lParams *node.NodeListParams) (*node.NodeListOK, error) {
	fc.InLsNObj = lParams
	rc := fc.RetLsNObj
	if len(fc.FutureLsNObj) > 0 { // pop
		fc.RetLsNObj, fc.FutureLsNObj = fc.FutureLsNObj[0], fc.FutureLsNObj[1:]
	}
	return rc, fc.RetLsNErr
}

// NodeUpdate fakes its namesake interface method
func (fc *Client) NodeUpdate(ctx context.Context, o *models.Node, items *crud.Updates) (*models.Node, error) {
	fc.InNodeUpdateCtx = ctx
	fc.InNodeUpdateObj = o
	fc.InNodeUpdateItems = items
	if fc.PassThroughNodeUpdateObj {
		return o, nil
	}
	return fc.RetNodeUpdateObj, fc.RetNodeUpdateErr
}

// NodeUpdater fakes its namesake interface method
func (fc *Client) NodeUpdater(ctx context.Context, oID string, modifyFn crud.NodeUpdaterCB, items *crud.Updates) (*models.Node, error) {
	fc.InNodeUpdaterCtx = ctx
	fc.InNodeUpdaterID = oID
	fc.InNodeUpdaterItems = items
	if fc.RetNodeUpdaterObj == nil && fc.RetNodeUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModNodeUpdaterObj = o
		fc.ModNodeUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchNodeUpdater {
			o = fc.FetchNodeUpdaterObj
			o, err = modifyFn(o)
			fc.ModNodeUpdaterObj2 = o
		} else if err == nil && fc.RetNodeUpdaterUpdateErr != nil {
			return nil, fc.RetNodeUpdaterUpdateErr
		}
		if fc.RetNodeUpdatedObj != nil {
			o = fc.RetNodeUpdatedObj
		}
		return o, err
	}
	return fc.RetNodeUpdaterObj, fc.RetNodeUpdaterErr
}

// PoolCreate fakes its namesake interface method
func (fc *Client) PoolCreate(ctx context.Context, o *models.Pool) (*models.Pool, error) {
	fc.InPoolCreateCtx = ctx
	fc.InPoolCreateObj = o
	return fc.RetPoolCreateObj, fc.RetPoolCreateErr
}

// PoolDelete fakes its namesake interface method
func (fc *Client) PoolDelete(ctx context.Context, objID string) error {
	fc.InPoolDeleteCtx = ctx
	fc.InPoolDeleteObj = objID
	return fc.RetPoolDeleteErr
}

// PoolFetch fakes its namesake interface method
func (fc *Client) PoolFetch(ctx context.Context, objID string) (*models.Pool, error) {
	fc.InPoolFetchID = objID
	return fc.RetPoolFetchObj, fc.RetPoolFetchErr
}

// PoolList fakes its namesake interface method
func (fc *Client) PoolList(ctx context.Context, lParams *pool.PoolListParams) (*pool.PoolListOK, error) {
	fc.InPoolListObj = lParams
	return fc.RetPoolListObj, fc.RetPoolListErr
}

// PoolUpdate fakes its namesake interface method
func (fc *Client) PoolUpdate(ctx context.Context, sp *models.Pool, items *crud.Updates) (*models.Pool, error) {
	fc.InPoolUpdateCtx = ctx
	fc.InPoolUpdateObj = sp
	fc.InPoolUpdateItems = items
	return fc.RetPoolUpdateObj, fc.RetPoolUpdateErr
}

// PoolUpdater fakes its namesake interface method
func (fc *Client) PoolUpdater(ctx context.Context, oID string, modifyFn crud.PoolUpdaterCB, items *crud.Updates) (*models.Pool, error) {
	fc.InPoolUpdaterCtx = ctx
	fc.InPoolUpdaterID = oID
	fc.InPoolUpdaterItems = items
	if fc.RetPoolUpdaterObj == nil && fc.RetPoolUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModPoolUpdaterObj = o
		fc.ModPoolUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchPoolUpdater {
			o = fc.FetchPoolUpdaterObj
			o, err = modifyFn(o)
			fc.ModPoolUpdaterObj2 = o
		} else if err == nil && fc.RetPoolUpdaterUpdateErr != nil {
			return nil, fc.RetPoolUpdaterUpdateErr
		}
		if fc.RetPoolUpdatedObj != nil {
			o = fc.RetPoolUpdatedObj
		}
		return o, err
	}
	return fc.RetPoolUpdaterObj, fc.RetPoolUpdaterErr
}

// ProtectionDomainCreate fakes its namesake interface method
func (fc *Client) ProtectionDomainCreate(ctx context.Context, o *models.ProtectionDomain) (*models.ProtectionDomain, error) {
	return nil, nil
}

// ProtectionDomainFetch fakes its namesake interface method
func (fc *Client) ProtectionDomainFetch(ctx context.Context, objID string) (*models.ProtectionDomain, error) {
	fc.InPDFetchID = objID
	return fc.RetPDFetchObj, fc.RetPDFetchErr
}

// ProtectionDomainList fakes its namesake interface method
func (fc *Client) ProtectionDomainList(ctx context.Context, lParams *protection_domain.ProtectionDomainListParams) (*protection_domain.ProtectionDomainListOK, error) {
	return nil, nil
}

// ProtectionDomainDelete fakes its namesake interface method
func (fc *Client) ProtectionDomainDelete(ctx context.Context, objID string) error {
	return nil
}

// ProtectionDomainUpdate fakes its namesake interface method
func (fc *Client) ProtectionDomainUpdate(ctx context.Context, o *models.ProtectionDomain, items *crud.Updates) (*models.ProtectionDomain, error) {
	return nil, nil
}

// ProtectionDomainUpdater fakes its namesake interface method
func (fc *Client) ProtectionDomainUpdater(ctx context.Context, oID string, modifyFn crud.ProtectionDomainUpdaterCB, items *crud.Updates) (*models.ProtectionDomain, error) {
	return nil, nil
}

// ServicePlanAllocationCreate fakes its namesake interface method
func (fc *Client) ServicePlanAllocationCreate(ctx context.Context, o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
	fc.InSPACreateCtx = ctx
	fc.InSPACreateObj = o
	return fc.RetSPACreateObj, fc.RetSPACreateErr
}

// ServicePlanAllocationDelete fakes its namesake interface method
func (fc *Client) ServicePlanAllocationDelete(ctx context.Context, objID string) error {
	fc.InSPADeleteCtx = ctx
	fc.InSPADeleteID = objID
	return fc.RetSPADeleteErr
}

// ServicePlanAllocationFetch fakes its namesake interface method
func (fc *Client) ServicePlanAllocationFetch(ctx context.Context, objID string) (*models.ServicePlanAllocation, error) {
	fc.InSPAFetchID = objID
	return fc.RetSPAFetchObj, fc.RetSPAFetchErr
}

// ServicePlanAllocationList fakes its namesake interface method
func (fc *Client) ServicePlanAllocationList(ctx context.Context, lParams *service_plan_allocation.ServicePlanAllocationListParams) (*service_plan_allocation.ServicePlanAllocationListOK, error) {
	fc.InSPAListParams = lParams
	return fc.RetSPAListOK, fc.RetSPAListErr
}

// ServicePlanAllocationUpdate fakes its namesake interface method
func (fc *Client) ServicePlanAllocationUpdate(ctx context.Context, o *models.ServicePlanAllocation, items *crud.Updates) (*models.ServicePlanAllocation, error) {
	fc.InSPAUpdateCtx = ctx
	fc.InSPAUpdateObj = o
	fc.InSPAUpdateItems = items
	return fc.RetSPAUpdateObj, fc.RetSPAUpdateErr
}

// ServicePlanAllocationUpdater fakes its namesake interface method
func (fc *Client) ServicePlanAllocationUpdater(ctx context.Context, oID string, modifyFn crud.SPAUpdaterCB, items *crud.Updates) (*models.ServicePlanAllocation, error) {
	fc.InSPAUpdaterCtx = ctx
	fc.InSPAUpdaterID = oID
	fc.InSPAUpdaterItems = items
	if fc.RetSPAUpdaterObj == nil && fc.RetSPAUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModSPAUpdaterObj = o
		fc.ModSPAUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchSPAUpdater {
			o = fc.FetchSPAUpdaterObj
			o, err = modifyFn(o)
			fc.ModSPAUpdaterObj2 = o
		} else if err == nil && fc.RetSPAUpdaterUpdateErr != nil {
			return nil, fc.RetSPAUpdaterUpdateErr
		}
		if fc.RetSPAUpdatedObj != nil {
			o = fc.RetSPAUpdatedObj
		}
		return o, err
	}
	return fc.RetSPAUpdaterObj, fc.RetSPAUpdaterErr
}

// ServicePlanFetch fakes its namesake interface method
func (fc *Client) ServicePlanFetch(ctx context.Context, objID string) (*models.ServicePlan, error) {
	fc.InSvPFetchID = objID
	return fc.RetSvPFetchObj, fc.RetSvPFetchErr
}

// ServicePlanList fakes its namesake interface method
func (fc *Client) ServicePlanList(ctx context.Context, lParams *service_plan.ServicePlanListParams) (*service_plan.ServicePlanListOK, error) {
	fc.InLsSvPObj = lParams
	return fc.RetLsSvPObj, fc.RetLsSvPErr
}

// ServicePlanUpdate fakes its namesake interface method
func (fc *Client) ServicePlanUpdate(ctx context.Context, o *models.ServicePlan, items *crud.Updates) (*models.ServicePlan, error) {
	fc.InSvPUpdateCtx = ctx
	fc.InSvPUpdateObj = o
	fc.InSvPUpdateItems = items
	return fc.RetSvPUpdateObj, fc.RetSvPUpdateErr
}

// ServicePlanUpdater fakes its namesake interface method
func (fc *Client) ServicePlanUpdater(ctx context.Context, oID string, modifyFn crud.SPlanUpdaterCB, items *crud.Updates) (*models.ServicePlan, error) {
	fc.InSvPUpdaterCtx = ctx
	fc.InSvPUpdaterID = oID
	fc.InSvPUpdaterItems = items
	if fc.RetSvPUpdaterObj == nil && fc.RetSvPUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModSvPUpdaterObj = o
		fc.ModSvPUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchSvPUpdater {
			o = fc.FetchSvPUpdaterObj
			o, err = modifyFn(o)
			fc.ModSvPUpdaterObj2 = o
		} else if err == nil && fc.RetSvPUpdaterUpdateErr != nil {
			return nil, fc.RetSvPUpdaterUpdateErr
		}
		if fc.RetSvPUpdatedObj != nil {
			o = fc.RetSvPUpdatedObj
		}
		return o, err
	}
	return fc.RetSvPUpdaterObj, fc.RetSvPUpdaterErr
}

// SnapshotCreate fakes its namesake interface method
func (fc *Client) SnapshotCreate(ctx context.Context, o *models.Snapshot) (*models.Snapshot, error) {
	fc.InCSSnapshotObj = o
	return fc.RetCSSnapshotObj, fc.RetCSSnapshotCreateObjErr
}

// SnapshotList fakes its namesake interface method
func (fc *Client) SnapshotList(ctx context.Context, lParams *snapshot.SnapshotListParams) (*snapshot.SnapshotListOK, error) {
	fc.InLsSnapshotObj = lParams
	return fc.RetLsSnapshotOk, fc.RetLsSnapshotErr
}

// SnapshotDelete fakes its namesake interface method
func (fc *Client) SnapshotDelete(ctx context.Context, objID string) error {
	fc.InDSnapshotObj = objID
	return fc.RetDSnapshotErr
}

// StorageCreate fakes its namesake interface method
func (fc *Client) StorageCreate(ctx context.Context, o *models.Storage) (*models.Storage, error) {
	fc.InCSobj = o
	return fc.RetCSObj, fc.RetCSErr
}

// StorageDelete fakes its namesake interface method
func (fc *Client) StorageDelete(ctx context.Context, objID string) error {
	fc.InDSObj = objID
	return fc.RetDSErr
}

// StorageFetch fakes its namesake interface method
func (fc *Client) StorageFetch(ctx context.Context, objID string) (*models.Storage, error) {
	fc.StorageFetchID = objID
	return fc.StorageFetchRetObj, fc.StorageFetchRetErr
}

// StorageIOMetricUpload fakes its namesake interface method
func (fc *Client) StorageIOMetricUpload(ctx context.Context, data []*models.IoMetricDatum) error {
	fc.InSIoMData = data
	return fc.RetSIoMErr
}

// StorageList fakes its namesake interface method
func (fc *Client) StorageList(ctx context.Context, lParams *storage.StorageListParams) (*storage.StorageListOK, error) {
	fc.InLsSObj = lParams
	return fc.RetLsSOk, fc.RetLsSErr
}

// StorageUpdate fakes its namesake interface method
func (fc *Client) StorageUpdate(ctx context.Context, o *models.Storage, items *crud.Updates) (*models.Storage, error) {
	fc.InUSitems = items
	fc.InUSObj = o
	if fc.PassThroughUSObj {
		return o, nil
	}
	return fc.RetUSObj, fc.RetUSErr
}

// StorageFormulaList fakes its namesake interface method
func (fc *Client) StorageFormulaList(ctx context.Context, lParams *storage_formula.StorageFormulaListParams) (*storage_formula.StorageFormulaListOK, error) {
	fc.InLsSFObj = lParams
	return fc.RetLsSFOk, fc.RetLsSFErr
}

// StorageRequestCreate fakes its namesake interface method
func (fc *Client) StorageRequestCreate(ctx context.Context, o *models.StorageRequest) (*models.StorageRequest, error) {
	fc.InSRCArgs = o
	if fc.PassThroughSRCObj {
		// must insert fake meta data
		o.Meta = &models.ObjMeta{
			ID:      models.ObjID(fmt.Sprintf("%p", o)),
			Version: 1,
		}
		return o, nil
	}
	return fc.RetSRCObj, fc.RetSRCErr
}

// StorageRequestList fakes its namesake interface method
func (fc *Client) StorageRequestList(ctx context.Context, lParams *storage_request.StorageRequestListParams) (*storage_request.StorageRequestListOK, error) {
	fc.InLsSRObj = lParams
	return fc.RetLsSRObj, fc.RetLsSRErr
}

// StorageRequestFetch fakes its namesake interface method
func (fc *Client) StorageRequestFetch(ctx context.Context, objID string) (*models.StorageRequest, error) {
	fc.InSRFetchID = objID
	return fc.RetSRFetchObj, fc.RetSRFetchErr
}

// StorageRequestUpdate fakes its namesake interface method
func (fc *Client) StorageRequestUpdate(ctx context.Context, sr *models.StorageRequest, items *crud.Updates) (*models.StorageRequest, error) {
	fc.InUSRObj = sr
	fc.InUSRItems = items
	if fc.PassThroughUSRObj {
		return sr, nil
	}
	return fc.RetUSRObj, fc.RetUSRErr
}

// StorageRequestUpdater fakes its namesake interface method
func (fc *Client) StorageRequestUpdater(ctx context.Context, oID string, modifyFn crud.SRUpdaterCB, items *crud.Updates) (*models.StorageRequest, error) {
	fc.InSRUpdaterID = oID
	fc.InSRUpdaterItems = items
	if fc.RetSRUpdaterObj == nil && fc.RetSRUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModSRUpdaterObj = o
		fc.ModSRUpdaterErr = err
		if o == nil && err == nil {
			o = fc.FetchSRUpdaterObj
			o, err = modifyFn(o)
			fc.ModSRUpdaterObj2 = o
		} else if err == nil && fc.RetSRUpdaterUpdateErr != nil {
			return nil, fc.RetSRUpdaterUpdateErr
		}
		if fc.RetSRUpdatedObj != nil {
			o = fc.RetSRUpdatedObj
		} else if fc.SRUpdaterClone {
			fc.SRUpdaterClonedObj = nil
			testutils.Clone(o, &fc.SRUpdaterClonedObj)
			fc.SRUpdaterClonedObj.Meta.Version++
			o = fc.SRUpdaterClonedObj
		}
		return o, err
	}
	return fc.RetSRUpdaterObj, fc.RetSRUpdaterErr
}

// SystemFetch fakes its namesake interface method
func (fc *Client) SystemFetch(ctx context.Context) (*models.System, error) {
	return fc.RetLSObj, fc.RetLSErr
}

// SystemHostnameFetch fakes its namesake interface method
func (fc *Client) SystemHostnameFetch(ctx context.Context) (string, error) {
	return fc.RetSHName, fc.RetSHErr
}

// TaskCreate fakes its namesake interface method
func (fc *Client) TaskCreate(ctx context.Context, o *models.Task) (*models.Task, error) {
	fc.InTaskCreateObj = o
	return fc.RetTaskCreateObj, fc.RetTaskCreateErr
}

// VolumeSeriesCreate fakes its namesake interface method
func (fc *Client) VolumeSeriesCreate(ctx context.Context, o *models.VolumeSeries) (*models.VolumeSeries, error) {
	fc.InCVCtx = ctx
	fc.InCVObj = o
	return fc.RetCVObj, fc.RetCVErr
}

// VolumeSeriesDelete fakes its namesake interface method
func (fc *Client) VolumeSeriesDelete(ctx context.Context, objID string) error {
	fc.InDVCtx = ctx
	fc.InDVObj = objID
	return fc.RetDVErr
}

// VolumeSeriesFetch fakes its namesake interface method
func (fc *Client) VolumeSeriesFetch(ctx context.Context, objID string) (*models.VolumeSeries, error) {
	fc.InVFetchID = objID
	return fc.RetVObj, fc.RetVErr
}

// VolumeSeriesIOMetricUpload fakes its namesake interface method
func (fc *Client) VolumeSeriesIOMetricUpload(ctx context.Context, data []*models.IoMetricDatum) error {
	fc.InVIoMData = data
	return fc.RetVIoMErr
}

// VolumeSeriesList fakes its namesake interface method
func (fc *Client) VolumeSeriesList(ctx context.Context, lParams *volume_series.VolumeSeriesListParams) (*volume_series.VolumeSeriesListOK, error) {
	fc.InLsVCtx = ctx
	fc.InLsVObj = lParams
	return fc.RetLsVOk, fc.RetLsVErr
}

// VolumeSeriesNewID fakes its namesake interface method
func (fc *Client) VolumeSeriesNewID(ctx context.Context) (*models.ValueType, error) {
	return fc.RetVsNewIDVt, fc.RetVsNewIDErr
}

// VolumeSeriesUpdate fakes its namesake interface method
func (fc *Client) VolumeSeriesUpdate(ctx context.Context, o *models.VolumeSeries, items *crud.Updates) (*models.VolumeSeries, error) {
	fc.InUVCtx = ctx
	fc.InUVItems = items
	fc.InUVObj = o
	if fc.PassThroughUVObj {
		return o, nil
	}
	return fc.RetUVObj, fc.RetUVErr
}

// VolumeSeriesUpdater fakes its namesake interface method
func (fc *Client) VolumeSeriesUpdater(ctx context.Context, vsID string, modifyFn crud.VSUpdaterCB, items *crud.Updates) (*models.VolumeSeries, error) {
	fc.InVSUpdaterCtx = ctx
	fc.InVSUpdaterID = vsID
	fc.InVSUpdaterItems = items
	if fc.RetVSUpdaterObj == nil && fc.RetVSUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModVSUpdaterObj = o
		fc.ModVSUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchVSUpdater {
			o = fc.FetchVSUpdaterObj
			o, err = modifyFn(o)
			fc.ModVSUpdaterObj2 = o
		} else if err == nil && fc.RetVSUpdaterUpdateErr != nil {
			return nil, fc.RetVSUpdaterUpdateErr
		}
		return o, err
	}
	return fc.RetVSUpdaterObj, fc.RetVSUpdaterErr
}

// VolumeSeriesRequestCancel fakes its namesake interface method
func (fc *Client) VolumeSeriesRequestCancel(ctx context.Context, objID string) (*models.VolumeSeriesRequest, error) {
	fc.InVsrCancelCtx = ctx
	fc.InVsrCancelID = objID
	return fc.RetVsrCancelObj, fc.RetVsrCancelErr
}

// VolumeSeriesRequestCreate fakes its namesake interface method
func (fc *Client) VolumeSeriesRequestCreate(ctx context.Context, o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
	fc.InVRCCtx = ctx
	fc.InVRCArgs = o
	if fc.PassThroughVRCObj {
		// must insert fake meta data
		o.Meta = &models.ObjMeta{
			ID:      models.ObjID(fmt.Sprintf("%p", o)),
			Version: 1,
		}
		return o, nil
	}
	return fc.RetVRCObj, fc.RetVRCErr
}

// VolumeSeriesRequestFetch fakes its namesake interface method
func (fc *Client) VolumeSeriesRequestFetch(ctx context.Context, objID string) (*models.VolumeSeriesRequest, error) {
	fc.InVRFetchID = objID
	return fc.RetVRObj, fc.RetVRErr
}

// VolumeSeriesRequestList fakes its namesake interface method
func (fc *Client) VolumeSeriesRequestList(ctx context.Context, lParams *volume_series_request.VolumeSeriesRequestListParams) (*volume_series_request.VolumeSeriesRequestListOK, error) {
	fc.InLsVRCtx = ctx
	fc.InLsVRObj = lParams
	return fc.RetLsVRObj, fc.RetLsVRErr
}

// VolumeSeriesRequestUpdate fakes its namesake interface method
func (fc *Client) VolumeSeriesRequestUpdate(ctx context.Context, vr *models.VolumeSeriesRequest, items *crud.Updates) (*models.VolumeSeriesRequest, error) {
	fc.InUVRCtx = ctx
	fc.InUVRObj = vr
	fc.InUVRItems = items
	fc.InUVRCount++
	if fc.PassThroughUVRObj {
		return vr, nil
	}
	return fc.RetUVRObj, fc.RetUVRErr
}

// VolumeSeriesRequestUpdater fakes its namesake interface method
func (fc *Client) VolumeSeriesRequestUpdater(ctx context.Context, oID string, modifyFn crud.VSRUpdaterCB, items *crud.Updates) (*models.VolumeSeriesRequest, error) {
	fc.InVSRUpdaterCtx = ctx
	fc.InVSRUpdaterID = oID
	fc.InVSRUpdaterItems = items
	if fc.RetVSRUpdaterObj == nil && fc.RetVSRUpdaterErr == nil {
		o, err := modifyFn(nil)
		fc.ModVSRUpdaterObj = o
		fc.ModVSRUpdaterErr = err
		if (o == nil && err == nil) || fc.ForceFetchVSRUpdater {
			o = fc.FetchVSRUpdaterObj
			o, err = modifyFn(o)
			fc.ModVSRUpdaterObj2 = o
			fc.ModVSRUpdaterErr2 = err
		} else if err == nil && fc.RetVSRUpdaterUpdateErr != nil {
			return nil, fc.RetVSRUpdaterUpdateErr
		}
		return o, err
	}
	return fc.RetVSRUpdaterObj, fc.RetVSRUpdaterErr
}

// WatcherCreate fakes its namesake interface method
func (fc *Client) WatcherCreate(params *watchers.WatcherCreateParams) (*watchers.WatcherCreateCreated, error) {
	return nil, nil
}

// WatcherFetch fakes its namesake interface method
func (fc *Client) WatcherFetch(params *watchers.WatcherFetchParams) (*watchers.WatcherFetchOK, error) {
	return nil, nil
}
