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

func TestAttachingingSteps(t *testing.T) {
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

	// Invoke the attach steps in order (success cases last to fall through)
	now := time.Now()
	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR-1",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			ClusterID:           "cluster-1",
			PoolID:              "poolID",
			CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
			CspStorageType:      "Amazon gp2",
			RequestedOperations: []string{"PROVISION", "ATTACH"},
			MinSizeBytes:        swag.Int64(1000),
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "node-1",
			},
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "ATTACHING",
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
			PoolID:         "PROV-1",
			StorageAccessibility: &models.StorageAccessibility{
				StorageAccessibilityMutable: models.StorageAccessibilityMutable{
					AccessibilityScope:      "CSPDOMAIN",
					AccessibilityScopeObjID: "CSP-DOMAIN-1",
				},
			},
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:    swag.Int64(1000),
			StorageIdentifier: "vol-1",
			StorageState: &models.StorageStateMutable{
				AttachmentState:  "DETACHED",
				DeviceState:      "UNFORMATTED",
				ProvisionedState: "PROVISIONED",
			},
		},
	}
	// the attachOp
	op := &attachOp{
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
	assert.Equal(AttachTimedOut, op.getInitialState(ctx))
	op.rhs.TimedOut = false

	op.rhs.InError = true
	assert.Equal(AttachDone, op.getInitialState(ctx))
	op.rhs.InError = false

	s.StorageState.AttachmentState = "ATTACHING"
	assert.Equal(AttachSetStorageAttached, op.getInitialState(ctx))

	s.StorageState.AttachmentState = "ATTACHED"
	assert.Equal(AttachDone, op.getInitialState(ctx))

	s.StorageState.AttachmentState = "ERROR"
	assert.Equal(AttachStorageStart, op.getInitialState(ctx))

	s.StorageState.AttachmentState = "DETACHED"
	assert.Equal(AttachStorageStart, op.getInitialState(ctx))

	//  ***************************** setAttaching
	// failure in update storage
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	tl.Logger().Info("Case: setAttaching: fail in updateStorageObj")
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetUSErr = fmt.Errorf("storage update error")
	fc.RetUSObj = nil
	fc.RetUSRObj = sr
	fc.RetUSRErr = nil
	op.setAttaching(nil)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("update failure", sr.RequestMessages[0].Message)
	tl.Flush()
	// success
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	tl.Logger().Info("Case: setAttaching: success")
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetUSErr = nil
	fc.RetUSObj = s
	op.setAttaching(nil)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(sr.RequestMessages)
	assert.Equal("ATTACHING", s.StorageState.AttachmentState)
	assert.EqualValues("node-1", s.StorageState.AttachedNodeID)
	tl.Flush()

	//rei
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	c.rei.SetProperty("block-before-attach", &rei.Property{BoolValue: true})
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	op.setAttaching(nil)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("block-before-attach", sr.RequestMessages[0].Message)

	// ***************************** attachStorage
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	assert.NotNil(op.rhs.Pool) // set
	// rei block
	c.rei.SetProperty("block-in-attach", &rei.Property{BoolValue: true})
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	op.attachStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("block-in-attach", sr.RequestMessages[0].Message)

	// test loadNodeObj failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetNErr = fmt.Errorf("node fetch failure")
	fc.RetNObj = nil
	op.attachStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("node fetch failure", sr.RequestMessages[0].Message)
	// test GetDomainClient failure
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
	op.attachStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Domain client failure", sr.RequestMessages[0].Message)
	// volume attach failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	vaa := &csp.VolumeAttachArgs{
		VolumeIdentifier:       s.StorageIdentifier,
		NodeIdentifier:         node.NodeIdentifier,
		ProvisioningAttributes: node.NodeAttributes,
	}
	cspDC.EXPECT().VolumeAttach(ctx, vaa).Return(nil, fmt.Errorf("volume attach error"))
	op.c.App.AppCSP = appCSP
	op.attachStorage(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("Failed to attach", sr.RequestMessages[0].Message)
	// storage object update failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	vol := &csp.Volume{
		CSPDomainType: dom.CspDomainType,
		Identifier:    "vol-1",
		Attachments: []csp.VolumeAttachment{
			{
				NodeIdentifier: node.NodeIdentifier,
				Device:         "/dev/abc",
				State:          csp.VolumeAttachmentAttached,
			},
		},
	}
	cspDC.EXPECT().VolumeAttach(ctx, vaa).Return(vol, nil)
	fc.RetUSObj = nil
	fc.RetUSErr = fmt.Errorf("storage update error")
	op.c.App.AppCSP = appCSP
	op.attachStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Attached storage", sr.RequestMessages[0].Message)
	assert.Regexp("Failed to update Storage state", sr.RequestMessages[1].Message)
	// success case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	s.StorageState.AttachmentState = "ATTACHING"            // reset
	op.rhs.RetryLater = false                               // reset
	op.rhs.InError = false                                  // reset
	sr.RequestMessages = []*models.TimestampedString{}      // reset
	s.StorageState.Messages = []*models.TimestampedString{} // reset
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeAttach(ctx, vaa).Return(vol, nil)
	newS := &models.Storage{}
	*newS = *s // shallow copy
	newS.Meta = &models.ObjMeta{}
	*newS.Meta = *s.Meta // shallow copy
	newS.Meta.Version++
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.c.App.AppCSP = appCSP
	op.attachStorage(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newS, op.rhs.Storage) // updated object in po
	assert.Equal(newS.Meta.Version, op.rhs.Storage.Meta.Version)
	op.rhs.Storage = s // put the original object back
	assert.Equal("ATTACHED", s.StorageState.AttachmentState)
	assert.Equal(vol.Identifier, s.StorageIdentifier)
	assert.Regexp("Attached storage", sr.RequestMessages[0].Message)
	assert.Regexp("AttachmentState change: ATTACHING.*ATTACHED", s.StorageState.Messages[0].Message)

	//  ***************************** timedOutCleanup
	// no errors
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op.rhs.Storage = s
	op.rhs.Request.PoolID = s.PoolID
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.timedOutCleanup(ctx)
	assert.True(newS == op.rhs.Storage)
	// fail to update storage
	op.rhs.Storage = s
	op.rhs.Request.PoolID = s.PoolID
	fc.RetUSObj = nil
	fc.RetUSErr = fmt.Errorf("update storage error")
	op.timedOutCleanup(ctx)
	assert.True(s == op.rhs.Storage)
}

func TestAttachStorage(t *testing.T) {
	assert := assert.New(t)

	op := &fakeAttachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = AttachStorageStart
	expCalled := []string{"GIS", "SA", "AS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeAttachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = AttachTimedOut
	expCalled = []string{"GIS", "TOC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeAttachOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = AttachDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real code with an error

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
	rhs := &requestHandlerState{
		InError: true,
	}
	c.AttachStorage(nil, rhs)
}

type fakeAttachOps struct {
	attachOp
	called []string
	retGIS attachSubState
}

func (op *fakeAttachOps) getInitialState(ctx context.Context) attachSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeAttachOps) setAttaching(ctx context.Context) {
	op.called = append(op.called, "SA")
}

func (op *fakeAttachOps) attachStorage(ctx context.Context) {
	op.called = append(op.called, "AS")
}

func (op *fakeAttachOps) timedOutCleanup(ctx context.Context) {
	op.called = append(op.called, "TOC")
}
