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
	"bytes"
	"errors"
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
	"github.com/stretchr/testify/assert"
)

func TestAttributes(t *testing.T) {
	assert := assert.New(t)

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)

	attrs := c.Attributes()
	assert.NotNil(attrs)
	assert.Equal(gcAttributes, attrs)

	attrs = c.CredentialAttributes()
	assert.NotNil(attrs)
	assert.Equal(credentialAttributesNames, util.SortedStringKeys(attrs))

	attrs = c.DomainAttributes()
	assert.NotNil(attrs)
	sort.Strings(domainAttributesNames)
	assert.Equal(domainAttributesNames, util.SortedStringKeys(attrs))
}

func TestLocalInstanceMetaData(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ft := &fakeTransport{
		t:    t,
		data: make(map[string]string),
	}
	ft.data[""] = goodData

	expProps := make(map[string]string)
	expProps["Hostname"] = "gke-dave-gce-default-pool-79e37a5e-dk97"
	expProps["InstanceName"] = "gke-dave-gce-default-pool-79e37a5e-dk97"
	expProps["LocalIP"] = "10.138.0.6"
	expProps["Zone"] = "us-west1-b"
	expProps["cluster-name"] = "dave-gce"
	expProps["id"] = "gke-dave-gce-default-pool-79e37a5e-dk97"
	expProps["local-hostname"] = "gke-dave-gce-default-pool-79e37a5e-dk97.c.igneous-river-254715.internal"
	expProps["machine-type"] = "n1-standard-2"
	expProps["project-number"] = "1064483090268"
	expProps["public-ipv4"] = "34.83.52.28"

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)
	cGC, ok := c.(*CSP)
	assert.NotNil(c)
	assert.True(ok)
	c.SetDebugLogger(tl.Logger())
	oldTr := cGC.imdClient.Transport
	defer func() {
		cGC.imdClient.Transport = oldTr
	}()
	cGC.imdClient.Transport = ft

	res, err := c.LocalInstanceMetadata()
	assert.Nil(err)
	assert.NotNil(res)
	assert.Equal(expProps, res)

	ft.data[""] = partialData
	expProps = make(map[string]string)
	expProps["Hostname"] = "gke-dave-gce-default-pool-79e37a5e-dk97"
	expProps["InstanceName"] = "gke-dave-gce-default-pool-79e37a5e-dk97"
	expProps["LocalIP"] = "10.138.0.6"
	expProps["id"] = "gke-dave-gce-default-pool-79e37a5e-dk97"
	expProps["local-hostname"] = "gke-dave-gce-default-pool-79e37a5e-dk97.c.igneous-river-254715.internal"
	res, err = c.LocalInstanceMetadata()
	assert.Nil(err)
	assert.NotNil(res)
	assert.Equal(expProps, res)

	dur, err := time.ParseDuration(fmt.Sprintf("%ds", imdDefaultTimeoutSecs))
	assert.NoError(err)
	assert.Equal(dur, cGC.IMDTimeout)

	// emulate network failure
	b := &bytes.Buffer{}
	log.SetOutput(b) // http client logs in this case, capture it so UT does not log
	ft.failConnect = true
	res, err = c.LocalInstanceMetadata()
	ft.failConnect = false
	assert.Nil(res)
	assert.Regexp("error getting instance metadata", err)
	log.SetOutput(os.Stderr) // reset

	// emulate read failure
	ft.failBody = true
	res, err = c.LocalInstanceMetadata()
	ft.failBody = false
	assert.Nil(res)
	assert.Regexp("fake read error", err)
}

func TestInDomain(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)

	// InDomain checks
	imd1 := map[string]string{}
	dom1 := map[string]models.ValueType{}
	err = c.InDomain(dom1, imd1)
	assert.Regexp("missing or invalid", err)

	imd1 = map[string]string{
		csp.IMDZone:         "zone",
		csp.IMDInstanceName: "instance",
	}
	dom1 = map[string]models.ValueType{
		AttrZone: models.ValueType{Kind: "STRING", Value: "zone"},
	}
	err = c.InDomain(dom1, imd1)
	assert.NoError(err)

	imd1[csp.IMDZone] = "foo"
	err = c.InDomain(dom1, imd1)
	assert.Regexp("zone does not match", err)

	dom2 := map[string]models.ValueType{
		AttrZone: models.ValueType{Kind: "SECRET", Value: "foo"},
	}
	err = c.InDomain(dom2, imd1)
	assert.Regexp("missing or invalid", err)

	imd2 := map[string]string{
		csp.IMDInstanceName: "instance",
	}
	err = c.InDomain(dom1, imd2)
	assert.Regexp("missing or invalid", err)
}

func TestLocalInstanceDeviceName(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)
	c.SetDebugLogger(tl.Logger())
	name, err := c.LocalInstanceDeviceName("somevolID", "somedevice")
	assert.Regexp("unsupported", err)
	assert.Empty(name)

	name, err = c.LocalInstanceDeviceName("gcs:somevolID", "somedevice")
	assert.Regexp("unsupported", err)
	assert.Empty(name)

	// no longer attached
	name, err = c.LocalInstanceDeviceName("gce:somevolID", "somedevice")
	assert.NoError(err)
	assert.Empty(name)

	name, err = c.LocalInstanceDeviceName("gce:somevolID", "/tmp")
	assert.NoError(err)
	assert.Equal("/tmp", name)
}

func TestStorageTypes(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)

	sTs := c.SupportedCspStorageTypes()
	assert.NotNil(sTs)
	assert.Len(gcCspStorageTypes, len(sTs))
	assert.Equal(gcCspStorageTypes, sTs)
	for _, sT := range sTs {
		assert.EqualValues(CSPDomainType, sT.CspDomainType)
		if sT.AccessibilityScope == "NODE" {
			ephemeralType, ok := sT.CspStorageTypeAttributes[csp.CSPEphemeralStorageType]
			assert.True(ok)
			assert.Equal("STRING", ephemeralType.Kind)
			assert.Equal(csp.EphemeralTypeSSD, ephemeralType.Value)
		}
		devType, _ := c.GetDeviceTypeByCspStorageType(sT.Name)
		if strings.Contains(sT.CspStorageTypeAttributes[PAVolumeType].Value, "ssd") {
			assert.Equal("SSD", devType)
		} else if sT.CspStorageTypeAttributes[PAVolumeType].Value != "" {
			assert.Equal("HDD", devType)
		}
	}

	devType, err := c.GetDeviceTypeByCspStorageType("someStorageType")
	assert.Empty(devType)
	assert.Regexp("incorrect storageType", err)
}

func TestStorageFormulas(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)

	sSF := c.SupportedStorageFormulas()
	assert.NotNil(sSF)
	assert.Len(gcStorageFormulas, len(sSF))
	assert.Equal(gcStorageFormulas, sSF)
}

func TestSanitizedAttributes(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)

	gen := uuidGenerator()
	assert.True(CompatibleBucketName("nuvoloso-" + gen))

	domainAttrs := map[string]models.ValueType{}
	saveUUIDGen := uuidGenerator
	defer func() { uuidGenerator = saveUUIDGen }()
	fakeUUID := "a17e6b01-e679-4ea8-9cae-0f4be22d67a7"
	uuidGenerator = func() string { return fakeUUID }

	// Empty attributes
	attr, err := c.SanitizedAttributes(domainAttrs)
	assert.NoError(err)
	assert.NotNil(attr)
	assert.Equal("nuvoloso-a17e6b01-e679-4ea8-9cae-0f4be22d67a7", attr[AttrPStoreBucketName].Value)

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
		assert.NoError(err)
		assert.NotNil(attr)
		assert.Equal(retVal, attr[AttrPStoreBucketName].Value)
	}

	// failure cases
	tcs = []string{"12", "google", "g0ogl", "dot..dottest", "192.168.5.4", "hyphen-.dottest", "dot.-hyphentest", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggg"}
	for _, tc := range tcs {
		domainAttrs[AttrPStoreBucketName] = models.ValueType{Kind: "STRING", Value: tc}
		attr, err := c.SanitizedAttributes(domainAttrs)
		assert.Nil(attr)
		assert.Error(err)
		assert.Equal(invalidBucketNameMsg, err.Error())
	}

	// invalid type
	domainAttrs[AttrPStoreBucketName] = models.ValueType{Kind: "NOTSTRING", Value: "randomValue"}
	attr, err = c.SanitizedAttributes(domainAttrs)
	assert.Nil(attr)
	assert.Regexp("persistent Store Bucket name must be a string", err)
}

func TestTransferRate(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(c)
	rate := c.ProtectionStoreUploadTransferRate()
	assert.EqualValues(PSUploadTransferRate, rate)
	assert.NotZero(rate)
}

const goodData = `{
	"attributes": {
	  "cluster-location": "us-west1-b",
	  "cluster-name": "dave-gce"
	},
	"cpuPlatform": "Intel Broadwell",
	"description": "",
	"hostname": "gke-dave-gce-default-pool-79e37a5e-dk97.c.igneous-river-254715.internal",
	"id": 9071225730288193806,
	"machineType": "projects/1064483090268/machineTypes/n1-standard-2",
	"name": "gke-dave-gce-default-pool-79e37a5e-dk97",
	"networkInterfaces": [
	  {
		"accessConfigs": [
		  {
			"externalIp": "34.83.52.28",
			"type": "ONE_TO_ONE_NAT"
		  }
		],
		"dnsServers": [
		  "169.254.169.254"
		],
		"forwardedIps": [
		  "35.203.131.52"
		],
		"gateway": "10.138.0.1",
		"ip": "10.138.0.6",
		"ipAliases": [
		  "10.44.0.0/24"
		],
		"mac": "42:01:0a:8a:00:06",
		"mtu": 1460,
		"network": "projects/1064483090268/networks/default",
		"subnetmask": "255.255.240.0",
		"targetInstanceIps": []
	  }
	],
	"zone": "projects/1064483090268/zones/us-west1-b"
  }`

const partialData = `{
	"attributes": {},
	"hostname": "gke-dave-gce-default-pool-79e37a5e-dk97.c.igneous-river-254715.internal",
	"id": 9071225730288193806,
	"machineType": "not-parsable",
	"name": "gke-dave-gce-default-pool-79e37a5e-dk97",
	"networkInterfaces": [
	  {
		"dnsServers": [
		  "169.254.169.254"
		],
		"forwardedIps": [
		  "35.203.131.52"
		],
		"gateway": "10.138.0.1",
		"ip": "10.138.0.6",
		"ipAliases": [
		  "10.44.0.0/24"
		],
		"mac": "42:01:0a:8a:00:06",
		"mtu": 1460,
		"network": "projects/1064483090268/networks/default",
		"subnetmask": "255.255.240.0",
		"targetInstanceIps": []
	  }
	],
	"zone": "not-parsable"
  }`

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
		return resp, errors.New("failed")
	}
	if tr.failBody {
		resp.Body = tr
		return resp, nil
	}
	if req.Header.Get("Metadata-Flavor") != "Google" {
		resp.StatusCode = http.StatusUnauthorized
		return resp, errors.New("denied")
	}
	if req.URL.String() != imdQuery {
		resp.StatusCode = http.StatusForbidden
		return resp, errors.New("not found")
	}
	resp.Body = ioutil.NopCloser(strings.NewReader(tr.data[""]))
	return resp, nil
}

func (tr *fakeTransport) Read(p []byte) (n int, err error) {
	return 0, errors.New("fake read error")
}

func (tr *fakeTransport) Close() error {
	return nil
}
