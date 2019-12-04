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


package metricmover

import (
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	mop "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/metrics"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// RegisterHandlers registers the metric mover handlers.
// Handlers will return an error until the mover is configured.
func (m *mover) RegisterHandlers(api *operations.NuvolosoAPI, ac auth.AccessControl) {
	api.MetricsVolumeSeriesIOMetricUploadHandler = mop.VolumeSeriesIOMetricUploadHandlerFunc(m.vsIOUpload)
	api.MetricsStorageIOMetricUploadHandler = mop.StorageIOMetricUploadHandlerFunc(m.sIOUpload)
	m.ac = ac
}

// vsIOUpload handles VolumeSeries I/O metrics
func (m *mover) vsIOUpload(params mop.VolumeSeriesIOMetricUploadParams) middleware.Responder {
	ai, e := m.ac.GetAuth(params.HTTPRequest)
	if e == nil && !ai.Internal() {
		e = &models.Error{Code: http.StatusForbidden, Message: swag.String(common.ErrorUnauthorizedOrForbidden)}
	}
	if e != nil {
		return mop.NewVolumeSeriesIOMetricUploadDefault(int(e.Code)).WithPayload(e)
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.consumer != nil {
		m.vsIoReceived += len(params.Payload.Data)
		m.consumer.ConsumeVolumeIoMetricData(params.Payload.Data)
		return mop.NewVolumeSeriesIOMetricUploadNoContent()
	}
	m.vsIoDropped += len(params.Payload.Data)
	e = &models.Error{Code: http.StatusServiceUnavailable, Message: swag.String("not ready")}
	return mop.NewVolumeSeriesIOMetricUploadDefault(int(e.Code)).WithPayload(e)
}

// sIOUpload handles VolumeSeries I/O metrics
func (m *mover) sIOUpload(params mop.StorageIOMetricUploadParams) middleware.Responder {
	ai, e := m.ac.GetAuth(params.HTTPRequest)
	if e == nil && !ai.Internal() {
		e = &models.Error{Code: http.StatusForbidden, Message: swag.String(common.ErrorUnauthorizedOrForbidden)}
	}
	if e != nil {
		return mop.NewStorageIOMetricUploadDefault(int(e.Code)).WithPayload(e)
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.consumer != nil {
		m.sIoReceived += len(params.Payload.Data)
		m.consumer.ConsumeStorageIoMetricData(params.Payload.Data)
		return mop.NewStorageIOMetricUploadNoContent()
	}
	m.sIoDropped += len(params.Payload.Data)
	e = &models.Error{Code: http.StatusServiceUnavailable, Message: swag.String("not ready")}
	return mop.NewStorageIOMetricUploadDefault(int(e.Code)).WithPayload(e)
}
