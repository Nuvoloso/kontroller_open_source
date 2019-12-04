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


package azure

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/stretchr/testify/assert"
)

var imdData = `
{
	"compute": {
		"azEnvironment": "AzurePublicCloud",
		"customData": "",
		"location": "westus",
		"name": "aks-nodepool1-35868206-0",
		"offer": "aks",
		"osType": "Linux",
		"placementGroupId": "",
		"plan": {
			"name": "",
			"product": "",
			"publisher": ""
		},
		"platformFaultDomain": "0",
		"platformUpdateDomain": "0",
		"provider": "Microsoft.Compute",
		"publicKeys": [{
			"keyData": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC6u6vcI9S9XmcTL4NzHeQBdZwkeIl4BFuJIJciEQjX1pYYnK3EWvtDhKM/aTymKGxuxFezZlN6XVdITtTZXiZL57WbXTOgDHvtcKtD3jbW2rIjhbK7ELgYxLjDEACIyFbAV3sJ3HqH6ADbdSUyKziAPyW5zvRkxiaedqb0pp1K7bRK1hbY8hfLSX6SScfPVvpZny0E5uZpy7swXRD4rQJfUgZQKqUHVt3MPJ7N/EvkT7LKmK66y+r2QUUNJYZMSZwHGHtArv6ZuEmWEvMhqtrweI8uQDvOuZ1GuD39XHIfSG2uj/88oNowr/8WjTbw5XcsRtOmxwpf0quUZSlZfagX carl.braganza@nuvoloso.com\n",
			"path": "/home/azureuser/.ssh/authorized_keys"
		}],
		"publisher": "microsoft-aks",
		"resourceGroupName": "MC_MyAksResourceGroup_MyCluster_westus",
		"resourceId": "/subscriptions/1e00899f-bd58-4e88-ae7e-0592630e34b7/resourceGroups/MC_MyAksResourceGroup_MyCluster_westus/providers/Microsoft.Compute/virtualMachines/aks-nodepool1-35868206-0",
		"sku": "aks-ubuntu-1604-201909",
		"subscriptionId": "1e00899f-bd58-4e88-ae7e-0592630e34b7",
		"tags": "aksEngineVersion:v0.40.2-aks;creationSource:aks-aks-nodepool1-35868206-0;orchestrator:Kubernetes:1.13.10;poolName:nodepool1;resourceNameSuffix:35868206",
		"version": "2019.09.25",
		"vmId": "8a0e6412-8fee-4c56-b8be-11adeb3203c8",
		"vmScaleSetName": "",
		"vmSize": "Standard_DS2_v2",
		"zone": ""
	},
	"network": {
		"interface": [{
			"ipv4": {
				"ipAddress": [{
					"privateIpAddress": "10.240.0.4",
					"publicIpAddress": "13.64.119.43"
				}],
				"subnet": [{
					"address": "10.240.0.0",
					"prefix": "16"
				}]
			},
			"ipv6": {
				"ipAddress": []
			},
			"macAddress": "000D3A5A07DF"
		}]
	}
}`

// fakeTransport is a RoundTripper and a ReadCloser
type fakeTransport struct {
	t           *testing.T
	req         *http.Request
	data        string
	failConnect bool
	failBody    bool
}

func (tr *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.req = req
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
	resp.Body = ioutil.NopCloser(strings.NewReader(tr.data))
	return resp, nil
}

func (tr *fakeTransport) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("fake read error")
}

func (tr *fakeTransport) Close() error {
	return nil
}
func TestInstanceMetadata(t *testing.T) {
	assert := assert.New(t)

	imd := &InstanceMetaData{}

	ft := &fakeTransport{
		t:    t,
		data: imdData,
	}
	client := &http.Client{}
	client.Transport = ft

	t.Log("fetch success")
	err := imd.Fetch(client)
	assert.NoError(err)
	assert.Equal("westus", imd.Compute.Location)
	assert.Equal("aks-nodepool1-35868206-0", imd.Compute.Name)
	assert.Equal("Linux", imd.Compute.OsType)
	assert.Equal("MC_MyAksResourceGroup_MyCluster_westus", imd.Compute.ResourceGroupName)
	assert.Equal("aksEngineVersion:v0.40.2-aks;creationSource:aks-aks-nodepool1-35868206-0;orchestrator:Kubernetes:1.13.10;poolName:nodepool1;resourceNameSuffix:35868206", imd.Compute.Tags)
	assert.Equal("Standard_DS2_v2", imd.Compute.VMSize)
	assert.Equal("", imd.Compute.Zone)
	assert.Len(imd.Network.Interface, 1)
	assert.Len(imd.Network.Interface[0].IPv4.IPAddress, 1)
	assert.Equal("10.240.0.4", imd.Network.Interface[0].IPv4.IPAddress[0].PrivateIPAddress)
	assert.Equal("13.64.119.43", imd.Network.Interface[0].IPv4.IPAddress[0].PublicIPAddress)

	assert.Equal("http://169.254.169.254/metadata/instance?api-version=2019-03-11&format=json", ft.req.URL.String())
	assert.Contains(ft.req.Header, "Metadata")
	assert.Equal("True", ft.req.Header.Get("Metadata"))

	t.Log("external interface success")
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)
	azCsp, ok := c.(*CSP)
	assert.NotNil(c)
	assert.True(ok)
	defer func() {
		azCsp.imdClient = nil
	}()
	azCsp.imdClient = client
	res, err := c.LocalInstanceMetadata()
	assert.NoError(err)
	assert.NotNil(res)
	assert.Equal(imd.Compute.Name, res[csp.IMDHostname])
	assert.Equal(imd.Compute.Name, res[csp.IMDInstanceName])
	assert.Equal(imd.Compute.Zone, res[csp.IMDZone])
	assert.Equal(imd.Compute.Location, res[ImdLocation])
	assert.Equal(imd.Compute.OsType, res[ImdOsType])
	assert.Equal(imd.Compute.ResourceGroupName, res[ImdResourceGroupName])
	assert.Equal(imd.Compute.Tags, res[ImdTags])
	assert.Equal(imd.Compute.VMSize, res[ImdVMSize])

	t.Log("external interface client failure")
	ft.failConnect = true
	res, err = c.LocalInstanceMetadata()
	assert.Error(err)

	t.Log("client factory")
	azCsp.imdClient = nil
	azCsp.IMDTimeout = 0
	hc := azCsp.imdGetClient()
	assert.NotNil(hc)
	assert.Equal(time.Second*azureIMDDefaultTimeoutSecs, azCsp.IMDTimeout)
	assert.Equal(azCsp.IMDTimeout, hc.Timeout)

	// InDomain checks
	imd1 := map[string]string{}
	dom1 := map[string]models.ValueType{}
	err = c.InDomain(dom1, imd1)
	assert.NotNil(err)

	dom2 := map[string]models.ValueType{
		AttrResourceGroupName: models.ValueType{Kind: "STRING", Value: "ResourceGroupName"},
	}
	imd2 := map[string]string{
		ImdResourceGroupName: "MC_ResourceGroupName_ClusterName_westus",
		ImdLocation:          "westus",
	}
	err = c.InDomain(dom2, imd2)
	assert.Nil(err)

	// resource group name is a cluster IMD property
	assert.True(strings.HasPrefix(ImdResourceGroupName, csp.IMDProvisioningPrefix))
}
