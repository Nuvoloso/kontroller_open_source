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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
)

// InstanceMetaData contains instance meta data of interest to us.
type InstanceMetaData struct {
	Compute ImdCompute
	Network ImdNetwork
}

// ImdCompute contains compute metadata
type ImdCompute struct {
	Location          string // The region
	Name              string // instance name and hostname
	OsType            string // The type of OS
	ResourceGroupName string
	Tags              string
	VMSize            string
	Zone              string // Likely to be empty unless explicitly enabled
}

// ImdNetwork contains network metadata
type ImdNetwork struct {
	Interface []ImdInterface
}

// ImdInterface contains network interface metadata
type ImdInterface struct {
	IPv4 ImdIPv4
	// IPv6
	// MacAddress
}

// ImdIPv4 contains IPv4 interface metadata
type ImdIPv4 struct {
	IPAddress []ImdIPv4Address
}

// ImdIPv4Address contains IPv4 IP addresses
type ImdIPv4Address struct {
	PrivateIPAddress string
	PublicIPAddress  string
}

// Fetch loads the instance metadata from the metadata server.
// This code is based on the example in
// https://github.com/microsoft/azureimds/blob/master/imdssample.go
// and the documentation in
// https://docs.microsoft.com/en-us/azure/virtual-machines/windows/instance-metadata-service
func (imd *InstanceMetaData) Fetch(client *http.Client) error {
	req, _ := http.NewRequest("GET", "http://169.254.169.254/metadata/instance", nil)
	req.Header.Add("Metadata", "True")
	q := req.URL.Query()
	q.Add("format", "json")
	q.Add("api-version", "2019-03-11")
	req.URL.RawQuery = q.Encode()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error getting instance metadata: %s", err.Error())
	}
	defer resp.Body.Close()
	return imd.Unmarshal(resp.Body)
}

// Unmarshal loads the data structure from a JSON byte array
func (imd *InstanceMetaData) Unmarshal(r io.Reader) error {
	jd := json.NewDecoder(r)
	return jd.Decode(imd)
}

// LocalInstanceMetadata satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceMetadata() (map[string]string, error) {
	imd := &InstanceMetaData{}
	if err := imd.Fetch(c.imdGetClient()); err != nil {
		return nil, err
	}
	res := make(map[string]string)
	res[csp.IMDHostname] = imd.Compute.Name
	res[csp.IMDInstanceName] = imd.Compute.Name
	res[csp.IMDZone] = imd.Compute.Zone
	res[ImdLocation] = imd.Compute.Location
	res[ImdOsType] = imd.Compute.OsType
	res[ImdResourceGroupName] = imd.Compute.ResourceGroupName
	res[ImdTags] = imd.Compute.Tags
	res[ImdVMSize] = imd.Compute.VMSize
	if len(imd.Network.Interface) > 0 && len(imd.Network.Interface[0].IPv4.IPAddress) > 0 {
		res[csp.IMDLocalIP] = imd.Network.Interface[0].IPv4.IPAddress[0].PrivateIPAddress
	}
	return res, nil
}

// imdGetClient provides an interceptable client factory for unit testing
func (c *CSP) imdGetClient() *http.Client {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.IMDTimeout == 0 {
		c.LocalInstanceMetadataSetTimeout(azureIMDDefaultTimeoutSecs)
	}
	if c.imdClient == nil {
		c.imdClient = &http.Client{}
		c.imdClient.Timeout = c.IMDTimeout
	}
	return c.imdClient
}

// LocalInstanceMetadataSetTimeout satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceMetadataSetTimeout(secs int) {
	c.IMDTimeout, _ = time.ParseDuration(fmt.Sprintf("%ds", secs))
}

// InDomain checks that the invoker is on a node in the CSPDomain
func (c *CSP) InDomain(cspDomainAttrs map[string]models.ValueType, imd map[string]string) error {
	// AKS specific: The naming format of the AKS resource group includes the CSP domain resource group name:
	//   MC_<GroupName>_<ClusterName>_<Location>
	domGN, hasDGN := cspDomainAttrs[AttrResourceGroupName]
	imdGN, hasIGN := imd[ImdResourceGroupName]
	imdL, hasIL := imd[ImdLocation]
	if hasDGN && hasIGN && hasIL {
		fields := strings.Split(imdGN, "_")
		if len(fields) == 4 && fields[0] == "MC" && fields[1] == domGN.Value && fields[3] == imdL {
			return nil
		}
	}
	return fmt.Errorf("not in domain")
}
