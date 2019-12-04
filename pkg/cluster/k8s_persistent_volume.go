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
	"strconv"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// K8sDriverTypes are the available Kubernetes volume driver types. The first element is used as a default.
var K8sDriverTypes = []string{com.K8sDriverTypeFlex, com.K8sDriverTypeCSI}

// GetDriverTypes returns the driver types
func (c *K8s) GetDriverTypes() []string {
	return K8sDriverTypes
}

// K8sFileSystemTypes are the available file system types. The first element is used as a default.
var K8sFileSystemTypes = []string{com.FSTypeExt4, com.FSTypeXfs}

// GetFileSystemTypes returns the file system types
func (c *K8s) GetFileSystemTypes() []string {
	return K8sFileSystemTypes
}

// CreatePublishedPersistentVolumeWatcher creates a watcher to keep track of the deletetion of persistent volumes in a cluster.
// It only keeps track of PVs with the label nuvoloso-volume. Identifying nuvoloso published PV objects.
func (c *K8s) CreatePublishedPersistentVolumeWatcher(ctx context.Context, args *PVWatcherArgs) (PVWatcher, []*PersistentVolumeObj, error) {
	if err := args.Validate(); err != nil {
		return nil, nil, err
	}
	url := "/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume"
	res, resVer, err := c.persistentVolumeLister(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	w := &K8sPVWatcher{}
	w.url = url
	w.c = c
	w.resourceVersion = resVer
	w.ops = w
	w.pvOps = args.Ops
	w.log = c.debugLog
	w.client, err = c.makeClient(args.Timeout)
	return w, res, nil
}

// K8sPVWatcher implements the K8sWatcher type
type K8sPVWatcher struct {
	K8sWatcher
	pvOps PVWatcherOps
}

// WatchEvent handles individual events returned by the K8sPVWatcher
func (w *K8sPVWatcher) WatchEvent(ctx context.Context, object interface{}) {
	event := object.(*K8sPVWatchEvent)
	var pv *PersistentVolumeObj
	pv = event.Object.ToModel()
	switch event.EventType {
	case "DELETED":
		w.log.Infof("pvWatcher DELETED received - pv(%s)", pv.Name)
		w.pvOps.PVDeleted(pv)
	}
}

// WatchObject returns the K8sPVWatchEvent type
func (w *K8sPVWatcher) WatchObject() interface{} {
	return &K8sPVWatchEvent{}
}

// K8sPVWatchEvent descibes the object returned by K8sPVWatcher
type K8sPVWatchEvent struct {
	EventType string `json:"type"`
	Object    K8sPersistentVolume
}

// PublishedPersistentVolumeList lists persistent volumes statically provisioned by Nuvoloso.
// These PVs will have the label["type"] == nuvoloso-volume
func (c *K8s) PublishedPersistentVolumeList(ctx context.Context) ([]*PersistentVolumeObj, error) {
	pvList, _, err := c.persistentVolumeLister(ctx, "/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume")
	return pvList, err
}

// PersistentVolumeList performs a GET operation to retrieve the list of persistent volumes in the cluster
func (c *K8s) PersistentVolumeList(ctx context.Context) ([]*PersistentVolumeObj, error) {
	pvList, _, err := c.persistentVolumeLister(ctx, "/api/v1/persistentvolumes")
	return pvList, err
}

// persistentVolumeLister performs a Get operation for a list of persistent volumes. It can take a queryString to filter results
// empty query string means no filtering
func (c *K8s) persistentVolumeLister(ctx context.Context, url string) ([]*PersistentVolumeObj, string, error) {
	var pvs *K8sPVlistRes
	if err := c.K8sClientGetJSON(ctx, url, &pvs); err != nil {
		return nil, "", err
	}
	res := make([]*PersistentVolumeObj, len(pvs.Items))
	for i, pv := range pvs.Items {
		res[i] = pv.ToModel()
	}
	return res, pvs.ListMeta.ResourceVersion, nil
}

// PersistentVolumeCreate performs a POST operation to create a Persistent Volume
func (c *K8s) PersistentVolumeCreate(ctx context.Context, pvca *PersistentVolumeCreateArgs) (*PersistentVolumeObj, error) {
	if err := pvca.Validate(c); err != nil {
		return nil, err
	}
	var pvBody pvPostBody
	pvBody.Init(c, pvca)
	var pv *K8sPersistentVolume
	if err := c.K8sClientPostJSON(ctx, "/api/v1/persistentvolumes", pvBody.Marshal(), &pv); err == nil {
		var res *PersistentVolumeObj
		res = pv.ToModel()
		return res, nil
	} else if e, ok := err.(*k8sError); ok {
		if e.ObjectExists() {
			pv, err := c.persistentVolumeFetch(ctx, VolID2K8sPvName(pvca.VolumeID))
			if err != nil {
				return nil, err
			}
			if err = c.validateExistingPV(pv, &pvBody); err != nil {
				return nil, fmt.Errorf("found existing PersistentVolume with essential properties modified: %s", err.Error())
			}
			return nil, nil
		}
		return nil, e
	} else {
		return nil, err
	}
}

func (c *K8s) validateExistingPV(pv *K8sPersistentVolume, pvpb *pvPostBody) error {
	for key, value := range pvpb.Metadata.Labels {
		if label, ok := pv.ObjectMeta.Labels[key]; !ok {
			return fmt.Errorf("label (%s) missing", key)
		} else if label != value {
			return fmt.Errorf("label (%s) value incorrect", key)
		}
	}
	return nil
}

// PersistentVolumeDelete performs a DELETE operation to delete a Persistent Volume
func (c *K8s) PersistentVolumeDelete(ctx context.Context, pvda *PersistentVolumeDeleteArgs) (*PersistentVolumeObj, error) {
	var pv *K8sPersistentVolume
	path := fmt.Sprintf("/api/v1/persistentvolumes/%s", VolID2K8sPvName(pvda.VolumeID))
	if err := c.K8sClientDeleteJSON(ctx, path, &pv); err != nil {
		return nil, err
	}
	if pv.Status.Phase == VolumeBound {
		return nil, ErrPvIsBoundOnDelete
	}
	var res *PersistentVolumeObj
	res = pv.ToModel()
	return res, nil
}

// PersistentVolumePublish performs a POST operation to create a Persistent Volume
func (c *K8s) PersistentVolumePublish(ctx context.Context, pvca *PersistentVolumeCreateArgs) (models.ClusterDescriptor, error) {
	pvObj, err := c.PersistentVolumeCreate(ctx, pvca)
	if err != nil {
		return nil, err
	}
	pvcSpec := K8sPVCSpecArgs{
		Capacity:        pvca.SizeBytes,
		VolumeID:        pvca.VolumeID,
		ServicePlanName: pvca.ServicePlanName,
	}
	cd := models.ClusterDescriptor{
		com.ClusterDescriptorPVName: models.ValueType{Kind: "STRING", Value: pvObj.Name},
	}
	if pvca.DriverType == com.K8sDriverTypeCSI {
		cd[com.K8sPvcYaml] = models.ValueType{Kind: "STRING", Value: pvcSpec.GetStaticYaml()}
	} else {
		cd[com.K8sPvcYaml] = models.ValueType{Kind: "STRING", Value: pvcSpec.GetStaticYaml4Flex()}
	}
	return cd, nil
}

// ToModel converts a K8sPersistentVolume object to a PersistentVolumeObj
func (pv *K8sPersistentVolume) ToModel() *PersistentVolumeObj {
	pvObj := &PersistentVolumeObj{
		Name:     pv.Name,
		UID:      pv.UID,
		Capacity: pv.Spec.Capacity["storage"],
		Raw:      pv,
	}
	if pvType, ok := pv.Labels["type"]; ok && pvType == K8sPvLabelType {
		if volID, ok := pv.Labels["volumeId"]; ok {
			pvObj.VolumeSeriesID = volID
		}
	}
	return pvObj
}

// pvPostBody describes a Body for post operations
type pvPostBody struct {
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Metadata   *ObjectMeta           `json:"metadata"`
	Spec       *PersistentVolumeSpec `json:"spec"`
}

func (pvpb *pvPostBody) Init(c *K8s, pvca *PersistentVolumeCreateArgs) {
	pvpb.APIVersion = "v1"
	pvpb.Kind = "PersistentVolume"
	plm := c.newPvLabelMap(pvca)
	pvpb.Metadata = &ObjectMeta{
		Name:   VolID2K8sPvName(pvca.VolumeID),
		Labels: plm,
	}
	pvpb.Spec = &PersistentVolumeSpec{
		Capacity: map[ResourceName]string{
			"storage": util.K8sSizeBytes(pvca.SizeBytes).String(),
		},
		AccessModes: []PersistentVolumeAccessMode{
			"ReadWriteOnce",
		},
	}
	if pvca.DriverType == com.K8sDriverTypeCSI {
		pvpb.Spec.PersistentVolumeSource = PersistentVolumeSource{
			CSI: &CSIPersistentVolumeSource{
				Driver:       com.CSIDriverName,
				VolumeHandle: pvca.VolumeID,
			},
		}
		// We shouldn't add a storage class in the case of static volumes since dynamic volumes may consume them
	} else { // default to FLEX
		pvpb.Metadata.Labels["type"] = K8sTypeLabelFlex
		pvpb.Spec.PersistentVolumeSource = PersistentVolumeSource{
			FlexVolume: &FlexVolumeSource{
				Driver: "nuvoloso.com/nuvo",
				FSType: pvca.FsType,
				Options: map[string]string{
					"nuvoSystemId": pvca.SystemID,
					"nuvoVolumeId": pvca.VolumeID,
				},
			},
		}
	}
}

func (pvpb *pvPostBody) Marshal() []byte {
	body, _ := json.Marshal(pvpb) // strict input type so Marshal won't fail
	return body
}

type pvLabelMap map[string]string

func (c *K8s) newPvLabelMap(pvca *PersistentVolumeCreateArgs) pvLabelMap {
	plm := make(map[string]string)
	plm["type"] = K8sPvLabelType
	plm["volumeId"] = pvca.VolumeID
	plm["accountId"] = pvca.AccountID
	plm["fsType"] = pvca.FsType
	plm["driverType"] = pvca.DriverType
	plm["systemId"] = pvca.SystemID
	plm["sizeBytes"] = strconv.FormatInt(pvca.SizeBytes, 10)
	return plm
}

func (c *K8s) persistentVolumeFetch(ctx context.Context, pvName string) (*K8sPersistentVolume, error) {
	var pv *K8sPersistentVolume
	url := fmt.Sprintf("/api/v1/persistentvolumes/%s", pvName)
	err := c.K8sClientGetJSON(ctx, url, &pv)
	if err != nil {
		return nil, err
	}
	return pv, nil
}
