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
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
)

// ReportVolumeMetric is part of agentd.MetricReporter
func (c *MetricComp) ReportVolumeMetric(ctx context.Context, vsID string) error {
	now := time.Now()
	lun := c.app.FindLUN(vsID, com.VolMountHeadIdentifier)
	if lun == nil {
		c.Log.Errorf("ReportVolumeMetric(%s) HEAD LUN not found", vsID)
		return fmt.Errorf("%s HEAD LUN not found", vsID)
	}
	c.Log.Debugf("ReportVolumeMetric(%s) disabled=%v", vsID, lun.DisableMetrics)
	if lun.DisableMetrics {
		return nil
	}
	var m *models.IoMetricDatum
	var err error
	if m, err = c.getVolumeIOStats(ctx, now, lun); err == nil && m != nil {
		md := []*models.IoMetricDatum{m}
		c.app.MetricMover.PublishVolumeIoMetrics(md)
	}
	return err
}

func (c *MetricComp) publishVolumeIOStats(ctx context.Context) {
	now := time.Now()
	md := []*models.IoMetricDatum{}
	for _, lun := range c.app.GetLUNs() {
		if lun.SnapIdentifier != com.VolMountHeadIdentifier {
			continue
		}
		if lun.DisableMetrics {
			continue
		}
		m, _ := c.getVolumeIOStats(ctx, now, lun)
		if m != nil {
			md = append(md, m)
		}
	}
	if len(md) > 0 {
		c.app.MetricMover.PublishVolumeIoMetrics(md)
	}
	c.cntVSIo++
}

func (c *MetricComp) getVolumeIOStats(ctx context.Context, now time.Time, lun *agentd.LUN) (*models.IoMetricDatum, error) {
	c.Log.Debugf("NUVOAPI GetVolumeStats(false, %s)", lun.NuvoVolumeIdentifier)
	wStatTime := time.Now()
	comboStats, err := c.app.NuvoAPI.GetVolumeStats(false, lun.NuvoVolumeIdentifier)
	if err != nil {
		c.Log.Warningf("NUVOAPI GetVolumeStats(volume, %s): %s", lun.NuvoVolumeIdentifier, err.Error())
		return nil, err
	}
	c.Log.Debugf("NUVOAPI Metrics on Volume %s (%s) READ: %v, WRITE: %v, CACHE USER: %v, CACHE META: %v", lun.VolumeSeriesID, lun.DeviceName, comboStats.IOReads, comboStats.IOWrites, comboStats.CacheUser, comboStats.CacheMetadata)
	readStats := &comboStats.IOReads
	writeStats := &comboStats.IOWrites

	var datum *models.IoMetricDatum
	mustUpdateVol := false
	if lun.LastReadStat != nil && lun.LastWriteStat != nil {
		if readStats.SeriesUUID == lun.LastReadStat.SeriesUUID && writeStats.SeriesUUID == lun.LastWriteStat.SeriesUUID {
			// Note: makeIOMetricDatum is common function for both volume and storage device metrics with the last 4 parameters being cache stats present for volumes only
			if datum, err = c.ioMDMaker.makeIOMetricDatum(now, lun.VolumeSeriesID, readStats, lun.LastReadStat, writeStats, lun.LastWriteStat, &comboStats.CacheUser, lun.LastCacheUserStat, &comboStats.CacheMetadata, lun.LastCacheMetaStat); err != nil {
				c.Log.Warningf("Unable to process nuvoapi metric data: %s", err.Error())
				return nil, err
			}
			mustUpdateVol = true
		} else {
			c.Log.Debugf("Volume %s: series UUID changed", lun.VolumeSeriesID)
		}
	} else {
		c.Log.Debugf("Volume %s: skipping first I/O stat", lun.VolumeSeriesID)
	}
	cst, err := c.app.LMDGuard.Enter("viom")
	if err != nil {
		c.Log.Errorf("Unable to obtain LMDGuard: %s", err.Error())
		return nil, err
	}
	defer cst.Leave()
	if mustUpdateVol {
		// If the volume is running out of free space, take snapshots more aggressively to get blocks out of PiTs
		var volumeFreeBytes util.SizeBytes
		c.Log.Debugf("NUVOAPI GetVolumeManifest(false, %s)", lun.NuvoVolumeIdentifier)
		devices, err := c.app.NuvoAPI.Manifest(lun.NuvoVolumeIdentifier, true)
		if err == nil {
			c.Log.Debugf("NUVOAPI Manifest on Volume %s (%s) NUM DEVICES (%d): %v", lun.VolumeSeriesID, lun.DeviceName, len(devices), devices)
			for _, device := range devices {
				volumeFreeBytes += util.SizeBytes((device.ParcelSize * uint64(device.TargetParcels)) - (device.BlocksUsed * uint64(device.BlockSize)))
			}
		} else {
			c.Log.Warningf("NUVOAPI Manifest(volume, %s): %s", lun.NuvoVolumeIdentifier, err.Error())
			volumeFreeBytes = -1
		}

		args := &SnapCalculationArgs{
			FreeBytes:         volumeFreeBytes,
			PrevSlope:         lun.LastEffectiveSlope,
			PrevTotalBytes:    lun.TotalWriteBytes,
			PrevTotalDuration: lun.TotalWriteDuration,
			SampleDeltaBytes:  util.SizeBytes(writeStats.SizeTotal - lun.LastWriteStat.SizeTotal),
			SampleDuration:    wStatTime.Sub(lun.LastWriteStatTime),
			SampleLastSize:    util.SizeBytes(lun.LastWriteStat.SizeTotal),
			SampleTime:        wStatTime,
		}
		if err := c.volLMDUpdater.updateVolLMD(ctx, lun.VolumeSeriesID, args); err == nil {
			lun.LastEffectiveSlope = args.EffectiveSlope
			c.Log.Debugf("LMD Vol[%s] %s", lun.VolumeSeriesID, args.Debug.String())
		}
		lun.TotalWriteBytes = args.NewTotalBytes
		lun.TotalWriteDuration = args.NewTotalDuration
	}
	lun.LastReadStat = readStats
	lun.LastWriteStat = writeStats
	lun.LastWriteStatTime = wStatTime
	lun.LastCacheUserStat = &comboStats.CacheUser
	lun.LastCacheMetaStat = &comboStats.CacheMetadata
	return datum, nil
}

// SnapCalculationArgs is used to pass arguments to CalculateNextSnapTime
type SnapCalculationArgs struct {
	EffectiveSlope     float64        // computed
	EstimatedSizeBytes util.SizeBytes // from vs lmd
	LastSnapTime       time.Time      // from vs lmd
	NextSnapTime       time.Time      // previous / re-computed
	FreeBytes          util.SizeBytes // from checking lun
	PrevSlope          float64        // from lun cache
	PrevTotalBytes     util.SizeBytes // from lun cache
	PrevTotalDuration  time.Duration  // from lun cache
	RPODuration        time.Duration  // from service plan via the VS
	SampleDeltaBytes   util.SizeBytes // from metric and lun cache
	SampleDuration     time.Duration  // from metric and lun cache
	SampleLastSize     util.SizeBytes // from metric and lun cache
	SampleTime         time.Time      // from metric
	NewTotalBytes      util.SizeBytes // computed
	NewTotalDuration   time.Duration  // computed
	SizeEstimateRatio  float64        // from vs lmd
	TransferRate       int32          // from vs lmd or csp default
	Debug              bytes.Buffer
}

// updateVolLMD updates the VS with a new snapshot time estimate
// Input arg fields:
//  - PrevSlope
//  - PrevTotalBytes
//  - PrevTotalDuration
//  - SampleDeltaBytes
//  - SampleDuration
//  - SampleLastSize
//  - SampleTime
// It must set the following on success or failure:
//  - NewTotalBytes
//  - NewTotalDuration
// It must set at least the following on success:
//  - EffectiveSlope
// Remaining properties will be set if the volume is updated.
func (c *MetricComp) updateVolLMD(ctx context.Context, vsID string, args *SnapCalculationArgs) error {
	if args.SampleDeltaBytes == 0 {
		fmt.Fprintf(&args.Debug, "no change")
		args.EffectiveSlope = args.PrevSlope
		args.NewTotalBytes = args.PrevTotalBytes
		args.NewTotalDuration = args.PrevTotalDuration
		return nil
	}
	args.NewTotalBytes += args.SampleDeltaBytes
	args.NewTotalDuration += args.SampleDuration
	var lmd *models.LifecycleManagementData
	var rpoDur time.Duration
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			return nil, nil
		}
		args.Debug.Reset()
		// get the service plan
		if rpoDur == 0 {
			sp, err := c.app.AppServant.GetServicePlan(ctx, string(vs.ServicePlanID))
			if err != nil {
				return nil, err
			}
			rpoDur = c.app.GetRPO(sp)
		}
		lmd = vs.LifecycleManagementData
		if lmd == nil {
			return nil, fmt.Errorf("lmd not found")
		}
		lmd.EstimatedSizeBytes += int64(args.SampleDeltaBytes) // unscaled
		// collate the arguments required
		args.LastSnapTime = time.Time(lmd.LastSnapTime)
		args.NextSnapTime = time.Time(lmd.NextSnapshotTime)
		args.RPODuration = rpoDur
		args.EstimatedSizeBytes = util.SizeBytes(lmd.EstimatedSizeBytes)
		args.TransferRate = lmd.LastUploadTransferRateBPS
		args.SizeEstimateRatio = lmd.SizeEstimateRatio
		// compute next snap time - easy version
		c.volLMDUpdater.snapComputeNextSnapTime(args)
		// compute slope and interpolate
		// c.volLMDUpdater.snapComputeEffectiveSlope(args)
		// c.volLMDUpdater.snapInterpolateNextSnapTime(args)
		lmd.NextSnapshotTime = strfmt.DateTime(args.NextSnapTime)
		return vs, nil
	}
	items := &crud.Updates{Set: []string{"lifecycleManagementData"}}
	_, err := c.app.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		c.Log.Errorf("Unable to update LMD in %s: %s", vsID, err.Error())
	}
	return err
}

// snapComputeNextSnapTime caclulates if we're running out of time to trigger a transfer
// Input arg fields:
// - SampleDeltaBytes > 0
// - SampleDuration > 0
// - NewTotalBytes
//  - NewTotalDuration
// It will set the following:
//  - NextSnapTime
func (c *MetricComp) snapComputeNextSnapTime(args *SnapCalculationArgs) {
	b := &args.Debug

	rpoDurSec := int64(args.RPODuration / time.Second)
	safetyBufferSec := int64(float64(rpoDurSec) * agentd.SnapshotSafetyBufferMultiplier)

	// Time it would take to transfer the bytes we're already got
	// NOTE: Bytes per Second should factor in dedupe
	// SizeEstimateRatio = bytes_xfer / bytes_total on last transfer
	// (bytes_xfer / second) / (bytes_xfer / bytes_total) = bytes_total / second
	bps := int32(float64(args.TransferRate) / args.SizeEstimateRatio)
	if bps <= 0 {
		bps = c.app.CSP.ProtectionStoreUploadTransferRate()
	}
	transferDurSec := int64(args.EstimatedSizeBytes) / int64(bps)
	fmt.Fprintf(b, "TR=%d BPS=%d TDS=%d FREE_BYTES=%d", args.TransferRate, bps, transferDurSec, args.FreeBytes)

	tNow := time.Now()
	timeSinceSnapSec := int64(tNow.Sub(args.LastSnapTime) / time.Second)

	// RPO - (buffer + time_to_xfer_data + time since last snap) = How long until we must start
	timeUntilTransferRPOSec := rpoDurSec - (safetyBufferSec + transferDurSec + timeSinceSnapSec)

	// Disk Full
	var bytesWrittenPerSec, timeToFullSec int64
	// Time it will take to fill the free capacity at this rate
	// If less than 1 second has passed since the last snapshot, estimate we are writing EstimatedSizeBytes per second
	if timeSinceSnapSec <= 0 {
		bytesWrittenPerSec = int64(args.EstimatedSizeBytes) / 1
	} else {
		bytesWrittenPerSec = int64(args.EstimatedSizeBytes) / timeSinceSnapSec
	}
	if bytesWrittenPerSec == 0 || int64(args.FreeBytes) < 0 {
		timeToFullSec = rpoDurSec
	} else {
		timeToFullSec = int64(int64(args.FreeBytes) / bytesWrittenPerSec)
	}

	timeUntilTransferDiskFullSec := timeToFullSec - (safetyBufferSec + transferDurSec)

	// NOTE: You can add a negative duration (i.e. if we should have already kicked off transfer)
	// It will just set NextSnapTime in the past & kick off ASAP
	if timeUntilTransferDiskFullSec < timeUntilTransferRPOSec {
		args.NextSnapTime = tNow.Add(time.Duration(timeUntilTransferDiskFullSec) * time.Second)
	} else {
		args.NextSnapTime = tNow.Add(time.Duration(timeUntilTransferRPOSec) * time.Second)
	}
}

// snapComputeEffectiveSlope calculates the effective slope using a weighted reduction algorithm.
// Input arg fields:
//  - PrevSlope
//  - SampleDeltaBytes > 0
//  - SampleDuration > 0
//  - SampleLastSize
//  - SizeEstimateRatio
//  - NewTotalBytes
//  - NewTotalDuration
// It will set the following:
//  - EffectiveSlope
func (c *MetricComp) snapComputeEffectiveSlope(args *SnapCalculationArgs) {
	b := &args.Debug

	fmt.Fprintf(b, "pSlope=%.0f ", args.PrevSlope)
	deltaS := int64(args.SampleDuration / time.Second)
	newSlope := float64(args.SampleDeltaBytes) * args.SizeEstimateRatio / float64(deltaS)
	fmt.Fprintf(b, "nSlope=(%s*%.0f)/%ds=%.0f ", args.SampleDeltaBytes, args.SizeEstimateRatio, deltaS, newSlope)
	args.EffectiveSlope = newSlope
	if newSlope < args.PrevSlope {
		totalS := float64(args.NewTotalDuration / time.Second)
		args.EffectiveSlope = float64(args.NewTotalBytes) * args.SizeEstimateRatio / totalS
		fmt.Fprintf(b, "avgSlope=%s*%.0f/%.0fs ", args.NewTotalBytes, args.SizeEstimateRatio, totalS)
	}
	fmt.Fprintf(b, "eSlope=%.0f ", args.EffectiveSlope)
}

// snapInterpolateNextSnapTime calculates the next snap time using interpolation.
// Input arg fields:
//  - EffectiveSlope
//  - EstimatedSizeBytes
//  - LastSnapTime
//  - NextSnapTime
//  - RPODuration
//  - SampleTime
//  - SizeEstimateRatio
//  - TransferRate
// It will set the following:
//  - NextSnapTime
func (c *MetricComp) snapInterpolateNextSnapTime(args *SnapCalculationArgs) {
	b := &args.Debug

	// compute limits
	rpoDurSec := int64(args.RPODuration / time.Second)
	safetyBufferSec := int64(float64(rpoDurSec) * agentd.SnapshotSafetyBufferMultiplier)
	safetyDurSec := rpoDurSec - safetyBufferSec
	fmt.Fprintf(b, "RPO=%ds Safe=%ds ", rpoDurSec, safetyBufferSec)
	maxSnapTime := args.LastSnapTime.Add(time.Duration(safetyDurSec) * time.Second)
	fmt.Fprintf(b, "MaxST={%s}+%ds={%s} ", args.LastSnapTime.Format(time.RFC3339), safetyDurSec, maxSnapTime.Format(time.RFC3339))
	if args.EffectiveSlope <= 0.0 {
		fmt.Fprintf(b, "NST:{%s}⇒", args.NextSnapTime.Format(time.RFC3339))
		args.NextSnapTime = maxSnapTime
		fmt.Fprintf(b, "{%s} (M,sl<0) ", args.NextSnapTime.Format(time.RFC3339))
		return
	}
	bps := args.TransferRate
	if bps <= 0 {
		bps = c.app.CSP.ProtectionStoreUploadTransferRate()
	}
	fmt.Fprintf(b, "TR=%d BPS=%d ", args.TransferRate, bps)
	maxBytes := util.SizeBytes(safetyDurSec * int64(bps))
	fmt.Fprintf(b, "MaxB=%ds*%dbps=%s ", safetyDurSec, bps, maxBytes)

	// get scaled size
	scaledSize := util.SizeBytes(float64(args.EstimatedSizeBytes) * args.SizeEstimateRatio)
	fmt.Fprintf(b, "SSz=%f*%s=%s ", args.SizeEstimateRatio, args.EstimatedSizeBytes, scaledSize)
	if scaledSize >= maxBytes { // help!
		fmt.Fprintf(b, "NST:{%s}⇒", args.NextSnapTime.Format(time.RFC3339))
		args.NextSnapTime = args.SampleTime // force immediate snapshot
		fmt.Fprintf(b, "{%s} (F,sz>max) ", args.NextSnapTime.Format(time.RFC3339))
		return
	}

	// interpolate
	deltaS := int64(float64(maxBytes-scaledSize) / args.EffectiveSlope) // time for remaining capacity
	fmt.Fprintf(b, "deltaS=(%s-%s)/%.0f=%ds ", maxBytes, scaledSize, args.EffectiveSlope, deltaS)
	intT := args.SampleTime.Add(time.Duration(deltaS) * time.Second)
	fmt.Fprintf(b, "intT={%s}+%ds={%s} ", args.SampleTime.Format(time.RFC3339), deltaS, intT.Format(time.RFC3339))
	fmt.Fprintf(b, "NST:{%s}⇒", args.NextSnapTime.Format(time.RFC3339))
	if intT.After(maxSnapTime) {
		args.NextSnapTime = maxSnapTime
		fmt.Fprintf(b, "{%s} (M,nt>mt) ", args.NextSnapTime.Format(time.RFC3339))
	} else {
		args.NextSnapTime = intT
		fmt.Fprintf(b, "{%s} (I) ", args.NextSnapTime.Format(time.RFC3339))
	}
}
