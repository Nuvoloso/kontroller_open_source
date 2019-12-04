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

	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
)

func TestVolumeSeriesFetch(t *testing.T) {
	assert := assert.New(t)

	hc := newHandlerComp()
	fc := &fake.Client{}
	hc.oCrud = fc
	params := ops.VolumeSeriesFetchParams{ID: "vsID"}

	fc.RetVObj = &models.VolumeSeries{}
	ret := hc.volumeSeriesFetch(params)
	obj, ok := ret.(*ops.VolumeSeriesFetchOK)
	if assert.True(ok, "VolumeSeriesFetchOK") {
		assert.Equal(fc.RetVObj, obj.Payload)
	}

	fc.RetVObj = nil
	expErr := &crud.Error{Payload: models.Error{Code: 402, Message: swag.String("Nebraska")}}
	fc.RetVErr = expErr
	ret = hc.volumeSeriesFetch(params)
	e, ok := ret.(*ops.VolumeSeriesFetchDefault)
	if assert.True(ok, "VolumeSeriesFetchDefault") {
		assert.Equal(&expErr.Payload, e.Payload)
	}
}
