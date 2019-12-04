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

// Adapted from https://github.com/travisjeffery/go-ec2-metadata, MIT license

package aws

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/util/mock"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

// fakeTransport is a RoundTripper and a ReadCloser
type fakeTransport struct {
	t           *testing.T
	data        map[string]string
	failConnect bool
	failBody    bool
}

func (tr *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		Header:     make(http.Header),
		Request:    req,
		StatusCode: http.StatusOK,
	}
	if tr.failConnect {
		resp.StatusCode = http.StatusBadGateway
		return resp, fmt.Errorf("failed")
	}
	if tr.failBody {
		resp.Body = tr
		return resp, nil
	}
	path := strings.Replace(req.URL.Path, awsBasePath, "", 1)
	tr.t.Log("path:", path, tr.data[path])
	resp.Body = ioutil.NopCloser(strings.NewReader(tr.data[path]))
	return resp, nil
}

func (tr *fakeTransport) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("fake read error")
}

func (tr *fakeTransport) Close() error {
	return nil
}

func TestAWSAttributes(t *testing.T) {
	assert := assert.New(t)

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)

	attrs := c.Attributes()
	assert.NotNil(attrs)
	assert.Equal(awsAttributes, attrs)

	attrs = c.CredentialAttributes()
	assert.NotNil(attrs)
	assert.Equal(credentialAttributesNames, util.SortedStringKeys(attrs))

	attrs = c.DomainAttributes()
	assert.NotNil(attrs)
	sort.Strings(domainAttributesNames)
	assert.Equal(domainAttributesNames, util.SortedStringKeys(attrs))
}

func TestAWSInstanceMetaData(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

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
	http.DefaultClient.Transport = ft

	expProps := make(map[string]string)
	for _, p := range awsIMDProps {
		v := "value of " + p
		if n, ok := awsIMDRenamedProps[p]; ok {
			expProps[n] = v
		} else {
			expProps[p] = v
		}
	}

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)
	c.SetDebugLogger(tl.Logger())

	res, err := c.LocalInstanceMetadata()
	assert.Nil(err)
	assert.NotNil(res)
	assert.Equal(expProps, res)

	dur, err := time.ParseDuration(fmt.Sprintf("%ds", awsIMDDefaultTimeoutSecs))
	assert.Nil(err)
	assert.Equal(dur, http.DefaultClient.Timeout)

	// emulate network failure
	b := &bytes.Buffer{}
	log.SetOutput(b) // http client logs in this case, capture it so UT does not log
	ft.failConnect = true
	res, err = c.LocalInstanceMetadata()
	ft.failConnect = false
	assert.Nil(res)
	assert.Regexp("failed to fetch AWS instance meta-data", err)
	log.SetOutput(os.Stderr) // reset

	// emulate read failure
	ft.failBody = true
	res, err = c.LocalInstanceMetadata()
	ft.failBody = false
	assert.Nil(res)
	assert.Regexp("failed to fetch AWS instance meta-data", err)

	// InDomain checks
	imd1 := map[string]string{
		csp.IMDZone:         "zone",
		csp.IMDInstanceName: "instance",
	}
	dom1 := map[string]models.ValueType{
		AttrAvailabilityZone: models.ValueType{Kind: "STRING", Value: "zone"},
	}
	err = c.InDomain(dom1, imd1)
	assert.Nil(err)

	imd1[csp.IMDZone] = "foo"
	err = c.InDomain(dom1, imd1)
	assert.Regexp("zone does not match", err)

	dom2 := map[string]models.ValueType{
		AttrAvailabilityZone: models.ValueType{Kind: "SECRET", Value: "foo"},
	}
	err = c.InDomain(dom2, imd1)
	assert.Regexp("missing or invalid", err)

	imd2 := map[string]string{
		csp.IMDInstanceName: "instance",
	}
	err = c.InDomain(dom1, imd2)
	assert.Regexp("missing or invalid", err)

	dom3 := map[string]models.ValueType{
		AttrRegion: models.ValueType{Kind: "SECRET", Value: "us-west"},
	}
	err = c.InDomain(dom3, imd1)
	assert.Regexp("missing or invalid", err)
}

func TestLocalInstanceDeviceName(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)
	caws, ok := c.(*CSP)
	assert.NotNil(c)
	assert.True(ok)
	caws.SetDebugLogger(tl.Logger())
	caws.devCache = nil

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	caws.exec = ex

	assert.False(caws.hasNVMeStorage(), "nil devCache should not call loadLocalDevices")
	caws.devCache = map[string]*InstanceDevice{}
	assert.False(caws.hasNVMeStorage(), "empty cache means no NVMe present")
	assert.NoError(caws.loadLocalDevices(false), "empty cache means no NVMe present")
	assert.False(caws.isLocalNVMeDeviceEphemeral("anything"))
	tl.Flush()

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

	t.Log("case: loadLocalDevices calls loadLocalDevicesLocked, error")
	caws.devCache = nil
	fakeReadDir.retError = fmt.Errorf("readDir")
	err = caws.loadLocalDevices(false)
	assert.Regexp(err, "readDir")
	assert.Equal(1, tl.CountPattern("readDir"))
	assert.Equal("/dev", fakeReadDir.inDirName)
	assert.Nil(caws.devCache)
	tl.Flush()

	t.Log("case: isLocalNVMeDeviceEphemeral returns false before cache is loaded")
	ok = caws.isLocalNVMeDeviceEphemeral("nvme0n1")
	assert.False(ok)

	t.Log("case: loadLocalDevices calls loadLocalDevicesLocked, success")
	fakeReadDir.inDirName = ""
	retDirList := []os.FileInfo{
		&fakeFileInfo{"fd"},
		&fakeFileInfo{"nvme0"},
		&fakeFileInfo{"nvme0n1"},
		&fakeFileInfo{"nvme0n1p1"},
		&fakeFileInfo{"nvme1"},
		&fakeFileInfo{"nvme1n1"},
		&fakeFileInfo{"tty"},
		&fakeFileInfo{"zero"},
	}
	fakeReadDir.retDirList, fakeReadDir.retError = retDirList, nil
	nvmeResultEBS := `{ "sn" : "vol03d2e01880eeda537", "mn" : "Amazon Elastic Block Store              " }`
	nvmeResultEph := `{ "sn" : "AWS77F34C9C7BA556331", "mn" : "Amazon EC2 NVMe Instance Storage        " }`
	cmd := mock.NewMockCmd(mockCtrl)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme0n1", "-o", "json").Return(cmd)
	call1 := cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEBS), nil)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme1n1", "-o", "json").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEph), nil).After(call1)
	err = caws.loadLocalDevices(false)
	assert.NoError(err)
	assert.Len(caws.devCache, 2)
	dev, _ := caws.devCache["nvme0n1"]
	dev0 := &InstanceDevice{VolumeIdentifier: "vol-03d2e01880eeda537"}
	assert.Equal(dev0, dev)
	dev, _ = caws.devCache["nvme1n1"]
	dev1 := &InstanceDevice{EphemeralDevice: true}
	assert.Equal(dev1, dev)
	assert.Equal("/dev", fakeReadDir.inDirName)
	assert.True(caws.hasNVMeStorage())
	tl.Flush()

	t.Log("case: isLocalNVMeDeviceEphemeral uses cache")
	assert.False(caws.isLocalNVMeDeviceEphemeral("nvme0n1"))
	assert.True(caws.isLocalNVMeDeviceEphemeral("nvme1n1"))
	assert.False(caws.isLocalNVMeDeviceEphemeral("other"))
	tl.Flush()

	t.Log("case: removeLocalDevice")
	caws.removeLocalDevice("nvme1n1")
	assert.Len(caws.devCache, 1)
	dev, _ = caws.devCache["nvme0n1"]
	assert.Equal(dev0, dev)
	caws.removeLocalDevice("nvme1n1") // call again is a no-op
	assert.Len(caws.devCache, 1)
	dev, _ = caws.devCache["nvme0n1"]
	assert.Equal(dev0, dev)
	assert.True(caws.hasNVMeStorage())
	tl.Flush()

	t.Log("case: readDir error with only EBS in cache, cache is dropped")
	fakeReadDir.retDirList, fakeReadDir.retError = nil, fmt.Errorf("readDir")
	err = caws.loadLocalDevices(true)
	assert.Regexp("readDir", err)
	assert.Nil(caws.devCache)
	tl.Flush()

	t.Log("case: nvme id-ctrl fails, non-empty cache")
	fakeReadDir.retDirList, fakeReadDir.retError = retDirList, nil
	caws.devCache = map[string]*InstanceDevice{}
	caws.devCache["nvme0n1"] = &InstanceDevice{VolumeIdentifier: dev0.VolumeIdentifier}
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme1n1", "-o", "json").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, fmt.Errorf("nvmeErr"))
	err = caws.loadLocalDevices(false)
	assert.Regexp("nvmeErr", err)
	assert.Len(caws.devCache, 1)
	dev, _ = caws.devCache["nvme0n1"]
	assert.Equal(dev0, dev)
	assert.Equal(1, tl.CountPattern("Failed to identify device"))
	tl.Flush()

	t.Log("case: reloadEBS success")
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme0n1", "-o", "json").Return(cmd)
	call1 = cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEBS), nil)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme1n1", "-o", "json").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEph), nil).After(call1)
	assert.NoError(caws.loadLocalDevices(true))
	assert.Len(caws.devCache, 2)
	tl.Flush()

	t.Log("case: findLocalNVMeDeviceByID success")
	name, err := caws.findLocalNVMeDeviceByID(dev0.VolumeIdentifier)
	assert.Equal("nvme0n1", name)
	assert.NoError(err)
	tl.Flush()

	t.Log("case: nvme id-ctrl fails, nil devCache")
	caws.devCache = nil
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme0n1", "-o", "json").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, fmt.Errorf("nvmeErr"))
	err = caws.loadLocalDevices(false)
	assert.Regexp("nvmeErr", err)
	assert.Nil(caws.devCache)
	tl.Flush()

	t.Log("case: no NVMe")
	noNVMeDirList := []os.FileInfo{
		&fakeFileInfo{"fd"},
		&fakeFileInfo{"xvda1"},
		&fakeFileInfo{"xvdba"},
		&fakeFileInfo{"tty"},
		&fakeFileInfo{"zero"},
	}
	fakeReadDir.retDirList = noNVMeDirList
	assert.NoError(caws.loadLocalDevices(false))
	assert.NotNil(caws.devCache)
	assert.Len(caws.devCache, 0)
	assert.Equal(1, tl.CountPattern("no NVMe devices"))
	tl.Flush()

	t.Log("case: findLocalNVMeDeviceByID and no NVMe")
	assert.False(caws.hasNVMeStorage())
	name, err = caws.findLocalNVMeDeviceByID(dev0.VolumeIdentifier)
	assert.NoError(err)
	assert.Empty(name)
	tl.Flush()

	t.Log("case: findLocalNVMeDeviceByID error during pass 1")
	caws.devCache = nil
	fakeReadDir.retDirList = retDirList
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme0n1", "-o", "json").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEBS)[:20], nil) // force json parse error path
	name, err = caws.findLocalNVMeDeviceByID(dev0.VolumeIdentifier)
	assert.Empty(name)
	assert.Regexp("output error", err)
	tl.Flush()

	t.Log("case: findLocalNVMeDeviceByID error during pass 2")
	unknownResult := `{ "sn": "sn0", "mn": "Unknown Model" }`
	caws.devCache = nil
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme0n1", "-o", "json").Return(cmd).Times(2)
	call1 = cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEBS), nil)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme1n1", "-o", "json").Return(cmd)
	call1 = cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEph), nil).After(call1)
	call1 = cmd.EXPECT().CombinedOutput().Return([]byte(unknownResult), nil).After(call1)
	name, err = caws.findLocalNVMeDeviceByID(dev0.VolumeIdentifier + "x") // force no match
	assert.Empty(name)
	assert.Regexp("unsupported mn", err)
	assert.True(caws.hasNVMeStorage())
	tl.Flush()

	t.Log("case: LocalInstanceDeviceName bad volID")
	name, err = c.LocalInstanceDeviceName("hi:there", "/dev/tty")
	assert.Empty(name)
	assert.Regexp("type.*unsupported", err)
	tl.Flush()

	t.Log("case: LocalInstanceDeviceName error")
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme0n1", "-o", "json").Return(cmd)
	unknownResult = `{ "sn" : "vox03d2e01880eeda537", "mn" : "Amazon Elastic Block Store              " }`
	cmd.EXPECT().CombinedOutput().Return([]byte(unknownResult), nil)
	name, err = c.LocalInstanceDeviceName("ec2:"+dev0.VolumeIdentifier, "/dev/tty")
	assert.Empty(name)
	assert.Regexp("unsupported mn", err)
	tl.Flush()

	t.Log("case: LocalInstanceDeviceName not found")
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "nvme", "id-ctrl", "/dev/nvme0n1", "-o", "json").Return(cmd).Times(2)
	cmd.EXPECT().CombinedOutput().Return([]byte(nvmeResultEBS), nil).Times(2)
	name, err = c.LocalInstanceDeviceName("ec2:"+dev0.VolumeIdentifier+"x", "/dev/tty")
	assert.Empty(name)
	assert.Regexp("not found:", err)
	tl.Flush()

	t.Log("case: LocalInstanceDeviceName success")
	name, err = c.LocalInstanceDeviceName("ec2:"+dev0.VolumeIdentifier, "/dev/xvdba")
	assert.Equal("/dev/nvme0n1", name)
	assert.NoError(err)
	tl.Flush()

	t.Log("case: LocalInstanceDeviceName no NVMe")
	caws.devCache = map[string]*InstanceDevice{}
	name, err = c.LocalInstanceDeviceName("ec2:"+dev0.VolumeIdentifier, "/dev/xvdba")
	assert.Equal("/dev/xvdba", name)
	assert.NoError(err)

	t.Log("case: LocalInstanceDeviceName no NVMe with remapping")
	caws.devCache = map[string]*InstanceDevice{}
	name, err = c.LocalInstanceDeviceName("ec2:"+dev0.VolumeIdentifier, "/dev/sda1")
	assert.Equal("/dev/xvda1", name)
	assert.NoError(err)
}

func TestAWSStorageTypes(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)

	sTs := c.SupportedCspStorageTypes()
	assert.NotNil(sTs)
	assert.Len(awsCspStorageTypes, len(sTs))
	assert.Equal(awsCspStorageTypes, sTs)
	for _, sT := range sTs {
		assert.EqualValues(CSPDomainType, sT.CspDomainType)
		if sT.AccessibilityScope == "NODE" {
			ephemeralType, ok := sT.CspStorageTypeAttributes[csp.CSPEphemeralStorageType]
			assert.True(ok)
			assert.Equal("STRING", ephemeralType.Kind)
			assert.Contains([]string{csp.EphemeralTypeHDD, csp.EphemeralTypeSSD}, ephemeralType.Value)
		}
		devType, _ := c.GetDeviceTypeByCspStorageType(sT.Name)
		if util.Contains([]string{"gp2", "io1"}, sT.CspStorageTypeAttributes[PAEC2VolumeType].Value) {
			assert.Equal("SSD", devType)
		} else if util.Contains([]string{"st1", "sc1"}, sT.CspStorageTypeAttributes[PAEC2VolumeType].Value) {
			assert.Equal("HDD", devType)
		}
	}
	_, err = c.GetDeviceTypeByCspStorageType("BAD_STORAGE_TYPE")
	assert.Regexp("incorrect storageType BAD_STORAGE_TYPE", err)
}

func TestAWSStorageFormulas(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)

	sSF := c.SupportedStorageFormulas()
	assert.NotNil(sSF)
	assert.Len(awsStorageFormulas, len(sSF))
	assert.Equal(awsStorageFormulas, sSF)
	for _, sF := range sSF {
		assert.Regexp("^AWS", sF.Name)
	}
}

func TestSanitizedAttributes(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	domainAttrs := map[string]models.ValueType{}
	saveUUIDGen := uuidGenerator
	defer func() { uuidGenerator = saveUUIDGen }()
	fakeUUID := "a17e6b01-e679-4ea8-9cae-0f4be22d67a7"
	uuidGenerator = func() string { return fakeUUID }

	// Empty attributes
	attr, err := c.SanitizedAttributes(domainAttrs)
	assert.Nil(err)
	assert.NotNil(attr)
	assert.Equal("nuvoloso.a17e6b01-e679-4ea8-9cae-0f4be22d67a7", attr[AttrPStoreBucketName].Value)

	// success cases
	tcs := []string{"", "bucketname", "bucket.name", "bucket-name", "123-bucket", "123", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffggg"}
	var retVal string
	for _, tc := range tcs {
		switch tc {
		case "":
			retVal = fmt.Sprintf(protectionStoreNameFormat, fakeUUID)
		default:
			domainAttrs[AttrPStoreBucketName] = models.ValueType{Kind: "STRING", Value: tc}
			retVal = tc
		}
		attr, err := c.SanitizedAttributes(domainAttrs)
		assert.Nil(err)
		assert.NotNil(attr)
		assert.Equal(retVal, attr[AttrPStoreBucketName].Value)
	}

	// failure cases
	tcs = []string{"12", "dot..dottest", "192.168.5.4", "hyphen-.dottest", "dot.-hyphentest", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggg"}
	for _, tc := range tcs {
		domainAttrs[AttrPStoreBucketName] = models.ValueType{Kind: "STRING", Value: tc}
		attr, err := c.SanitizedAttributes(domainAttrs)
		assert.NotNil(err)
		assert.Nil(attr)
		assert.Equal(awsInvalidBucketNameMsg, err.Error())
	}

	// invalid type
	domainAttrs[AttrPStoreBucketName] = models.ValueType{Kind: "NOTSTRING", Value: "randomValue"}
	attr, err = c.SanitizedAttributes(domainAttrs)
	assert.Nil(attr)
	assert.Regexp("persistent store bucket name must be a string", err)
}

func TestTransferRate(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)
	rate := c.ProtectionStoreUploadTransferRate()
	assert.EqualValues(S3UploadTransferRate, rate)
	assert.NotZero(rate)
}

func TestUUIDGenerator(t *testing.T) {
	assert := assert.New(t)

	uuidS := uuidGenerator()
	assert.NotEmpty(uuidS)
	uuidV, err := uuid.FromString(uuidS)
	assert.NoError(err)
	assert.Equal(uuidS, uuidV.String())
}

type fakeReadDirData struct {
	inDirName  string
	retDirList []os.FileInfo
	retError   error
}

// fakeFileInfo implements os.FileInfo
type fakeFileInfo struct {
	name string
}

func (f *fakeFileInfo) Name() string {
	return f.name
}

func (f *fakeFileInfo) Size() int64 {
	return 0
}

func (f *fakeFileInfo) Mode() os.FileMode {
	return os.ModeDevice
}

func (f *fakeFileInfo) ModTime() time.Time {
	return time.Now()
}

func (f *fakeFileInfo) IsDir() bool {
	return false
}

func (f *fakeFileInfo) Sys() interface{} {
	return nil
}
