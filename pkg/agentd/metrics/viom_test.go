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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	appServant "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestVolumeIOStats(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		LMDGuard: util.NewCriticalSectionGuard(),
	}
	fc := &fake.Client{}
	app.OCrud = fc

	c := &MetricComp{}
	c.Init(app)

	ctx := context.Background()
	epoch := time.Now()
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
	cUserStat0 := &nuvoapi.StatsCache{
		IOReadTotal:               123,
		CacheIOReadLineTotalCount: 111,
		CacheIOReadLineHitCount:   99,
	}
	cMetaStat0 := &nuvoapi.StatsCache{
		IOReadTotal:               246,
		CacheIOReadLineTotalCount: 222,
		CacheIOReadLineHitCount:   135,
	}
	comboStat0 := &nuvoapi.StatsCombinedVolume{
		IOReads:       *rStat0,
		IOWrites:      *wStat0,
		CacheUser:     *cUserStat0,
		CacheMetadata: *cMetaStat0,
	}
	deviceStat := []nuvoapi.Device{
		nuvoapi.Device{
			Index:          1,
			UUID:           "03n4js",
			ParcelSize:     536870912,
			TargetParcels:  1,
			AllocedParcels: 1,
			FreeSegments:   0,
			DeviceClass:    4,
			BlocksUsed:     100000,
			BlockSize:      4096,
			Parcels:        nil,
		},
		nuvoapi.Device{
			Index:          2,
			UUID:           "03n4jt",
			ParcelSize:     536870912,
			TargetParcels:  1,
			AllocedParcels: 0,
			FreeSegments:   0,
			DeviceClass:    4,
			BlocksUsed:     120000,
			BlockSize:      4096,
			Parcels:        nil,
		},
	}
	lun := &agentd.LUN{
		VolumeSeriesID:       "VS-1",
		SnapIdentifier:       com.VolMountHeadIdentifier,
		NuvoVolumeIdentifier: "nuvo-vol-1",
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI

	tl.Logger().Info("** IO stats error")
	tl.Flush()
	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(nil, fmt.Errorf("read-stats-error"))
	d, err := c.getVolumeIOStats(ctx, epoch, lun)
	assert.Error(err)
	assert.Regexp("read-stats-error", err)
	assert.Nil(d)

	tl.Logger().Info("** first stats are cached ⇒ no datum returned")
	tl.Flush()
	lmdGuardCntr := app.LMDGuard.Used
	assert.Nil(lun.LastReadStat)
	assert.Nil(lun.LastWriteStat)
	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat0, nil)
	d, err = c.getVolumeIOStats(ctx, epoch, lun)
	assert.NoError(err)
	assert.Nil(d)
	assert.Equal(rStat0, lun.LastReadStat)
	assert.Equal(wStat0, lun.LastWriteStat)
	assert.Equal(1, tl.CountPattern("skipping first I/O stat"))
	assert.Equal(lmdGuardCntr+1, app.LMDGuard.Used)

	tl.Logger().Info("** second call same series creates a datum; no change in write size")
	tl.Flush()
	comboStat1 := &nuvoapi.StatsCombinedVolume{}
	testutils.Clone(comboStat0, comboStat1)
	comboStat1.IOReads.Count++ // no change in size
	comboStat1.IOWrites.Count++

	lmdGuardCntr = app.LMDGuard.Used
	fc.InVSUpdaterID = ""
	fIoMDM := &fakeIoMetricDatumMaker{}
	c.ioMDMaker = fIoMDM
	expD := &models.IoMetricDatum{}
	fIoMDM.RetD = expD
	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat1, nil)
	nvAPI.EXPECT().Manifest(lun.NuvoVolumeIdentifier, true).Return(deviceStat, nil)
	d, err = c.getVolumeIOStats(ctx, epoch, lun)
	assert.NoError(err)
	assert.NotNil(d)
	assert.Equal(expD, d)
	assert.Equal(epoch, fIoMDM.InNow)
	assert.Equal(lun.VolumeSeriesID, fIoMDM.InObjID)
	assert.Equal(&comboStat0.IOReads, fIoMDM.InLRS)
	assert.Equal(&comboStat1.IOReads, fIoMDM.InRS)
	assert.Equal(&comboStat0.IOWrites, fIoMDM.InLWS)
	assert.Equal(&comboStat1.IOWrites, fIoMDM.InWS)
	assert.Equal(&comboStat1.IOReads, lun.LastReadStat)
	assert.Equal(&comboStat1.IOWrites, lun.LastWriteStat)
	assert.Equal(lmdGuardCntr+1, app.LMDGuard.Used)
	assert.Equal("", fc.InVSUpdaterID) // vol not updated as no change in bytes written

	tl.Logger().Info("** fake error importing StatIo")
	tl.Flush()
	fIoMDM.RetErr = fmt.Errorf("metric-maker-error")
	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat1, nil)
	nvAPI.EXPECT().Manifest(lun.NuvoVolumeIdentifier, true).Return(deviceStat, nil)
	d, err = c.getVolumeIOStats(ctx, epoch, lun)
	assert.Error(err)
	assert.Regexp("metric-maker-error", err)
	assert.Nil(d)

	tl.Logger().Info("** series change ⇒ no datum returned")
	tl.Flush()
	fIoMDM.RetErr = nil
	comboStat2 := &nuvoapi.StatsCombinedVolume{}
	testutils.Clone(comboStat0, comboStat2)
	comboStat1.IOReads.SeriesUUID = "rUUID2"
	comboStat1.IOWrites.SeriesUUID = "wUUID2"

	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat2, nil)
	nvAPI.EXPECT().Manifest(lun.NuvoVolumeIdentifier, true).Return(deviceStat, nil)
	fIoMDM.InObjID = ""
	d, err = c.getVolumeIOStats(ctx, epoch, lun)
	assert.NoError(err)
	assert.Nil(d)
	assert.Equal("", fIoMDM.InObjID)
	assert.Equal(&comboStat2.IOReads, lun.LastReadStat)
	assert.Equal(&comboStat2.IOWrites, lun.LastWriteStat)
	assert.Equal(1, tl.CountPattern("series UUID changed"))

	tl.Logger().Info("** series change ⇒ no datum returned + LMDGuard error")
	tl.Flush()
	app.LMDGuard.Drain()
	fIoMDM.RetErr = nil
	comboStat2 = &nuvoapi.StatsCombinedVolume{}
	testutils.Clone(comboStat0, comboStat2)
	comboStat1.IOReads.SeriesUUID = "rUUID2"
	comboStat1.IOWrites.SeriesUUID = "wUUID2"

	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat2, nil)
	fIoMDM.InObjID = ""
	d, err = c.getVolumeIOStats(ctx, epoch, lun)
	assert.Error(err)
	assert.Nil(d)
	assert.Regexp("closed", err)
	app.LMDGuard = util.NewCriticalSectionGuard() // restore

	tl.Logger().Info("** publishVolumeIOStats (no head luns)")
	tl.Flush()
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	app.AddLUN(&agentd.LUN{
		VolumeSeriesID: "VS-1",  // same id used above
		SnapIdentifier: "snap1", // but not head
	})
	assert.Len(app.GetLUNs(), 1)
	assert.Equal(0, c.cntVSIo)
	c.publishVolumeIOStats(ctx)
	assert.Equal(1, c.cntVSIo)
	assert.Nil(fMM.InPVIoMData)

	tl.Logger().Info("** publishVolumeIOStats metrics collection disabled")
	tl.Flush()
	fMM = &fmm.MetricMover{}
	app.MetricMover = fMM
	app.AddLUN(&agentd.LUN{
		VolumeSeriesID:       "VS-2", // same id used above
		SnapIdentifier:       "HEAD", // head
		NuvoVolumeIdentifier: "nuvo-vol-2",
		DisableMetrics:       true, // but disabled
	})
	assert.Len(app.GetLUNs(), 2)
	assert.Equal(1, c.cntVSIo)
	c.publishVolumeIOStats(ctx)
	assert.Equal(2, c.cntVSIo)
	assert.Nil(fMM.InPVIoMData)

	tl.Logger().Info("** publishVolumeIOStats (1 head lun)")
	tl.Flush()
	deltaB := uint64(1000)
	lun.LastEffectiveSlope = 16666.0
	les := lun.LastEffectiveSlope
	lun.LastWriteStatTime = time.Now()
	lwst := lun.LastWriteStatTime
	fVSsdU := &fakeVolLMDUpdater{}
	c.volLMDUpdater = fVSsdU
	lmdGuardCntr = app.LMDGuard.Used
	comboStat3 := &nuvoapi.StatsCombinedVolume{}
	testutils.Clone(comboStat2, comboStat3)
	comboStat3.IOReads.Count++
	comboStat3.IOWrites.Count++
	comboStat3.IOWrites.SizeTotal += deltaB
	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat3, nil)
	app.AddLUN(lun)
	assert.Len(app.GetLUNs(), 3)
	fIoMDM.RetD = expD
	tBefore := time.Now()
	c.publishVolumeIOStats(ctx)
	tAfter := time.Now()
	assert.Equal(3, c.cntVSIo)
	assert.NotNil(fMM.InPVIoMData)
	assert.Len(fMM.InPVIoMData, 1)
	assert.Equal(expD, fMM.InPVIoMData[0])
	assert.True(tBefore.Before(fIoMDM.InNow))
	assert.True(tAfter.After(fIoMDM.InNow))
	assert.Equal(lun.VolumeSeriesID, fIoMDM.InObjID)
	assert.Equal(&comboStat2.IOReads, fIoMDM.InLRS)
	assert.Equal(&comboStat3.IOReads, fIoMDM.InRS)
	assert.Equal(&comboStat2.IOWrites, fIoMDM.InLWS)
	assert.Equal(&comboStat3.IOWrites, fIoMDM.InWS)
	assert.Equal(&comboStat3.IOReads, lun.LastReadStat)
	assert.Equal(&comboStat3.IOWrites, lun.LastWriteStat)
	assert.Equal(lmdGuardCntr+1, app.LMDGuard.Used)
	assert.NotEqual(lun.LastWriteStatTime, lwst)
	assert.True(tAfter.After(lun.LastWriteStatTime))
	assert.Equal(lun.VolumeSeriesID, fVSsdU.InUVSid)
	assert.NotNil(fVSsdU.InUVSargs)
	args := fVSsdU.InUVSargs
	assert.Equal(les, args.PrevSlope)
	assert.EqualValues(deltaB, args.SampleDeltaBytes)
	assert.EqualValues(comboStat2.IOWrites.SizeTotal, args.SampleLastSize)
	assert.Equal(lun.LastWriteStatTime.Sub(lwst), args.SampleDuration)
	assert.Equal(lun.LastWriteStatTime, args.SampleTime)

	tl.Logger().Info("** ReportVolumeMetric lun not found")
	tl.Flush()
	err = c.ReportVolumeMetric(ctx, "UNKNOWN-VOL")
	assert.Error(err)
	assert.Regexp("HEAD LUN not found", err)

	tl.Logger().Info("** ReportVolumeMetric metrics disabled")
	tl.Flush()
	l := app.FindLUN("VS-2", com.VolMountHeadIdentifier)
	assert.True(l.DisableMetrics)
	err = c.ReportVolumeMetric(ctx, "VS-2")
	assert.NoError(err)

	tl.Logger().Info("** ReportVolumeMetric metrics")
	tl.Flush()
	comboStat4 := &nuvoapi.StatsCombinedVolume{}
	testutils.Clone(comboStat2, comboStat4)
	comboStat4.IOWrites.Count++
	comboStat4.IOWrites.SizeTotal += deltaB
	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat4, nil)
	assert.NotNil(c.app.FindLUN(lun.VolumeSeriesID, lun.SnapIdentifier))
	assert.Equal(com.VolMountHeadIdentifier, lun.SnapIdentifier)
	err = c.ReportVolumeMetric(ctx, lun.VolumeSeriesID)
	assert.NoError(err)

	tl.Logger().Info("** Manifest stats error")
	tl.Flush()
	comboStat5 := &nuvoapi.StatsCombinedVolume{}
	testutils.Clone(comboStat2, comboStat5)
	comboStat5.IOWrites.Count++
	comboStat5.IOWrites.SizeTotal += deltaB
	nvAPI.EXPECT().GetVolumeStats(false, lun.NuvoVolumeIdentifier).Return(comboStat5, nil)
	nvAPI.EXPECT().Manifest(lun.NuvoVolumeIdentifier, true).Return(nil, fmt.Errorf("manifest-stats-error"))
	d, err = c.getVolumeIOStats(ctx, epoch, lun)
	assert.NoError(err)
}

func TestUpdateVolLMD(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		LMDGuard: util.NewCriticalSectionGuard(),
	}
	fc := &fake.Client{}
	app.OCrud = fc
	appS := &appServant.AppServant{}
	app.AppServant = appS

	c := &MetricComp{}
	c.Init(app)

	ctx := context.Background()

	d30m := time.Duration(30 * time.Minute)

	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{ID: "SP-1"},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Slos: map[string]models.RestrictedValueType{
				"RPO": models.RestrictedValueType{ValueType: models.ValueType{Kind: "DURATION", Value: "4h"}},
			},
		},
	}
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "CG-1",
				ServicePlanID:      "SP-1",
			},
		},
	}
	vsID := string(vsObj.Meta.ID)

	tl.Logger().Info("** no bytes written ⇒ no update")
	tl.Flush()
	args := &SnapCalculationArgs{
		PrevSlope: 16666.0,
	}
	err := c.updateVolLMD(ctx, vsID, args)
	assert.NoError(err)
	assert.Equal(args.PrevSlope, args.EffectiveSlope)

	tl.Logger().Info("** updater error")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("vs-updater")
	args = &SnapCalculationArgs{
		PrevSlope:        16666.0,
		SampleDeltaBytes: 1000,
		SampleDuration:   d30m,
	}
	err = c.updateVolLMD(ctx, vsID, args)
	assert.Error(err)
	assert.Regexp("vs-updater", err)
	assert.Equal(vsID, fc.InVSUpdaterID)
	assert.Equal(&crud.Updates{Set: []string{"lifecycleManagementData"}}, fc.InVSUpdaterItems)
	assert.EqualValues(1000, args.NewTotalBytes)
	assert.Equal(d30m, args.NewTotalDuration)

	tl.Logger().Info("** rpo error")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = nil
	fc.FetchVSUpdaterObj = vsClone(vsObj)
	appS.RetGSPErr = fmt.Errorf("gsp-error")
	appS.RetGSPObj = nil
	err = c.updateVolLMD(ctx, vsID, args)
	assert.Error(err)
	assert.Regexp("gsp-error", err)
	assert.EqualValues(vsObj.ServicePlanID, appS.InGSPid)
	assert.EqualValues(2*1000, args.NewTotalBytes)
	assert.Equal(2*d30m, args.NewTotalDuration)

	tl.Logger().Info("** no lmd")
	tl.Flush()
	appS.RetGSPErr = nil
	appS.RetGSPObj = spObj
	err = c.updateVolLMD(ctx, vsID, args)
	assert.Error(err)
	assert.Regexp("lmd not found", err)
	assert.EqualValues(3*1000, args.NewTotalBytes)
	assert.Equal(3*d30m, args.NewTotalDuration)

	tl.Logger().Info("** lmd updated")
	tl.Flush()
	lst := time.Now().Add(-30 * time.Minute)
	pnst := lst.Add(3 * time.Hour)
	nst := lst.Add(2 * time.Hour)
	lmd := &models.LifecycleManagementData{
		LastSnapTime:              strfmt.DateTime(lst),
		EstimatedSizeBytes:        100000,
		LastUploadTransferRateBPS: 1000000,
		SizeEstimateRatio:         1.0,
		NextSnapshotTime:          strfmt.DateTime(pnst),
	}
	fVSsdU := &fakeVolLMDUpdater{}
	fVSsdU.OutSINSTst = nst
	c.volLMDUpdater = fVSsdU
	vs := vsClone(vsObj)
	vs.LifecycleManagementData = lmd
	fc.FetchVSUpdaterObj = vs
	expArgs := &SnapCalculationArgs{}
	*expArgs = *args
	expArgs.LastSnapTime = lst
	expArgs.NextSnapTime = pnst
	expArgs.RPODuration = 4 * time.Hour
	expArgs.EstimatedSizeBytes = util.SizeBytes(lmd.EstimatedSizeBytes) + args.SampleDeltaBytes
	expArgs.TransferRate = lmd.LastUploadTransferRateBPS
	expArgs.SizeEstimateRatio = lmd.SizeEstimateRatio
	expArgs.NewTotalBytes += args.SampleDeltaBytes
	expArgs.NewTotalDuration += args.SampleDuration
	expLMD := &models.LifecycleManagementData{}
	*expLMD = *lmd
	expLMD.EstimatedSizeBytes += int64(args.SampleDeltaBytes)
	expLMD.NextSnapshotTime = strfmt.DateTime(nst)
	err = c.updateVolLMD(ctx, vsID, args)
	assert.NoError(err)
	assert.NotNil(fVSsdU.InSCESarg)
	assert.NotNil(fVSsdU.InSINSTarg)
	assert.Equal(expLMD, lmd)
	assert.Equal(nst, time.Time(lmd.NextSnapshotTime))
	assert.Equal(expArgs, fVSsdU.InSCESarg)
	assert.EqualValues(4*1000, args.NewTotalBytes)
	assert.Equal(4*d30m, args.NewTotalDuration)
}

func TestSnapComputeNextSnapTime(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		LMDGuard: util.NewCriticalSectionGuard(),
	}
	fc := &fake.Client{}
	app.OCrud = fc
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cspOp := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.CSP = cspOp

	c := &MetricComp{}
	c.Init(app)

	rpoDuration := 4 * time.Hour
	rpoDurSec := int64(rpoDuration / time.Second)
	safetyBufferSec := int64(float64(rpoDurSec) * agentd.SnapshotSafetyBufferMultiplier)
	safetyDurSec := rpoDurSec - safetyBufferSec
	tNow := time.Now()
	maxSnapTime := tNow.Add(time.Duration(safetyDurSec) * time.Second)
	sz1M := util.SizeBytes(1000 * 1000)
	transferRate := int32(sz1M)
	maxSzBytes := util.SizeBytes(int64(safetyDurSec) * int64(transferRate))
	volSzBytes := sz1M * sz1M * 16
	safetyBufferSzBytes := util.SizeBytes(int64(safetyBufferSec) * int64(transferRate))
	tl.Logger().Debugf("RPO Dur %d, RPO Dur Sec %d, safetyBufferSec %d, safetyDurSec %d, tNow %s, maxSnapTime %s, sz1M %d, transferRate %d, maxSzBytes %d, volSzBytes %d", rpoDuration, rpoDurSec, safetyBufferSec, safetyDurSec, tNow, maxSnapTime, sz1M, transferRate, maxSzBytes, volSzBytes)

	// No Data Changed, No Space Pressure - maxSnapTime
	tc := SnapCalculationArgs{EstimatedSizeBytes: 0, TransferRate: transferRate, SizeEstimateRatio: 1.0, NextSnapTime: maxSnapTime, LastSnapTime: tNow, FreeBytes: volSzBytes, RPODuration: rpoDuration}
	expNST := tc.NextSnapTime
	tc.NextSnapTime = time.Time{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspOp = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.CSP = cspOp
	c.snapComputeNextSnapTime(&tc)
	tl.Logger().Debugf("tc 1 : %s", tc.Debug.String())
	tl.Flush()
	assert.Equal(expNST.Truncate(time.Second), tc.NextSnapTime.Truncate(time.Second), "tc 1")

	// Time to kick off - NOW
	tcs := []SnapCalculationArgs{
		// So much Data Changed - go NOW!
		{EstimatedSizeBytes: 2 * maxSzBytes, TransferRate: transferRate, SizeEstimateRatio: 1.0, LastSnapTime: tNow, FreeBytes: volSzBytes}, // 0
		// Not Much Data Changed, but the TransferRate is slow - go NOW!
		{EstimatedSizeBytes: maxSzBytes / 8, TransferRate: transferRate / 16, SizeEstimateRatio: 1.0, LastSnapTime: tNow, FreeBytes: volSzBytes}, // 0
		// It's been a long time - go NOW!
		{EstimatedSizeBytes: 0, TransferRate: transferRate, SizeEstimateRatio: 1.0, LastSnapTime: time.Time{}, FreeBytes: volSzBytes}, // 0
		// Data Changed and we have a 2x multiplier - go NOW!
		{EstimatedSizeBytes: maxSzBytes, TransferRate: transferRate, SizeEstimateRatio: 2.0, LastSnapTime: tNow, FreeBytes: volSzBytes}, // 0
		// The Volume is almost full - go NOW!
		{EstimatedSizeBytes: maxSzBytes / 8, TransferRate: transferRate, SizeEstimateRatio: 1.0, LastSnapTime: tNow, FreeBytes: 1}, // 0

	}
	for i, tc := range tcs {
		tc.RPODuration = rpoDuration
		tc.NextSnapTime = time.Time{}
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		cspOp = mockcsp.NewMockCloudServiceProvider(mockCtrl)
		app.CSP = cspOp
		if tc.TransferRate == 0 {
			cspOp.EXPECT().ProtectionStoreUploadTransferRate().Return(transferRate)
		}
		c.snapComputeNextSnapTime(&tc)
		tl.Logger().Debugf("tc %d: %s", i, tc.Debug.String())
		tl.Flush()
		assert.True(tc.NextSnapTime.Before(tNow), "tc %d", i)
	}

	// Kick off after now, but before MaxSnapTime
	tcs = []SnapCalculationArgs{
		// Some Data Changed - go Sooner!
		{EstimatedSizeBytes: safetyBufferSzBytes, TransferRate: transferRate, SizeEstimateRatio: 1.0, LastSnapTime: tNow, FreeBytes: volSzBytes}, // 0
		// Some Data Changed and the TransferRate is messed up - go Sooner!
		{EstimatedSizeBytes: safetyBufferSzBytes, TransferRate: 0, SizeEstimateRatio: 1.0, LastSnapTime: tNow, FreeBytes: volSzBytes}, // 0
		// It's been some time - go Sooner!
		{EstimatedSizeBytes: 0, TransferRate: transferRate, SizeEstimateRatio: 1.0, LastSnapTime: tNow.Add(-1 * time.Duration(safetyBufferSec) * time.Second), FreeBytes: volSzBytes}, // 0
		// Data Changed and we have a 0.5x multiplier - go Sooner!
		{EstimatedSizeBytes: safetyBufferSzBytes, TransferRate: transferRate, SizeEstimateRatio: 0.5, LastSnapTime: tNow, FreeBytes: volSzBytes}, // 0
		// Data Changed and we have a pretty full volume - go Sooner!
		{EstimatedSizeBytes: safetyBufferSzBytes, TransferRate: transferRate, SizeEstimateRatio: 1.0, LastSnapTime: tNow, FreeBytes: volSzBytes * 3 / 4}, // 0
	}
	for i, tc := range tcs {
		tc.RPODuration = rpoDuration
		tc.NextSnapTime = time.Time{}
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		cspOp = mockcsp.NewMockCloudServiceProvider(mockCtrl)
		app.CSP = cspOp
		if tc.TransferRate == 0 {
			cspOp.EXPECT().ProtectionStoreUploadTransferRate().Return(transferRate)
		}
		c.snapComputeNextSnapTime(&tc)
		tl.Logger().Debugf("tc %d: %s", i, tc.Debug.String())
		tl.Flush()
		assert.True(tc.NextSnapTime.Before(maxSnapTime), "tc-2a %d", i)
		assert.True(tNow.Before(tc.NextSnapTime), "tc-2b %d", i)
	}
}

func TestSnapComputeEffectiveSlope(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		LMDGuard: util.NewCriticalSectionGuard(),
	}
	fc := &fake.Client{}
	app.OCrud = fc

	c := &MetricComp{}
	c.Init(app)

	d1m := time.Minute
	d5m := 5 * d1m
	d300m := 300 * d1m
	sz1M := util.SizeBytes(1000 * 1000)
	sz5M := 5 * sz1M
	sz10M := 10 * sz1M
	sz30M := 30 * sz1M
	sz100M := 100 * sz1M
	sz300M := 300 * sz1M
	sz3G := 3000 * sz1M

	tcs := []SnapCalculationArgs{
		{PrevSlope: 0.0, SampleDeltaBytes: sz30M, SampleDuration: d5m, SampleLastSize: 0, SizeEstimateRatio: 1.0, NewTotalBytes: sz3G, NewTotalDuration: d300m, EffectiveSlope: 100000.0},    // 0
		{PrevSlope: 5000.0, SampleDeltaBytes: sz30M, SampleDuration: d5m, SampleLastSize: 0, SizeEstimateRatio: 1.0, NewTotalBytes: sz3G, NewTotalDuration: d300m, EffectiveSlope: 100000.0}, // 1
		{PrevSlope: 150000.0, SampleDeltaBytes: sz30M, SampleDuration: d5m, SampleLastSize: 0, SizeEstimateRatio: 1.0, NewTotalBytes: sz3G, NewTotalDuration: d300m, EffectiveSlope: 166666}, // 2 no prev data

		{PrevSlope: 160000.0, SampleDeltaBytes: sz30M, SampleDuration: d5m, SampleLastSize: sz100M, SizeEstimateRatio: 1.0, NewTotalBytes: sz3G, NewTotalDuration: d300m, EffectiveSlope: 166666},     // 3
		{PrevSlope: 160000.0, SampleDeltaBytes: sz10M, SampleDuration: d5m, SampleLastSize: sz100M, SizeEstimateRatio: 1.0, NewTotalBytes: sz3G, NewTotalDuration: d300m, EffectiveSlope: 166666},     // 4
		{PrevSlope: 160000.0, SampleDeltaBytes: sz300M, SampleDuration: d5m, SampleLastSize: sz100M, SizeEstimateRatio: 1.0, NewTotalBytes: sz3G, NewTotalDuration: d300m, EffectiveSlope: 1000000.0}, // 5

		// Realistic large data write rate = 1MB/min ⇒ slope of 16666
		{PrevSlope: 0.0, SampleDeltaBytes: sz5M, SampleDuration: d5m, SampleLastSize: 0, SizeEstimateRatio: 1.0, NewTotalBytes: sz3G, NewTotalDuration: d300m, EffectiveSlope: 16666}, // 6
	}
	for i, tc := range tcs {
		expES := tc.EffectiveSlope
		tc.EffectiveSlope = 0.0
		c.snapComputeEffectiveSlope(&tc)
		tl.Logger().Debugf("tc %d: %s", i, tc.Debug.String())
		tl.Flush()
		assert.Equal(int(expES), int(tc.EffectiveSlope), "tc %d", i)
	}
}

func TestSnapInterpolateNextSnapTime(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		LMDGuard: util.NewCriticalSectionGuard(),
	}
	fc := &fake.Client{}
	app.OCrud = fc
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cspOp := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.CSP = cspOp

	c := &MetricComp{}
	c.Init(app)

	rpoDuration := 4 * time.Hour
	rpoDurSec := int64(rpoDuration / time.Second)
	safetyBufferSec := int64(float64(rpoDurSec) * agentd.SnapshotSafetyBufferMultiplier)
	safetyDurSec := rpoDurSec - safetyBufferSec
	epoch, err := time.Parse("2006-01-02T15:04:05Z", "2018-09-20T00:00:00Z")
	assert.NoError(err)
	lastSnapTime := epoch
	maxSnapTime := lastSnapTime.Add(time.Duration(safetyDurSec) * time.Second)
	t1 := lastSnapTime.Add(2 * time.Hour)
	t2 := lastSnapTime.Add(3 * time.Hour)

	d1m := time.Minute
	d5m := 5 * d1m
	d10m := 10 * d1m
	sz1M := util.SizeBytes(1000 * 1000)
	sz5M := 5 * sz1M
	sz30M := 30 * sz1M
	transferRate := int32(sz1M)
	// Max bytes in 4h-30m @ transfer rate of 1MB/s = 12600MB.
	// Realistic large data write rate = 1MB/min ⇒ slope of 16666 (from slope UT)
	maxBytes := util.SizeBytes(safetyDurSec * int64(transferRate))
	sampleSize := sz5M

	tcs := []SnapCalculationArgs{
		{EffectiveSlope: 0.0, EstimatedSizeBytes: sz30M, SampleTime: t1, TransferRate: transferRate, NextSnapTime: maxSnapTime},     // 0
		{EffectiveSlope: 10000.0, EstimatedSizeBytes: sz30M, SampleTime: t1, TransferRate: 0, NextSnapTime: maxSnapTime},            // 1
		{EffectiveSlope: 10000.0, EstimatedSizeBytes: sz30M, SampleTime: t1, TransferRate: transferRate, NextSnapTime: maxSnapTime}, // 2 same as prev

		{EffectiveSlope: 16666.0, EstimatedSizeBytes: maxBytes - sampleSize, SampleTime: t2, TransferRate: transferRate, NextSnapTime: t2.Add(d5m)},    // 3
		{EffectiveSlope: 16666.0, EstimatedSizeBytes: maxBytes - 2*sampleSize, SampleTime: t2, TransferRate: transferRate, NextSnapTime: t2.Add(d10m)}, // 4
		{EffectiveSlope: 16666.0, EstimatedSizeBytes: maxBytes, SampleTime: t2, TransferRate: transferRate, NextSnapTime: t2},                          // 5 - force immediate
	}
	for i, tc := range tcs {
		tc.LastSnapTime = lastSnapTime
		tc.RPODuration = rpoDuration
		tc.SizeEstimateRatio = 1.0
		expNST := tc.NextSnapTime
		tc.NextSnapTime = time.Time{}
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		cspOp = mockcsp.NewMockCloudServiceProvider(mockCtrl)
		app.CSP = cspOp
		if tc.TransferRate == 0 {
			cspOp.EXPECT().ProtectionStoreUploadTransferRate().Return(transferRate)
		}
		c.snapInterpolateNextSnapTime(&tc)
		tl.Logger().Debugf("tc %d: %s", i, tc.Debug.String())
		tl.Flush()
		assert.Equal(expNST.Truncate(time.Second), tc.NextSnapTime.Truncate(time.Second), "tc %d", i)
	}
}

type fakeVolLMDUpdater struct {
	InUVSid   string
	InUVSargs *SnapCalculationArgs
	RetUVSerr error

	InSCESarg *SnapCalculationArgs

	InSINSTarg *SnapCalculationArgs
	OutSINSTst time.Time
}

func (c *fakeVolLMDUpdater) updateVolLMD(ctx context.Context, vsID string, args *SnapCalculationArgs) error {
	c.InUVSid = vsID
	c.InUVSargs = args
	return c.RetUVSerr
}
func (c *fakeVolLMDUpdater) snapComputeNextSnapTime(args *SnapCalculationArgs) {
	c.InSCESarg = scaClone(args)
	c.InSINSTarg = scaClone(args)
	args.NextSnapTime = c.OutSINSTst
}
func (c *fakeVolLMDUpdater) snapComputeEffectiveSlope(args *SnapCalculationArgs) {
	c.InSCESarg = scaClone(args)
}
func (c *fakeVolLMDUpdater) snapInterpolateNextSnapTime(args *SnapCalculationArgs) {
	c.InSINSTarg = scaClone(args)
	args.NextSnapTime = c.OutSINSTst
}

func vsClone(vs *models.VolumeSeries) *models.VolumeSeries {
	n := new(models.VolumeSeries)
	testutils.Clone(vs, n)
	return n
}

func scaClone(o *SnapCalculationArgs) *SnapCalculationArgs {
	n := new(SnapCalculationArgs)
	*n = *o
	return n
}
