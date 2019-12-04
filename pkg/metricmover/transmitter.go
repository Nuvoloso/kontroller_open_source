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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

type transmitter struct {
	TxArgs
	m         *mover
	oCrud     crud.Ops
	worker    util.Worker
	started   bool
	vsIoQueue []*models.IoMetricDatum
	sIoQueue  []*models.IoMetricDatum
}

func newTransmitter(m *mover, args *TxArgs) (*transmitter, error) {
	tx := &transmitter{m: m}
	tx.TxArgs = *args
	if err := tx.TxArgs.sanitize(); err != nil {
		return nil, err
	}
	tx.oCrud = crud.NewClient(tx.API, m.Log)
	wa := &util.WorkerArgs{
		Name:          "MetricMover",
		Log:           tx.m.Log,
		SleepInterval: tx.RetryInterval,
	}
	w, err := util.NewWorker(wa, tx)
	if err != nil {
		return nil, err
	}
	tx.worker = w
	return tx, nil
}

func (tx *transmitter) Start() {
	tx.worker.Start()
	tx.started = true
}

func (tx *transmitter) Stop() {
	tx.worker.Stop()
	tx.started = false
}

func (tx *transmitter) vsIoQPopNInLock(j int) {
	if len(tx.vsIoQueue) >= j {
		// https://github.com/golang/go/wiki/SliceTricks (non-leaky cut)
		copy(tx.vsIoQueue[0:], tx.vsIoQueue[j:])
		for k, n := len(tx.vsIoQueue)-j, len(tx.vsIoQueue); k < n; k++ {
			tx.vsIoQueue[k] = nil
		}
		tx.vsIoQueue = tx.vsIoQueue[:len(tx.vsIoQueue)-j]
	}
}

// ConsumeVolumeIoMetricData satisfies the MetricConsumer interface
func (tx *transmitter) ConsumeVolumeIoMetricData(data []*models.IoMetricDatum) {
	// Protected by manager mux
	tx.vsIoQueue = append(tx.vsIoQueue, data...)
	tx.m.Log.Debugf("MetricMover: TxConsumer: VsIo +%d ⇒ %d", len(data), len(tx.vsIoQueue))
	tx.worker.Notify()
}

func (tx *transmitter) sIoQPopNInLock(j int) {
	if len(tx.sIoQueue) >= j {
		// https://github.com/golang/go/wiki/SliceTricks (non-leaky cut)
		copy(tx.sIoQueue[0:], tx.sIoQueue[j:])
		for k, n := len(tx.sIoQueue)-j, len(tx.sIoQueue); k < n; k++ {
			tx.sIoQueue[k] = nil
		}
		tx.sIoQueue = tx.sIoQueue[:len(tx.sIoQueue)-j]
	}
}

// ConsumeStorageIoMetricData satisfies the MetricConsumer interface
func (tx *transmitter) ConsumeStorageIoMetricData(data []*models.IoMetricDatum) {
	// Protected by manager mux
	tx.sIoQueue = append(tx.sIoQueue, data...)
	tx.m.Log.Debugf("MetricMover: TxConsumer: SIo +%d ⇒ %d", len(data), len(tx.sIoQueue))
	tx.worker.Notify()
}

// Buzz satisfies the util.WorkerBee interface
func (tx *transmitter) Buzz(ctx context.Context) error {
	tx.m.mux.Lock()
	nVsIO := len(tx.vsIoQueue)
	vsIOData := make([]*models.IoMetricDatum, nVsIO)
	copy(vsIOData, tx.vsIoQueue)
	nSIO := len(tx.sIoQueue)
	sIOData := make([]*models.IoMetricDatum, nSIO)
	copy(sIOData, tx.sIoQueue)
	tx.m.mux.Unlock()
	var err, errVsIO, errSIO error
	if nVsIO > 0 {
		errVsIO = tx.oCrud.VolumeSeriesIOMetricUpload(ctx, vsIOData)
		tx.m.Log.Debugf("MetricMover: VsIOUpload: %d, %v", len(vsIOData), errVsIO)
	}
	if nSIO > 0 {
		errSIO = tx.oCrud.StorageIOMetricUpload(ctx, sIOData)
		tx.m.Log.Debugf("MetricMover: SIOUpload: %d, %v", len(sIOData), errSIO)
	}
	err = errVsIO
	if err == nil {
		err = errSIO
	}
	tx.m.mux.Lock()
	defer tx.m.mux.Unlock()
	if errVsIO != nil { // trim queue
		nVsIO = len(tx.vsIoQueue) - tx.VolumeIOMaxBuffered
	}
	if nVsIO > 0 {
		if errVsIO == nil {
			tx.m.vsIoSent += nVsIO
		} else {
			tx.m.vsIoDropped += nVsIO
			tx.m.Log.Warningf("MetricMover: Dropping %d VolumeSeries I/O metrics buffered records", nVsIO)
		}
		tx.vsIoQPopNInLock(nVsIO)
	}
	if errSIO != nil { // trim queue
		nSIO = len(tx.sIoQueue) - tx.StorageIOMaxBuffered
	}
	if nSIO > 0 {
		if errSIO == nil {
			tx.m.sIoSent += nSIO
		} else {
			tx.m.sIoDropped += nSIO
			tx.m.Log.Warningf("MetricMover: Dropping %d Storage I/O metrics buffered records", nSIO)
		}
		tx.sIoQPopNInLock(nSIO)
	}
	return err
}
