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
	"fmt"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appAuth "github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	hk "github.com/Nuvoloso/kontroller/pkg/housekeeping"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

const chtTimeoutTaskMessage = "Cluster timeout task"

// clusterHeartbeatTask executes the consolidated heartbeat task created by clusterd
type clusterHeartbeatTask struct {
	c                  *HBComp
	ops                chtOps
	timeoutCheckPeriod time.Duration
	lastTimeOutCheck   time.Time
	clusterMaxMissed   int64
}

type chtOps interface {
	getTimedoutClusters(ctx context.Context, timeoutTime time.Time) ([]*models.Cluster, error)
	makeTimeoutTask(clObj *models.Cluster, timeoutTime time.Time) *models.Task
	sendNodeCrudEvent(clObj *models.Cluster, numNodeChanges, numNodeTimeouts int) error
	timeoutNodes(ctx context.Context, clObj *models.Cluster) (int, int, error)
	updateCluster(ctx context.Context, clID string, ss *models.ServiceState, isClusterTimeoutTask bool) (*models.Cluster, error)
	updateNodes(ctx context.Context, nIDs []string, ss *models.ServiceState) (int, int, error)
	reportRestartedNodesToAuditLog(ctx context.Context, clObj *models.Cluster, knownIDs []string)
	reportTimedoutNodesToAuditLog(ctx context.Context, clObj *models.Cluster)
}

func chtRegisterAnimator(c *HBComp) *clusterHeartbeatTask {
	if c.app.ClusterTimeoutAfterMisses <= 0 {
		c.app.ClusterTimeoutAfterMisses = 2
	}
	cht := &clusterHeartbeatTask{
		c:                  c,
		timeoutCheckPeriod: c.app.ClusterTimeoutCheckPeriod,
		clusterMaxMissed:   int64(c.app.ClusterTimeoutAfterMisses),
	}
	cht.ops = cht // self-reference
	c.app.TaskScheduler.RegisterAnimator(com.TaskClusterHeartbeat, cht)
	return cht
}

// TaskValidate is part of hk.TaskAnimator
func (cht *clusterHeartbeatTask) TaskValidate(args *models.TaskCreateOnce) error {
	if args.Operation != com.TaskClusterHeartbeat {
		return hk.ErrInvalidAnimator
	}
	if args.ObjectID == "" || len(args.ServiceStates) < 1 {
		return hk.ErrInvalidArguments
	}
	if _, found := args.ServiceStates[string(args.ObjectID)]; !found {
		return hk.ErrInvalidArguments
	}
	for _, ss := range args.ServiceStates {
		if ss.HeartbeatPeriodSecs <= 0 || ss.State == "" || time.Time(ss.HeartbeatTime).IsZero() {
			return hk.ErrInvalidArguments
		}
	}
	return nil
}

// TaskExec is part of hk.TaskAnimator
func (cht *clusterHeartbeatTask) TaskExec(ctx context.Context, tOps hk.TaskOps) {
	c := cht.c
	app := c.app
	o := tOps.Object()
	msgs := util.NewMsgList(o.Messages)
	// establish an existential lock across the updates
	if app.CrudHelpers == nil {
		c.Log.Errorf("Task %s: not ready", o.Meta.ID)
		msgs.Insert("Not ready")
		o.Messages = msgs.ToModel()
		tOps.SetState(hk.TaskStateFailed) // does not return
	}
	app.CrudHelpers.RLock()
	defer app.CrudHelpers.RUnlock()
	app.CrudHelpers.NodeLock()
	defer app.CrudHelpers.NodeUnlock()
	app.CrudHelpers.ClusterLock()
	defer app.CrudHelpers.ClusterUnlock()
	// is this a cluster timeout task?
	isClusterTimeoutTask := false
	if len(o.Messages) == 1 && o.Messages[0].Message == chtTimeoutTaskMessage {
		isClusterTimeoutTask = true
	}
	// update the cluster object
	clID := string(o.ObjectID)
	clSS := o.ServiceStates[clID]
	clObj, err := cht.ops.updateCluster(ctx, clID, &clSS, isClusterTimeoutTask)
	if err != nil {
		c.Log.Errorf("Task %s: cluster[%s] %s", o.Meta.ID, clID, err.Error())
		msgs.Insert("Failed to update cluster")
		o.Messages = msgs.ToModel()
		tOps.SetState(hk.TaskStateFailed) // does not return
	}
	c.Log.Debugf("Task %s: cluster[%s] state %s", o.Meta.ID, clID, clSS.State)
	msgs.Insert("Updated cluster")
	// organize nodes by state; save a representative node state for its temporal settings
	var nodeSS *models.ServiceState
	nodesByState := map[string][]string{}
	for nID, ss := range o.ServiceStates {
		if nID == clID {
			continue
		}
		if _, found := nodesByState[ss.State]; !found {
			nodesByState[ss.State] = make([]string, 0, 1)
		}
		nodesByState[ss.State] = append(nodesByState[ss.State], nID)
		if nodeSS == nil {
			nodeSS = &models.ServiceState{}
			*nodeSS = ss
		}
	}
	// update nodes in bulk
	numN, upN := 0, 0
	for state, nIDs := range nodesByState {
		ss := *nodeSS    // shallow copy with heartbeat pulse of nodes
		ss.State = state // override the state
		// check if any of the nodes have restarted, e.g. changing state to READY, then send audit log event
		c.Log.Debugf("State: %s, nodes: %s", state, strings.Join(nIDs, " ,"))
		if state == com.ServiceStateReady && len(nIDs) > 0 {
			cht.ops.reportRestartedNodesToAuditLog(ctx, clObj, nIDs)
		}
		chgN, foundN, err := cht.ops.updateNodes(ctx, nIDs, &ss)
		if err == nil {
			upN += chgN
			numN += foundN
		}
	}
	c.Log.Debugf("Updated %d/%d nodes", upN, numN)
	msgs.Insert("Updated %d/%d nodes", upN, numN)
	// timeout nodes older than the cluster
	toN := 0
	if chgN, foundN, err := cht.ops.timeoutNodes(ctx, clObj); err == nil && foundN > 0 {
		msgs.Insert("Timed out %d/%d nodes", chgN, foundN)
		toN = chgN
		cht.ops.reportTimedoutNodesToAuditLog(ctx, clObj)
	}
	o.Messages = msgs.ToModel()
	// send the summary node CRUDE
	cht.ops.sendNodeCrudEvent(clObj, upN, toN)
}

// TimeoutClusters is used to timeout clusters that have not reported their heartbeats
func (cht *clusterHeartbeatTask) TimeoutClusters(ctx context.Context) error {
	c := cht.c
	app := c.app
	timeoutTime := time.Now()
	if timeoutTime.Before(cht.lastTimeOutCheck.Add(cht.timeoutCheckPeriod)) {
		return nil
	}
	timedOut, err := cht.ops.getTimedoutClusters(ctx, timeoutTime)
	if err != nil {
		c.Log.Errorf("Failed to list clusters: %s", err.Error())
		return err
	}
	for _, cl := range timedOut {
		var tid string
		if tid, err = app.TaskScheduler.RunTask(cht.ops.makeTimeoutTask(cl, timeoutTime)); err != nil {
			c.Log.Errorf("Failed to launch task to time out cluster [%s]: %s", cl.Meta.ID, err.Error())
			break
		}
		c.Log.Debugf("Cluster [%s] timed out: %s", cl.Meta.ID, tid)
	}
	if err == nil {
		cht.lastTimeOutCheck = timeoutTime
	}
	return err
}

// update args to replace the service state fields
var chtServiceStateUpdatedFields = [centrald.NumActionTypes][]string{
	centrald.UpdateSet: []string{"service.state", "service.heartbeatTime", "service.heartbeatPeriodSecs"},
}
var chtClusterTimeoutStateChangeUpdateFields = [centrald.NumActionTypes][]string{
	centrald.UpdateSet: []string{"service.state", "service.heartbeatTime", "service.heartbeatPeriodSecs", "state", "messages"},
}

func (cht *clusterHeartbeatTask) updateCluster(ctx context.Context, clID string, ss *models.ServiceState, isClusterTimeoutTask bool) (*models.Cluster, error) {
	c := cht.c
	app := c.app
	clObj, err := app.DS.OpsCluster().Fetch(ctx, clID)
	if err != nil {
		return nil, fmt.Errorf("fetch: %s", err.Error())
	}
	if clObj.Service == nil {
		clObj.Service = &models.NuvoService{}
	}
	// check for race between cluster timeout and real heartbeat
	oss := &clObj.Service.ServiceState
	if isClusterTimeoutTask && time.Time(ss.HeartbeatTime).Before(cht.clusterTimeoutAfter(oss)) {
		return nil, fmt.Errorf("race between cluster timeout and last heartbeat") // abort
	}
	updateFields := chtServiceStateUpdatedFields
	if ss.State == com.ServiceStateUnknown && clObj.State == com.ClusterStateManaged {
		msgList := util.NewMsgList(clObj.Messages)
		msgList.Insert("State change: %s ⇒ %s", clObj.State, com.ClusterStateTimedOut)
		clObj.Messages = msgList.ToModel()
		clObj.State = com.ClusterStateTimedOut
		updateFields = chtClusterTimeoutStateChangeUpdateFields
	}
	oss.State = ss.State
	oss.HeartbeatPeriodSecs = ss.HeartbeatPeriodSecs
	oss.HeartbeatTime = ss.HeartbeatTime
	ver := int32(0)
	ua, err := app.CrudHelpers.MakeStdUpdateArgs(clObj, clID, &ver, updateFields)
	if err != nil {
		return nil, fmt.Errorf("cluster updateArgs: %s", err.Error())
	}
	var obj *models.Cluster
	if obj, err = app.DS.OpsCluster().Update(ctx, ua, &clObj.ClusterMutable); err != nil {
		return nil, fmt.Errorf("update: %s", err.Error())
	}
	// synthesize a CRUD event
	var uriArgs strings.Builder
	for _, f := range updateFields[centrald.UpdateSet] {
		uriArgs.WriteString("set=")
		uriArgs.WriteString(f)
		uriArgs.WriteString("&")
	}
	ce := &crude.CrudEvent{
		Method:             "PATCH",
		TrimmedURI:         fmt.Sprintf("/clusters/%s?%s"+"version=%d", clID, uriArgs.String(), clObj.Meta.Version),
		AccessControlScope: clObj,
	}
	app.CrudeOps.InjectEvent(ce)
	return obj, nil
}

func (cht *clusterHeartbeatTask) sendNodeCrudEvent(clObj *models.Cluster, numNodeChanges, numNodeTimeouts int) error {
	c := cht.c
	app := c.app
	clID := string(clObj.Meta.ID)
	// synthesize the summary node event if any node was updated
	var err error
	if numNodeChanges+numNodeTimeouts > 0 {
		ssc := false // node state changes happen directly so assume no change here
		if numNodeTimeouts > 0 {
			ssc = true // timeouts would indicate change
		}
		ce := &crude.CrudEvent{
			Method:     "PATCH",
			TrimmedURI: "/nodes/summary-heartbeat",
			Scope: map[string]string{
				"clusterId":           clID,
				"serviceStateChanged": fmt.Sprintf("%v", ssc),
			},
			AccessControlScope: clObj, // use the cluster object for access control
		}
		err = app.CrudeOps.InjectEvent(ce)
	}
	return err
}

var chtNodeStateUpdatedFields = [centrald.NumActionTypes][]string{
	centrald.UpdateSet: []string{"state", "service.state", "service.heartbeatTime", "service.heartbeatPeriodSecs"},
}

func (cht *clusterHeartbeatTask) updateNodes(ctx context.Context, nIDs []string, ss *models.ServiceState) (int, int, error) {
	c := cht.c
	app := c.app
	state := com.NodeStateManaged
	if ss.State == com.ServiceStateUnknown {
		state = com.NodeStateTimedOut
	}
	nObj := &models.Node{
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: *ss,
			},
			State: state,
		},
	}
	lP := node.NewNodeListParams()
	lP.NodeIds = nIDs
	lP.StateNE = swag.String(com.NodeStateTearDown)
	ver := int32(0)
	ua, err := app.CrudHelpers.MakeStdUpdateArgs(nObj, "", &ver, chtNodeStateUpdatedFields)
	if err != nil {
		return 0, 0, fmt.Errorf("updateNodes updateArgs: %s", err.Error())
	}
	return app.DS.OpsNode().UpdateMultiple(ctx, lP, ua, &nObj.NodeMutable)
}

var chtNodeTimeoutUpdateFields = [centrald.NumActionTypes][]string{
	centrald.UpdateSet: []string{"service.state", "state"},
}

func (cht *clusterHeartbeatTask) timeoutNodes(ctx context.Context, clObj *models.Cluster) (int, int, error) {
	c := cht.c
	app := c.app
	nObj := &models.Node{
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: com.ServiceStateUnknown,
				},
			},
			State: com.NodeStateTimedOut,
		},
	}
	lP := node.NewNodeListParams()
	lP.ClusterID = swag.String(string(clObj.Meta.ID))
	lP.ServiceStateNE = swag.String(com.ServiceStateUnknown)
	lP.StateNE = swag.String(com.NodeStateTearDown)
	timeOutTime := strfmt.DateTime(time.Time(clObj.Service.HeartbeatTime).Add(-time.Nanosecond)) // < clObj.Service.HeartbeatTime
	lP.ServiceHeartbeatTimeLE = &timeOutTime
	ver := int32(0)
	ua, err := app.CrudHelpers.MakeStdUpdateArgs(nObj, "", &ver, chtNodeTimeoutUpdateFields)
	if err != nil {
		return 0, 0, fmt.Errorf("timeoutNodes updateArgs: %s", err.Error())
	}
	return app.DS.OpsNode().UpdateMultiple(ctx, lP, ua, &nObj.NodeMutable)
}

// a cluster is considered to have timed out if it misses chtMaxMissedClusterHeartbeats
func (cht *clusterHeartbeatTask) clusterTimeoutAfter(ss *models.ServiceState) time.Time {
	return time.Time(ss.HeartbeatTime).Add(time.Duration(cht.clusterMaxMissed*ss.HeartbeatPeriodSecs) * time.Second)
}

func (cht *clusterHeartbeatTask) getTimedoutClusters(ctx context.Context, timeoutTime time.Time) ([]*models.Cluster, error) {
	app := cht.c.app
	lP := cluster.NewClusterListParams()
	lP.ServiceStateNE = swag.String(com.ServiceStateUnknown)
	ret, err := app.DS.OpsCluster().List(ctx, lP)
	if err != nil {
		return nil, err
	}
	timedOut := []*models.Cluster{}
	for _, cl := range ret {
		hasTimedOut := false
		if cl.Service == nil {
			hasTimedOut = true
			cl.Service = &models.NuvoService{}
		} else {
			if timeoutTime.After(cht.clusterTimeoutAfter(&cl.Service.ServiceState)) {
				hasTimedOut = true
			}
		}
		if hasTimedOut {
			timedOut = append(timedOut, cl)
		}
	}
	return timedOut, nil
}

func (cht *clusterHeartbeatTask) makeTimeoutTask(clObj *models.Cluster, timeoutTime time.Time) *models.Task {
	clID := string(clObj.Meta.ID)
	ss := &clObj.Service.ServiceState
	hbt := strfmt.DateTime(timeoutTime)
	task := &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: com.TaskClusterHeartbeat,
			ObjectID:  models.ObjIDMutable(clID),
			ServiceStates: map[string]models.ServiceState{
				clID: models.ServiceState{
					HeartbeatPeriodSecs: ss.HeartbeatPeriodSecs,
					HeartbeatTime:       hbt, // will be set in Cluster and used to timeout Nodes
					State:               com.ServiceStateUnknown,
				},
			},
		},
	}
	msgs := util.NewMsgList(nil)
	msgs.Insert(chtTimeoutTaskMessage)
	task.Messages = msgs.ToModel()
	return task
}

func (cht *clusterHeartbeatTask) reportTimedoutNodesToAuditLog(ctx context.Context, clObj *models.Cluster) {
	lP := node.NewNodeListParams()
	lP.ClusterID = swag.String(string(clObj.Meta.ID))
	lP.ServiceStateEQ = swag.String(com.ServiceStateUnknown)
	timeOutTime := strfmt.DateTime(time.Time(clObj.Service.HeartbeatTime).Add(-time.Nanosecond)) // < clObj.Service.HeartbeatTime
	lP.ServiceHeartbeatTimeLE = &timeOutTime
	nodes, err := cht.c.app.DS.OpsNode().List(ctx, lP)
	if err != nil {
		cht.c.Log.Debugf("Failure to list cluster nodes: %s", err.Error())
		return
	}
	for _, nObj := range nodes {
		if (time.Time(nObj.Meta.TimeModified)).After(time.Time(clObj.Service.HeartbeatTime)) { // consider only recently updated nodes
			cht.c.app.AuditLog.Event(ctx, &appAuth.Info{AccountID: string(clObj.AccountID)}, centrald.NodeUpdateAction, nObj.Meta.ID, nObj.Name, "", false, "Stopped heartbeat")
		}
	}

}

func (cht *clusterHeartbeatTask) reportRestartedNodesToAuditLog(ctx context.Context, clObj *models.Cluster, knownIDs []string) {
	lP := node.NewNodeListParams()
	lP.NodeIds = knownIDs
	lP.ServiceStateNE = swag.String(com.ServiceStateReady)
	nodes, err := cht.c.app.DS.OpsNode().List(ctx, lP)
	if err != nil {
		cht.c.Log.Debugf("Failure to list nodes: %s", err.Error())
		return
	}
	for _, nObj := range nodes {
		cht.c.app.AuditLog.Event(ctx, &appAuth.Info{AccountID: string(clObj.AccountID)}, centrald.NodeUpdateAction, nObj.Meta.ID, nObj.Name, "", false, "Started heartbeat")
	}
}
