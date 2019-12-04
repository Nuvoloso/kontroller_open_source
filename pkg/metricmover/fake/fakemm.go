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


package fake

import (
	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	mm "github.com/Nuvoloso/kontroller/pkg/metricmover"
)

// MetricMover is a fake
type MetricMover struct {
	// ConfigureTransmitter
	InCTArgs *mm.TxArgs
	RetCTErr error

	// PublishStorageIoMetrics
	CntPSIoM    int
	InPSIoMData []*models.IoMetricDatum

	// PublishVolumeIoMetrics
	CntPVIoM    int
	InPVIoMData []*models.IoMetricDatum

	// RegisterConsumer
	InRC     mm.MetricConsumer
	RetRCErr error

	// RegisterHandlers
	InRHApi *operations.NuvolosoAPI
	InAC    auth.AccessControl

	// StartTransmitter
	CntStartTCalled int
	RetStartTErr    error

	// Status
	RetStatus *mm.Status

	// StopTransmitter
	CntStopTCalled int
}

var _ = mm.MetricMover(&MetricMover{})

// ConfigureTransmitter fakes its namesake
func (m *MetricMover) ConfigureTransmitter(args *mm.TxArgs) error {
	m.InCTArgs = args
	return m.RetCTErr
}

// PublishStorageIoMetrics fakes its namesake
func (m *MetricMover) PublishStorageIoMetrics(data []*models.IoMetricDatum) {
	m.CntPSIoM++
	m.InPSIoMData = data
}

// PublishVolumeIoMetrics fakes its namesake
func (m *MetricMover) PublishVolumeIoMetrics(data []*models.IoMetricDatum) {
	m.CntPVIoM++
	m.InPVIoMData = data
}

// RegisterConsumer fakes its namesake
func (m *MetricMover) RegisterConsumer(c mm.MetricConsumer) error {
	m.InRC = c
	return m.RetRCErr
}

// RegisterHandlers fakes its namesake
func (m *MetricMover) RegisterHandlers(api *operations.NuvolosoAPI, ac auth.AccessControl) {
	m.InRHApi = api
	m.InAC = ac
}

// StartTransmitter fakes its namesake
func (m *MetricMover) StartTransmitter() error {
	m.CntStartTCalled++
	return m.RetStartTErr
}

// Status fakes its namesake
func (m *MetricMover) Status() *mm.Status {
	return m.RetStatus
}

// StopTransmitter fakes its namesake
func (m *MetricMover) StopTransmitter() {
	m.CntStopTCalled++
}
