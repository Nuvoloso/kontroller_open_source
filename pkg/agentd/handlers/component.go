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
	"net/http"
	"sync"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
)

// HandlerComp is used to manage this handler component
type HandlerComp struct {
	app   *agentd.AppCtx
	Log   *logging.Logger
	mux   sync.RWMutex
	oCrud crud.Ops
	ctx   context.Context
	auth.Extractor
}

func newHandlerComp() *HandlerComp {
	hc := &HandlerComp{}
	return hc
}

func init() {
	agentd.AppRegisterComponent(newHandlerComp())
}

// Init registers handlers for this component
func (c *HandlerComp) Init(app *agentd.AppCtx) {
	c.app = app
	c.Log = app.Log

	c.volumeSeriesRegisterHandlers()
	c.volumeSeriesRequestRegisterHandlers()
	c.debugRegisterHandlers()
	c.app.CrudeOps.RegisterHandlers(c.app.API, c)
}

// Start starts this component
func (c *HandlerComp) Start() {
	c.Log.Info("Starting HandlerComponent")
	c.oCrud = c.app.OCrud
	c.ctx = context.Background()
}

// Stop terminates this component
func (c *HandlerComp) Stop() {
	c.Log.Info("Stopped HandlerComponent")
}

// ConvertError converts an error (back) into a crud.Error
func (c *HandlerComp) ConvertError(err error) *crud.Error {
	if err == nil {
		return nil
	}
	e, ok := err.(*crud.Error)
	if !ok {
		e = &crud.Error{Payload: models.Error{Code: http.StatusInternalServerError, Message: swag.String(err.Error())}}
	}
	return e
}

// crude.AccessControl functions

// EventAllowed only allows events if the caller is a trusted client (eg unix socket client or internally created client with nil WatcherAuth)
func (c *HandlerComp) EventAllowed(a auth.Subject, ev *crude.CrudEvent) bool {
	return (a == nil || a.Internal()) && ev != nil
}
