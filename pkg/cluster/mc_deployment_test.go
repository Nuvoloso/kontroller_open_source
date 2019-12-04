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
	"encoding/base64"
	"fmt"
	"reflect"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestMCTemplateProcessor(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	tCACert := base64.StdEncoding.EncodeToString([]byte("ca cert"))
	tAgentdCert := base64.StdEncoding.EncodeToString([]byte("agentd cert"))
	tAgentdKey := base64.StdEncoding.EncodeToString([]byte("agentd key"))
	tClusterdCert := base64.StdEncoding.EncodeToString([]byte("clusterd cert"))
	tClusterdKey := base64.StdEncoding.EncodeToString([]byte("clusterd key"))
	tImagePullSecret := base64.StdEncoding.EncodeToString([]byte("image pull secret"))

	tA := &MCDeploymentArgs{
		AgentdCert:     tAgentdCert,
		AgentdKey:      tAgentdKey,
		CACert:         tCACert,
		CSPDomainID:    "cspDomainID",
		CSPDomainType:  "cspDomainType",
		ClusterID:      "clusterID",
		ClusterName:    "clusterName",
		ClusterType:    "mesos",
		ClusterdCert:   tClusterdCert,
		ClusterdKey:    tClusterdKey,
		ImagePath:      "path",
		ImageTag:       "v1",
		ManagementHost: "managementHost",
		SystemID:       "systemID",
	}
	assert.Error(tA.Validate())

	tA.ClusterType = ""
	assert.NoError(tA.Validate())

	savedMCTProc := defaultMCTemplateProcessor
	saveMCTemplateDir := mcTemplateDir
	defer func() {
		mcTemplateDir = saveMCTemplateDir
		defaultMCTemplateProcessor = savedMCTProc
	}()

	var err error

	mcTemplateDir = "*-*" // bad pattern
	tProc := GetMCDeployer(tl.Logger())
	assert.NotNil(tProc)
	assert.NotNil(defaultMCTemplateProcessor)
	assert.Equal(defaultMCTemplateProcessor, tProc)
	tp, ok := tProc.(*mcTemplateProcessor)
	assert.True(ok)
	assert.NotNil(tp)
	assert.Nil(tp.tt)
	err = tp.init()
	assert.Error(err)
	assert.Regexp("matches no files", err)
	assert.Nil(tp.tt)

	mcTemplateDir = "./testdata"
	defaultMCTemplateProcessor = nil
	tProc = GetMCDeployer(tl.Logger())
	assert.NotNil(tProc)
	assert.NotNil(defaultMCTemplateProcessor)
	assert.Equal(defaultMCTemplateProcessor, tProc)
	tp, ok = tProc.(*mcTemplateProcessor)
	assert.True(ok)
	assert.NotNil(tp)
	assert.Nil(tp.tt)
	err = tp.init()
	assert.NoError(err)

	expNames := []string{"mc.yaml.tmpl", "mc-csp.yaml.tmpl", "mc-badcall.yaml.tmpl", "mc-missingkey.yaml.tmpl"}
	foundNames := []string{}
	for _, p := range tp.tt.Templates() {
		n := p.Name()
		tl.Logger().Info("TemplateFile:", n)
		foundNames = append(foundNames, n)
	}
	assert.Len(foundNames, len(expNames))
	for _, n := range foundNames {
		assert.Contains(expNames, n)
	}
	assert.NotContains(foundNames, "invalid.yaml.tmpl")

	tmpl := tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("mc.yaml.tmpl", tmpl.Name())
	dep, err := tProc.GetMCDeployment(tA)
	assert.Nil(err)
	assert.NotNil(dep)
	assert.Equal("yaml", dep.Format)
	assert.Regexp("File=mc.yaml", dep.Deployment)
	assert.Regexp("arch="+tA.Arch, dep.Deployment)
	assert.Regexp("clusterID="+tA.ClusterID, dep.Deployment)
	assert.Regexp("clusterType="+tA.ClusterType, dep.Deployment)
	assert.Regexp("cspDomainID="+tA.CSPDomainID, dep.Deployment)
	assert.Regexp("cspDomainType="+tA.CSPDomainType, dep.Deployment)
	assert.Regexp("managementHost="+tA.ManagementHost, dep.Deployment)
	assert.Regexp("os="+tA.OS, dep.Deployment)
	assert.Regexp("caCert="+tA.CACert, dep.Deployment)
	assert.Regexp("agentdCert="+tA.AgentdCert, dep.Deployment)
	assert.Regexp("agentdKey="+tA.AgentdKey, dep.Deployment)
	assert.Regexp("clusterdCert="+tA.ClusterdCert, dep.Deployment)
	assert.Regexp("clusterdKey="+tA.ClusterdKey, dep.Deployment)
	assert.Regexp("imageTag="+tA.ImageTag, dep.Deployment)
	assert.Regexp(fmt.Sprintf("nuvoPort=%d", tA.NuvoPort), dep.Deployment)
	assert.Regexp("driverType="+tA.DriverType, dep.Deployment)

	tA.CSPDomainType = "csp"
	tA.ClusterName = "" // optional
	tmpl = tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("mc-csp.yaml.tmpl", tmpl.Name())
	dep, err = tProc.GetMCDeployment(tA)
	assert.Nil(err)
	assert.NotNil(dep)
	assert.Equal("yaml", dep.Format)
	assert.Regexp("File=mc-csp.yaml", dep.Deployment)
	assert.Regexp("arch="+tA.Arch, dep.Deployment)
	assert.Regexp("clusterID="+tA.ClusterID, dep.Deployment)
	assert.Regexp("clusterType="+tA.ClusterType, dep.Deployment)
	assert.Regexp("clusterName="+tA.ClusterName, dep.Deployment)
	assert.Regexp("cspDomainID="+tA.CSPDomainID, dep.Deployment)
	assert.Regexp("cspDomainType="+tA.CSPDomainType, dep.Deployment)
	assert.Regexp("managementHost="+tA.ManagementHost, dep.Deployment)
	assert.Regexp("os="+tA.OS, dep.Deployment)
	assert.Regexp("caCert="+tA.CACert, dep.Deployment)
	assert.Regexp("agentdCert="+tA.AgentdCert, dep.Deployment)
	assert.Regexp("agentdKey="+tA.AgentdKey, dep.Deployment)
	assert.Regexp("clusterdCert="+tA.ClusterdCert, dep.Deployment)
	assert.Regexp("clusterdKey="+tA.ClusterdKey, dep.Deployment)
	assert.Regexp("imageTag="+tA.ImageTag, dep.Deployment)

	// error case: fail because a value is not specified
	assert.Equal(20, reflect.TypeOf(*tA).NumField())
	for i := 0; i <= 12; i++ {
		var sp *string
		var ip *int
		var sv string
		var iv int
		switch i {
		case 0:
			sp = &tA.CSPDomainID
		case 1:
			sp = &tA.ManagementHost
		case 2:
			sp = &tA.CSPDomainType
		case 3:
			sp = &tA.CSPDomainID
		case 4:
			sp = &tA.CACert
		case 5:
			sp = &tA.AgentdCert
		case 6:
			sp = &tA.AgentdKey
		case 7:
			sp = &tA.ClusterdCert
		case 8:
			sp = &tA.ClusterdKey
		case 9:
			sp = &tA.ImageTag
		case 10:
			sp = &tA.SystemID
		case 11:
			sp = &tA.ImagePath
		case 12:
			sp = &tA.ClusterID
		default:
			continue
		}
		if sp != nil {
			sv = *sp
			*sp = ""
		}
		tl.Logger().Infof("error case: %d", i)
		assert.Error(tA.Validate(), "error case %d %#v", i, tA)
		tl.Flush()
		dep, err = tProc.GetMCDeployment(tA)
		assert.NotNil(err, "case: %d", i)
		assert.Nil(dep)
		assert.Regexp("invalid arguments", err.Error())
		if sp != nil {
			*sp = sv
		}
		if ip != nil {
			*ip = iv
		}
		tl.Flush()
	}

	// error case: fail because a value is not base64
	for i := 0; i <= 5; i++ {
		var sp *string
		switch i {
		case 0:
			sp = &tA.CACert
		case 1:
			sp = &tA.AgentdCert
		case 2:
			sp = &tA.AgentdKey
		case 3:
			sp = &tA.ClusterdCert
		case 4:
			sp = &tA.ClusterdKey
		case 5:
			sp = &tA.ImagePullSecret
		}
		sv := *sp
		*sp = "not base64"
		dep, err = tProc.GetMCDeployment(tA)
		assert.NotNil(err)
		assert.Nil(dep)
		assert.Regexp("invalid arguments", err.Error())
		*sp = sv
	}

	tA.ClusterVersion = "v1.12.9"
	dep, err = tProc.GetMCDeployment(tA)
	assert.NotNil(err)
	assert.Nil(dep)
	assert.Regexp("invalid arguments", err.Error())
	tA.ClusterVersion = "" // reset

	// error case: execute failure because of bad function
	tA.CSPDomainType = "badcall"
	tmpl = tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("mc-badcall.yaml.tmpl", tmpl.Name())
	dep, err = tProc.GetMCDeployment(tA)
	assert.NotNil(err)
	assert.Nil(dep)
	assert.Regexp("MissingFunc", err.Error())

	// error case: execute failure because of missing key
	tA.CSPDomainType = "missingkey"
	tmpl = tp.findTemplate(tA)
	assert.NotNil(tmpl)
	assert.Equal("mc-missingkey.yaml.tmpl", tmpl.Name())
	dep, err = tProc.GetMCDeployment(tA)
	assert.NotNil(err)
	assert.Nil(dep)
	assert.Regexp("MissingKey", err.Error())

	// error case: no fallback template
	mcTemplateDir = "./testdata/badmcdir"
	defaultMCTemplateProcessor = nil
	tProc = GetMCDeployer(tl.Logger())
	assert.NotNil(tProc)
	assert.NotNil(defaultMCTemplateProcessor)
	assert.Equal(defaultMCTemplateProcessor, tProc)
	tp, ok = tProc.(*mcTemplateProcessor)
	assert.True(ok)
	assert.NotNil(tp)
	err = tp.init()
	assert.NoError(err)

	expNames = []string{"mc-nosuchcsp.yaml.tmpl"}
	foundNames = []string{}
	for _, p := range tp.tt.Templates() {
		n := p.Name()
		tl.Logger().Info("TemplateFile:", n)
		foundNames = append(foundNames, n)
	}
	assert.Len(foundNames, len(expNames))
	for _, n := range foundNames {
		assert.Contains(expNames, n)
	}

	tA = &MCDeploymentArgs{
		AgentdCert:     tAgentdCert,
		AgentdKey:      tAgentdKey,
		Arch:           MCDeployDefaultArch,
		CACert:         tCACert,
		CSPDomainID:    "cspDomainID",
		CSPDomainType:  "cspDomainType",
		ClusterID:      "clusterID",
		ClusterType:    MCDeployDefaultClusterType,
		ClusterVersion: "v1.14.6",
		ClusterdCert:   tClusterdCert,
		ClusterdKey:    tClusterdKey,
		ImagePath:      "path",
		ImageTag:       "v1",
		ManagementHost: "managementHost",
		NuvoPort:       MCDeployDefaultNuvoPort,
		OS:             MCDeployDefaultOS,
		SystemID:       "systemID",
	}
	tmpl = tp.findTemplate(tA)
	assert.Nil(tmpl)
	dep, err = tProc.GetMCDeployment(tA)
	assert.NotNil(err)
	assert.Nil(dep)
	assert.Regexp("could not find appropriate deployment template for.*cspDomainType", err.Error())

	// point to the real deployment template
	mcTemplateDir = "../../deploy/centrald/lib/deploy"
	defaultMCTemplateProcessor = nil
	tProc = GetMCDeployer(tl.Logger())
	assert.NotNil(tProc)
	assert.NotNil(defaultMCTemplateProcessor)
	assert.Equal(defaultMCTemplateProcessor, tProc)
	tp, ok = tProc.(*mcTemplateProcessor)
	assert.True(ok)
	assert.NotNil(tp)
	assert.Nil(tp.tt)
	tl.Flush()

	// No ImagePullSecret
	tl.Logger().Info("Expand real template without ImagePullSecret")
	dt := "AWS"
	assert.NotEqual("", dt)
	tA = &MCDeploymentArgs{
		AgentdCert:     tAgentdCert,
		AgentdKey:      tAgentdKey,
		Arch:           MCDeployDefaultArch,
		CACert:         tCACert,
		CSPDomainID:    "cspDomainXyzID",
		CSPDomainType:  dt,
		ClusterID:      "clusterID",
		ClusterType:    MCDeployDefaultClusterType,
		ClusterVersion: "1.14.6",
		ClusterdCert:   tClusterdCert,
		ClusterdKey:    tClusterdKey,
		DriverType:     "csi",
		ImagePath:      "image-path",
		ImageTag:       "v1",
		ManagementHost: "managementXXXHost",
		NuvoPort:       MCDeployDefaultNuvoPort,
		OS:             MCDeployDefaultOS,
		SystemID:       "systemID",
	}
	dep, err = tProc.GetMCDeployment(tA)
	assert.Nil(err)
	assert.NotNil(dep)
	assert.NotNil(tp.tt) // lazy initialization
	tl.Logger().Info(dep.Deployment)
	assert.Equal("yaml", dep.Format)
	assert.NotRegexp("{{.ManagementHost}}", dep.Deployment)
	assert.Regexp("managementXXXHost", dep.Deployment)
	assert.NotRegexp("{{.CSPDomainID}}", dep.Deployment)
	assert.Regexp("cspDomainXyzID", dep.Deployment)
	assert.NotRegexp("{{.CSPDomainType}}", dep.Deployment)
	assert.Regexp(dt, dep.Deployment)
	assert.NotRegexp("{{.CACert}}", dep.Deployment)
	assert.NotRegexp("{{.AgentdCert}}", dep.Deployment)
	assert.NotRegexp("{{.AgentdCert}}", dep.Deployment)
	assert.NotRegexp("{{.ClusterdCert}}", dep.Deployment)
	assert.NotRegexp("{{.ClusterdCert}}", dep.Deployment)
	assert.NotRegexp("{{.SystemID}}", dep.Deployment)
	assert.NotRegexp("{{.ImageTag}}", dep.Deployment)
	assert.NotRegexp("customresourcedefinitions", dep.Deployment)
	assert.NotRegexp("cluster-driver-registrar", dep.Deployment)
	assert.NotRegexp("CSINodeInfo", dep.Deployment)
	assert.Regexp("CSIDriver", dep.Deployment)
	assert.Regexp("systemID", dep.Deployment)
	nps := fmt.Sprintf("%d", tA.NuvoPort)
	assert.NotRegexp("{{.NuvoPort}}", dep.Deployment)
	assert.Regexp(nps, dep.Deployment)
	assert.NotRegexp("{{.ImagePullSecret}}", dep.Deployment)
	assert.Regexp(tA.ImagePath+"/", dep.Deployment)
	tl.Flush()

	// With ImagePullSecret
	tl.Logger().Info("Expand real template with ImagePullSecret")
	dt = "AWS"
	assert.NotEqual("", dt)
	tA = &MCDeploymentArgs{
		AgentdCert:      tAgentdCert,
		AgentdKey:       tAgentdKey,
		Arch:            MCDeployDefaultArch,
		CACert:          tCACert,
		CSPDomainID:     "cspDomainXyzID",
		CSPDomainType:   dt,
		ClusterVersion:  "1.13.5",
		ClusterID:       "clusterID",
		ClusterType:     MCDeployDefaultClusterType,
		ClusterdCert:    tClusterdCert,
		ClusterdKey:     tClusterdKey,
		ImagePath:       "image-path",
		ImagePullSecret: tImagePullSecret,
		ImageTag:        "v1",
		ManagementHost:  "managementXXXHost",
		NuvoPort:        MCDeployDefaultNuvoPort,
		OS:              MCDeployDefaultOS,
		SystemID:        "systemID",
	}
	dep, err = tProc.GetMCDeployment(tA)
	assert.Nil(err)
	assert.NotNil(dep)
	tl.Logger().Info(dep.Deployment)
	assert.Equal("yaml", dep.Format)
	assert.NotRegexp("{{.ManagementHost}}", dep.Deployment)
	assert.Regexp("managementXXXHost", dep.Deployment)
	assert.NotRegexp("{{.CSPDomainID}}", dep.Deployment)
	assert.Regexp("cspDomainXyzID", dep.Deployment)
	assert.NotRegexp("{{.CSPDomainType}}", dep.Deployment)
	assert.Regexp(dt, dep.Deployment)
	assert.NotRegexp("{{.CACert}}", dep.Deployment)
	assert.NotRegexp("{{.AgentdCert}}", dep.Deployment)
	assert.NotRegexp("{{.AgentdCert}}", dep.Deployment)
	assert.NotRegexp("{{.ClusterdCert}}", dep.Deployment)
	assert.NotRegexp("{{.ClusterdCert}}", dep.Deployment)
	assert.NotRegexp("{{.SystemID}}", dep.Deployment)
	assert.NotRegexp("{{.ImageTag}}", dep.Deployment)
	assert.Regexp("customresourcedefinitions", dep.Deployment)
	assert.Regexp("cluster-driver-registrar", dep.Deployment)
	assert.Regexp("CSINodeInfo", dep.Deployment)
	assert.Regexp("systemID", dep.Deployment)
	assert.NotRegexp("CSIDriver", dep.Deployment)
	nps = fmt.Sprintf("%d", tA.NuvoPort)
	assert.NotRegexp("{{.NuvoPort}}", dep.Deployment)
	assert.Regexp(nps, dep.Deployment)
	assert.NotRegexp("{{.ImagePullSecret}}", dep.Deployment)
	assert.Regexp(tA.ImagePullSecret, dep.Deployment)
	assert.Regexp(tA.ImagePath+"/", dep.Deployment)
	tl.Flush()

	tl.Logger().Info("semVer1 is invalid")
	tmplProc := mcTemplateProcessor{}
	retVal, err := tmplProc.validateSemverStrings("garbage", "1.14.6", "blah")
	assert.False(retVal)
	assert.NotNil(err)

	tl.Logger().Info("semVer2 is invalid")
	retVal, err = tmplProc.validateSemverStrings("1.14.6", "garbage", "blah")
	assert.False(retVal)
	assert.NotNil(err)

	tl.Logger().Info("invalid semver comparison")
	retVal, err = tmplProc.validateSemverStrings("1.14.6", "1.13.1", "blah")
	assert.False(retVal)
	assert.NotNil(err)
	assert.Regexp("invalid semver comparison", err.Error())

	tcs := map[string]string{ // big: small
		"1.3.0":             "1.2.3",
		"1.0.0-alpha.1":     "1.0.0-alpha",
		"1.0.0-alpha.beta":  "1.0.0-alpha.1",
		"1.0.0-beta":        "1.0.0-alpha.beta",
		"1.0.0-beta.2":      "1.0.0-beta",
		"1.0.0-beta.11":     "1.0.0-beta.2",
		"1.0.0-rc.1":        "1.0.0-beta.11",
		"1.0.0":             "1.0.0-rc.1",
		"v1.3.0":            "1.2.3",
		"v1.0.0-alpha.1":    "1.0.0-alpha",
		"v1.0.0-alpha.beta": "1.0.0-alpha.1",
		"v1.0.0-beta":       "1.0.0-alpha.beta",
		"v1.0.0-beta.2":     "1.0.0-beta",
		"v1.0.0-beta.11":    "1.0.0-beta.2",
		"v1.0.0-rc.1":       "1.0.0-beta.11",
		"v1.0.0":            "1.0.0-rc.1",
	}

	for big, small := range tcs {
		// less than
		retVal, err = tmplProc.semverLessThan(small, big)
		assert.True(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverLessThan(big, small)
		assert.False(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverLessThan(small, small)
		assert.False(retVal)
		assert.Nil(err)

		// less than equal
		retVal, err = tmplProc.semverLessThanEquals(small, big)
		assert.True(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverLessThanEquals(big, small)
		assert.False(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverLessThanEquals(small, small)
		assert.True(retVal)
		assert.Nil(err)

		// Equal
		retVal, err = tmplProc.semverEquals(big, small)
		assert.False(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverEquals(small, small)
		assert.True(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverEquals(small, big)
		assert.False(retVal)
		assert.Nil(err)

		// greater than equal
		retVal, err = tmplProc.semverGreaterThanEquals(small, big)
		assert.False(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverGreaterThanEquals(big, small)
		assert.True(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverGreaterThanEquals(small, small)
		assert.True(retVal)
		assert.Nil(err)

		// greater than
		retVal, err = tmplProc.semverGreaterThan(small, big)
		assert.False(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverGreaterThan(big, small)
		assert.True(retVal)
		assert.Nil(err)

		retVal, err = tmplProc.semverGreaterThan(small, small)
		assert.False(retVal)
		assert.Nil(err)
	}
}
