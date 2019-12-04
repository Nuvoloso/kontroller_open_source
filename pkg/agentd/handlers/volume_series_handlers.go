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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/go-openapi/runtime/middleware"
)

// Register handlers for VolumeSeries, only those that agentd will forward to centrald, all else will fail
func (c *HandlerComp) volumeSeriesRegisterHandlers() {
	c.app.API.VolumeSeriesVolumeSeriesFetchHandler = ops.VolumeSeriesFetchHandlerFunc(c.volumeSeriesFetch)
}

func (c *HandlerComp) volumeSeriesFetch(params ops.VolumeSeriesFetchParams) middleware.Responder {
	obj, err := c.oCrud.VolumeSeriesFetch(c.ctx, params.ID)
	if err != nil {
		e := c.ConvertError(err)
		return ops.NewVolumeSeriesFetchDefault(int(e.Payload.Code)).WithPayload(&e.Payload)
	}
	return ops.NewVolumeSeriesFetchOK().WithPayload(obj)
}
