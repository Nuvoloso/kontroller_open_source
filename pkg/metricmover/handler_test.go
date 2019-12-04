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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	fa "github.com/Nuvoloso/kontroller/pkg/auth/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	mop "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/metrics"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestHandlers(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mO := NewMetricMover(tl.Logger())
	assert.NotNil(mO)
	m, ok := mO.(*mover)
	assert.True(ok)

	nuvoAPI := &operations.NuvolosoAPI{}
	assert.Nil(nuvoAPI.MetricsVolumeSeriesIOMetricUploadHandler)
	assert.Nil(nuvoAPI.MetricsStorageIOMetricUploadHandler)

	ai := &auth.Info{}
	fe := &fa.Extractor{RetSubj: ai}
	m.RegisterHandlers(nuvoAPI, fe)
	assert.NotNil(nuvoAPI.MetricsVolumeSeriesIOMetricUploadHandler)
	assert.NotNil(nuvoAPI.MetricsStorageIOMetricUploadHandler)

	// VolumeSeries I/O metrics
	vsD1 := &models.IoMetricDatum{}
	vsD2 := &models.IoMetricDatum{}
	vsD3 := &models.IoMetricDatum{}
	vsData := []*models.IoMetricDatum{vsD1, vsD2, vsD3}

	vsIoP := mop.NewVolumeSeriesIOMetricUploadParams()
	vsIoP.HTTPRequest = &http.Request{}
	vsIoP.Payload = &models.IoMetricData{Data: vsData}
	assert.Nil(m.consumer)
	ret := m.vsIOUpload(vsIoP)
	vsIoErr, ok := ret.(*mop.VolumeSeriesIOMetricUploadDefault)
	assert.True(ok)
	assert.Equal(int32(503), vsIoErr.Payload.Code)
	assert.Equal("not ready", swag.StringValue(vsIoErr.Payload.Message))
	s := mO.Status()
	assert.Equal(len(vsData), s.VolumeIODropped)

	fe.RetErr = &models.Error{Code: 500, Message: swag.String("internal-error")}
	ret = m.vsIOUpload(vsIoP)
	vsIoErr, ok = ret.(*mop.VolumeSeriesIOMetricUploadDefault)
	assert.True(ok)
	assert.Equal(int32(500), vsIoErr.Payload.Code)
	assert.Equal("internal-error", swag.StringValue(vsIoErr.Payload.Message))
	fe.RetErr = nil

	ai.RemoteAddr = "1.2.3.4"
	ret = m.vsIOUpload(vsIoP)
	vsIoErr, ok = ret.(*mop.VolumeSeriesIOMetricUploadDefault)
	assert.True(ok)
	assert.Equal(int32(403), vsIoErr.Payload.Code)
	assert.Equal(common.ErrorUnauthorizedOrForbidden, swag.StringValue(vsIoErr.Payload.Message))
	ai.RemoteAddr = ""

	sD1 := &models.IoMetricDatum{}
	sD2 := &models.IoMetricDatum{}
	sData := []*models.IoMetricDatum{sD1, sD2}

	sIoP := mop.NewStorageIOMetricUploadParams()
	sIoP.HTTPRequest = &http.Request{}
	sIoP.Payload = &models.IoMetricData{Data: sData}
	ret = m.sIOUpload(sIoP)
	sIoErr, ok := ret.(*mop.StorageIOMetricUploadDefault)
	assert.True(ok)
	assert.Equal(int32(503), sIoErr.Payload.Code)
	assert.Equal("not ready", swag.StringValue(sIoErr.Payload.Message))
	s = mO.Status()
	assert.Equal(len(sData), s.StorageIODropped)

	fe.RetErr = &models.Error{Code: 500, Message: swag.String("internal-error")}
	ret = m.sIOUpload(sIoP)
	sIoErr, ok = ret.(*mop.StorageIOMetricUploadDefault)
	assert.True(ok)
	assert.Equal(int32(500), sIoErr.Payload.Code)
	assert.Equal("internal-error", swag.StringValue(sIoErr.Payload.Message))
	fe.RetErr = nil

	ai.RemoteAddr = "1.2.3.4"
	ret = m.sIOUpload(sIoP)
	sIoErr, ok = ret.(*mop.StorageIOMetricUploadDefault)
	assert.True(ok)
	assert.Equal(int32(403), sIoErr.Payload.Code)
	assert.Equal(common.ErrorUnauthorizedOrForbidden, swag.StringValue(sIoErr.Payload.Message))
	ai.RemoteAddr = ""

	// fake a consumer with a transmitter
	tx := &transmitter{m: m}
	tw := &fw.Worker{} // use a fake worker in the transmitter
	tx.worker = tw
	m.consumer = tx
	m.tx = tx
	assert.Empty(tx.vsIoQueue)
	ret = m.vsIOUpload(vsIoP)
	assert.Len(tx.vsIoQueue, 3)
	assert.Equal(vsData, tx.vsIoQueue)
	s = mO.Status()
	assert.False(s.ConsumerConfigured)
	assert.True(s.TxConfigured)
	assert.False(s.TxStarted)
	assert.Equal(3, s.VolumeIOReceived)
	assert.Equal(3, s.VolumeIOBuffered)

	assert.Empty(tx.sIoQueue)
	ret = m.sIOUpload(sIoP)
	assert.Len(tx.sIoQueue, 2)
	assert.Equal(sData, tx.sIoQueue)
	s = mO.Status()
	assert.Equal(2, s.StorageIOReceived)
	assert.Equal(2, s.StorageIOBuffered)
}
