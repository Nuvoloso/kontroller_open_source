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
	"net/http"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register proxy handlers for Node
func (c *HandlerComp) registerNodeProxyHandlers() {
	c.app.API.NodeNodeCreateHandler = ops.NodeCreateHandlerFunc(c.nodeCreate)
	c.app.API.NodeNodeDeleteHandler = ops.NodeDeleteHandlerFunc(c.nodeDelete)
	c.app.API.NodeNodeFetchHandler = ops.NodeFetchHandlerFunc(c.nodeFetch)
	c.app.API.NodeNodeListHandler = ops.NodeListHandlerFunc(c.nodeList)
	c.app.API.NodeNodeUpdateHandler = ops.NodeUpdateHandlerFunc(c.nodeUpdate)
}

func (c *HandlerComp) proxyReady() *models.Error {
	if c.app.StateOps == nil || c.app.StateUpdater == nil {
		return &models.Error{
			Message: swag.String("service unavailable: initializing"),
			Code:    http.StatusServiceUnavailable,
		}
	}
	return nil
}

func (c *HandlerComp) nodeCreate(inParams ops.NodeCreateParams) middleware.Responder {
	if e := c.proxyReady(); e != nil {
		return ops.NewNodeCreateDefault(int(e.Code)).WithPayload(e)
	}
	p := node.NewNodeCreateParams().WithPayload(inParams.Payload).WithContext(inParams.HTTPRequest.Context())
	if p.Payload.Service != nil {
		p.Payload.Service.HeartbeatPeriodSecs = c.app.StateUpdater.HeartbeatTaskPeriodSecs()
	}
	res, err := c.app.ClientAPI.Node().NodeCreate(p)
	if err != nil {
		e := c.nodeError(err)
		return ops.NewNodeCreateDefault(int(e.Code)).WithPayload(e)
	}
	rc := ops.NewNodeCreateCreated().WithPayload(res.Payload)
	if rc.Payload.Service != nil {
		c.app.StateOps.NodeServiceState(string(rc.Payload.Meta.ID), &rc.Payload.Service.ServiceState)
	}
	return rc
}

func (c *HandlerComp) nodeDelete(inParams ops.NodeDeleteParams) middleware.Responder {
	if e := c.proxyReady(); e != nil {
		return ops.NewNodeDeleteDefault(int(e.Code)).WithPayload(e)
	}
	p := node.NewNodeDeleteParams().WithID(inParams.ID).WithContext(inParams.HTTPRequest.Context())
	_, err := c.app.ClientAPI.Node().NodeDelete(p)
	if err != nil {
		e := c.nodeError(err)
		return ops.NewNodeDeleteDefault(int(e.Code)).WithPayload(e)
	}
	if c.app.StateOps != nil {
		c.app.StateOps.NodeDeleted(inParams.ID)
	}
	return ops.NewNodeDeleteNoContent()
}

func (c *HandlerComp) nodeFetch(inParams ops.NodeFetchParams) middleware.Responder {
	if e := c.proxyReady(); e != nil {
		return ops.NewNodeFetchDefault(int(e.Code)).WithPayload(e)
	}
	p := node.NewNodeFetchParams().WithID(inParams.ID).WithContext(inParams.HTTPRequest.Context())
	res, err := c.app.ClientAPI.Node().NodeFetch(p)
	if err != nil {
		e := c.nodeError(err)
		return ops.NewNodeFetchDefault(int(e.Code)).WithPayload(e)
	}
	return ops.NewNodeFetchOK().WithPayload(res.Payload)
}

func (c *HandlerComp) nodeList(inParams ops.NodeListParams) middleware.Responder {
	if e := c.proxyReady(); e != nil {
		return ops.NewNodeListDefault(int(e.Code)).WithPayload(e)
	}
	p := node.NewNodeListParams().WithContext(inParams.HTTPRequest.Context())
	// Note: add new properties as needed
	p.ClusterID = inParams.ClusterID
	p.Name = inParams.Name
	p.NodeIdentifier = inParams.NodeIdentifier
	p.Tags = inParams.Tags
	p.ServiceHeartbeatTimeGE = inParams.ServiceHeartbeatTimeGE
	p.ServiceHeartbeatTimeLE = inParams.ServiceHeartbeatTimeLE
	p.ServiceStateEQ = inParams.ServiceStateEQ
	p.ServiceStateNE = inParams.ServiceStateNE
	p.StateEQ = inParams.StateEQ
	p.StateNE = inParams.StateNE
	p.NodeIds = inParams.NodeIds
	res, err := c.app.ClientAPI.Node().NodeList(p)
	if err != nil {
		e := c.nodeError(err)
		return ops.NewNodeListDefault(int(e.Code)).WithPayload(e)
	}
	return ops.NewNodeListOK().WithPayload(res.Payload)
}

func (c *HandlerComp) nodeUpdateHasHeartbeatData(inParams ops.NodeUpdateParams) bool {
	if inParams.ID == "" || len(inParams.Set) == 0 || inParams.Payload == nil ||
		inParams.Payload.Service == nil {
		return false
	}
	referencedInPayload := false // the Service must be referenced in a Set query for the payload to be valid
	for _, s := range inParams.Set {
		if strings.HasPrefix(s, "service.") || s == "service" { // Assumption: Entire NuvoService object sent in payload
			referencedInPayload = true
			break
		}
	}
	return referencedInPayload
}

var nodeHbOnlySet = []string{"service.state", "service.heartbeatTime", "service.heartbeatPeriodSecs"}

func (c *HandlerComp) isNodeHeartbeatOnlyUpdate(inParams ops.NodeUpdateParams) bool {
	if len(inParams.Set) != 3 || len(inParams.Append) > 0 || len(inParams.Remove) > 0 {
		return false
	}
	for _, s := range nodeHbOnlySet {
		if !util.Contains(inParams.Set, s) {
			return false
		}
	}
	return true
}

func (c *HandlerComp) nodeUpdate(inParams ops.NodeUpdateParams) middleware.Responder {
	if e := c.proxyReady(); e != nil {
		return ops.NewNodeUpdateDefault(int(e.Code)).WithPayload(e)
	}
	if c.nodeUpdateHasHeartbeatData(inParams) {
		c.app.StateOps.NodeServiceState(inParams.ID, &inParams.Payload.Service.ServiceState)
		if c.isNodeHeartbeatOnlyUpdate(inParams) {
			return ops.NewNodeUpdateOK().WithPayload(&models.Node{}) // return an empty object
		}
	}
	p := node.NewNodeUpdateParams().WithPayload(inParams.Payload).WithContext(inParams.HTTPRequest.Context())
	if p.Payload.Service != nil {
		p.Payload.Service.HeartbeatPeriodSecs = c.app.StateUpdater.HeartbeatTaskPeriodSecs()
	}
	// Note: add new properties as needed
	p.Append = inParams.Append
	p.Remove = inParams.Remove
	p.Set = inParams.Set
	p.ID = inParams.ID
	p.Version = inParams.Version
	res, err := c.app.ClientAPI.Node().NodeUpdate(p)
	if err != nil {
		e := c.nodeError(err)
		return ops.NewNodeUpdateDefault(int(e.Code)).WithPayload(e)
	}
	return ops.NewNodeUpdateOK().WithPayload(res.Payload)
}

func (c *HandlerComp) nodeError(err error) *models.Error {
	var ret *models.Error
	var s string
	switch e := err.(type) {
	case *node.NodeCreateDefault:
		ret = e.Payload
		s = e.Error()
	case *node.NodeDeleteDefault:
		ret = e.Payload
		s = e.Error()
	case *node.NodeFetchDefault:
		ret = e.Payload
		s = e.Error()
	case *node.NodeListDefault:
		ret = e.Payload
		s = e.Error()
	case *node.NodeUpdateDefault:
		ret = e.Payload
		s = e.Error()
	default:
		s = err.Error()
		ret = &models.Error{
			Code:    http.StatusInternalServerError,
			Message: swag.String(s),
		}
	}
	c.Log.Debug(s)
	return ret
}
