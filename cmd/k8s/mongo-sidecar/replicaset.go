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


package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

// See https://github.com/mongodb/mongo/blob/master/src/mongo/base/error_codes.err for the full list
const (
	rsOK                    int = 0
	invalidReplicaSetConfig     = 93
	notYetInitialized           = 94
)

// DefaultMongoDBPort is the default port for mongod
const DefaultMongoDBPort uint16 = 27017

// RSInitiateResult serves to decode the result of the replSetInitiate command.
// Only needed fields are decoded
type RSInitiateResult struct {
	Code     int    // error code if !Ok
	CodeName string `bson:"codeName"`
	ErrMsg   string // error message if !Ok
	Ok       int    // 1: command succeeded, 0: command failed
}

// RSStatusResult serves to decode the result of the replSetGetStatus command.
// Only needed fields are decoded
type RSStatusResult struct {
	Code     int    // error code if !Ok
	CodeName string `bson:"codeName"`
	ErrMsg   string // error message if !Ok
	Ok       int    // 1: command succeeded, 0: command failed

	Set     string // replica set name
	Members []MemberStatus
}

// MemberStatus serves to decode the state of a single member of the replica set.
// Only a subset of fields are currently decoded.
// See https://docs.mongodb.com/manual/reference/command/replSetGetStatus/#rs-status-output
type MemberStatus struct {
	ID       int    `bson:"_id"`
	Name     string // eg. "hostname:27017"
	Health   int
	Self     bool
	State    int
	StateStr string `bson:"stateStr"`
}

// CheckReplicaSet checks if a replica set has been configured. If not and this sidecar is elected, initiates the replica set.
// Only fatal errors are returned. Other errors treated as transient and operations will be retried.
func (app *mainContext) CheckReplicaSet() error {
	// use the retry backoff requested in the mongo args
	var delay time.Duration
	for {
		if app.db.MustBeReady() != nil {
			app.log.Info("Waiting for", app.MongoArgs.URL)
			for app.db.MustBeReady() != nil {
				if delay < app.MongoArgs.MaxRetryInterval {
					delay += app.MongoArgs.RetryIncrement
				}
				time.Sleep(delay)
			}
			delay = 0
		}
		ctx := context.Background() // timeout is configured in the DB client
		sr := app.db.Client().Database(AdminDBName).RunCommand(ctx, bson.M{"replSetGetStatus": 1})
		result := &RSStatusResult{}
		err := sr.Decode(result)
		if cmdErr, ok := err.(mongo.CommandError); ok && cmdErr.Code == notYetInitialized {
			err = nil
			if app.elected() {
				app.log.Info("Elected to initiate the replica set")
				if app.ops.InitiateReplicaSet() == nil { // initializeReplicaSet logs errors in context
					app.log.Info("Replica set has been initiated")
					delay = 0
				}
			} else {
				app.log.Info("Not elected to initialize the replica set, waiting...")
			}
		} else if err != nil {
			app.log.Error("RunCommand replSetGetStatus", err)
		} else {
			b, _ := json.MarshalIndent(result, "", "  ")
			app.log.Debug("replSetGetStatus result", string(b))
			app.db.ErrorCode(err)
			app.log.Info("Replica set is configured")
			break
		}
		app.db.ErrorCode(err)
		if delay < app.MongoArgs.MaxRetryInterval {
			delay += app.MongoArgs.RetryIncrement
		}
		time.Sleep(delay)
	}
	return nil
}

// elected picks one of the sidecar replicas to initialize mongo. Since this is in a StatefulSet, just pick the 0th replica
func (app *mainContext) elected() bool {
	return strings.HasSuffix(app.PodName, "-0")
}

// InitiateReplicaSet queries k8s to find the number of replicas in the StatefulSet, builds the replica set configuration
// document for the mongo pods in the StatefulSet and applies this configuration to the the local mongod.
func (app *mainContext) InitiateReplicaSet() error {
	var pod *cluster.PodObj
	var err error
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(app.ClientTimeout))
		defer cancel()
		pod, err = app.clusterClient.PodFetch(ctx, &cluster.PodFetchArgs{Name: app.PodName, Namespace: app.Namespace})
	}()
	if err != nil {
		app.log.Errorf("PodFetch(%s, %s): %s", app.PodName, app.Namespace, err)
		return err
	} else if pod.ControllerName == "" || pod.ControllerKind != string(cluster.StatefulSet) {
		err = fmt.Errorf("pod %s is not in a %s", app.PodName, cluster.StatefulSet)
		app.log.Errorf("PodFetch(%s, %s): %s", app.PodName, app.Namespace, err)
		return err
	}

	var ss *cluster.ControllerObj
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(app.ClientTimeout))
		defer cancel()
		ss, err = app.clusterClient.ControllerFetch(ctx, &cluster.ControllerFetchArgs{Kind: pod.ControllerKind, Name: pod.ControllerName, Namespace: pod.Namespace})
	}()
	if err != nil {
		app.log.Errorf("ControllerFetch(%s, %s, %s): %s", pod.ControllerKind, pod.ControllerName, pod.Namespace, err)
		return err
	} else if !(ss.Replicas == 1 || ss.Replicas == 3) {
		// TBD k8s event for this condition
		err = fmt.Errorf("%s %s replicas must be 1 or 3, scale up/down required", cluster.StatefulSet, ss.Name)
		app.log.Errorf("ControllerFetch(%s, %s, %s): %s", pod.ControllerKind, pod.ControllerName, pod.Namespace, err)
		return err
	} else if ss.Replicas != ss.ReadyReplicas {
		err = fmt.Errorf("some replicas are not ready")
		app.log.Infof("ControllerFetch(%s, %s, %s): %s", pod.ControllerKind, pod.ControllerName, pod.Namespace, err)
		return err
	}

	// because this is a StatefulSet all mongod must be listening on the same port. Check our bootstrap URL for a port and use it if specified
	mongoPort := DefaultMongoDBPort
	cs, _ := connstring.Parse(app.MongoArgs.URL) // cannot fail, must have already succeeded when starting mongo client
	_, mongoPortStr, err := net.SplitHostPort(cs.Hosts[0])
	// The only error possible (same reason) is that the host does not include a port
	if err == nil {
		port, _ := strconv.ParseUint(mongoPortStr, 10, 16) // cannot fail
		mongoPort = uint16(port)
	}

	members := bson.A{}
	for id := 0; id < int(ss.Replicas); id++ {
		// use the stable network IDs of the pods in the StatefulSet
		hostPort := fmt.Sprintf("%s-%d.%s.%s.svc.%s:%d", ss.Name, id, ss.ServiceName, ss.Namespace, app.ClusterDNSName, mongoPort)
		members = append(members, bson.M{"_id": id, "host": hostPort})
	}
	cmd := bson.M{"replSetInitiate": bson.M{"_id": app.ReplicaSetName, "members": members}}
	b, _ := bson.MarshalExtJSON(cmd, true, false)
	doc := strings.TrimSpace(string(b))
	app.log.Debugf("RunCommand(%s)", doc)
	ctx := context.Background() // timeout is configured in the DB client
	sr := app.db.Client().Database(AdminDBName).RunCommand(ctx, cmd)
	result := &RSInitiateResult{}
	err = sr.Decode(result)
	app.db.ErrorCode(err)
	if err != nil {
		app.log.Errorf("RunCommand(%s).Decode(): %s", doc, err)
		return err
	}
	b, _ = json.MarshalIndent(result, "", "  ")
	app.log.Debug("replSetInitiate result", string(b))
	app.log.Info("Replica set successfully initiated")
	return nil
}

// Monitor monitors the replica set state.
// TBD actually monitor mongo replica set state, generate k8s events, auto-repair if possible
func (app *mainContext) Monitor() {
	app.log.Info("Blocking until terminated")
	<-app.stopMonitor
	app.log.Info("Monitor stopped")
}
