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


package heartbeat

import (
	"context"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

const detectorName = "NodeTerminationDetector"
const failedNodeDeleteVSRDetector = "FailedNodeDeleteVSRDetector"

type terminatedNodeHandler interface {
	handleTerminatedNodes(context.Context)
}

type termState struct {
	c        *HBComp
	wasReady bool
}

func newTerminatedNodeHandler(c *HBComp) terminatedNodeHandler {
	return &termState{c: c}
}

func (ts *termState) handleTerminatedNodes(ctx context.Context) {
	ts.detectTerminatedNodes(ctx)
	ts.relaunchFailedNodeDelete(ctx)
}

func (ts *termState) detectTerminatedNodes(ctx context.Context) {
	if !ts.c.app.IsReady() {
		if ts.wasReady {
			ts.wasReady = false
			ts.c.Log.Infof("%s: waiting for service to be ready", detectorName)
		}
		return
	} else if !ts.wasReady {
		ts.wasReady = true
		ts.c.Log.Infof("%s: starting detection of terminated nodes", detectorName)
	}
	nl := node.NewNodeListParams().WithClusterID(swag.String(string(ts.c.mObj.Meta.ID))).WithStateEQ(swag.String(com.NodeStateTimedOut))
	nodes, err := ts.c.app.OCrud.NodeList(ctx, nl)
	if err != nil {
		ts.c.Log.Warningf("%s: NodeList(%s,%s): %s", detectorName, ts.c.mObj.Meta.ID, com.NodeStateTimedOut, err.Error())
		return
	}

	for _, node := range nodes.Payload {
		// node.Name is mutable so use the name stashed in the NodeAttributes
		var nodeName string
		if nodeAttr, ok := node.NodeAttributes[csp.IMDHostname]; ok {
			nodeName = nodeAttr.Value
		} else {
			ts.c.Log.Debugf("%s: node %s[%s] has no nodeAttributes.%s, skipped", detectorName, node.Name, node.Meta.ID, csp.IMDHostname)
			continue
		}
		if _, err := ts.c.app.ClusterClient.NodeFetch(ctx, &cluster.NodeFetchArgs{Name: nodeName}); err != nil {
			if cErr, ok := err.(cluster.Error); ok && cErr.NotFound() {
				ts.c.Log.Infof("%s: cluster node %s has been deleted", detectorName, nodeName)
				ts.launchNodeDelete(ctx, node.Meta.ID, detectorName)
			} else {
				ts.c.Log.Warningf("%s: cluster.NodeFetch(%s): %s", detectorName, nodeName, err.Error())
			}
			continue
		}
		cfa := &cluster.ControllerFetchArgs{
			Kind:      ts.c.app.NodeControllerKind,
			Name:      ts.c.app.NodeControllerName,
			Namespace: ts.c.app.ClusterNamespace,
		}
		ctl, err := ts.c.app.ClusterClient.ControllerFetch(ctx, cfa)
		if err != nil {
			ts.c.Log.Warningf("%s: cluster.ControllerFetch(%s,%s,%s): %s", detectorName, cfa.Namespace, cfa.Kind, cfa.Name, err.Error())
			continue
		}
		if ctl.Replicas != ctl.ReadyReplicas {
			ts.c.Log.Infof("%s: %s %s not all replicas are ready, %d < %d", detectorName, cfa.Kind, cfa.Name, ctl.ReadyReplicas, ctl.Replicas)
			continue
		}
		pla := &cluster.PodListArgs{
			Namespace: ts.c.app.ClusterNamespace,
			NodeName:  nodeName,
			Labels:    ctl.Selectors,
		}
		pods, err := ts.c.app.ClusterClient.PodList(ctx, pla)
		if err != nil {
			ts.c.Log.Warningf("%s: cluster.PodList(%s,%s,%v): %s", detectorName, pla.Namespace, pla.NodeName, pla.Labels, err.Error())
			continue
		}
		if len(pods) == 0 {
			ts.c.Log.Infof("%s: no pod for %s %s found on cluster node %s", detectorName, cfa.Kind, cfa.Name, pla.NodeName)
			ts.launchNodeDelete(ctx, node.Meta.ID, detectorName)
		}
		ts.c.Log.Infof("%s: pod found for %s %s on cluster node %s", detectorName, cfa.Kind, cfa.Name, pla.NodeName)
	}
}

// list nodes in teardown state
// list terminated vsrs, with the same nodeID, that have NODE_DELETE op
// launch vsrs again.
func (ts *termState) relaunchFailedNodeDelete(ctx context.Context) {
	nl := node.NewNodeListParams().WithClusterID(swag.String(string(ts.c.mObj.Meta.ID))).WithStateEQ(swag.String(com.NodeStateTearDown))
	nodes, err := ts.c.app.OCrud.NodeList(ctx, nl)
	if err != nil {
		ts.c.Log.Warningf("%s: NodeList(%s,%s): %s", failedNodeDeleteVSRDetector, ts.c.mObj.Meta.ID, com.NodeStateTearDown, err.Error())
		return
	}
	for _, node := range nodes.Payload {
		vsrlParams := &volume_series_request.VolumeSeriesRequestListParams{
			NodeID:              swag.String(string(node.Meta.ID)),
			RequestedOperations: []string{com.VolReqOpNodeDelete},
			IsTerminated:        swag.Bool(true),
		}
		vsrl, err := ts.c.app.OCrud.VolumeSeriesRequestList(ctx, vsrlParams)
		if err != nil {
			ts.c.Log.Warningf("%s: failed VolumeSeriesRequestList(%s,%s) %s", failedNodeDeleteVSRDetector, com.VolReqOpNodeDelete, string(node.Meta.ID), err.Error())
			continue
		}
		if len(vsrl.Payload) > 0 {
			ts.launchNodeDelete(ctx, node.Meta.ID, failedNodeDeleteVSRDetector)
		}
	}
}

func (ts *termState) launchNodeDelete(ctx context.Context, id models.ObjID, caller string) {
	vsr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			CompleteByTime:      strfmt.DateTime(util.DateTimeMaxUpperBound()),
			RequestedOperations: []string{com.VolReqOpNodeDelete},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID: models.ObjIDMutable(id),
			},
		},
	}
	if _, err := ts.c.app.OCrud.VolumeSeriesRequestCreate(ctx, vsr); err != nil {
		ts.c.Log.Errorf("%s: VolumeSeriesRequestCreate(%s,%s): %s", caller, com.VolReqOpNodeDelete, id, err.Error())
		return
	}
	ts.c.Log.Infof("%s: launched VolumeSeriesRequestCreate(%s,%s)", caller, com.VolReqOpNodeDelete, vsr.NodeID)
}
