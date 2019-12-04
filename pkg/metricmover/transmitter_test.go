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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestTransmitter(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mO := NewMetricMover(tl.Logger())
	assert.NotNil(mO)
	m, ok := mO.(*mover)
	assert.True(ok)

	// ConfigureTransmitter failure (missing API)
	err := mO.ConfigureTransmitter(&TxArgs{})
	assert.Error(err)
	assert.Regexp("missing arguments", err)

	// ConfigureTransmitter failure (no log)
	m.Log = nil // will force failure in underlying worker
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	txa := &TxArgs{
		API: mAPI,
	}
	err = mO.ConfigureTransmitter(txa)
	assert.Error(err)

	// StartTransmitter error if not configured
	err = mO.StartTransmitter()
	assert.Error(err)
	assert.Regexp("not configured", err)

	// StopTransmitter is a noop when not configured
	assert.Nil(m.tx)
	mO.StopTransmitter()

	// published data dropped if no consumer
	vsD1 := &models.IoMetricDatum{}
	vsD2 := &models.IoMetricDatum{}
	vsD3 := &models.IoMetricDatum{}
	vsData := []*models.IoMetricDatum{vsD1, vsD2, vsD3}
	sD1 := &models.IoMetricDatum{}
	sD2 := &models.IoMetricDatum{}
	sData := []*models.IoMetricDatum{sD1, sD2}

	assert.Nil(m.consumer)
	mO.PublishVolumeIoMetrics(vsData)
	mO.PublishStorageIoMetrics(sData)
	s := mO.Status()
	assert.False(s.ConsumerConfigured)
	assert.False(s.TxConfigured)
	assert.Equal(0, s.VolumeIOPublished)
	assert.Equal(0, s.StorageIOPublished)

	// ConfigureTransmitter success (no defaults)
	mO = NewMetricMover(tl.Logger())
	assert.NotNil(mO)
	m, ok = mO.(*mover)
	assert.True(ok)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	txa = &TxArgs{
		API:                  mAPI,
		RetryInterval:        time.Second,
		VolumeIOMaxBuffered:  2,
		StorageIOMaxBuffered: 1,
	}
	err = mO.ConfigureTransmitter(txa)
	assert.NoError(err)
	assert.NotNil(m.tx)
	assert.Equal(m.tx, m.consumer)
	tx := m.tx
	assert.NotNil(tx.oCrud)
	assert.Equal(time.Second, tx.RetryInterval)
	assert.Equal(2, tx.VolumeIOMaxBuffered)
	assert.Equal(1, tx.StorageIOMaxBuffered)
	s = mO.Status()
	assert.True(s.TxConfigured)
	assert.False(s.TxStarted)

	// ConfigureTransmitter success (all defaults)
	mO = NewMetricMover(tl.Logger())
	assert.NotNil(mO)
	m, ok = mO.(*mover)
	assert.True(ok)
	txa = &TxArgs{
		API:                  mAPI,
		VolumeIOMaxBuffered:  -1,
		StorageIOMaxBuffered: -1,
	}
	err = mO.ConfigureTransmitter(txa)
	assert.NoError(err)
	assert.NotNil(m.tx)
	assert.Equal(m.tx, m.consumer)
	tx = m.tx
	assert.NotNil(tx.oCrud)
	assert.Equal(TxRetryIntervalDefault, tx.RetryInterval)
	assert.Equal(TxVolumeIOMaxBufferedDefault, tx.VolumeIOMaxBuffered)
	assert.Equal(TxStorageIOMaxBufferedDefault, tx.StorageIOMaxBuffered)
	s = mO.Status()
	assert.True(s.TxConfigured)
	assert.False(s.TxStarted)

	tw := &fw.Worker{} // use a fake worker in the transmitter
	tx.worker = tw

	// StartTransmitter calls the worker
	err = mO.StartTransmitter()
	assert.NoError(err)
	assert.Equal(1, tw.CntStart)
	s = mO.Status()
	assert.True(s.TxConfigured)
	assert.True(s.TxStarted)

	// Published data now enqueued
	assert.NotNil(m.consumer)
	assert.Empty(tx.vsIoQueue)
	mO.PublishVolumeIoMetrics(vsData)
	mO.PublishVolumeIoMetrics(vsData) // twice!
	assert.Len(tx.vsIoQueue, 2*len(vsData))
	assert.Empty(tx.sIoQueue)
	mO.PublishStorageIoMetrics(sData)
	assert.Len(tx.sIoQueue, len(sData))
	s = mO.Status()
	assert.True(s.TxConfigured)
	assert.True(s.TxStarted)
	assert.Equal(2*len(vsData), s.VolumeIOPublished)
	assert.Equal(2*len(vsData), s.VolumeIOBuffered)
	assert.Equal(0, s.VolumeIODropped)
	assert.Equal(len(sData), s.StorageIOPublished)
	assert.Equal(len(sData), s.StorageIOBuffered)
	assert.Equal(0, s.StorageIODropped)

	// StopTransmitter calls the worker
	mO.StopTransmitter()
	assert.Equal(1, tw.CntStop)
	s = mO.Status()
	assert.True(s.TxConfigured)
	assert.False(s.TxStarted)

	// Buzz tests
	fc := &fake.Client{}
	tx.oCrud = fc
	ctx := context.Background()

	// Upload error, some records get dropped due to max buffer
	tl.Flush()
	assert.NotEmpty(tx.vsIoQueue)
	assert.Len(tx.vsIoQueue, 6)
	tx.VolumeIOMaxBuffered = 2
	fc.RetVIoMErr = fmt.Errorf("vs-iom-error")
	assert.NotEmpty(tx.sIoQueue)
	assert.Len(tx.sIoQueue, 2)
	tx.StorageIOMaxBuffered = 2
	fc.RetSIoMErr = fmt.Errorf("s-iom-error")
	err = tx.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("vs-iom-error", err)
	assert.Len(tx.vsIoQueue, 2)
	assert.Equal(1, tl.CountPattern("Dropping 4 VolumeSeries.*records"))
	assert.Equal(0, tl.CountPattern("Dropping.*Storage.*records"))
	s = mO.Status()
	assert.Equal(4, s.VolumeIODropped)
	assert.Equal(0, s.StorageIODropped)

	// vs Upload success, all records get removed; s error 1 record removed
	tl.Flush()
	tx.vsIoQueue = []*models.IoMetricDatum{vsD1, vsD2, vsD3}
	fc.RetVIoMErr = nil
	tx.StorageIOMaxBuffered = 1
	err = tx.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("s-iom-error", err)
	assert.Empty(tx.vsIoQueue)
	assert.Equal(0, tl.CountPattern("Dropping.*VolumeSeries.*records"))
	assert.Equal(1, tl.CountPattern("Dropping 1 Storage.*records"))
	assert.Equal([]*models.IoMetricDatum{vsD1, vsD2, vsD3}, fc.InVIoMData)
	s = mO.Status()
	assert.Equal(3, s.VolumeIOSent)
	assert.Equal(0, s.StorageIOSent)
	assert.Equal(1, s.StorageIODropped)

	// s Upload success, all records sent
	tl.Flush()
	assert.Empty(tx.vsIoQueue)
	tx.sIoQueue = []*models.IoMetricDatum{sD1, sD2}
	fc.RetSIoMErr = nil
	err = tx.Buzz(ctx)
	assert.NoError(err)
	assert.Empty(tx.sIoQueue)
	assert.Equal(0, tl.CountPattern("Dropping.*Storage.*records"))
	assert.Equal([]*models.IoMetricDatum{sD1, sD2}, fc.InSIoMData)
	s = mO.Status()
	assert.Equal(2, s.StorageIOSent)
}

func TestVsIOQueue(t *testing.T) {
	assert := assert.New(t)

	tx := &transmitter{}

	vsD1 := &models.IoMetricDatum{}
	vsD2 := &models.IoMetricDatum{}
	vsD3 := &models.IoMetricDatum{}

	tx.vsIoQueue = append(tx.vsIoQueue, vsD1, vsD2, vsD3)
	assert.Len(tx.vsIoQueue, 3)
	tx.vsIoQPopNInLock(2)
	assert.Len(tx.vsIoQueue, 1)
	assert.Equal([]*models.IoMetricDatum{vsD3}, tx.vsIoQueue)
}

func TestSIOQueue(t *testing.T) {
	assert := assert.New(t)

	tx := &transmitter{}

	sD1 := &models.IoMetricDatum{}
	sD2 := &models.IoMetricDatum{}
	sD3 := &models.IoMetricDatum{}

	tx.sIoQueue = append(tx.sIoQueue, sD1, sD2, sD3)
	assert.Len(tx.sIoQueue, 3)
	tx.sIoQPopNInLock(2)
	assert.Len(tx.sIoQueue, 1)
	assert.Equal([]*models.IoMetricDatum{sD3}, tx.sIoQueue)
}
