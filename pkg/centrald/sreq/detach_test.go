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


package sreq

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appmock "github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestDetachingingSteps(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.systemID = "SYSTEM"
	ctx := context.Background()

	// Invoke the detach steps in order (success cases last to fall through)
	now := time.Now()
	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR-1",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
			RequestedOperations: []string{"DETACH"},
			MinSizeBytes:        swag.Int64(1000),
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "node-1",
			},
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "DETACHING",
			},
		},
	}
	sp := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID:      "PROV-1",
				Version: 1,
			},
		},
	}
	dom := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CSP-DOMAIN-1",
				Version: 1,
			},
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "MyCSPDomain",
		},
	}
	node := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:      "node-1",
				Version: 1,
			},
			ClusterID:      "cluster-1",
			NodeIdentifier: "i-0123456789abcdef",
		},
		NodeMutable: models.NodeMutable{
			NodeAttributes: map[string]models.ValueType{},
		},
	}
	s := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:      "STORAGE-1",
				Version: 1,
			},
			CspDomainID:    "CSP-DOMAIN-1",
			CspStorageType: "Amazon gp2",
			SizeBytes:      swag.Int64(1000),
			StorageAccessibility: &models.StorageAccessibility{
				StorageAccessibilityMutable: models.StorageAccessibilityMutable{
					AccessibilityScope:      "CSPDOMAIN",
					AccessibilityScopeObjID: "CSP-DOMAIN-1",
				},
			},
			PoolID: "PROV-1",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:    swag.Int64(1000),
			StorageIdentifier: "vol-1",
			StorageState: &models.StorageStateMutable{
				AttachedNodeID:     "node-1",
				AttachedNodeDevice: "/dev/abc",
				AttachmentState:    "ATTACHED",
				ProvisionedState:   "PROVISIONED",
			},
		},
	}
	// the detachOp
	op := &detachOp{
		c: c,
		rhs: &requestHandlerState{
			c:         c,
			Request:   sr,
			CSPDomain: dom,
			Pool:      sp,
			Storage:   s,
		},
	}

	//  ***************************** getInitialState
	op.rhs.TimedOut = true
	assert.Equal(DetachCleanup, op.getInitialState(ctx))
	op.rhs.TimedOut = false

	op.rhs.InError = true
	assert.Equal(DetachDone, op.getInitialState(ctx))
	op.rhs.InError = false

	s.StorageState.AttachmentState = "DETACHING"
	assert.Equal(DetachCSPVolume, op.getInitialState(ctx))

	s.StorageState.AttachmentState = "DETACHED"
	assert.Equal(DetachDone, op.getInitialState(ctx))

	s.StorageState.AttachmentState = "ERROR"
	assert.Equal(DetachStorageStart, op.getInitialState(ctx))

	s.StorageState.AttachmentState = "ATTACHED"
	assert.Equal(DetachStorageStart, op.getInitialState(ctx))

	sr.StorageRequestState = "UNDO_ATTACHING"
	op.rhs.InError = true
	assert.Equal(DetachStorageStart, op.getInitialState(ctx))
	assert.True(op.wasInError)
	assert.False(op.rhs.InError)

	s.StorageState.AttachmentState = "ATTACHING"
	op.rhs.InError = true
	assert.Equal(DetachCSPVolume, op.getInitialState(ctx))
	assert.True(op.wasInError)
	assert.False(op.rhs.InError)

	sr.StorageRequestState = "DETACHING"

	//  ***************************** setDetaching
	// failure in update storage
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	tl.Logger().Info("Case: setDetaching: fail in updateStorageObj")
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetUSErr = fmt.Errorf("storage update error")
	fc.RetUSObj = nil
	fc.RetUSRErr = nil
	fc.RetUSRObj = sr
	op.setDetaching(nil)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("update failure", sr.RequestMessages[0].Message)
	tl.Flush()
	// success
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	tl.Logger().Info("Case: setDetaching: success")
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetUSErr = nil
	fc.RetUSObj = s
	op.setDetaching(nil)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(sr.RequestMessages)
	assert.Equal("DETACHING", s.StorageState.AttachmentState)
	tl.Flush()

	// ***************************** detachStorage

	// rei
	t.Log("detachStorage: REI")
	op.rhs.RetryLater = false
	c.rei.SetProperty("detach-storage-fail", &rei.Property{BoolValue: true})
	op.detachStorage(ctx)
	assert.True(op.rhs.RetryLater)

	// test loadNodeObj failure
	t.Log("detachStorage: nodeFetch failure")
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetNErr = fmt.Errorf("node fetch failure")
	fc.RetNObj = nil
	op.detachStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("node fetch failure", sr.RequestMessages[0].Message)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	assert.NotNil(op.rhs.Pool) // set

	// test GetDomainClient failure
	t.Log("detachStorage: GetDomainClient failure")
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetNErr = nil
	fc.RetNObj = node
	appCSP := appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC := mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(nil, fmt.Errorf("get domain client error"))
	assert.False(op.rhs.RetryLater)
	op.c.App.AppCSP = appCSP
	op.detachStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Domain client failure", sr.RequestMessages[0].Message)

	// volume detach failure
	t.Log("detachStorage: VolumeDetach failure")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	vda := &csp.VolumeDetachArgs{
		VolumeIdentifier:       s.StorageIdentifier,
		NodeIdentifier:         node.NodeIdentifier,
		ProvisioningAttributes: node.NodeAttributes,
	}
	cspDC.EXPECT().VolumeDetach(ctx, vda).Return(nil, fmt.Errorf("volume detach error"))
	op.c.App.AppCSP = appCSP
	op.detachStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Failed to detach", sr.RequestMessages[0].Message)

	// storage object update failure, cover use of force
	mockCtrl.Finish()
	t.Log("detachStorage: VolumeDetach with force")
	mockCtrl = gomock.NewController(t)
	op.rhs.RetryLater = false                // reset
	op.rhs.InError = false                   // reset
	s.StorageState.AttachedNodeID = "node-1" // reset
	s.StorageState.DeviceState = "OPEN"
	sr.RequestMessages = []*models.TimestampedString{} // reset
	sr.SystemTags = models.ObjTags{com.SystemTagForceDetachNodeID + ":node-1"}
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	vol := &csp.Volume{
		CSPDomainType: dom.CspDomainType,
		Identifier:    "vol-1",
		Attachments:   []csp.VolumeAttachment{},
	}
	vda.Force = true
	cspDC.EXPECT().VolumeDetach(ctx, vda).Return(vol, nil)
	op.c.App.AppCSP = appCSP
	op.detachStorage(ctx)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("Detached volume.*forced", sr.RequestMessages[0].Message)
	vda.Force = false // reset

	// success, already detached
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	s.StorageState.AttachedNodeID = "node-1"                // reset
	s.StorageState.AttachmentState = "DETACHING"            // reset
	s.StorageState.DeviceState = "ERROR"                    // not UNUSED
	op.rhs.RetryLater = false                               // reset
	op.rhs.InError = false                                  // reset
	sr.RequestMessages = []*models.TimestampedString{}      // reset
	sr.SystemTags = nil                                     // reset
	s.StorageState.Messages = []*models.TimestampedString{} // reset
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeDetach(ctx, vda).Return(nil, csp.ErrorVolumeNotAttached)
	newS := &models.Storage{}
	*newS = *s // shallow copy
	newS.Meta = &models.ObjMeta{}
	*newS.Meta = *s.Meta // shallow copy
	newS.Meta.Version++
	op.c.App.AppCSP = appCSP
	op.detachStorage(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// success case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	s.StorageState.AttachedNodeID = "node-1"                // reset
	s.StorageState.AttachmentState = "DETACHING"            // reset
	s.StorageState.DeviceState = "UNUSED"                   // reset
	op.rhs.RetryLater = false                               // reset
	op.rhs.InError = false                                  // reset
	sr.RequestMessages = []*models.TimestampedString{}      // reset
	s.StorageState.Messages = []*models.TimestampedString{} // reset
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeDetach(ctx, vda).Return(vol, nil)
	op.c.App.AppCSP = appCSP
	op.detachStorage(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	//  ***************************** setDetached

	t.Log("setDetached: update failure (with forced)")
	s.StorageState.AttachedNodeID = "node-1" // reset
	s.StorageState.DeviceState = "OPEN"
	sr.RequestMessages = []*models.TimestampedString{} // reset
	sr.SystemTags = models.ObjTags{com.SystemTagForceDetachNodeID + ":node-1"}
	fc.RetUSObj = nil
	fc.RetUSErr = fmt.Errorf("storage update error")
	op.setDetached(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Failed to update Storage state", sr.RequestMessages[0].Message)
	assert.Empty(s.StorageState.AttachedNodeID)
	assert.Equal("UNUSED", s.StorageState.DeviceState)

	t.Log("setDetached: update ok")
	s.StorageState.AttachedNodeID = "node-1"                // reset
	s.StorageState.AttachmentState = "DETACHING"            // reset
	s.StorageState.DeviceState = "UNUSED"                   // reset
	op.rhs.RetryLater = false                               // reset
	op.rhs.InError = false                                  // reset
	sr.RequestMessages = []*models.TimestampedString{}      // reset
	s.StorageState.Messages = []*models.TimestampedString{} // reset
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.setDetached(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	newS = &models.Storage{}
	*newS = *s // shallow copy
	newS.Meta = &models.ObjMeta{}
	*newS.Meta = *s.Meta // shallow copy
	newS.Meta.Version++
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	assert.Equal(newS, op.rhs.Storage) // updated object
	assert.Equal(newS.Meta.Version, op.rhs.Storage.Meta.Version)
	op.rhs.Storage = s // put the original object back
	assert.Empty(s.StorageState.AttachedNodeID)
	assert.Equal("DETACHED", s.StorageState.AttachmentState)
	assert.Equal(vol.Identifier, s.StorageIdentifier)
	assert.Len(sr.RequestMessages, 0)
	assert.Regexp("AttachmentState change: DETACHING.*DETACHED", s.StorageState.Messages[0].Message)

	//  ***************************** cleanup
	// no errors
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op.rhs.Storage = s
	op.rhs.Request.PoolID = s.PoolID
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.cleanup(ctx)
	assert.True(newS == op.rhs.Storage)
	// fail to update storage
	op.rhs.Storage = s
	op.rhs.Request.PoolID = s.PoolID
	fc.RetUSObj = nil
	fc.RetUSErr = fmt.Errorf("update storage error")
	op.cleanup(ctx)
	assert.True(s == op.rhs.Storage)
}

func TestDetachStorage(t *testing.T) {
	assert := assert.New(t)

	op := &fakeDetachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = DetachStorageStart
	expCalled := []string{"GIS", "SDG", "DS", "SDD"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeDetachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = DetachCleanup
	expCalled = []string{"GIS", "TOC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeDetachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = DetachDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// DetachStorage called to undo successful AttachStorage
	op = &fakeDetachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = DetachStorageStart
	op.wasInError = true
	expCalled = []string{"GIS", "SDG", "DS", "SDD"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)

	// DetachStorage called to undo failed AttachStorage
	op = &fakeDetachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = DetachCSPVolume
	op.wasInError = true
	expCalled = []string{"GIS", "DS", "SDD"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)

	// call the real handler with an error
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := &Component{}
	c.Init(app)
	req := &models.StorageRequest{}
	stg := &models.Storage{}
	stg.StorageState = &models.StorageStateMutable{}
	rhs := &requestHandlerState{
		InError: true,
		Request: req,
		Storage: stg,
	}
	c.DetachStorage(nil, rhs)
}

func TestUndoDetachStorage(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	// always calls cleanup
	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	fc.PassThroughUSObj = true
	rhs := &requestHandlerState{
		InError: true,
		Storage: &models.Storage{},
	}
	rhs.Storage.Meta = &models.ObjMeta{}
	rhs.Storage.StorageState = &models.StorageStateMutable{}
	c.UndoDetachStorage(nil, rhs)
	assert.Len(rhs.Storage.StorageState.Messages, 1)
}

type fakeDetachOps struct {
	detachOp
	called []string
	retGIS detachSubState
}

func (op *fakeDetachOps) getInitialState(ctx context.Context) detachSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeDetachOps) setDetached(ctx context.Context) {
	op.called = append(op.called, "SDD")
}

func (op *fakeDetachOps) setDetaching(ctx context.Context) {
	op.called = append(op.called, "SDG")
}

func (op *fakeDetachOps) detachStorage(ctx context.Context) {
	op.called = append(op.called, "DS")
}

func (op *fakeDetachOps) cleanup(ctx context.Context) {
	op.called = append(op.called, "TOC")
}
