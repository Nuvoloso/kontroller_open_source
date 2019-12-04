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
	"errors"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
			API: api,
		},
	}
	app.OCrud = &fake.Client{}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()

	// Init
	assert.NotPanics(func() { hc.Init(app) })
	assert.Equal(app.Log, hc.Log)
	assert.Equal(app, hc.app)
	assert.Nil(api.VolumeSeriesVolumeSeriesCreateHandler)
	assert.Nil(api.VolumeSeriesVolumeSeriesDeleteHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesFetchHandler)
	assert.Nil(api.VolumeSeriesVolumeSeriesListHandler)
	assert.Nil(api.VolumeSeriesVolumeSeriesUpdateHandler)
	assert.Nil(api.VolumeSeriesRequestVolumeSeriesRequestCancelHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestCreateHandler)
	assert.Nil(api.VolumeSeriesRequestVolumeSeriesRequestDeleteHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestFetchHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestListHandler)
	assert.Nil(api.VolumeSeriesRequestVolumeSeriesRequestUpdateHandler)
	assert.True(evM.CalledRH)

	assert.NotPanics(func() { hc.Start() })
	assert.NotNil(hc.oCrud)
	assert.NotPanics(func() { hc.Stop() })

	assert.Nil(hc.ConvertError(nil))
	err := hc.ConvertError(errors.New("error msg"))
	if assert.NotNil(err) {
		assert.Equal(int32(500), err.Payload.Code)
		assert.Equal("error msg", swag.StringValue(err.Payload.Message))
	}
	err = &crud.Error{Payload: models.Error{Code: 415, Message: swag.String("San Francisco")}}
	var e error
	e = err
	assert.Equal(err, hc.ConvertError(e))
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
