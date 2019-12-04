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
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mount"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// AttachFs performs the ATTACH_FS operation
func (c *Component) AttachFs(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &attachFsOp{}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoAttachFs performs the DETACH_FS operation and undoes ATTACH_FS
func (c *Component) UndoAttachFs(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &attachFsOp{}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.run(ctx)
}

type attachFsSubState int

// attachFsSubState values (the order is meaningful)
const (
	// ATTACH_FS
	AttachFsPreMountCheck attachFsSubState = iota
	AttachFsDoMount
	AttachFsSetConditionInVolume
	AttachFsMountDone

	// DETACH_FS/UNDO
	AttachFsPreUmountCheck
	AttachFsPreMountNotifyNuvo // notify nuvo that an fs flush is likely
	AttachFsDoUnmount
	AttachFsPostMountNotifyNuvo // notify nuvo that fs flush is done
	AttachFsClearConditionInVolume
	AttachFsUnmountDone

	AttachFsError

	// LAST: No operation is performed in this state.
	AttachFsNoOp
)

func (ss attachFsSubState) String() string {
	switch ss {
	case AttachFsPreMountCheck:
		return "AttachFsPreMountCheck"
	case AttachFsDoMount:
		return "AttachFsDoMount"
	case AttachFsSetConditionInVolume:
		return "AttachFsSetConditionInVolume"
	case AttachFsMountDone:
		return "AttachFsMountDone"
	case AttachFsPreUmountCheck:
		return "AttachFsPreUmountCheck"
	case AttachFsPreMountNotifyNuvo:
		return "AttachFsPreMountNotifyNuvo"
	case AttachFsDoUnmount:
		return "AttachFsDoUnmount"
	case AttachFsPostMountNotifyNuvo:
		return "AttachFsPostMountNotifyNuvo"
	case AttachFsClearConditionInVolume:
		return "AttachFsClearConditionInVolume"
	case AttachFsUnmountDone:
		return "AttachFsUnmountDone"
	case AttachFsError:
		return "AttachFsError"
	}
	return fmt.Sprintf("attachFsSubState(%d)", ss)
}

type attachFsOp struct {
	baseOp
	ops        attachFsOperators
	headDevice string
	isMounted  bool
}

type attachFsOperators interface {
	checkIfFsMounted(ctx context.Context, flagError bool)
	clearVolumeFsTag(ctx context.Context)
	doMount(ctx context.Context)
	doUnmount(ctx context.Context)
	getInitialState(ctx context.Context) attachFsSubState
	nuvoPauseIO(ctx context.Context)  // from baseOp
	nuvoResumeIO(ctx context.Context) // from baseOp
	setVolumeFsTag(ctx context.Context)
}

func (op *attachFsOp) run(ctx context.Context) {
	jumpToState := AttachFsNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != AttachFsNoOp {
			ss = jumpToState
			jumpToState = AttachFsNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		// ATTACH_FS
		case AttachFsPreMountCheck:
			op.ops.checkIfFsMounted(ctx, false)
			if op.isMounted {
				jumpToState = AttachFsSetConditionInVolume
			}
		case AttachFsDoMount:
			op.ops.doMount(ctx)
		case AttachFsSetConditionInVolume:
			op.ops.setVolumeFsTag(ctx)

		// DETACH_FS/UNDO
		case AttachFsPreUmountCheck:
			op.ops.checkIfFsMounted(ctx, false)
			if !op.isMounted {
				jumpToState = AttachFsPostMountNotifyNuvo
			}
		case AttachFsPreMountNotifyNuvo:
			op.ops.nuvoPauseIO(ctx)
		case AttachFsDoUnmount:
			op.ops.doUnmount(ctx)
		case AttachFsPostMountNotifyNuvo:
			op.ops.nuvoResumeIO(ctx)
		case AttachFsClearConditionInVolume:
			op.ops.clearVolumeFsTag(ctx)

		default:
			break out
		}
	}
	if op.inError {
		op.rhs.InError = true
	}
}

func (op *attachFsOp) getInitialState(ctx context.Context) attachFsSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	foundHead := false
	for _, mnt := range op.rhs.VolumeSeries.Mounts {
		if mnt.SnapIdentifier == com.VolMountHeadIdentifier {
			foundHead = true
			op.headDevice = mnt.MountedNodeDevice
			break
		}
	}
	op.c.Log.Debugf("VolumeSeries %s: headDevice:%s", op.rhs.Request.Meta.ID, op.headDevice)
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoAttachingFs {
		op.inError = op.rhs.InError
		op.rhs.InError = false
		if !foundHead && !op.planOnly {
			return AttachFsClearConditionInVolume
		}
		return AttachFsPreUmountCheck
	}
	if !foundHead {
		op.rhs.SetRequestError("HEAD not mounted")
		return AttachFsError
	}
	return AttachFsPreMountCheck
}

func (op *attachFsOp) checkIfFsMounted(ctx context.Context, ignoreError bool) {
	args := op.getMountArgs()
	rc, err := op.c.mounter.IsFilesystemMounted(ctx, args)
	op.recordCommands(args.Commands)
	op.isMounted = false
	if err == nil {
		op.isMounted = rc
	} else if !ignoreError {
		op.rhs.SetRequestError("%s", err.Error())
	}
}

func (op *attachFsOp) clearVolumeFsTag(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Clear volume system tag")
		return
	}
	op.rhs.VSUpdater.RemoveVolumeSystemTags(ctx, fmt.Sprintf("%s:%s", com.SystemTagVolumeFsAttached, op.rhs.Request.TargetPath))
}

func (op *attachFsOp) doMount(ctx context.Context) {
	args := op.getMountArgs()
	err := op.c.mounter.MountFilesystem(ctx, args)
	op.recordCommands(args.Commands)
	if err != nil {
		op.rhs.SetRequestError("%s", err.Error())
	}
}

func (op *attachFsOp) doUnmount(ctx context.Context) {
	args := op.getUnmountArgs()
	err := op.c.mounter.UnmountFilesystem(ctx, args)
	op.recordCommands(args.Commands)
	if err != nil {
		op.rhs.SetRequestError("%s", err.Error())
	}
}

func (op *attachFsOp) setVolumeFsTag(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Set volume system tag")
		return
	}
	op.rhs.VSUpdater.SetVolumeSystemTags(ctx, fmt.Sprintf("%s:%s", com.SystemTagVolumeFsAttached, op.rhs.Request.TargetPath))
}

// Helpers

func (op *attachFsOp) getMountArgs() *mount.FilesystemMountArgs {
	req := op.rhs.Request
	if util.Contains(req.RequestedOperations, com.VolReqOpVolRestoreSnapshot) &&
		time.Time(req.CompleteByTime).Before(time.Now().Add(time.Minute*5)) {
		req.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Minute * 5)) // extend the timeout incase of previous long running ops
	}
	sPath := op.c.App.NuvoAPI.LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice)
	opts := []string{}
	if req.ReadOnly {
		opts = append(opts, "ro")
	}
	return &mount.FilesystemMountArgs{
		LogPrefix: fmt.Sprintf("VolumeSeriesRequest %s: Vol [%s]", req.Meta.ID, req.VolumeSeriesID),
		Source:    sPath,
		Target:    req.TargetPath,
		FsType:    req.FsType,
		Options:   opts,
		Deadline:  time.Time(req.CompleteByTime),
		PlanOnly:  op.planOnly,
		Commands:  &bytes.Buffer{},
	}
}

func (op *attachFsOp) getUnmountArgs() *mount.FilesystemUnmountArgs {
	req := op.rhs.Request
	return &mount.FilesystemUnmountArgs{
		LogPrefix: fmt.Sprintf("VolumeSeriesRequest %s: Vol [%s]", req.Meta.ID, req.VolumeSeriesID),
		Target:    req.TargetPath,
		Deadline:  time.Time(req.CompleteByTime),
		PlanOnly:  op.planOnly,
		Commands:  &bytes.Buffer{},
	}
}

func (op *attachFsOp) recordCommands(buf *bytes.Buffer) {
	for _, cmd := range strings.Split(buf.String(), "\n") {
		if len(cmd) == 0 {
			continue
		}
		op.rhs.SetRequestMessage("%s", cmd)
	}
}
