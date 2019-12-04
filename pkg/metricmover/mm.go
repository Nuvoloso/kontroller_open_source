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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/op/go-logging"
)

// Module constants
const (
	TxRetryIntervalDefault        = 20 * time.Second
	TxVolumeIOMaxBufferedDefault  = 100
	TxStorageIOMaxBufferedDefault = 50
)

// MetricConsumer consumes incoming metric data. The interfaces are assumed to be thread safe.
type MetricConsumer interface {
	ConsumeVolumeIoMetricData([]*models.IoMetricDatum)
	ConsumeStorageIoMetricData([]*models.IoMetricDatum)
}

// TxArgs contains arguments to create a MetricMover transmitter
type TxArgs struct {
	API                  mgmtclient.API
	RetryInterval        time.Duration
	VolumeIOMaxBuffered  int
	StorageIOMaxBuffered int
}

func (a *TxArgs) sanitize() error {
	if a.API == nil {
		return fmt.Errorf("missing arguments")
	}
	if a.RetryInterval <= 0 {
		a.RetryInterval = TxRetryIntervalDefault
	}
	if a.VolumeIOMaxBuffered < 0 {
		a.VolumeIOMaxBuffered = TxVolumeIOMaxBufferedDefault
	}
	if a.StorageIOMaxBuffered < 0 {
		a.StorageIOMaxBuffered = TxStorageIOMaxBufferedDefault
	}
	return nil
}

// Status returns information on the MetricMover
type Status struct {
	ConsumerConfigured bool
	TxConfigured       bool
	TxStarted          bool
	VolumeIOPublished  int
	VolumeIOBuffered   int
	VolumeIOSent       int
	VolumeIOReceived   int
	VolumeIODropped    int
	StorageIOPublished int
	StorageIOBuffered  int
	StorageIOSent      int
	StorageIOReceived  int
	StorageIODropped   int
}

func (s *Status) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// MetricMover provides the ability to publish and to consume or convey metric data
type MetricMover interface {
	ConfigureTransmitter(args *TxArgs) error
	PublishVolumeIoMetrics([]*models.IoMetricDatum)
	PublishStorageIoMetrics([]*models.IoMetricDatum)
	RegisterConsumer(MetricConsumer) error
	RegisterHandlers(*operations.NuvolosoAPI, auth.AccessControl)
	StartTransmitter() error
	Status() *Status
	StopTransmitter()
}

// mover coordinates the transmission and reception of metric data.
// It must be configured to either transmit metric data to a remote receiver
// or to consume metric data locally.
type mover struct {
	Log           *logging.Logger
	mux           sync.Mutex
	consumer      MetricConsumer
	tx            *transmitter
	ac            auth.AccessControl
	vsIoPublished int
	vsIoReceived  int
	vsIoSent      int
	vsIoDropped   int
	sIoPublished  int
	sIoReceived   int
	sIoSent       int
	sIoDropped    int
}

var _ = MetricMover(&mover{})

// NewMetricMover returns a new mover
func NewMetricMover(log *logging.Logger) MetricMover {
	m := &mover{}
	m.Log = log
	return m
}

// RegisterConsumer provides an interface to consume incoming metric data.
func (m *mover) RegisterConsumer(mc MetricConsumer) error {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.consumer != nil {
		return fmt.Errorf("already configured")
	}
	m.consumer = mc
	m.Log.Debugf("MetricMover: Consumer registered")
	return nil
}

// ConfigureTransmitter enables the transmission of published metrics to an upstream receiver.
func (m *mover) ConfigureTransmitter(args *TxArgs) error {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.consumer != nil {
		return fmt.Errorf("already configured")
	}
	tx, err := newTransmitter(m, args)
	if err != nil {
		return err
	}
	m.tx = tx
	m.consumer = tx // internal consumer
	return nil
}

// StartTransmitter starts the transmitter.
func (m *mover) StartTransmitter() error {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.tx == nil {
		return fmt.Errorf("not configured")
	}
	m.tx.Start()
	m.Log.Debugf("MetricMover: Transmitter started")
	return nil
}

// StopTransmitter terminates the transmitter.
func (m *mover) StopTransmitter() {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.tx != nil {
		m.tx.Stop()
		m.Log.Debugf("MetricMover: Transmitter stopped")
	}
}

// PublishVolumeIoMetrics enqueues volume IO metric data for transmission.
// It will drop the metrics silently if there is no transmitter or local receiver.
func (m *mover) PublishVolumeIoMetrics(data []*models.IoMetricDatum) {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.consumer == nil {
		m.vsIoDropped += len(data)
		return
	}
	m.vsIoPublished += len(data)
	m.Log.Debugf("MetricMover: VsIO published %d", len(data))
	m.consumer.ConsumeVolumeIoMetricData(data)
}

// PublishStorageIoMetrics enqueues storage IO metric data for transmission.
// It will drop the metrics silently if there is no transmitter or local receiver.
func (m *mover) PublishStorageIoMetrics(data []*models.IoMetricDatum) {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.consumer == nil {
		m.sIoDropped += len(data)
		return
	}
	m.sIoPublished += len(data)
	m.Log.Debugf("MetricMover: VsIO published %d", len(data))
	m.consumer.ConsumeStorageIoMetricData(data)
}

func (m *mover) Status() *Status {
	s := &Status{}
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.consumer != nil {
		if m.tx != nil {
			s.TxConfigured = true
			s.TxStarted = m.tx.started
			s.VolumeIOBuffered = len(m.tx.vsIoQueue)
			s.StorageIOBuffered = len(m.tx.sIoQueue)
		} else {
			s.ConsumerConfigured = true
		}
		s.VolumeIOPublished = m.vsIoPublished
		s.VolumeIOSent = m.vsIoSent
		s.VolumeIOReceived = m.vsIoReceived
		s.StorageIOPublished = m.sIoPublished
		s.StorageIOSent = m.sIoSent
		s.StorageIOReceived = m.sIoReceived
	}
	s.VolumeIODropped = m.vsIoDropped
	s.StorageIODropped = m.sIoDropped
	return s
}
