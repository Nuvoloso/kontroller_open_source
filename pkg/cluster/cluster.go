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
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
)

// TBD: move cluster methods from centrald to here

// Client is an abstraction of a cluster service client
type Client interface {
	Type() string
	SetDebugLogger(log *logging.Logger)
	SetTimeout(timeSec int)
	MetaData(ctx context.Context) (map[string]string, error)
	ValidateClusterObjID(ctx context.Context, url, formatType string) (string, error)
	Controller
	Node
	PersistentVolume
	Pod
	Secrets
	Services
	ServicePlan
	Recorder
}

// These properties are returned in the metadata.
const (
	CMDIdentifier     = "ClusterIdentifier"
	CMDVersion        = "ClusterVersion"
	CMDServiceIP      = "ClusterServiceIP"
	CMDServiceLocator = "ClusterServiceLocator"
)

var clusterRegistry = make(map[string]Client)

// NewClient returns a Client interface
func NewClient(clusterType string) (Client, error) {
	cc, ok := clusterRegistry[clusterType]
	if !ok {
		return nil, fmt.Errorf("invalid cluster type \"%s\"", clusterType)
	}
	return cc, nil
}

// SupportedClusterTypes returns the names of the supported cluster types
func SupportedClusterTypes() []string {
	cts := make([]string, 0, len(clusterRegistry))
	for n := range clusterRegistry {
		cts = append(cts, n)
	}
	return cts
}

// PersistentVolume provides the persistent volume Support
type PersistentVolume interface {
	CreatePublishedPersistentVolumeWatcher(ctx context.Context, args *PVWatcherArgs) (PVWatcher, []*PersistentVolumeObj, error)
	GetPVSpec(ctx context.Context, args *PVSpecArgs) (*PVSpec, error)
	GetDriverTypes() []string
	GetFileSystemTypes() []string
	PersistentVolumeList(ctx context.Context) ([]*PersistentVolumeObj, error)
	PersistentVolumeCreate(ctx context.Context, pvca *PersistentVolumeCreateArgs) (*PersistentVolumeObj, error)
	PersistentVolumeDelete(ctx context.Context, pvda *PersistentVolumeDeleteArgs) (*PersistentVolumeObj, error)
	PersistentVolumePublish(ctx context.Context, pvca *PersistentVolumeCreateArgs) (models.ClusterDescriptor, error)
	PublishedPersistentVolumeList(ctx context.Context) ([]*PersistentVolumeObj, error)
}

// PVWatcher implements the K8sWatcher for Persistent Volumes
// The watcher may terminate if the watch window expires. Caller should periodically check IsActive and restart
type PVWatcher interface {
	Start(context.Context) error
	Stop() // finalizes watcher
	IsActive() bool
}

// PVWatcherArgs describes the args to pvwatcher
type PVWatcherArgs struct {
	Timeout time.Duration
	Ops     PVWatcherOps
}

// Validate verifies the arguments passed to PVWatcher
func (pvwa *PVWatcherArgs) Validate() error {
	if pvwa.Ops == nil || pvwa.Timeout < 0 {
		return fmt.Errorf("invalid pvWatcher arguments")
	}
	return nil
}

// PVWatcherOps describes the additional ops for a PV Watcher
type PVWatcherOps interface {
	PVDeleted(*PersistentVolumeObj) error
}

// Recorder is an interface for creating historical records in a Container Orchestrator
type Recorder interface {
	RecordIncident(ctx context.Context, inc *Incident) (*EventObj, error)
	RecordCondition(ctx context.Context, con *Condition) (*EventObj, error)
}

// ServicePlan is an interface for creating a nuvo servicePlan representation in a Container Orchestrator
type ServicePlan interface {
	ServicePlanPublish(ctx context.Context, sp *models.ServicePlan) (models.ClusterDescriptor, error)
}

// PVSpecArgs contains the properties required to create a PV yaml
// ClusterVersion will be set to default value of PVSpecDefaultVersion if not specified
// FsType will be set to default value of PVSpecDefaultFsType if not specified
type PVSpecArgs struct {
	AccountID      string
	ClusterVersion string
	VolumeID       string
	SystemID       string
	FsType         string
	Capacity       string
}

// Default values for PVSpecArgs
const (
	PVSpecDefaultVersion = "v1"
	PVSpecDefaultFsType  = "ext4"
)

// PVSpec returns text containing a persistent volume definition
type PVSpec struct {
	PvSpec string
	Format string
}

// PersistentVolumeObj is a container orchestrator neutral structure to return Persistent Volume data
// Name - The Name that identifies a Persistent volume
// UID - The unique ID that identifies a Persistent volume
// Capacity - the String representation of the capacity of a Persistent volume.
//            In Kubernetes an example would look like "10Gi"
// Raw - The raw representation of persistent volume obj, would vary depending on the CO
type PersistentVolumeObj struct {
	Name           string
	UID            string
	Capacity       string
	VolumeSeriesID string
	Raw            interface{}
}

// PersistentVolumeCreateArgs contains the parameters for PersistentVolumeCreate
type PersistentVolumeCreateArgs struct {
	SizeBytes       int64
	VolumeID        string
	FsType          string
	AccountID       string
	SystemID        string
	DriverType      string
	ServicePlanName string
}

// Validate verifies the arguments required to create a persistent volume
func (p *PersistentVolumeCreateArgs) Validate(o PersistentVolume) error {
	if p.SizeBytes <= 0 || p.VolumeID == "" || p.AccountID == "" || p.SystemID == "" || p.ServicePlanName == "" {
		return fmt.Errorf("invalid arguments")
	}
	fsTypes := o.GetFileSystemTypes()
	if p.FsType == "" {
		p.FsType = fsTypes[0] // Default fs type
	} else if !util.Contains(fsTypes, p.FsType) {
		return fmt.Errorf("invalid arguments")
	}
	driverTypes := o.GetDriverTypes()
	if p.DriverType == "" {
		p.DriverType = driverTypes[0] // Default driver type
	} else if !util.Contains(driverTypes, p.DriverType) {
		return fmt.Errorf("invalid arguments")
	}
	return nil
}

// PersistentVolumeDeleteArgs contains the parameters for PersistentVolumeDelete
type PersistentVolumeDeleteArgs struct {
	VolumeID string
}

// EventObj is a container orchestrator neutral structure to return event information
type EventObj struct {
	Message       string
	Name          string
	Type          string
	LastTimeStamp string
	Reason        string
	Count         int32
	Raw           interface{}
}

// IncidentSeverity is a type that defines the various incident severity types
type IncidentSeverity int

const (
	// IncidentNormal indicates a normal non-repeating event
	IncidentNormal IncidentSeverity = iota
	// IncidentWarning indicates a warning event
	IncidentWarning
	// IncidentFatal indicates a fatal occurrence
	IncidentFatal

	// MaxIncident is a place holder for max Incident. Add new values above
	MaxIncident
)

// Incident contains the parameters required to create a new Incident event
// Severity - 0: Normal, 1: Warning, 2: Fatal
// Message describing the incident
type Incident struct {
	Severity IncidentSeverity
	Message  string
}

// Validate makes sure the input parameters of an incident are within bounds
func (inc *Incident) Validate() error {
	if inc.Message == "" || inc.Severity < 0 || inc.Severity >= MaxIncident {
		return fmt.Errorf("Invalid values for incident")
	}
	return nil
}

// ConditionStatus is a type that defines the various condition status
type ConditionStatus int

const (
	// ServiceReady may be a repeating event
	ServiceReady ConditionStatus = iota + 1
	// ServiceNotReady may be a repeating event
	ServiceNotReady

	// MaxCondition place holder for max condition. Add new values above
	MaxCondition
)

// Condition contains the parameters required to create a condition event
// Status - 0: ConditionOK, 1: ConditionFailure
// Count indicates the number of times the condition has occurred
type Condition struct {
	Status ConditionStatus
}

// Validate makes sure the input parameters of an incident are within bounds
func (con *Condition) Validate() error {
	if con.Status <= 0 || con.Status >= MaxCondition {
		return fmt.Errorf("Invalid values for Condition")
	}
	return nil
}

// Error extends error and is returned by methods of the Client interface
type Error interface {
	error
	PVIsBound() bool
	ObjectExists() bool
	NotFound() bool
}

// Services is an interface for a service provided by a cluster orchestrator
type Services interface {
	GetService(ctx context.Context, name, namespace string) (*Service, error)
}

// Service is a container orchestrator neutral structure to describe a service
type Service struct {
	Name      string
	Namespace string
	Hostname  string
	Raw       interface{}
}
