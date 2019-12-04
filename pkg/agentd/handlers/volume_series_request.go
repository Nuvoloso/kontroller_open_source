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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/go-openapi/runtime/middleware"
)

// Register handlers for VolumeSeriesRequest, only those that agentd will forward to centrald, all else will fail
func (c *HandlerComp) volumeSeriesRequestRegisterHandlers() {
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestCreateHandler = ops.VolumeSeriesRequestCreateHandlerFunc(c.volumeSeriesRequestCreate)
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestFetchHandler = ops.VolumeSeriesRequestFetchHandlerFunc(c.volumeSeriesRequestFetch)
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestListHandler = ops.VolumeSeriesRequestListHandlerFunc(c.volumeSeriesRequestList)
}

func (c *HandlerComp) volumeSeriesRequestCreate(params ops.VolumeSeriesRequestCreateParams) middleware.Responder {
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestCreateOnce: params.Payload.VolumeSeriesRequestCreateOnce,
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: params.Payload.VolumeSeriesRequestCreateMutable,
		},
	}
	obj, err := c.oCrud.VolumeSeriesRequestCreate(c.ctx, vr)
	if err != nil {
		e := c.ConvertError(err)
		return ops.NewVolumeSeriesRequestCreateDefault(int(e.Payload.Code)).WithPayload(&e.Payload)
	}
	return ops.NewVolumeSeriesRequestCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesRequestFetch(params ops.VolumeSeriesRequestFetchParams) middleware.Responder {
	obj, err := c.oCrud.VolumeSeriesRequestFetch(c.ctx, params.ID)
	if err != nil {
		e := c.ConvertError(err)
		return ops.NewVolumeSeriesRequestFetchDefault(int(e.Payload.Code)).WithPayload(&e.Payload)
	}
	return ops.NewVolumeSeriesRequestFetchOK().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesRequestList(params ops.VolumeSeriesRequestListParams) middleware.Responder {
	lParams := volume_series_request.NewVolumeSeriesRequestListParams()
	lParams.AccountID = params.AccountID
	lParams.ActiveOrTimeModifiedGE = params.ActiveOrTimeModifiedGE
	lParams.ClusterID = params.ClusterID
	lParams.ConsistencyGroupID = params.ConsistencyGroupID
	lParams.IsTerminated = params.IsTerminated
	lParams.NodeID = params.NodeID
	lParams.PoolID = params.PoolID
	lParams.ProtectionDomainID = params.ProtectionDomainID
	lParams.RequestedOperations = params.RequestedOperations
	lParams.RequestedOperationsNot = params.RequestedOperationsNot
	lParams.ServicePlanAllocationID = params.ServicePlanAllocationID
	lParams.ServicePlanID = params.ServicePlanID
	lParams.SnapshotID = params.SnapshotID
	lParams.StorageID = params.StorageID
	lParams.SyncCoordinatorID = params.SyncCoordinatorID
	lParams.SystemTags = params.SystemTags
	lParams.TenantAccountID = params.TenantAccountID
	lParams.VolumeSeriesID = params.VolumeSeriesID
	lParams.VolumeSeriesRequestState = params.VolumeSeriesRequestState

	list, err := c.oCrud.VolumeSeriesRequestList(c.ctx, lParams)
	if err != nil {
		e := c.ConvertError(err)
		return ops.NewVolumeSeriesRequestListDefault(int(e.Payload.Code)).WithPayload(&e.Payload)
	}
	return ops.NewVolumeSeriesRequestListOK().WithPayload(list.Payload)
}
