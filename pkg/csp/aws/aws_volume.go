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


package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/alecthomas/units"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// VolumeIdentifierCreate creates a volumeIdentifier from the service and volume-id
// This allows us to serve CSP volumes backed by different AWS services.
func VolumeIdentifierCreate(service, vid string) string {
	return fmt.Sprintf("%s:%s", service, vid)
}

// VolumeIdentifierParse maps volumeIdentifier to the service and volume-id
func VolumeIdentifierParse(volumeIdentifier string) (string, string, error) {
	parts := strings.SplitN(volumeIdentifier, ":", 2)
	switch parts[0] {
	case ServiceEC2:
	case ServiceS3:
	default:
		return "", "", fmt.Errorf("invalid volume identifier")
	}
	if len(parts) == 1 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], nil
}

// VolumeAttach attaches an AWS CSP volume
func (cl *Client) VolumeAttach(ctx context.Context, vaa *csp.VolumeAttachArgs) (*csp.Volume, error) {
	svc, vid, _ := VolumeIdentifierParse(vaa.VolumeIdentifier)
	switch svc {
	case ServiceEC2:
		return cl.awsEC2VolumeAttach(ctx, vaa, vid)
	}
	return nil, fmt.Errorf("storage type currently unsupported")
}

// VolumeCreate creates an AWS CSP volume
func (cl *Client) VolumeCreate(ctx context.Context, vca *csp.VolumeCreateArgs) (*csp.Volume, error) {
	svc, volType, storageType := StorageTypeToServiceVolumeType(vca.StorageTypeName)
	if storageType == nil {
		return nil, fmt.Errorf("invalid storage type for CSPDomain")
	}
	switch svc {
	case "":
		return nil, fmt.Errorf("storage type is not dynamically provisionable")
	case ServiceEC2:
		return cl.awsEC2VolumeCreate(ctx, vca, volType, storageType)

	}
	return nil, fmt.Errorf("storage type currently unsupported")
}

// VolumeDelete deletes an AWS CSP volume
func (cl *Client) VolumeDelete(ctx context.Context, vda *csp.VolumeDeleteArgs) error {
	svc, vid, _ := VolumeIdentifierParse(vda.VolumeIdentifier)
	switch svc {
	case ServiceEC2:
		return cl.awsEC2VolumeDelete(ctx, vda, vid)
	}
	return fmt.Errorf("invalid volume identifier")
}

// VolumeDetach detaches an AWS CSP volume
func (cl *Client) VolumeDetach(ctx context.Context, vda *csp.VolumeDetachArgs) (*csp.Volume, error) {
	svc, vid, _ := VolumeIdentifierParse(vda.VolumeIdentifier)
	switch svc {
	case ServiceEC2:
		return cl.awsEC2VolumeDetach(ctx, vda, vid)
	}
	return nil, fmt.Errorf("storage type currently unsupported")
}

// VolumeFetch returns information on an AWS CSP volume
func (cl *Client) VolumeFetch(ctx context.Context, vfa *csp.VolumeFetchArgs) (*csp.Volume, error) {
	svc, vid, _ := VolumeIdentifierParse(vfa.VolumeIdentifier)
	switch svc {
	case ServiceEC2:
		return cl.awsEC2VolumeFetch(ctx, vfa, vid)
	}
	return nil, fmt.Errorf("invalid volumeIdentifier")
}

// VolumeList performs a search for AWS EBS CSP volumes
func (cl *Client) VolumeList(ctx context.Context, vla *csp.VolumeListArgs) ([]*csp.Volume, error) {
	var svc string
	var obj *models.CSPStorageType
	if vla.StorageTypeName != "" {
		if svc, _, obj = StorageTypeToServiceVolumeType(vla.StorageTypeName); obj == nil || svc != ServiceEC2 {
			return nil, fmt.Errorf("invalid storage type")
		}
	}
	return cl.awsEC2VolumeList(ctx, vla)
}

// VolumeTagsDelete removes tags on AWS CSP volumes
func (cl *Client) VolumeTagsDelete(ctx context.Context, vta *csp.VolumeTagArgs) (*csp.Volume, error) {
	if vta.Tags == nil || len(vta.Tags) == 0 {
		return nil, fmt.Errorf("no tags specified")
	}
	svc, vid, _ := VolumeIdentifierParse(vta.VolumeIdentifier)
	switch svc {
	case ServiceEC2:
		return cl.awsEC2VolumeTagsDelete(ctx, vta, vid)
	}
	return nil, fmt.Errorf("invalid volume identifier")
}

// VolumeTagsSet creates or modifies tags on AWS CSP volumes
func (cl *Client) VolumeTagsSet(ctx context.Context, vta *csp.VolumeTagArgs) (*csp.Volume, error) {
	if vta.Tags == nil || len(vta.Tags) == 0 {
		return nil, fmt.Errorf("no tags specified")
	}
	svc, vid, _ := VolumeIdentifierParse(vta.VolumeIdentifier)
	switch svc {
	case ServiceEC2:
		return cl.awsEC2VolumeTagsSet(ctx, vta, vid)
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
	case ServiceEC2:
		sb, _ := cl.awsEC2VolSize(ctx, volType, requestedSizeBytes)
		return sb, nil
	}
	return 0, fmt.Errorf("storage type currently unsupported")
}

// awsEC2VolumeAttach attaches a volume using the EC2 service
func (cl *Client) awsEC2VolumeAttach(ctx context.Context, vaa *csp.VolumeAttachArgs, vid string) (*csp.Volume, error) {
	ec2client := cl.ec2Client()
	devicePath, newMapping, err := cl.awsEC2VolumeDevicePath(ctx, vid, vaa.NodeIdentifier)
	if err != nil {
		return nil, err
	}
	if newMapping {
		defer cl.awsEC2VolumeDevicePathRelease(vaa.NodeIdentifier, devicePath)
		req := &ec2.AttachVolumeInput{
			Device:     aws.String(devicePath),
			InstanceId: aws.String(vaa.NodeIdentifier),
			VolumeId:   aws.String(vid),
		}
		if ctx == nil {
			var cancelFn func()
			ctx = context.Background()
			ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
			defer cancelFn()
		}
		if _, err = ec2client.AttachVolumeWithContext(ctx, req); err != nil {
			return nil, err
		}
	}
	// AttachVolumeWithContext is asynchronous, must wait for attach to complete
	err = cl.awsEC2VolumeWaitForAttachmentState(ctx, vid, ec2.AttachmentStatusAttached)
	if err != nil {
		return nil, err
	}
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: vaa.VolumeIdentifier,
	}
	return cl.awsEC2VolumeFetch(ctx, vfa, vid)
}

// awsEC2VolumeCreate creates a volume using the EC2 service
func (cl *Client) awsEC2VolumeCreate(ctx context.Context, vca *csp.VolumeCreateArgs, volumeType string, storageType *models.CSPStorageType) (*csp.Volume, error) {
	ec2client := cl.ec2Client()
	cvi := &ec2.CreateVolumeInput{}
	cvi.SetEncrypted(true)
	cvi.SetVolumeType(volumeType)
	zone, _ := cl.attrs[AttrAvailabilityZone]
	cvi.SetAvailabilityZone(zone.Value)
	_, volSize := cl.awsEC2VolSize(ctx, volumeType, vca.SizeBytes)
	cvi.SetSize(volSize)
	// Only specify iops if we need to; otherwise AWS calculates for us
	if iops := awsEC2VolumeIops(storageType, volSize); iops > 0 {
		cvi.SetIops(iops)
	}
	tags := awsEC2TagsFromModel(vca.Tags)
	if len(tags) == 0 {
		tags = nil
	}
	cvi.TagSpecifications = []*ec2.TagSpecification{
		&ec2.TagSpecification{
			ResourceType: aws.String("volume"),
			Tags:         tags,
		},
	}
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	cvo, err := ec2client.CreateVolumeWithContext(ctx, cvi)
	if err != nil {
		return nil, err
	}
	// Wait for completion
	if aws.StringValue(cvo.State) != ec2.VolumeStateCreating {
		return awsEC2VolumeToVolume(cvo), nil
	}
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{cvo.VolumeId},
	}
	err = ec2client.WaitUntilVolumeAvailableWithContext(ctx, dvi)
	if err != nil {
		return nil, err
	}
	vid := aws.StringValue(cvo.VolumeId)
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: vid,
	}
	return cl.awsEC2VolumeFetch(ctx, vfa, vid)
}

// awsEC2VolumeDelete deletes an EC2 volume
func (cl *Client) awsEC2VolumeDelete(ctx context.Context, vda *csp.VolumeDeleteArgs, vid string) error {
	ec2client := cl.ec2Client()
	deleteVolumeInput := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(vid),
	}
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	if _, err := ec2client.DeleteVolumeWithContext(ctx, deleteVolumeInput); err != nil {
		if strings.Contains(err.Error(), "InvalidVolume.NotFound") {
			return csp.ErrorVolumeNotFound
		}
		return err
	}
	// Wait for completion
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{aws.String(vid)},
	}
	if err := ec2client.WaitUntilVolumeDeletedWithContext(ctx, dvi); err != nil {
		return err
	}
	return nil
}

// awsEC2VolumeDetach detaches an EC2 volume
func (cl *Client) awsEC2VolumeDetach(ctx context.Context, vda *csp.VolumeDetachArgs, vid string) (*csp.Volume, error) {
	ec2client := cl.ec2Client()
	req := &ec2.DetachVolumeInput{VolumeId: aws.String(vid)} // only volume ID is required
	if vda.NodeIdentifier != "" {
		req.InstanceId = aws.String(vda.NodeIdentifier)
	}
	if vda.NodeDevice != "" {
		req.Device = aws.String(vda.NodeDevice)
	}
	req.Force = aws.Bool(vda.Force)
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	if _, err := ec2client.DetachVolumeWithContext(ctx, req); err != nil {
		if strings.Contains(err.Error(), "is in the 'available' state") {
			return nil, csp.ErrorVolumeNotAttached
		}
		return nil, err
	}
	// DetachVolumeWithContext is asynchronous, must wait for detach to complete
	if err := ec2client.WaitUntilVolumeAvailableWithContext(ctx, &ec2.DescribeVolumesInput{VolumeIds: []*string{&vid}}); err != nil {
		return nil, err
	}
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: vda.VolumeIdentifier,
	}
	return cl.awsEC2VolumeFetch(ctx, vfa, vid)
}

// attachingNodeDevices is a set of nid:deviceName which tracks devices to which volumes are being attached
var attachingNodeDevices = map[string]struct{}{}

// awsEC2VolumeDevicePath will look up the device path for the volume, vid, on the node, nid.
// Unused device path will be returned when the volume is not already attached to the node.
// The boolean return value indicates whether the returned device name is a new device name or not.
// When true, the caller of awsEC2VolumeDevicePath must call awsEC2VolumeDevicePathRelease when attachment completes (success or fail).
func (cl *Client) awsEC2VolumeDevicePath(ctx context.Context, vid string, nid string) (string, bool, error) {
	instance, err := cl.awsEC2InstanceFetch(ctx, nid)
	if err != nil {
		return "", false, err
	}

	attached := map[string]string{}
	for _, mapping := range instance.BlockDeviceMappings {
		deviceName := aws.StringValue(mapping.DeviceName)
		mappedVID := aws.StringValue(mapping.Ebs.VolumeId)
		attached[deviceName] = mappedVID
		if mappedVID == vid {
			return deviceName, false, nil
		}
	}

	// Find and return the next available device name in the desired range, /dev/xvd[bc][a-z] (same range as Kubernetes)
	// This range is guaranteed not to conflict with ephemeral devices. Note: this means we only support HVM instances.
	// Even if the instance uses NVMe, the names in the BlockDeviceMappings will be in this range.
	// TBD: validate support for Windows instances
	cspMutex.Lock()
	defer cspMutex.Unlock()
	prefix := "/dev/xvd"
	for first := 'b'; first <= 'c'; first++ {
		for second := 'a'; second <= 'z'; second++ {
			path := prefix + string([]rune{first, second})
			if _, found := attached[path]; !found {
				key := nid + ":" + path
				if _, found = attachingNodeDevices[key]; !found {
					attachingNodeDevices[key] = struct{}{}
					return path, true, nil
				}
			}
		}
	}
	return "", false, fmt.Errorf("all device names are in use")
}

// awsEC2VolumeDevicePathRelease releases a devicePath that was previously allocated by awsEC2VolumeDevicePath
func (cl *Client) awsEC2VolumeDevicePathRelease(nid string, devicePath string) {
	cspMutex.Lock()
	defer cspMutex.Unlock()
	delete(attachingNodeDevices, nid+":"+devicePath)
}

// awsEC2VolumeFetch fetches an EC2 volume
func (cl *Client) awsEC2VolumeFetch(ctx context.Context, vfa *csp.VolumeFetchArgs, vid string) (*csp.Volume, error) {
	ec2client := cl.ec2Client()
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{&vid},
	}
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	dvo, err := ec2client.DescribeVolumesWithContext(ctx, dvi)
	if err != nil {
		return nil, err
	}
	return awsEC2VolumeToVolume(dvo.Volumes[0]), nil
}

// awsEC2VolumeList searches for EC2 volumes
func (cl *Client) awsEC2VolumeList(ctx context.Context, vla *csp.VolumeListArgs) ([]*csp.Volume, error) {
	ec2client := cl.ec2Client()
	filters := []*ec2.Filter{}
	if vla.StorageTypeName != "" {
		_, volumeType, _ := StorageTypeToServiceVolumeType(vla.StorageTypeName)
		filters = append(filters, &ec2.Filter{
			Name:   aws.String("volume-type"),
			Values: []*string{aws.String(volumeType)},
		})
	}
	for _, tag := range vla.Tags {
		kv := strings.SplitN(tag, ":", 2)
		if len(kv) == 2 {
			filters = append(filters, &ec2.Filter{
				Name:   aws.String("tag:" + kv[0]),
				Values: []*string{aws.String(kv[1])},
			})
		} else {
			filters = append(filters, &ec2.Filter{
				Name:   aws.String("tag-key"),
				Values: []*string{aws.String(kv[0])},
			})
		}
	}
	dvi := &ec2.DescribeVolumesInput{}
	if len(filters) > 0 {
		dvi.Filters = filters
	}
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	dvo, err := ec2client.DescribeVolumesWithContext(ctx, dvi)
	if err != nil {
		return nil, err
	}
	res := make([]*csp.Volume, len(dvo.Volumes))
	for i, eV := range dvo.Volumes {
		res[i] = awsEC2VolumeToVolume(eV)
	}
	return res, nil
}

// awsEC2VolumeTagsDelete removes tags on EC2 volumes
func (cl *Client) awsEC2VolumeTagsDelete(ctx context.Context, vta *csp.VolumeTagArgs, vid string) (*csp.Volume, error) {
	ec2client := cl.ec2Client()
	dti := &ec2.DeleteTagsInput{
		Resources: []*string{aws.String(vid)},
		Tags:      awsEC2TagsFromModel(vta.Tags),
	}
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	if _, err := ec2client.DeleteTagsWithContext(ctx, dti); err != nil {
		return nil, err
	}
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: vta.VolumeIdentifier,
	}
	return cl.awsEC2VolumeFetch(ctx, vfa, vid)
}

// awsEC2VolumeTagsSet creates or modifies tags on EC2 volumes
func (cl *Client) awsEC2VolumeTagsSet(ctx context.Context, vta *csp.VolumeTagArgs, vid string) (*csp.Volume, error) {
	ec2client := cl.ec2Client()
	cti := &ec2.CreateTagsInput{
		Resources: []*string{aws.String(vid)},
		Tags:      awsEC2TagsFromModel(vta.Tags),
	}
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	if _, err := ec2client.CreateTagsWithContext(ctx, cti); err != nil {
		return nil, err
	}
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: vta.VolumeIdentifier,
	}
	return cl.awsEC2VolumeFetch(ctx, vfa, vid)
}

// awsEC2VolSize returns the actual size that will be allocated for a requested volume size.
// The method returns the size in bytes as well as the size in GiB
func (cl *Client) awsEC2VolSize(ctx context.Context, volumeType string, requestedSizeBytes int64) (int64, int64) {
	oneGib := int64(units.GiB)
	volSize := requestedSizeBytes / oneGib
	if volSize*oneGib < requestedSizeBytes {
		volSize++
	}
	return volSize * oneGib, volSize
}

func (cl *Client) awsEC2VolumeWaitForAttachmentState(ctx aws.Context, vid string, state string) error {
	ec2client := cl.ec2Client()
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	w := &request.Waiter{
		Name:        "WaitForAttachmentState",
		MaxAttempts: 40,
		Delay:       request.ConstantWaiterDelay(15 * time.Second),
		Acceptors: []request.WaiterAcceptor{
			{
				State:   request.SuccessWaiterState,
				Matcher: request.PathAllWaiterMatch, Argument: "Volumes[].Attachments[].State",
				Expected: state,
			}, {
				State:   request.FailureWaiterState,
				Matcher: request.PathAnyWaiterMatch, Argument: "Volumes[].State",
				Expected: ec2.VolumeStateDeleted,
			},
		},
		NewRequest: func(opts []request.Option) (*request.Request, error) {
			input := &ec2.DescribeVolumesInput{VolumeIds: []*string{&vid}}
			req, _ := ec2client.DescribeVolumesRequest(input)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}
	waiter := cl.client.NewWaiter(w)

	return waiter.WaitWithContext(ctx)
}

// awsEC2VolumeToVolume is the AWS to the CSP data type converter
func awsEC2VolumeToVolume(eV *ec2.Volume) *csp.Volume {
	v := &csp.Volume{
		CSPDomainType:   CSPDomainType,
		StorageTypeName: EC2VolTypeToCSPStorageType(aws.StringValue(eV.VolumeType)),
		Identifier:      VolumeIdentifierCreate(ServiceEC2, aws.StringValue(eV.VolumeId)),
		Type:            aws.StringValue(eV.VolumeType),
		SizeBytes:       aws.Int64Value(eV.Size) * int64(units.GiB),
		Raw:             eV,
	}
	var vps csp.VolumeProvisioningState
	switch aws.StringValue(eV.State) {
	case ec2.VolumeStateCreating:
		vps = csp.VolumeProvisioningProvisioning
	case ec2.VolumeStateAvailable:
		fallthrough
	case ec2.VolumeStateInUse:
		vps = csp.VolumeProvisioningProvisioned
	case ec2.VolumeStateDeleting:
		vps = csp.VolumeProvisioningUnprovisioning
	case ec2.VolumeStateDeleted:
		vps = csp.VolumeProvisioningUnprovisioned
	}
	v.ProvisioningState = vps
	v.Tags = awsEC2TagsToModel(eV.Tags)
	v.Attachments = make([]csp.VolumeAttachment, len(eV.Attachments))
	for i, a := range eV.Attachments {
		var vas csp.VolumeAttachmentState
		switch aws.StringValue(a.State) {
		case ec2.AttachmentStatusAttaching:
			vas = csp.VolumeAttachmentAttaching
		case ec2.AttachmentStatusAttached:
			vas = csp.VolumeAttachmentAttached
		case ec2.AttachmentStatusDetaching:
			vas = csp.VolumeAttachmentDetaching
		case ec2.AttachmentStatusDetached:
			vas = csp.VolumeAttachmentDetached
		}
		v.Attachments[i] = csp.VolumeAttachment{
			NodeIdentifier: aws.StringValue(a.InstanceId),
			Device:         aws.StringValue(a.Device),
			State:          vas,
		}
	}
	return v
}

// awsEC2VolumeIops assigns the number of IOPS to ask AWS for (if needed - io1 only)
func awsEC2VolumeIops(storageType *models.CSPStorageType, volSizeGiB int64) int64 {
	var sizeIops, minIops, maxIops int64

	if vt, ok := storageType.CspStorageTypeAttributes[PAIopsMin]; ok {
		// ignore Kind and parsing errors (internal)
		minIops, _ = strconv.ParseInt(vt.Value, 10, 64)
	}
	if vt, ok := storageType.CspStorageTypeAttributes[PAIopsMax]; ok {
		// ignore Kind and parsing errors (internal)
		maxIops, _ = strconv.ParseInt(vt.Value, 10, 64)
	}
	if vt, ok := storageType.CspStorageTypeAttributes[PAIopsGB]; ok {
		// ignore Kind and parsing errors (internal)
		iopsGB, _ := strconv.ParseInt(vt.Value, 10, 64)
		sizeIops = iopsGB * volSizeGiB
	}
	if sizeIops < minIops {
		return minIops
	} else if sizeIops > maxIops {
		return maxIops
	}
	return sizeIops
}

// awsEC2TagsToModel converts AWS EC2 tag format to the CSP tag format
func awsEC2TagsToModel(eTags []*ec2.Tag) []string {
	mTags := make([]string, len(eTags))
	for i, t := range eTags {
		if t.Value != nil {
			mTags[i] = fmt.Sprintf("%s:%s", aws.StringValue(t.Key), aws.StringValue(t.Value))
		} else {
			mTags[i] = fmt.Sprintf("%s", aws.StringValue(t.Key))
		}
	}
	return mTags
}

// awsEC2TagsFromModel converts CSP tag format to the AWS EC2 tag format
// Automatically adds a Name tag if one is not present and a storage-id tag is present
func awsEC2TagsFromModel(mTags []string) []*ec2.Tag {
	eTags := make([]*ec2.Tag, len(mTags), len(mTags)+1)
	hasNameTag := false
	storageID := ""
	for i, mt := range mTags {
		kv := strings.SplitN(mt, ":", 2)
		eT := &ec2.Tag{}
		eT.Key = aws.String(kv[0])
		if kv[0] == NameTag {
			hasNameTag = true
		}
		if len(kv) == 2 {
			eT.Value = aws.String(kv[1])
			if kv[0] == com.VolTagStorageID {
				storageID = kv[1]
			}
		}
		eTags[i] = eT
	}
	if !hasNameTag && storageID != "" {
		// create a name tag using the storageID and a prefix
		eT := &ec2.Tag{}
		eT.Key = aws.String(NameTag)
		eT.Value = aws.String(awsNuvoNamePrefix + storageID)
		eTags = append(eTags, eT)
	}
	return eTags
}
