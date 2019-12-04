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
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/aws/aws-sdk-go/aws"
)

// LocalEphemeralDevices satisfies the EphemeralDiscoverer interface
func (cl *Client) LocalEphemeralDevices() ([]*csp.EphemeralDevice, error) {
	// determine the instance type, also verifies this node acts like an AWS node
	md, err := cl.csp.LocalInstanceMetadata()
	if err != nil {
		return nil, err
	}
	instanceType := md["instance-type"] // guaranteed to exist
	cacheInfo, ok := awsInstanceStorage[instanceType]
	if !ok {
		cl.csp.dbgF("instance-type[%s] no documented ephemeral devices", instanceType)
		return nil, nil // instance has no local devices
	}

	if err = cl.csp.loadLocalDevices(false); err != nil {
		return nil, err
	}

	// "lsblk -Jb" to find all of the block devices
	ctx := context.Background()
	output, err := cl.csp.exec.CommandContext(ctx, "lsblk", "-J", "-b").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("lsblk -J -b failed: %w", err)
	}
	res := &LsBlkOutput{}
	if err = json.Unmarshal(output, res); err != nil {
		return nil, fmt.Errorf("lsblk -J -b output error: %w", err)
	}

	// filter list to include only devices in the ephemeral range xvd[b-y] (xvda is the boot device) or nvme[0-9]+n1.
	// In the case of NVMe, EBS and ephemeral share the same range; isLocalNVMeDeviceEphemeral() provides the necessary filter.
	// Tentatively mark as Usable if not mounted, no children and read-write with valid size.
	// Note: in a container, only those mounts that are visible in the container are included in the output
	disks := map[string]*csp.EphemeralDevice{}
	paths := []string{} // only contains disks that pass this first filter
	re := regexp.MustCompile(`^(xvd[b-y]|nvme[0-9]+n1)$`)
	hasNVMe := cl.csp.hasNVMeStorage()
	for _, disk := range res.BlockDevices {
		if disk.Type == "disk" && re.MatchString(disk.Name) && (!hasNVMe || cl.csp.isLocalNVMeDeviceEphemeral(disk.Name)) {
			dev := &csp.EphemeralDevice{}
			dev.Path = "/dev/" + disk.Name
			dev.SizeBytes, err = strconv.ParseInt(disk.Size, 10, 64)
			dev.Type = cacheInfo.deviceType
			dev.Initialized = cacheInfo.initialized
			if err == nil && disk.MountPoint == nil && disk.ReadOnly == "0" && len(disk.Children) == 0 {
				dev.Usable = true
				paths = append(paths, dev.Path)
			}
			disks[disk.Name] = dev
		}
	}

	// Use blkid to filter any formatted devices. If so, they may be mounted but not visible to lsblk.
	// Only devices that are formatted will be included in the output.
	// Note: this only works natively as root or in a privileged container with /dev:/dev mapped
	usable := len(paths)
	if usable > 0 {
		output, err = cl.csp.exec.CommandContext(ctx, "blkid", paths...).CombinedOutput()
		if err != nil {
			// blkid exits with an error if none of the devices contain a formatted filesystem, ignore the error
			cl.csp.dbgF("blkid %s error: %v", strings.Join(paths, " "), err)
		} else {
			re = regexp.MustCompile(`^/dev/([^:]+): `)
			for _, line := range strings.Split(string(output), "\n") {
				if match := re.FindStringSubmatch(line); len(match) > 0 {
					if dev, ok := disks[string(match[1])]; ok {
						dev.Usable = false
						usable--
					}
				}
			}
		}
	}

	// Filter again, removing all the EBS volumes. Depending on the AMI, it is possible for an
	// EBS volume to be attached as a device name that the instance meta-data says should be an ephemeral.
	// Not required on instances with NVMe; filtering was already performed above.
	// This pass, delete elements that are EBS volumes rather than marking them as unusable, they cannot be ephemeral.
	if usable > 0 && !hasNVMe {
		ctx, cancelFn := context.WithTimeout(context.Background(), cl.Timeout)
		defer cancelFn()
		instance, err := cl.awsEC2InstanceFetch(ctx, md[csp.IMDInstanceName])
		if err != nil {
			return nil, fmt.Errorf("instance[%s] fetch failure: %w", md[csp.IMDInstanceName], err)
		}
		for _, ebs := range instance.BlockDeviceMappings {
			name := strings.TrimPrefix(aws.StringValue(ebs.DeviceName), "/dev/")
			// EBS devices may claim to be named "sdA", but on HVM instances their actual name is "xvdA"
			if strings.HasPrefix(name, "sd") {
				name = "xvd" + name[2:]
			}
			delete(disks, name)
		}
	}
	names := util.SortedStringKeys(disks)
	if len(disks) < cacheInfo.numDevices {
		cl.csp.dbgF("instance-type[%s] found %d(<%d) ephemeral devices", instanceType, len(disks), cacheInfo.numDevices)
	} else if len(disks) > cacheInfo.numDevices {
		panic(fmt.Sprintf("LocalEphemeralDevices filtering bug: instance-type[%s] found %d(>%d) ephemeral devices: %v",
			instanceType, len(disks), cacheInfo.numDevices, names))
	}

	ret := make([]*csp.EphemeralDevice, len(names))
	for i, name := range names {
		ret[i] = disks[name]
	}
	return ret, nil
}

// LsBlkDevice represents a single device in the lsblk output (some fields ignored)
type LsBlkDevice struct {
	Name       string         `json:"name"` // device name, without the /dev/ prefix
	MountPoint *string        `json:"mountpoint"`
	ReadOnly   string         `json:"ro"`
	Size       string         `json:"size"` // bytes as a string
	Type       string         `json:"type"`
	Children   []*LsBlkDevice `json:"children,omitempty"`
}

// MountInfo is used to record information about the mounted volume for use by later unmount command
type MountInfo struct {
	VolumeID       string `json:"volumeId"`
	SnapIdentifier string `json:"snapIdentifier"`
}

// LsBlkOutput represents the expected output of the lsblk -Jb command
type LsBlkOutput struct {
	BlockDevices []*LsBlkDevice `json:"blockdevices"`
}

// InstanceStorage provides documented instance storage properties.
// Size is not recorded here because the documented values are rounded for human consumption.
type awsInstanceStorageInfo struct {
	numDevices  int
	deviceType  string
	initialized bool
}

// These properties are not available via the AWS EC2 SDK but are documented at
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/InstanceStorage.html
var awsInstanceStorage = map[string]awsInstanceStorageInfo{
	"c1.medium":     {1, csp.EphemeralTypeHDD, false},
	"c1.xlarge":     {4, csp.EphemeralTypeHDD, false},
	"c3.large":      {2, csp.EphemeralTypeSSD, false},
	"c3.xlarge":     {2, csp.EphemeralTypeSSD, false},
	"c3.2xlarge":    {2, csp.EphemeralTypeSSD, false},
	"c3.4xlarge":    {2, csp.EphemeralTypeSSD, false},
	"c3.8xlarge":    {2, csp.EphemeralTypeSSD, false},
	"c5d.large":     {1, csp.EphemeralTypeSSD, true},
	"c5d.xlarge":    {1, csp.EphemeralTypeSSD, true},
	"c5d.2xlarge":   {1, csp.EphemeralTypeSSD, true},
	"c5d.4xlarge":   {1, csp.EphemeralTypeSSD, true},
	"c5d.9xlarge":   {1, csp.EphemeralTypeSSD, true},
	"c5d.18xlarge":  {2, csp.EphemeralTypeSSD, true},
	"cc2.8xlarge":   {4, csp.EphemeralTypeHDD, false},
	"cr1.8xlarge":   {2, csp.EphemeralTypeSSD, false},
	"d2.xlarge":     {3, csp.EphemeralTypeHDD, true},
	"d2.2xlarge":    {6, csp.EphemeralTypeHDD, true},
	"d2.4xlarge":    {12, csp.EphemeralTypeHDD, true},
	"d2.8xlarge":    {24, csp.EphemeralTypeHDD, true},
	"f1.2xlarge":    {1, csp.EphemeralTypeSSD, true},
	"f1.4xlarge":    {1, csp.EphemeralTypeSSD, true},
	"f1.16xlarge":   {4, csp.EphemeralTypeSSD, true},
	"g2.2xlarge":    {1, csp.EphemeralTypeSSD, false},
	"g2.8xlarge":    {2, csp.EphemeralTypeSSD, false},
	"h1.2xlarge":    {1, csp.EphemeralTypeHDD, true},
	"h1.4xlarge":    {2, csp.EphemeralTypeHDD, true},
	"h1.8xlarge":    {4, csp.EphemeralTypeHDD, true},
	"h1.16xlarge":   {8, csp.EphemeralTypeHDD, true},
	"hs1.8xlarge":   {24, csp.EphemeralTypeHDD, false},
	"i2.xlarge":     {1, csp.EphemeralTypeSSD, true},
	"i2.2xlarge":    {2, csp.EphemeralTypeSSD, true},
	"i2.4xlarge":    {4, csp.EphemeralTypeSSD, true},
	"i2.8xlarge":    {8, csp.EphemeralTypeSSD, true},
	"i3.large":      {1, csp.EphemeralTypeSSD, true},
	"i3.xlarge":     {1, csp.EphemeralTypeSSD, true},
	"i3.2xlarge":    {1, csp.EphemeralTypeSSD, true},
	"i3.4xlarge":    {2, csp.EphemeralTypeSSD, true},
	"i3.8xlarge":    {4, csp.EphemeralTypeSSD, true},
	"i3.16xlarge":   {8, csp.EphemeralTypeSSD, true},
	"i3.metal":      {8, csp.EphemeralTypeSSD, true},
	"i3en.large":    {1, csp.EphemeralTypeSSD, true},
	"i3en.xlarge":   {1, csp.EphemeralTypeSSD, true},
	"i3en.2xlarge":  {2, csp.EphemeralTypeSSD, true},
	"i3en.3xlarge":  {1, csp.EphemeralTypeSSD, true},
	"i3en.6xlarge":  {2, csp.EphemeralTypeSSD, true},
	"i3en.12xlarge": {4, csp.EphemeralTypeSSD, true},
	"i3en.24xlarge": {8, csp.EphemeralTypeSSD, true},
	"i3en.metal":    {8, csp.EphemeralTypeSSD, true},
	"m1.small":      {1, csp.EphemeralTypeHDD, false},
	"m1.medium":     {1, csp.EphemeralTypeHDD, false},
	"m1.large":      {2, csp.EphemeralTypeHDD, false},
	"m1.xlarge":     {4, csp.EphemeralTypeHDD, false},
	"m2.xlarge":     {1, csp.EphemeralTypeHDD, false},
	"m2.2xlarge":    {1, csp.EphemeralTypeHDD, false},
	"m2.4xlarge":    {2, csp.EphemeralTypeHDD, false},
	"m3.medium":     {1, csp.EphemeralTypeSSD, false},
	"m3.large":      {1, csp.EphemeralTypeSSD, false},
	"m3.xlarge":     {2, csp.EphemeralTypeSSD, false},
	"m3.2xlarge":    {2, csp.EphemeralTypeSSD, false},
	"m5ad.large":    {1, csp.EphemeralTypeSSD, true},
	"m5ad.xlarge":   {1, csp.EphemeralTypeSSD, true},
	"m5ad.2xlarge":  {1, csp.EphemeralTypeSSD, true},
	"m5ad.4xlarge":  {2, csp.EphemeralTypeSSD, true},
	"m5ad.12xlarge": {2, csp.EphemeralTypeSSD, true},
	"m5ad.24xlarge": {4, csp.EphemeralTypeSSD, true},
	"m5d.large":     {1, csp.EphemeralTypeSSD, true},
	"m5d.xlarge":    {1, csp.EphemeralTypeSSD, true},
	"m5d.2xlarge":   {1, csp.EphemeralTypeSSD, true},
	"m5d.4xlarge":   {2, csp.EphemeralTypeSSD, true},
	"m5d.8xlarge":   {2, csp.EphemeralTypeSSD, true},
	"m5d.12xlarge":  {2, csp.EphemeralTypeSSD, true},
	"m5d.16xlarge":  {4, csp.EphemeralTypeSSD, true},
	"m5d.24xlarge":  {4, csp.EphemeralTypeSSD, true},
	"m5d.metal":     {4, csp.EphemeralTypeSSD, true},
	"p3dn.24xlarge": {2, csp.EphemeralTypeSSD, true},
	"r3.large":      {1, csp.EphemeralTypeSSD, true},
	"r3.xlarge":     {1, csp.EphemeralTypeSSD, true},
	"r3.2xlarge":    {1, csp.EphemeralTypeSSD, true},
	"r3.4xlarge":    {1, csp.EphemeralTypeSSD, true},
	"r3.8xlarge":    {2, csp.EphemeralTypeSSD, true},
	"r5ad.large":    {1, csp.EphemeralTypeSSD, true},
	"r5ad.xlarge":   {1, csp.EphemeralTypeSSD, true},
	"r5ad.2xlarge":  {1, csp.EphemeralTypeSSD, true},
	"r5ad.4xlarge":  {2, csp.EphemeralTypeSSD, true},
	"r5ad.12xlarge": {2, csp.EphemeralTypeSSD, true},
	"r5ad.24xlarge": {4, csp.EphemeralTypeSSD, true},
	"r5d.large":     {1, csp.EphemeralTypeSSD, true},
	"r5d.xlarge":    {1, csp.EphemeralTypeSSD, true},
	"r5d.2xlarge":   {1, csp.EphemeralTypeSSD, true},
	"r5d.4xlarge":   {2, csp.EphemeralTypeSSD, true},
	"r5d.8xlarge":   {2, csp.EphemeralTypeSSD, true},
	"r5d.12xlarge":  {2, csp.EphemeralTypeSSD, true},
	"r5d.16xlarge":  {4, csp.EphemeralTypeSSD, true},
	"r5d.24xlarge":  {4, csp.EphemeralTypeSSD, true},
	"r5d.metal":     {4, csp.EphemeralTypeSSD, true},
	"x1.16xlarge":   {1, csp.EphemeralTypeSSD, true},
	"x1.32xlarge":   {2, csp.EphemeralTypeSSD, true},
	"x1e.xlarge":    {1, csp.EphemeralTypeSSD, true},
	"x1e.2xlarge":   {1, csp.EphemeralTypeSSD, true},
	"x1e.4xlarge":   {1, csp.EphemeralTypeSSD, true},
	"x1e.8xlarge":   {1, csp.EphemeralTypeSSD, true},
	"x1e.16xlarge":  {1, csp.EphemeralTypeSSD, true},
	"x1e.32xlarge":  {2, csp.EphemeralTypeSSD, true},
	"z1d.large":     {1, csp.EphemeralTypeSSD, true},
	"z1d.xlarge":    {1, csp.EphemeralTypeSSD, true},
	"z1d.2xlarge":   {1, csp.EphemeralTypeSSD, true},
	"z1d.3xlarge":   {1, csp.EphemeralTypeSSD, true},
	"z1d.6xlarge":   {1, csp.EphemeralTypeSSD, true},
	"z1d.12xlarge":  {2, csp.EphemeralTypeSSD, true},
	"z1d.metal":     {2, csp.EphemeralTypeSSD, true},
}
