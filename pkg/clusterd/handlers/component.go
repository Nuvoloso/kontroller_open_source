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
	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/op/go-logging"
)

// HandlerComp is used to manage this handler component
type HandlerComp struct {
	app *clusterd.AppCtx
	Log *logging.Logger
	auth.Extractor
}

func newHandlerComp() *HandlerComp {
	hc := &HandlerComp{}
	return hc
}

func init() {
	clusterd.AppRegisterComponent(newHandlerComp())
}

// Init registers handlers for this component
func (c *HandlerComp) Init(app *clusterd.AppCtx) {
	c.app = app
	c.Log = app.Log
	c.app.CrudeOps.RegisterHandlers(c.app.API, c)
	c.app.MetricMover.RegisterHandlers(c.app.API, c)
	c.registerNodeProxyHandlers()
	c.registerDebugHandlers()
}

// Start starts this component
func (c *HandlerComp) Start() {
	c.Log.Info("Starting HandlerComponent")
}

// Stop terminates this component
func (c *HandlerComp) Stop() {
	c.Log.Info("Stopped HandlerComponent")
}

// crude.AccessControl functions

// EventAllowed only allows events if the caller is a trusted client (eg agentd or internally created client with nil WatcherAuth)
func (c *HandlerComp) EventAllowed(a auth.Subject, ev *crude.CrudEvent) bool {
	return (a == nil || a.Internal()) && ev != nil
}
