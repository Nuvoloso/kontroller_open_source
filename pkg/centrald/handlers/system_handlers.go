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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/system"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/go-openapi/runtime/middleware"
)

// register handlers for System
func (c *HandlerComp) systemRegisterHandlers() {
	c.app.API.SystemSystemFetchHandler = ops.SystemFetchHandlerFunc(c.systemFetch)
	c.app.API.SystemSystemUpdateHandler = ops.SystemUpdateHandlerFunc(c.systemUpdate)
	c.app.API.SystemSystemHostnameFetchHandler = ops.SystemHostnameFetchHandlerFunc(c.systemHostnameFetch)
}

var nmSystemMutable JSONToAttrNameMap

func (c *HandlerComp) systemMutableNameMap() JSONToAttrNameMap {
	if nmSystemMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmSystemMutable == nil {
			nmSystemMutable = c.makeJSONToAttrNameMap(models.SystemMutable{})
		}
	}
	return nmSystemMutable
}

// Handlers

func (c *HandlerComp) systemFetch(params ops.SystemFetchParams) middleware.Responder {
	obj, err := c.intSystemFetch()
	if err != nil {
		return ops.NewSystemFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewSystemFetchOK().WithPayload(obj)
}

func (c *HandlerComp) intSystemFetch() (*models.System, error) {
	obj, err := c.DS.OpsSystem().Fetch()
	if err != nil {
		return nil, err
	}
	// TBD Service is not yet persistent
	obj.Service = c.app.Service.ModelObj()
	return obj, nil
}

func (c *HandlerComp) systemUpdate(params ops.SystemUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateAppend: params.Append,
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.systemMutableNameMap(), "", params.Version, uP)
	if err != nil {
		return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = c.eUpdateInvalidMsg("missing payload")
		return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err := c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("ClusterUsagePolicy") {
		if err := c.validateClusterUsagePolicy(params.Payload.ClusterUsagePolicy, common.AccountSecretScopeGlobal); err != nil {
			return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("SnapshotManagementPolicy") {
		if err := c.validateSnapshotManagementPolicy(params.Payload.SnapshotManagementPolicy); err != nil {
			return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("SnapshotCatalogPolicy") {
		// Once set in the System object, this policy may not be cleared though its values may be modified over time.
		if params.Payload.SnapshotCatalogPolicy == nil || params.Payload.SnapshotCatalogPolicy.CspDomainID == "" || params.Payload.SnapshotCatalogPolicy.ProtectionDomainID == "" {
			err = c.eUpdateInvalidMsg("System snapshotCatalogPolicy may not be cleared")
			return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("VsrManagementPolicy") {
		if err := c.validateVSRManagementPolicy(params.Payload.VsrManagementPolicy); err != nil {
			return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("SystemTags") {
		if err = ai.InternalOK(); err != nil {
			return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	} else if err = ai.CapOK(centrald.SystemManagementCap); err != nil {
		return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsSystem().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewSystemUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewSystemUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) systemHostnameFetch(params ops.SystemHostnameFetchParams) middleware.Responder {
	hostname, err := c.intSystemHostnameFetch(params.HTTPRequest.Context())
	if err != nil {
		return ops.NewSystemHostnameFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewSystemHostnameFetchOK().WithPayload(hostname)
}

func (c *HandlerComp) intSystemHostnameFetch(ctx context.Context) (string, error) {
	sysObj, err := c.intSystemFetch()
	if err != nil {
		return "", c.eErrorNotFound("unable to fetch system: %s", err.Error())
	}
	if sysObj.ManagementHostCName != "" {
		return sysObj.ManagementHostCName, nil
	}
	cc := c.lookupClusterClient(c.app.AppArgs.ClusterType)
	if cc == nil {
		return "", c.eInvalidData("support for clusterType '%s' missing", cluster.K8sClusterType)
	}
	service, err := cc.GetService(ctx, c.app.AppArgs.ManagementServiceName, c.app.AppArgs.ManagementServiceNamespace)
	if err != nil {
		return "", c.eErrorNotFound("%s", err.Error())
	}
	return service.Hostname, nil
}
