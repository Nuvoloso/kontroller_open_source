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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
			API: api,
		},
	}
	evM := fake.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	hc := newHandlerComp()

	// verify that no proxy handler exists
	assert.Nil(app.API.NodeNodeCreateHandler)
	assert.Nil(app.API.NodeNodeDeleteHandler)
	assert.Nil(app.API.NodeNodeFetchHandler)
	assert.Nil(app.API.NodeNodeListHandler)
	assert.Nil(app.API.NodeNodeUpdateHandler)

	// Init
	assert.NotPanics(func() { hc.Init(app) })
	assert.Equal(app.Log, hc.Log)
	assert.Equal(app, hc.app)
	assert.True(evM.CalledRH)
	assert.Equal(app.API, fMM.InRHApi)

	// check proxy handlers
	assert.NotNil(app.API.NodeNodeCreateHandler)
	assert.NotNil(app.API.NodeNodeDeleteHandler)
	assert.NotNil(app.API.NodeNodeFetchHandler)
	assert.NotNil(app.API.NodeNodeListHandler)
	assert.NotNil(app.API.NodeNodeUpdateHandler)

	assert.NotPanics(func() { hc.Start() })
	assert.NotPanics(func() { hc.Stop() })
}

func TestEventAllowed(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	ev := &crude.CrudEvent{}

	// only trusted and internal allowed
	a := &auth.Info{}
	assert.True(hc.EventAllowed(a, ev))
	assert.True(hc.EventAllowed(nil, ev))
	a = &auth.Info{RemoteAddr: "addr", CertCN: "cn"}
	assert.True(hc.EventAllowed(a, ev))
	a = &auth.Info{RemoteAddr: "addr", CertCN: ""}
	assert.False(hc.EventAllowed(a, ev))

	// invalid invocations
	assert.False(hc.EventAllowed(nil, nil))
}
