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


package gc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/gcsdk"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/docker/go-units"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// VolumeIdentifierCreate creates a volumeIdentifier from the service and volume-id
// This allows us to serve CSP volumes backed by different GCP services
func VolumeIdentifierCreate(service, vid string) string {
	return fmt.Sprintf("%s:%s", service, vid)
}

// VolumeIdentifierParse maps volumeIdentifier to the service and volume-id
func VolumeIdentifierParse(volumeIdentifier string) (string, string, error) {
	parts := strings.SplitN(volumeIdentifier, ":", 2)
	switch parts[0] {
	case ServiceGCE:
	case ServiceGCS:
	default:
		return "", "", fmt.Errorf("invalid volume identifier")
	}
	if len(parts) == 1 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], nil
}

// VolumeAttach attaches a GC CSP volume
func (cl *Client) VolumeAttach(ctx context.Context, vaa *csp.VolumeAttachArgs) (*csp.Volume, error) {
	svc, vid, _ := VolumeIdentifierParse(vaa.VolumeIdentifier)
	switch svc {
	case ServiceGCE:
		return cl.gceVolumeAttach(ctx, vaa, vid)
	}
	return nil, fmt.Errorf("storage type currently unsupported")
}

// VolumeCreate creates a GC CSP volume
func (cl *Client) VolumeCreate(ctx context.Context, vca *csp.VolumeCreateArgs) (*csp.Volume, error) {
	svc, volType, storageType := StorageTypeToServiceVolumeType(vca.StorageTypeName)
	if storageType == nil {
		return nil, fmt.Errorf("invalid storage type for CSPDomain")
	}
	volType = fmt.Sprintf(volTypeURL, cl.projectID, cl.attrs[AttrZone].Value, volType)
	switch svc {
	case "":
		return nil, fmt.Errorf("storage type is not dynamically provisionable")
	case ServiceGCE:
		return cl.gceVolumeCreate(ctx, vca, volType, storageType)
	}
	return nil, fmt.Errorf("storage type currently unsupported")
}

// VolumeDelete deletes a GC CSP volume
func (cl *Client) VolumeDelete(ctx context.Context, vda *csp.VolumeDeleteArgs) error {
	svc, vid, _ := VolumeIdentifierParse(vda.VolumeIdentifier)
	switch svc {
	case ServiceGCE:
		return cl.gceVolumeDelete(ctx, vda, vid)
	}
	return fmt.Errorf("invalid volume identifier")
}

// VolumeDetach detaches a GC CSP volume
func (cl *Client) VolumeDetach(ctx context.Context, vda *csp.VolumeDetachArgs) (*csp.Volume, error) {
	svc, vid, _ := VolumeIdentifierParse(vda.VolumeIdentifier)
	switch svc {
	case ServiceGCE:
		return cl.gceVolumeDetach(ctx, vda, vid)
	}
	return nil, fmt.Errorf("storage type currently unsupported")
}

// VolumeFetch returns information on a GC CSP volume
func (cl *Client) VolumeFetch(ctx context.Context, vfa *csp.VolumeFetchArgs) (*csp.Volume, error) {
	svc, vid, _ := VolumeIdentifierParse(vfa.VolumeIdentifier)
	switch svc {
	case ServiceGCE:
		return cl.gceVolumeFetch(ctx, vfa, vid)
	}
	return nil, fmt.Errorf("invalid volume identifier")
}

// VolumeList performs a search for GC CSP volumes
func (cl *Client) VolumeList(ctx context.Context, vla *csp.VolumeListArgs) ([]*csp.Volume, error) {
	var svc, volumeType string
	if vla.StorageTypeName != "" {
		var obj *models.CSPStorageType
		if svc, volumeType, obj = StorageTypeToServiceVolumeType(vla.StorageTypeName); obj == nil || svc != ServiceGCE {
			return nil, fmt.Errorf("invalid storage type")
		}
		volumeType = fmt.Sprintf(volTypeURL, cl.projectID, cl.attrs[AttrZone].Value, volumeType)
	}
	return cl.gceVolumeList(ctx, vla, volumeType)
}

// VolumeTagsDelete removes labels on GC CSP volumes
func (cl *Client) VolumeTagsDelete(ctx context.Context, vta *csp.VolumeTagArgs) (*csp.Volume, error) {
	if len(vta.Tags) == 0 {
		return nil, fmt.Errorf("no tags specified")
	}
	svc, vid, _ := VolumeIdentifierParse(vta.VolumeIdentifier)
	switch svc {
	case ServiceGCE:
		return cl.gceVolumeTagsDelete(ctx, vta, vid)
	}
	return nil, fmt.Errorf("invalid volume identifier")
}

// VolumeTagsSet creates or modifies labels on GC CSP volumes
func (cl *Client) VolumeTagsSet(ctx context.Context, vta *csp.VolumeTagArgs) (*csp.Volume, error) {
	if len(vta.Tags) == 0 {
		return nil, fmt.Errorf("no tags specified")
	}
	svc, vid, _ := VolumeIdentifierParse(vta.VolumeIdentifier)
	switch svc {
	case ServiceGCE:
		return cl.gceVolumeTagsSet(ctx, vta, vid)
	}
	return nil, fmt.Errorf("invalid volume identifier")
}

// VolumeSize returns the actual size that will be allocated for a volume.
// A zero sized request gets the minimum allocable
func (cl *Client) VolumeSize(ctx context.Context, stName models.CspStorageType, requestedSizeBytes int64) (int64, error) {
	if requestedSizeBytes < 0 {
		return 0, fmt.Errorf("invalid allocation size")
	}
	svc, volType, storageType := StorageTypeToServiceVolumeType(stName)
	if storageType == nil {
		return 0, fmt.Errorf("invalid storage type for CSPDomain")
	}
	if requestedSizeBytes < *storageType.MinAllocationSizeBytes {
		requestedSizeBytes = *storageType.MinAllocationSizeBytes
	} else if requestedSizeBytes > *storageType.MaxAllocationSizeBytes {
		return 0, fmt.Errorf("requested size exceeds the storage type maximum")
	}
	switch svc {
	case "":
		return 0, fmt.Errorf("storage type is not dynamically provisionable")
	case ServiceGCE:
		sb, _ := cl.gceVolSize(ctx, volType, requestedSizeBytes)
		return sb, nil
	}
	return 0, fmt.Errorf("storage type currently unsupported")
}

func (cl *Client) getComputeService(ctx context.Context) (gcsdk.ComputeService, error) {
	if cl.computeService == nil {
		if ctx == nil {
			var cancelFn func()
			ctx = context.Background()
			ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
			defer cancelFn()
		}
		var err error
		service, err := cl.api.NewComputeService(ctx, option.WithCredentialsJSON([]byte(cl.attrs[AttrCred].Value)))
		if err != nil {
			return nil, fmt.Errorf("failed to create GC compute service: %w", err)
		}
		cl.computeService = service
	}
	return cl.computeService, nil
}

// gceVolumeAttach attaches a volume using the GCE service
func (cl *Client) gceVolumeAttach(ctx context.Context, vaa *csp.VolumeAttachArgs, vid string) (*csp.Volume, error) {
	computeService, err := cl.getComputeService(ctx)
	if err != nil {
		return nil, err
	}
	deviceName := vid
	if !strings.HasPrefix(deviceName, nuvoNamePrefix) {
		deviceName = nuvoNamePrefix + vid
	}
	disk := &compute.AttachedDisk{
		DeviceName: deviceName,
		Source:     fmt.Sprintf(diskSourceURL, cl.projectID, cl.attrs[AttrZone].Value, deviceName),
	}
	op, err := computeService.Instances().AttachDisk(cl.projectID, cl.attrs[AttrZone].Value, vaa.NodeIdentifier, disk).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if err = cl.waitForOperation(ctx, op); err != nil {
		return nil, err
	}
	vol, err := cl.vr.gceVolumeGet(ctx, vid)
	if err != nil {
		return nil, err
	}
	return vol, nil
}

// gceVolumeCreate creates a volume using the GCE service
func (cl *Client) gceVolumeCreate(ctx context.Context, vca *csp.VolumeCreateArgs, volumeType string, storageType *models.CSPStorageType) (*csp.Volume, error) {
	computeService, err := cl.getComputeService(ctx)
	if err != nil {
		return nil, err
	}
	labels := gceLabelsFromModel(vca.Tags)
	if len(labels) == 0 {
		labels = nil
	}
	var diskName string
	if storageID, ok := labels[com.VolTagStorageID]; ok {
		diskName = storageID
	} else {
		diskName = uuidGenerator()
	}
	diskName = nuvoNamePrefix + diskName
	disk := &compute.Disk{
		Name:   diskName,
		Labels: labels,
		SizeGb: util.RoundUpBytes(vca.SizeBytes, units.GiB) / units.GiB,
		Type:   volumeType,
	}
	op, err := computeService.Disks().Insert(cl.projectID, cl.attrs[AttrZone].Value, disk).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if err = cl.waitForOperation(ctx, op); err != nil {
		return nil, err
	}
	vol, err := cl.vr.gceVolumeGet(ctx, diskName)
	if err != nil {
		return nil, err
	}
	return vol, nil
}

// gceVolumeDelete deletes a volume using the GCE service
func (cl *Client) gceVolumeDelete(ctx context.Context, vda *csp.VolumeDeleteArgs, vid string) error {
	computeService, err := cl.getComputeService(ctx)
	if err != nil {
		return err
	}
	op, err := computeService.Disks().Delete(cl.projectID, cl.attrs[AttrZone].Value, vid).Context(ctx).Do()
	if err != nil {
		return err
	}
	err = cl.waitForOperation(ctx, op)
	return err
}

// gceVolumeDetach detaches a volume using the GCE service
func (cl *Client) gceVolumeDetach(ctx context.Context, vda *csp.VolumeDetachArgs, vid string) (*csp.Volume, error) {
	if vda.Force {
		cl.csp.dbgF("ignoring unsupported force on detach [%s]", vid)
	}
	computeService, err := cl.getComputeService(ctx)
	if err != nil {
		return nil, err
	}
	op, err := computeService.Instances().DetachDisk(cl.projectID, cl.attrs[AttrZone].Value, vda.NodeIdentifier, vid).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if err = cl.waitForOperation(ctx, op); err != nil {
		return nil, err
	}
	vol, err := cl.vr.gceVolumeGet(ctx, vid)
	if err != nil {
		return nil, err
	}
	return vol, nil
}

// gceVolumeFetch fetches a GCE volume
func (cl *Client) gceVolumeFetch(ctx context.Context, vfa *csp.VolumeFetchArgs, vid string) (*csp.Volume, error) {
	if _, err := cl.getComputeService(ctx); err != nil {
		return nil, err
	}
	vol, err := cl.vr.gceVolumeGet(ctx, vid)
	if err != nil {
		return nil, err
	}
	return vol, nil
}

type volumeRetriever interface {
	gceVolumeGet(ctx context.Context, name string) (*csp.Volume, error)
}

// gceVolumeGet retrieves specified GCE volume
// Meant to be used after a prior GC operation so assumes the cl.computeService has already been set
func (cl *Client) gceVolumeGet(ctx context.Context, name string) (*csp.Volume, error) {
	disk, err := cl.computeService.Disks().Get(cl.projectID, cl.attrs[AttrZone].Value, name).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return gceDiskToVolume(disk), nil
}

// gceVolumeList searches for GCE volumes
func (cl *Client) gceVolumeList(ctx context.Context, vla *csp.VolumeListArgs, volumeType string) ([]*csp.Volume, error) {
	computeService, err := cl.getComputeService(ctx)
	if err != nil {
		return nil, err
	}
	filter := ""
	if volumeType != "" {
		filter = fmt.Sprintf(`type="%s"`, volumeType)
	}
	for _, tag := range vla.Tags {
		if filter != "" {
			filter += " AND "
		}
		kv := strings.SplitN(tag, ":", 2)
		if len(kv) == 1 { // if just "key" is specified then the existence of a label with that key will be matched
			filter += fmt.Sprintf("labels.%s:*", kv[0])
		} else { // if specified here as "key:value" then both the key and value will be matched
			filter += fmt.Sprintf(`labels.%s="%s"`, kv[0], kv[1])
		}
	}
	req := computeService.Disks().List(cl.projectID, cl.attrs[AttrZone].Value).Filter(filter)
	result := []*csp.Volume{}
	if err = req.Pages(ctx, func(page *compute.DiskList) error {
		for _, disk := range page.Items {
			vol := gceDiskToVolume(disk)
			result = append(result, vol)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list GC disks: %w", err)
	}
	return result, nil
}

// gceVolumeTagsDelete removes specified labels for GC volumes
func (cl *Client) gceVolumeTagsDelete(ctx context.Context, vta *csp.VolumeTagArgs, vid string) (*csp.Volume, error) {
	if _, err := cl.getComputeService(ctx); err != nil {
		return nil, err
	}
	labelsToDelete := gceLabelsFromModel(vta.Tags)
	return cl.intSetLabels(ctx, vid, labelsToDelete, true)
}

// gceVolumeTagsSet adds or modifies labels for GC volumes
func (cl *Client) gceVolumeTagsSet(ctx context.Context, vta *csp.VolumeTagArgs, vid string) (*csp.Volume, error) {
	if _, err := cl.getComputeService(ctx); err != nil {
		return nil, err
	}
	labelsToSet := gceLabelsFromModel(vta.Tags)
	return cl.intSetLabels(ctx, vid, labelsToSet, false)
}

// Meant to be used after a prior GC operation so assumes the cl.computeService has already been set
func (cl *Client) intSetLabels(ctx context.Context, vid string, labelsToUpdate map[string]string, delete bool) (*csp.Volume, error) {
	var err error
	var vol *csp.Volume
	var op *compute.Operation
	for {
		// get the latest fingerprint first
		vol, err = cl.vr.gceVolumeGet(ctx, vid)
		if err != nil {
			return nil, err
		}
		disk := vol.Raw.(*compute.Disk) // or panic
		labelFingerprint, existingLabels := disk.LabelFingerprint, disk.Labels
		labelsToSet := make(map[string]string)
		if !delete {
			labelsToSet = labelsToUpdate
		}
		for k, v := range existingLabels {
			if delete {
				if _, ok := labelsToUpdate[k]; !ok {
					labelsToSet[k] = v
				}
			} else if _, ok := labelsToSet[k]; !ok {
				labelsToSet[k] = v
			}
		}
		labelsRequest := &compute.ZoneSetLabelsRequest{
			LabelFingerprint: labelFingerprint,
			Labels:           labelsToSet,
		}
		// Expected error in case labels get outdated in between get call and update:
		// googleapi: Error 412: Labels fingerprint either invalid or resource labels have changed, conditionNotMet
		if op, err = cl.computeService.Disks().SetLabels(cl.projectID, cl.attrs[AttrZone].Value, vid, labelsRequest).Context(ctx).Do(); err == nil {
			if err = cl.waitForOperation(ctx, op); err == nil {
				if vol, err = cl.vr.gceVolumeGet(ctx, vid); err == nil {
					return vol, nil
				}
			}
		} else if !(strings.Contains(err.Error(), "conditionNotMet") || strings.Contains(err.Error(), "Error 412")) { // if outdated label fingerprint, will retry
			return nil, err
		}
	}
}

// waitForOperation periodically polls the operation and returns when the operation status is DONE
// Meant to be used after a prior GC operation so assumes the cl.computeService has already been set
func (cl *Client) waitForOperation(ctx context.Context, operation *compute.Operation) error {
	for { // ZoneOperations should fail once the ctx.Deadline() is passed
		res, err := cl.computeService.ZoneOperations().Get(cl.projectID, cl.attrs[AttrZone].Value, operation.Name).Context(ctx).Do()
		if err != nil {
			return err
		}
		if res.Status == "DONE" {
			if res.Error != nil {
				return fmt.Errorf("operation failure: code: %d, message: %s", res.HttpErrorStatusCode, res.HttpErrorMessage)
			}
			return nil
		}
		time.Sleep(time.Second)
	}
}

// gceDiskToVolume is the GC to the CSP data type converter
func gceDiskToVolume(gceDisk *compute.Disk) *csp.Volume {
	v := &csp.Volume{
		CSPDomainType:   CSPDomainType,
		StorageTypeName: VolTypeToCSPStorageType(gceDisk.Type),
		Identifier:      VolumeIdentifierCreate(ServiceGCE, gceDisk.Name), // GCE disk names are immutable
		Type:            gceDisk.Type,
		SizeBytes:       gceDisk.SizeGb * units.GiB,
		Raw:             gceDisk,
	}
	if i := strings.LastIndex(v.Type, "/"); i >= 0 {
		// volType is typically a URL in the form of volTypeURL (see gc.go), actual type is the final part of the path
		v.Type = v.Type[i+1:]
	}
	var vps csp.VolumeProvisioningState
	switch gceDisk.Status {
	case "CREATING":
		fallthrough
	case "RESTORING":
		vps = csp.VolumeProvisioningProvisioning
	case "READY":
		vps = csp.VolumeProvisioningProvisioned
	case "DELETING":
		vps = csp.VolumeProvisioningUnprovisioning
	case "FAILED":
		vps = csp.VolumeProvisioningError
	}
	v.ProvisioningState = vps
	v.Tags = gceLabelsToModel(gceDisk.Labels)
	v.Attachments = make([]csp.VolumeAttachment, len(gceDisk.Users))
	for i, user := range gceDisk.Users {
		if i := strings.LastIndex(user, "/"); i >= 0 { // format: projects/project/zones/zone/instances/instance
			user = user[i+1:]
		}
		v.Attachments[i] = csp.VolumeAttachment{
			NodeIdentifier: user,
			Device:         fmt.Sprintf(diskPathFormat, gceDisk.Name),
			State:          csp.VolumeAttachmentAttached, // GCE does not track this outside an active compute.Operation so assume attached
		}
	}
	return v
}

// gceLabelsToModel converts GC labels format to the CSP tag format
func gceLabelsToModel(labels map[string]string) []string {
	idx := 0
	mLabels := make([]string, len(labels))
	for k, v := range labels {
		mLabels[idx] = fmt.Sprintf("%s:%s", k, v)
		idx++
	}
	return mLabels
}

// gceLabelsFromModel converts CSP tag format to the GC disk labels format
// Automatically adds a Name tag if one is not present and a storage-id tag is present
func gceLabelsFromModel(mTags []string) map[string]string {
	labels := make(map[string]string, len(mTags))
	for _, mt := range mTags {
		kv := strings.SplitN(mt, ":", 2)
		labels[kv[0]] = kv[1]
	}
	return labels
}

// gceVolSize returns the actual size that will be allocated for a requested volume size.
// The method returns the size in bytes as well as the size in GiB
func (cl *Client) gceVolSize(ctx context.Context, volumeType string, requestedSizeBytes int64) (int64, int64) {
	oneGib := int64(units.GiB)
	volSize := requestedSizeBytes / oneGib
	if volSize*oneGib < requestedSizeBytes {
		volSize++
	}
	return volSize * oneGib, volSize
}
