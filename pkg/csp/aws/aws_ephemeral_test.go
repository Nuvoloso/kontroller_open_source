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
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	mockAWS "github.com/Nuvoloso/kontroller/pkg/awssdk/mock"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestLocalEphemeralDevices(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockAWS.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	caws.SetDebugLogger(tl.Logger())
	caws.devCache = nil
	ex := mock.NewMockExec(mockCtrl)
	caws.exec = ex
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		Timeout: time.Second * awsClientDefaultTimeoutSecs,
	}

	// fake out the instance meta data
	oldTr := http.DefaultClient.Transport
	defer func() {
		http.DefaultClient.Transport = oldTr
	}()
	ft := &fakeTransport{
		t:    t,
		data: make(map[string]string),
	}
	for _, p := range awsIMDProps {
		ft.data[p] = "value of " + p
	}
	instanceID := "i-1"
	ft.data["instance-id"] = instanceID
	http.DefaultClient.Transport = ft

	// fake readDir
	fakeReadDir := &fakeReadDirData{}
	origReadDirHook := readDirHook
	readDirHook = func(dirName string) ([]os.FileInfo, error) {
		fakeReadDir.inDirName = dirName
		return fakeReadDir.retDirList, fakeReadDir.retError
	}
	defer func() {
		readDirHook = origReadDirHook
		caws.devCache = nil
	}()

	t.Log("case: meta-data lookup fails")
	b := &bytes.Buffer{}
	log.SetOutput(b) // http client logs in this case, capture it so UT does not log
	ft.failConnect = true
	list, err := awsCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.Regexp("failed to fetch", err)
	ft.failConnect = false
	log.SetOutput(os.Stderr) // reset

	t.Log("case: no devices present for instance type")
	ft.data["instance-type"] = "t2.medium"
	list, err = awsCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("t2.medium.* no .* devices"))
	tl.Flush()

	t.Log("case: loadLocalDevices fails")
	caws.devCache = nil
	fakeReadDir.retError = errors.New("readDir")
	ft.data["instance-type"] = "m1.large"
	list, err = awsCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.Regexp("readDir", err)
	fakeReadDir.retError = nil
	tl.Flush()

	t.Log("case: lsblk fails")
	cmd := mock.NewMockCmd(mockCtrl)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "lsblk", "-J", "-b").Return(cmd).MinTimes(2)
	cmd.EXPECT().CombinedOutput().Return(nil, errors.New("blkFail"))
	list, err = awsCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.Regexp("lsblk .*failed: blkFail", err)

	t.Log("case: lsblk bad output")
	cmd.EXPECT().CombinedOutput().Return([]byte(""), nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.Regexp("lsblk .*output error", err)

	t.Log("case: lsblk empty list")
	blkResult := `{ "blockdevices": [] }`
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.NotNil(list)
	assert.Empty(list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 0.<2"))
	tl.Flush()

	t.Log("case: not disk type")
	blkResultFmt := `{
		"blockdevices": [
			{"name": "%s", "maj:min": "7:0", "rm": "0", "size": "%d", "ro": "%d", "type": "%s", "mountpoint": null}
		]
	}`
	size := int64(40256929792)
	blkResult = fmt.Sprintf(blkResultFmt, "xvdb", size, 0, "dusk")
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.NotNil(list)
	assert.Empty(list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 0.<2"))
	tl.Flush()

	t.Log("case: name not in range")
	blkResult = fmt.Sprintf(blkResultFmt, "xvdba", size, 0, "disk")
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.NotNil(list)
	assert.Empty(list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 0.<2"))
	tl.Flush()

	t.Log("case: read-only")
	blkResult = fmt.Sprintf(blkResultFmt, "xvdb", size, 1, "disk")
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	list, err = awsCl.LocalEphemeralDevices()
	expList := []*csp.EphemeralDevice{
		{Path: "/dev/xvdb", Type: "HDD", Initialized: false, Usable: false, SizeBytes: size},
	}
	assert.Equal(expList, list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 1.<2"))
	tl.Flush()

	t.Log("case: mounted")
	blkResult = fmt.Sprintf(blkResultFmt, "xvdb", size, 0, "disk")
	blkResult = strings.Replace(blkResult, "null", `"/mnt"`, 1)
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.Equal(expList, list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 1.<2"))
	tl.Flush()

	t.Log("case: invalid size")
	blkResult = fmt.Sprintf(blkResultFmt, "xvdb", 1235, 0, "disk")
	blkResult = strings.Replace(blkResult, "1235", "1abcd", 1)
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	list, err = awsCl.LocalEphemeralDevices()
	expList = []*csp.EphemeralDevice{
		{Path: "/dev/xvdb", Type: "HDD", Initialized: false, Usable: false, SizeBytes: 0},
	}
	assert.Equal(expList, list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 1.<2"))
	tl.Flush()

	t.Log("case: has children")
	blkResult = `{
		"blockdevices": [
			{"name": "xvdb", "maj:min": "7:0", "rm": "0", "size": "1234", "ro": "0", "type": "disk", "mountpoint": null,
			  "children": [
				{"name": "xvdb1", "maj:min": "202:1", "rm": "0", "size": "8588869120", "ro": "0", "type": "part", "mountpoint": "/"}
			  ]
			}
		]
	}`
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	list, err = awsCl.LocalEphemeralDevices()
	expList = []*csp.EphemeralDevice{
		{Path: "/dev/xvdb", Type: "HDD", Initialized: false, Usable: false, SizeBytes: 1234},
	}
	assert.Equal(expList, list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 1.<2"))
	tl.Flush()

	t.Log("case: blkid reports formatted device")
	blkResult = fmt.Sprintf(blkResultFmt, "xvdb", size, 0, "disk")
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	blkCmd := mock.NewMockCmd(mockCtrl)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/xvdb").Return(blkCmd)
	blkCmd.EXPECT().CombinedOutput().Return([]byte("/dev/xvdb: ID=1\n"), nil)
	list, err = awsCl.LocalEphemeralDevices()
	expList = []*csp.EphemeralDevice{
		{Path: "/dev/xvdb", Type: "HDD", Initialized: false, Usable: false, SizeBytes: size},
	}
	assert.Equal(expList, list)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 1.<2"))
	tl.Flush()

	t.Log("case: failure looking up instance")
	req := &ec2.DescribeInstancesInput{InstanceIds: []*string{&instanceID}}
	blkResult = fmt.Sprintf(blkResultFmt, "xvdb", size, 0, "disk")
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	blkCmd = mock.NewMockCmd(mockCtrl)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/xvdb").Return(blkCmd)
	blkCmd.EXPECT().CombinedOutput().Return([]byte(""), errors.New("no match"))
	mEC2 := mockAWS.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2).MinTimes(1)
	mEC2.EXPECT().DescribeInstancesWithContext(gomock.Not(gomock.Nil()), req).Return(nil, errors.New("aws Error"))
	list, err = awsCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.Regexp("instance.i-1. fetch failure.*aws Error", err)
	tl.Flush()

	t.Log("case: devices are EBS")
	dio := &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{
				Instances: []*ec2.Instance{
					{
						InstanceId: &instanceID,
						BlockDeviceMappings: []*ec2.InstanceBlockDeviceMapping{
							{DeviceName: aws.String("/dev/xvda1")},
							{DeviceName: aws.String("/dev/xvdb")},
							{DeviceName: aws.String("/dev/sdc")},
						},
					},
				},
			},
		},
	}
	blkResult = `{
		"blockdevices": [
			{"name": "xvda", "maj:min": "7:0", "rm": "0", "size": "1231", "ro": "0", "type": "disk", "mountpoint": "/"},
			{"name": "xvdb", "maj:min": "7:0", "rm": "0", "size": "1234", "ro": "0", "type": "disk", "mountpoint": null},
			{"name": "xvdc", "maj:min": "7:8", "rm": "0", "size": "2358", "ro": "0", "type": "disk", "mountpoint": null}
		]
	}`
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/xvdb", "/dev/xvdc").Return(blkCmd)
	blkCmd.EXPECT().CombinedOutput().Return([]byte("/dev/xvda: ID=1\n"), nil)
	mEC2.EXPECT().DescribeInstancesWithContext(gomock.Not(gomock.Nil()), req).Return(dio, nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.Len(list, 0)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("found 0.<2"))
	tl.Flush()

	t.Log("case: too many devices")
	dio.Reservations[0].Instances[0].BlockDeviceMappings = []*ec2.InstanceBlockDeviceMapping{
		{DeviceName: aws.String("/dev/xvda1")},
	}
	ft.data["instance-type"] = "m3.large"
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/xvdb", "/dev/xvdc").Return(blkCmd)
	blkCmd.EXPECT().CombinedOutput().Return([]byte("/dev/xvda: ID=1\n"), nil)
	mEC2.EXPECT().DescribeInstancesWithContext(gomock.Not(gomock.Nil()), req).Return(dio, nil)
	assert.Panics(func() { awsCl.LocalEphemeralDevices() })
	tl.Flush()

	t.Log("case: success")
	dio.Reservations[0].Instances[0].BlockDeviceMappings = []*ec2.InstanceBlockDeviceMapping{
		{DeviceName: aws.String("/dev/xvda1")},
	}
	ft.data["instance-type"] = "m1.large"
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/xvdb", "/dev/xvdc").Return(blkCmd)
	blkCmd.EXPECT().CombinedOutput().Return([]byte("/dev/xvda: ID=1\n"), nil)
	mEC2.EXPECT().DescribeInstancesWithContext(gomock.Not(gomock.Nil()), req).Return(dio, nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.NoError(err)
	assert.Len(list, 2)
	expList = []*csp.EphemeralDevice{
		{Path: "/dev/xvdb", Type: "HDD", Initialized: false, Usable: true, SizeBytes: 1234},
		{Path: "/dev/xvdc", Type: "HDD", Initialized: false, Usable: true, SizeBytes: 2358},
	}
	assert.Equal(expList, list)
	tl.Flush()

	t.Log("case: success with NVMe")
	assert.Empty(caws.devCache)
	caws.devCache["nvme0n1"] = &InstanceDevice{EphemeralDevice: true}
	caws.devCache["nvme1n1"] = &InstanceDevice{VolumeIdentifier: "vol1"}
	caws.devCache["nvme2n1"] = &InstanceDevice{EphemeralDevice: true}
	blkResult = `{
		"blockdevices": [
			{"name": "nvme0n1", "maj:min": "7:0", "rm": "0", "size": "1234", "ro": "0", "type": "disk", "mountpoint": null},
			{"name": "nvme1n1", "maj:min": "7:0", "rm": "0", "size": "1231", "ro": "0", "type": "disk", "mountpoint": "/"},
			{"name": "nvme2n1", "maj:min": "7:8", "rm": "0", "size": "2358", "ro": "0", "type": "disk", "mountpoint": null}
		]
	}`
	ft.data["instance-type"] = "m3.xlarge"
	cmd.EXPECT().CombinedOutput().Return([]byte(blkResult), nil)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/nvme0n1", "/dev/nvme2n1").Return(blkCmd)
	blkCmd.EXPECT().CombinedOutput().Return([]byte(""), nil)
	list, err = awsCl.LocalEphemeralDevices()
	assert.NoError(err)
	assert.Len(list, 2)
	expList = []*csp.EphemeralDevice{
		{Path: "/dev/nvme0n1", Type: "SSD", Initialized: false, Usable: true, SizeBytes: 1234},
		{Path: "/dev/nvme2n1", Type: "SSD", Initialized: false, Usable: true, SizeBytes: 2358},
	}
	assert.Equal(expList, list)
}
