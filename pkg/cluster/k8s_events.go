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


package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

var k8sIncidentMap = map[IncidentSeverity]k8sIncident{
	IncidentNormal:  k8sIncident{Type: "Normal", Reason: "NuvolosoNormalIncident"},
	IncidentWarning: k8sIncident{Type: "Warning", Reason: "NuvolosoWarning"},
	IncidentFatal:   k8sIncident{Type: "Fatal", Reason: "NuvolosoFatalIncident"},
}

type k8sIncident struct {
	Type   string
	Reason string
}

// RecordIncident performs a POST operation to create an Incident Event
func (c *K8s) RecordIncident(ctx context.Context, inc *Incident) (*EventObj, error) {
	err := inc.Validate()
	if err != nil {
		return nil, err
	}
	eca := &eventCreateArgs{
		NamePrefix:     os.Getenv(NuvoEnvPodName) + ".",
		Type:           k8sIncidentMap[inc.Severity].Type,
		Message:        inc.Message,
		Count:          1,
		Reason:         k8sIncidentMap[inc.Severity].Reason,
		FirstTimeStamp: time.Now(),
		LastTimeStamp:  time.Now(),
	}
	return c.eventCreate(ctx, eca)
}

var k8sConditionMap = map[ConditionStatus]k8sCondition{
	ServiceReady: k8sCondition{
		Name:    "ServiceReady",
		Type:    "Info",
		Reason:  "ServiceReady",
		Message: "Service is ready",
	},
	ServiceNotReady: k8sCondition{
		Name:    "ServiceNotReady",
		Type:    "Info",
		Reason:  "ServiceNotReady",
		Message: "Service is not ready",
	},
}

type k8sCondition struct {
	Name    string
	Type    string
	Reason  string
	Message string
}

func (c *K8s) generateConditionName(status ConditionStatus) string {
	return os.Getenv(NuvoEnvPodName) + "." + k8sConditionMap[status].Name
}

// RecordCondition checks to see if a condition is repeated. If it is it performs an
// update on an existing event it will fail.
func (c *K8s) RecordCondition(ctx context.Context, con *Condition) (*EventObj, error) {
	err := con.Validate()
	if err != nil {
		return nil, err
	}
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.conditionUpdateRequired(con) {
		return nil, nil
	}
	conditionCount, _ := c.getEventCount(ctx, con)
	eca := &eventCreateArgs{
		Name:          c.generateConditionName(con.Status),
		Type:          k8sConditionMap[con.Status].Type,
		Message:       k8sConditionMap[con.Status].Message,
		Reason:        k8sConditionMap[con.Status].Reason,
		LastTimeStamp: time.Now(),
		Count:         conditionCount + 1,
	}
	if conditionCount == 0 {
		eca.FirstTimeStamp = time.Now()
	}
	return c.eventUpdate(ctx, eca)
}

func (c *K8s) conditionUpdateRequired(con *Condition) bool {
	if c.conditionTimeMap == nil {
		c.conditionTimeMap = make(map[ConditionStatus]time.Time)
	}
	if c.conditionRefreshPeriod == 0 {
		c.conditionRefreshPeriod, _ = time.ParseDuration(K8sConditionRefreshPeriod)
	}
	if conditionTime, ok := c.conditionTimeMap[con.Status]; !ok {
		if con.Status == ServiceReady {
			delete(c.conditionTimeMap, ServiceNotReady)
		} else if con.Status == ServiceNotReady {
			delete(c.conditionTimeMap, ServiceReady)
		}
	} else {
		if time.Since(conditionTime) < c.conditionRefreshPeriod {
			return false
		}
	}
	c.conditionTimeMap[con.Status] = time.Now()
	return true
}

func (c *K8s) getEventCount(ctx context.Context, con *Condition) (int32, error) {
	efa := &eventFetchArgs{
		Name:      c.generateConditionName(con.Status),
		Namespace: os.Getenv(NuvoEnvPodNamespace),
	}
	eventRes, err := c.eventFetch(ctx, efa)
	if err == nil {
		return eventRes.Count, nil
	}
	return 0, err
}

func (c *K8s) eventFetch(ctx context.Context, efa *eventFetchArgs) (*EventObj, error) {
	var ev *K8sEvent
	url := fmt.Sprintf("/api/v1/namespaces/%s/events/%s", efa.Namespace, efa.Name)
	err := c.K8sClientGetJSON(ctx, url, &ev)
	if err != nil {
		return nil, err
	}
	var res *EventObj
	res = ev.ToModel()
	return res, nil
}

func (c *K8s) eventCreate(ctx context.Context, eca *eventCreateArgs) (*EventObj, error) {
	var evBody eventPostBody
	evBody.Init(eca)
	var ev *K8sEvent
	url := fmt.Sprintf("/api/v1/namespaces/%s/events", os.Getenv(NuvoEnvPodNamespace))
	err := c.K8sClientPostJSON(ctx, url, evBody.Marshal(), &ev)
	if err != nil {
		return nil, err
	}
	var res *EventObj
	res = ev.ToModel()
	return res, nil
}

func (c *K8s) eventUpdate(ctx context.Context, eca *eventCreateArgs) (*EventObj, error) {
	var evBody eventPostBody
	evBody.Init(eca)
	var ev *K8sEvent
	url := fmt.Sprintf("/api/v1/namespaces/%s/events/%s", os.Getenv(NuvoEnvPodNamespace), eca.Name)
	err := c.K8sClientPutJSON(ctx, url, evBody.Marshal(), &ev)
	if err != nil {
		return nil, err
	}
	var res *EventObj
	res = ev.ToModel()
	return res, nil
}

// ToModel converts a K8sEvent object to a EventObj
func (ev *K8sEvent) ToModel() *EventObj {
	evObj := &EventObj{
		Message:       ev.Message,
		Name:          ev.Name,
		Type:          ev.Type,
		LastTimeStamp: ev.LastTimestamp.String(),
		Reason:        ev.Reason,
		Count:         ev.Count,
		Raw:           ev,
	}
	return evObj
}

type eventCreateArgs struct {
	Name           string
	NamePrefix     string
	Type           string
	Reason         string
	Message        string
	Count          int32
	FirstTimeStamp time.Time
	LastTimeStamp  time.Time
}

type eventFetchArgs struct {
	Name      string
	Namespace string
}

// eventPostBody describes a Body for post operations
type eventPostBody struct {
	APIVersion     string           `json:"apiVersion"`
	Kind           string           `json:"kind"`
	Metadata       *ObjectMeta      `json:"metadata"`
	InvolvedObject *ObjectReference `json:"involvedObject"`
	Reason         string           `json:"reason,omitempty"`
	Message        string           `json:"message,omitempty"`
	FirstTimeStamp time.Time        `json:"firstTimestamp,omitempty"`
	LastTimestamp  time.Time        `json:"lastTimestamp,omitempty"`
	Count          int32            `json:"count,omitempty"`
	Source         *EventSource     `json:"source,omitempty"`
	Type           string           `json:"type,omitempty"`
}

func (evpb *eventPostBody) Init(eva *eventCreateArgs) {
	namespace := os.Getenv(NuvoEnvPodNamespace)
	evpb.APIVersion = K8sAPIVersion
	evpb.Kind = "Event"
	evpb.Reason = eva.Reason
	evpb.Type = eva.Type
	evpb.Message = eva.Message
	evpb.FirstTimeStamp = eva.FirstTimeStamp
	evpb.LastTimestamp = eva.LastTimeStamp
	evpb.Metadata = &ObjectMeta{
		Name:         eva.Name,
		GenerateName: eva.NamePrefix,
		Namespace:    namespace,
	}
	evpb.InvolvedObject = &ObjectReference{
		Kind:       "Pod",
		APIVersion: K8sAPIVersion,
		Name:       os.Getenv(NuvoEnvPodName),
		Namespace:  namespace,
		UID:        os.Getenv(NuvoEnvPodUID),
	}
	evpb.Source = &EventSource{
		Component: "Nuvoloso",
		Host:      os.Getenv(NuvoEnvNodeName),
	}
	evpb.Count = eva.Count
}

func (evpb *eventPostBody) Marshal() []byte {
	body, _ := json.Marshal(evpb) // strict input type so Marshal won't fail
	return body
}
