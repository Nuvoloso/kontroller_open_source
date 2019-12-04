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


package vreq

import (
	"context"

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/vra"
)

// Operation base type with common helper methods
type baseOp struct {
	c        *Component
	rhs      *vra.RequestHandlerState
	inError  bool
	planOnly bool
}

// nuvoPauseIO issues a PauseIo to nuvo
func (op *baseOp) nuvoPauseIO(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Pause IO")
		return
	}
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI PauseIo(%s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier)
	err := op.c.App.NuvoAPI.PauseIo(nuvoVolumeIdentifier)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI PauseIo temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		op.rhs.SetRequestError("NUVOAPI PauseIo(%s) failed: %s", nuvoVolumeIdentifier, err.Error())
	}
}

// nuvoResumeIO issues a ResumeIo to nuvo
func (op *baseOp) nuvoResumeIO(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Resume IO")
		return
	}
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI ResumeIo(%s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier)
	err := op.c.App.NuvoAPI.ResumeIo(nuvoVolumeIdentifier)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI ResumeIo temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		op.rhs.SetRequestError("NUVOAPI ResumeIo(%s) failed: %s", nuvoVolumeIdentifier, err.Error())
	} else {
		op.rhs.SetRequestMessage("IO Resumed")
	}
}
