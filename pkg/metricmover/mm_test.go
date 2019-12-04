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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestMetricMover(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mO := NewMetricMover(tl.Logger())
	assert.NotNil(mO)
	m, ok := mO.(*mover)
	assert.True(ok)

	// I/O metrics
	vsD1 := &models.IoMetricDatum{}
	vsD2 := &models.IoMetricDatum{}
	vsD3 := &models.IoMetricDatum{}
	vsData := []*models.IoMetricDatum{vsD1, vsD2, vsD3}
	mO.PublishVolumeIoMetrics(vsData)
	sD1 := &models.IoMetricDatum{}
	sD2 := &models.IoMetricDatum{}
	sData := []*models.IoMetricDatum{sD1, sD2}
	mO.PublishStorageIoMetrics(sData)
	s := mO.Status()
	assert.Equal(len(vsData), s.VolumeIODropped)
	assert.Regexp("VolumeIODropped.:3", s.String())
	assert.Equal(len(sData), s.StorageIODropped)
	assert.Regexp("StorageIODropped.:2", s.String())

	fakeConsumer := &transmitter{m: m} // use a fake Tx object as a consumer
	tw := &fw.Worker{}                 // use a fake worker
	fakeConsumer.worker = tw

	// Consumer => no transmitter
	err := mO.RegisterConsumer(fakeConsumer)
	assert.NoError(err)
	assert.Equal(fakeConsumer, m.consumer)
	s = mO.Status()
	assert.True(s.ConsumerConfigured)
	assert.False(s.TxConfigured)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	txa := &TxArgs{
		API: mAPI,
	}
	err = mO.ConfigureTransmitter(txa)
	assert.Error(err)
	assert.Regexp("already configured", err)
	mockCtrl.Finish()

	mO.PublishVolumeIoMetrics(vsData)
	mO.PublishStorageIoMetrics(sData)
	s = mO.Status()
	assert.Equal(len(vsData), s.VolumeIOPublished)
	assert.Equal(len(sData), s.StorageIOPublished)

	// Transmitter => no consumer
	mO = NewMetricMover(tl.Logger())
	m, ok = mO.(*mover)
	assert.True(ok)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	txa = &TxArgs{
		API: mAPI,
	}
	err = mO.ConfigureTransmitter(txa)
	assert.NoError(err)
	assert.NotNil(m.tx)
	assert.Equal(m.tx, m.consumer)
	s = mO.Status()
	assert.False(s.ConsumerConfigured)
	assert.True(s.TxConfigured)
	err = mO.RegisterConsumer(fakeConsumer)
	assert.Error(err)
	assert.Regexp("already configured", err)
}
