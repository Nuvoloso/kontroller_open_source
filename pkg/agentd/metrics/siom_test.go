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


package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestStorageIOStats(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	c := &MetricComp{}
	c.Init(app)

	now := time.Now()
	rStat0 := &nuvoapi.StatsIO{
		SeriesUUID: "rUUID1",
		Count:      100,
		SizeTotal:  10000,
	}
	wStat0 := &nuvoapi.StatsIO{
		SeriesUUID: "wUUID1",
		Count:      200,
		SizeTotal:  20000,
	}
	storage := &agentd.Storage{
		StorageID:      "S-1",
		DeviceName:     "d",
		CspStorageType: "st",
	}
	assert.NoError(storage.Validate())

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI

	// read stats error
	nvAPI.EXPECT().GetStats(true, true, false, storage.StorageID).Return(nil, fmt.Errorf("read-stats-error"))
	d, err := c.getStorageIOStats(now, storage)
	assert.Error(err)
	assert.Regexp("read-stats-error", err)
	assert.Nil(d)

	// write stats error
	nvAPI.EXPECT().GetStats(true, true, false, storage.StorageID).Return(rStat0, nil)
	nvAPI.EXPECT().GetStats(true, false, false, storage.StorageID).Return(nil, fmt.Errorf("write-stats-error"))
	d, err = c.getStorageIOStats(now, storage)
	assert.Error(err)
	assert.Regexp("write-stats-error", err)
	assert.Nil(d)

	// first stats are cached => no datum returned
	tl.Flush()
	assert.Nil(storage.LastReadStat)
	assert.Nil(storage.LastWriteStat)
	nvAPI.EXPECT().GetStats(true, true, false, storage.StorageID).Return(rStat0, nil)
	nvAPI.EXPECT().GetStats(true, false, false, storage.StorageID).Return(wStat0, nil)
	d, err = c.getStorageIOStats(now, storage)
	assert.NoError(err)
	assert.Nil(d)
	assert.Equal(rStat0, storage.LastReadStat)
	assert.Equal(wStat0, storage.LastWriteStat)
	assert.Equal(1, tl.CountPattern("skipping first I/O stat"))

	// second call same series creates a datum
	rStat1 := &nuvoapi.StatsIO{}
	testutils.Clone(rStat0, rStat1)
	rStat1.Count++
	wStat1 := &nuvoapi.StatsIO{}
	testutils.Clone(wStat0, wStat1)
	wStat1.Count++

	fIoMDM := &fakeIoMetricDatumMaker{}
	c.ioMDMaker = fIoMDM
	expD := &models.IoMetricDatum{}
	fIoMDM.RetD = expD
	tl.Flush()
	nvAPI.EXPECT().GetStats(true, true, false, storage.StorageID).Return(rStat1, nil)
	nvAPI.EXPECT().GetStats(true, false, false, storage.StorageID).Return(wStat1, nil)
	d, err = c.getStorageIOStats(now, storage)
	assert.NoError(err)
	assert.NotNil(d)
	assert.Equal(expD, d)
	assert.Equal(now, fIoMDM.InNow)
	assert.Equal(storage.StorageID, fIoMDM.InObjID)
	assert.Equal(rStat0, fIoMDM.InLRS)
	assert.Equal(rStat1, fIoMDM.InRS)
	assert.Equal(wStat0, fIoMDM.InLWS)
	assert.Equal(wStat1, fIoMDM.InWS)
	assert.Equal(rStat1, storage.LastReadStat)
	assert.Equal(wStat1, storage.LastWriteStat)

	// fake error importing StatIo
	fIoMDM.RetErr = fmt.Errorf("metric-maker-error")
	tl.Flush()
	nvAPI.EXPECT().GetStats(true, true, false, storage.StorageID).Return(rStat1, nil)
	nvAPI.EXPECT().GetStats(true, false, false, storage.StorageID).Return(wStat1, nil)
	d, err = c.getStorageIOStats(now, storage)
	assert.Error(err)
	assert.Regexp("metric-maker-error", err)
	assert.Nil(d)

	// series change => no datum returned
	fIoMDM.RetErr = nil
	rStat2 := &nuvoapi.StatsIO{}
	testutils.Clone(rStat0, rStat2)
	rStat2.SeriesUUID = "rUUID2"
	wStat2 := &nuvoapi.StatsIO{}
	testutils.Clone(wStat0, wStat2)
	wStat2.SeriesUUID = "wUUID2"

	tl.Flush()
	nvAPI.EXPECT().GetStats(true, true, false, storage.StorageID).Return(rStat2, nil)
	nvAPI.EXPECT().GetStats(true, false, false, storage.StorageID).Return(wStat2, nil)
	fIoMDM.InObjID = ""
	d, err = c.getStorageIOStats(now, storage)
	assert.NoError(err)
	assert.Nil(d)
	assert.Equal("", fIoMDM.InObjID)
	assert.Equal(rStat2, storage.LastReadStat)
	assert.Equal(wStat2, storage.LastWriteStat)
	assert.Equal(1, tl.CountPattern("series UUID changed"))

	// publishStorageIOStats
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM

	rStat3 := &nuvoapi.StatsIO{}
	testutils.Clone(rStat2, rStat3)
	rStat3.Count++
	wStat3 := &nuvoapi.StatsIO{}
	testutils.Clone(wStat2, wStat3)
	wStat3.Count++
	nvAPI.EXPECT().GetStats(true, true, false, storage.StorageID).Return(rStat3, nil)
	nvAPI.EXPECT().GetStats(true, false, false, storage.StorageID).Return(wStat3, nil)
	app.AddStorage(storage)
	assert.Len(app.GetStorage(), 1)
	fIoMDM.RetD = expD
	tBefore := time.Now()
	c.publishStorageIOStats()
	tAfter := time.Now()
	assert.Equal(1, c.cntSIo)
	assert.NotNil(fMM.InPSIoMData)
	assert.Len(fMM.InPSIoMData, 1)
	assert.Equal(expD, fMM.InPSIoMData[0])
	assert.True(tBefore.Before(fIoMDM.InNow))
	assert.True(tAfter.After(fIoMDM.InNow))
	assert.Equal(storage.StorageID, fIoMDM.InObjID)
	assert.Equal(rStat2, fIoMDM.InLRS)
	assert.Equal(rStat3, fIoMDM.InRS)
	assert.Equal(wStat2, fIoMDM.InLWS)
	assert.Equal(wStat3, fIoMDM.InWS)
	assert.Equal(rStat3, storage.LastReadStat)
	assert.Equal(wStat3, storage.LastWriteStat)
}
