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


package mgmtclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestHTTPClient(t *testing.T) {
	assert := assert.New(t)

	// specify port
	apiArgs := &APIArgs{
		Host: "host1",
		Port: 8999,
	}
	api1, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api1)
	nc1, ok := api1.(*NuvoClient)
	assert.True(ok)
	assert.Equal("http", nc1.scheme)
	assert.Equal("host1:8999", nc1.HP)
	rt, ok := nc1.Transport().(*httptransport.Runtime)
	assert.True(ok)
	consumer, ok := rt.Consumers[HTMLContentType]
	assert.True(ok)
	assert.NotNil(consumer)

	// default port
	apiArgs = &APIArgs{
		Host: "host2",
	}
	api2, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api2)
	nc2, ok := api2.(*NuvoClient)
	assert.True(ok)
	assert.Equal("http", nc2.scheme)
	assert.Equal("host2:80", nc2.HP)

	assert.NotEqual(nc1, nc2)

	// test accessor methods
	assert.Equal(nc1.API.Account, api1.Account())
	assert.Equal(nc1.API.ApplicationGroup, api1.ApplicationGroup())
	assert.Equal(nc1.API.AuditLog, api1.AuditLog())
	assert.Equal(nc1, api1.Authentication())
	assert.Equal(nc1.API.Cluster, api1.Cluster())
	assert.Equal(nc1.API.ConsistencyGroup, api1.ConsistencyGroup())
	assert.Equal(nc1.API.CspCredential, api1.CspCredential())
	assert.Equal(nc1.API.CspDomain, api1.CspDomain())
	assert.Equal(nc1.API.CspStorageType, api1.CspStorageType())
	assert.Equal(nc1.API.Metrics, api1.Metrics())
	assert.Equal(nc1.API.Node, api1.Node())
	assert.Equal(nc1.API.Pool, api1.Pool())
	assert.Equal(nc1.API.ProtectionDomain, api1.ProtectionDomain())
	assert.Equal(nc1.API.Role, api1.Role())
	assert.Equal(nc1.API.ServiceDebug, api1.ServiceDebug())
	assert.Equal(nc1.API.ServicePlan, api1.ServicePlan())
	assert.Equal(nc1.API.ServicePlanAllocation, api1.ServicePlanAllocation())
	assert.Equal(nc1.API.Slo, api1.Slo())
	assert.Equal(nc1.API.Snapshot, api1.Snapshot())
	assert.Equal(nc1.API.Storage, api1.Storage())
	assert.Equal(nc1.API.StorageFormula, api1.StorageFormula())
	assert.Equal(nc1.API.StorageRequest, api1.StorageRequest())
	assert.Equal(nc1.API.System, api1.System())
	assert.Equal(nc1.API.Task, api1.Task())
	assert.Equal(nc1.API.User, api1.User())
	assert.Equal(nc1.API.VolumeSeries, api1.VolumeSeries())
	assert.Equal(nc1.API.VolumeSeriesRequest, api1.VolumeSeriesRequest())
	assert.Equal(nc1.API.Watchers, api1.Watchers())
	assert.Equal(nc1.API.Transport, api1.Transport())
}

func TestHTTPSClient(t *testing.T) {
	assert := assert.New(t)

	// specify port
	apiArgs := &APIArgs{
		Host:              "host1",
		Port:              6000,
		TLSCACertificate:  "./ca.crt",
		TLSCertificate:    "./client.crt",
		TLSCertificateKey: "./client.key",
		TLSServerName:     "host1",
	}
	api1, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api1)
	nc1, ok := api1.(*NuvoClient)
	assert.True(ok)
	assert.Equal("https", nc1.scheme)
	assert.Equal("host1:6000", nc1.HP)
	rt, ok := nc1.Transport().(*httptransport.Runtime)
	assert.True(ok)
	consumer, ok := rt.Consumers[HTMLContentType]
	assert.True(ok)
	assert.NotNil(consumer)
	ht, ok := nc1.baseTransport.(*http.Transport)
	if assert.True(ok) && assert.NotNil(ht.TLSClientConfig) {
		assert.False(ht.TLSClientConfig.InsecureSkipVerify)
		assert.Equal("host1", ht.TLSClientConfig.ServerName)
		assert.Len(ht.TLSClientConfig.Certificates, 1)
		assert.NotNil(ht.TLSClientConfig.RootCAs)
	}

	// default port and insecure and no ca crt
	apiArgs = &APIArgs{
		Host:              "host2",
		TLSCertificate:    "./client.crt",
		TLSCertificateKey: "./client.key",
		Insecure:          true,
	}
	api2, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api2)
	nc2, ok := api2.(*NuvoClient)
	assert.True(ok)
	assert.Equal("https", nc2.scheme)
	assert.Equal("host2:443", nc2.HP)
	ht, ok = nc2.baseTransport.(*http.Transport)
	if assert.True(ok) && assert.NotNil(ht.TLSClientConfig) {
		assert.True(ht.TLSClientConfig.InsecureSkipVerify)
		assert.Empty(ht.TLSClientConfig.ServerName)
		assert.Len(ht.TLSClientConfig.Certificates, 1)
		assert.Nil(ht.TLSClientConfig.RootCAs)
	}

	// ForceTLS
	apiArgs = &APIArgs{
		Host:              "host2",
		TLSCertificate:    "./client.crt",
		TLSCertificateKey: "",
		TLSServerName:     "ignored",
		ForceTLS:          true,
		Insecure:          true,
	}
	api2, err = NewAPI(apiArgs)
	assert.NoError(err)
	assert.NotNil(api2)
	nc2, ok = api2.(*NuvoClient)
	assert.True(ok)
	assert.Equal("https", nc2.scheme)
	assert.Equal("host2:443", nc2.HP)
	ht, ok = nc2.baseTransport.(*http.Transport)
	if assert.True(ok) && assert.NotNil(ht.TLSClientConfig) {
		assert.True(ht.TLSClientConfig.InsecureSkipVerify)
		assert.Empty(ht.TLSClientConfig.ServerName)
		assert.Empty(ht.TLSClientConfig.Certificates)
		assert.Nil(ht.TLSClientConfig.RootCAs)
	}
}

func TestUnixClient(t *testing.T) {
	assert := assert.New(t)

	// specify socket
	apiArgs := &APIArgs{
		SocketPath: "./socket.sock",
	}
	api1, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api1)
	nc1, ok := api1.(*NuvoClient)
	assert.True(ok)
	assert.Equal("unix", nc1.scheme)
	assert.Equal(apiArgs.SocketPath, nc1.SocketPath)
	rt, ok := nc1.Transport().(*httptransport.Runtime)
	assert.True(ok)
	consumer, ok := rt.Consumers[HTMLContentType]
	assert.True(ok)
	assert.NotNil(consumer)
}

func TestInvalidClients(t *testing.T) {
	assert := assert.New(t)

	// invalid cases
	tcs := map[string]APIArgs{
		"missing host": APIArgs{
			Port: 80,
		},
		"missing host name": APIArgs{
			TLSCertificate:    "./client.crt",
			TLSCertificateKey: "./client.key",
		},
		"TLS certificate.*must be specified together": APIArgs{
			Host:              "someHost",
			TLSCertificateKey: "./client.key",
		},
		"TLS key.*must be specified together": APIArgs{
			Host:           "someHost",
			TLSCertificate: "./client.crt",
		},
		"no such file or directory": APIArgs{
			Host:              "someHost",
			TLSCertificate:    "./bad.crt",
			TLSCertificateKey: "./bad.key",
		},
	}
	for eS, arg := range tcs {
		api, err := NewAPI(&arg)
		assert.Nil(api)
		assert.NotNil(err)
		assert.Regexp(eS, err.Error())
	}
}

func TestLoggingHTTP(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ft := &fakeTransport{
		t:   t,
		log: tl.Logger(),
	}

	// specify port
	apiArgs := &APIArgs{
		Host:  fakeHost,
		Port:  443,
		Log:   tl.Logger(),
		Debug: true,
	}
	api, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api)
	nc, ok := api.(*NuvoClient)
	assert.True(ok)
	assert.Equal(tl.Logger(), nc.Log)
	// replace the base transport
	assert.NotNil(nc.baseTransport)
	nc.baseTransport = ft

	creator := &models.Identity{AccountID: "aid1", UserID: "uid1"}
	creatorCtx := context.WithValue(context.Background(), AuthIdentityKey{}, creator)
	clAPI := api.Cluster()
	clCa := &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: "kubernetes",
			CspDomainID: "0989d6f4-a79b-499e-837a-03188c16510d",
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			Name:              "k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680",
			ClusterIdentifier: "k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680",
		},
	}
	resp := &http.Response{
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "201 Created",
		StatusCode: http.StatusCreated,
	}
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Header["Server"] = []string{"nginx/1.12.1"}
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeRespBody))
	ft.resp = resp
	clCp := cluster.NewClusterCreateParams().WithPayload(clCa).WithContext(creatorCtx)
	_, err = clAPI.ClusterCreate(clCp)
	assert.Nil(err)
	assert.NotNil(ft.req)
	if assert.NotNil(ft.req.URL) {
		assert.Equal(fakeURL, ft.req.URL.Path)
	}
	assert.Equal("aid1", ft.req.Header.Get("X-Account"))
	assert.Equal("uid1", ft.req.Header.Get("X-User"))

	lRecs := []string{}
	var lastN uint64
	tl.Iterate(func(n uint64, rec string) {
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.True(len(lRecs) >= 3)
	assert.Equal(uint64(3), lastN)
	r := lRecs[0]
	assert.Regexp("RoundTrip: account: aid1 user: uid1", r)
	r = lRecs[1]
	for n, v := range ft.req.Header {
		tl.Logger().Info("req**", n)
		assert.Regexp(n+": "+v[0], r)
	}
	assert.Contains(r, fakeReqBody)
	r = lRecs[2]
	for n, v := range ft.resp.Header {
		tl.Logger().Info("res**", n)
		assert.Regexp(n+": "+v[0], r)
	}
	assert.Contains(r, fakeRespBody)

	// fake a transport error
	creator.AccountID, creator.UserID = "", ""
	ft.req = nil
	ft.failConnect = true
	tl.Iterate(func(n uint64, s string) { lastN = n })
	curN := lastN
	_, err = clAPI.ClusterCreate(clCp)
	assert.NotNil(err)
	assert.Regexp("failed connect", err.Error())
	assert.NotNil(ft.req)
	assert.Empty(ft.req.Header.Get("X-Account"))
	assert.Empty(ft.req.Header.Get("X-User"))
	assert.Equal(fakeURL, ft.req.URL.Path)
	lRecs = []string{}
	tl.Iterate(func(n uint64, rec string) {
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.Equal(curN+2, lastN)
	r = lRecs[curN+1]
	for n, v := range ft.req.Header {
		tl.Logger().Info("req**", n)
		assert.Regexp(n+": "+v[0], r)
	}
	assert.Contains(r, fakeReqBody)

	// redo without debug - no logging
	nc.Debug = false
	tl.Iterate(func(n uint64, s string) { lastN = n })
	curN = lastN
	_, err = clAPI.ClusterCreate(clCp)
	assert.NotNil(err)
	assert.Regexp("failed connect", err.Error())
	assert.NotNil(ft.req)
	assert.Equal(fakeURL, ft.req.URL.Path)
	lRecs = []string{}
	tl.Iterate(func(n uint64, rec string) {
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.Equal(curN, lastN)
	tl.Flush()

	// fake failure in logging request
	nc.Debug = true
	_, err = nc.RoundTrip(&http.Request{Body: ft})
	assert.NotNil(err)
	cntFakeReadErr := 0
	dumpReqOutErr := false
	lRecs = []string{}
	tl.Iterate(func(n uint64, rec string) {
		if strings.Contains(rec, "fake read error") {
			cntFakeReadErr++
		}
		if strings.Contains(rec, "DumpRequestOut") {
			dumpReqOutErr = true
		}
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.Equal(1, cntFakeReadErr)
	assert.True(dumpReqOutErr)
	tl.Flush()

	// fake failure in logging response
	resp.Body = ft
	ft.resp = resp
	ft.failConnect = false
	_, err = clAPI.ClusterCreate(clCp)
	assert.NotNil(err)
	cntFakeReadErr = 0
	dumpResErr := false
	lRecs = []string{}
	tl.Iterate(func(n uint64, rec string) {
		if strings.Contains(rec, "fake read error") {
			cntFakeReadErr++
		}
		if strings.Contains(rec, "DumpResponse") {
			dumpResErr = true
		}
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.Equal(2, cntFakeReadErr)
	assert.True(dumpResErr)
}

func TestLoggingHTTPS(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ft := &fakeTransport{
		t:   t,
		log: tl.Logger(),
	}

	// specify port
	apiArgs := &APIArgs{
		Host:              fakeHost,
		Port:              443,
		Log:               tl.Logger(),
		Debug:             true,
		TLSCertificate:    "./client.crt",
		TLSCertificateKey: "./client.key",
	}
	api, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api)
	nc, ok := api.(*NuvoClient)
	assert.True(ok)
	assert.Equal(tl.Logger(), nc.Log)
	// replace the base transport
	assert.NotNil(nc.baseTransport)
	nc.baseTransport = ft

	clAPI := api.Cluster()
	clCa := &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: "kubernetes",
			CspDomainID: "0989d6f4-a79b-499e-837a-03188c16510d",
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			Name:              "k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680",
			ClusterIdentifier: "k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680",
		},
	}
	resp := &http.Response{
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "201 Created",
		StatusCode: http.StatusCreated,
	}
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Header["Server"] = []string{"nginx/1.12.1"}
	resp.Header["X-Auth"] = []string{"new-token"} // cover path where token is updated
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeRespBody))
	ft.resp = resp
	clCp := cluster.NewClusterCreateParams().WithPayload(clCa)
	_, err = clAPI.ClusterCreate(clCp)
	assert.Nil(err)
	assert.NotNil(ft.req)
	assert.Equal(fakeURL, ft.req.URL.Path)
	assert.Equal("new-token", nc.AuthToken)

	lRecs := []string{}
	var lastN uint64
	tl.Iterate(func(n uint64, rec string) {
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.True(len(lRecs) >= 2)
	assert.Equal(uint64(2), lastN)
	r := lRecs[0]
	for n, v := range ft.req.Header {
		tl.Logger().Info("req**", n)
		assert.Regexp(n+": "+v[0], r)
	}
	assert.Contains(r, fakeReqBody)
	r = lRecs[1]
	for n, v := range ft.resp.Header {
		tl.Logger().Info("res**", n)
		assert.Regexp(n+": "+v[0], r)
	}
	assert.Contains(r, fakeRespBody)

	// fake an error
	ft.req = nil
	ft.failConnect = true
	tl.Iterate(func(n uint64, s string) { lastN = n })
	curN := lastN
	_, err = clAPI.ClusterCreate(clCp)
	assert.NotNil(err)
	assert.Regexp("failed connect", err.Error())
	assert.NotNil(ft.req)
	assert.Equal(fakeURL, ft.req.URL.Path)
	lRecs = []string{}
	tl.Iterate(func(n uint64, rec string) {
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.Equal(curN+1, lastN)
	r = lRecs[curN]
	for n, v := range ft.req.Header {
		tl.Logger().Info("req**", n)
		assert.Regexp(n+": "+v[0], r)
	}
	assert.Contains(r, fakeReqBody)

	// redo without debug - no logging
	nc.Debug = false
	tl.Iterate(func(n uint64, s string) { lastN = n })
	curN = lastN
	_, err = clAPI.ClusterCreate(clCp)
	assert.NotNil(err)
	assert.Regexp("failed connect", err.Error())
	assert.NotNil(ft.req)
	assert.Equal(fakeURL, ft.req.URL.Path)
	lRecs = []string{}
	tl.Iterate(func(n uint64, rec string) {
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.Equal(curN, lastN)
}

func TestLoggingUnix(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// specify socket
	apiArgs := &APIArgs{
		SocketPath: "./socket.sock",
		Log:        tl.Logger(),
	}
	api, err := NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api)
	nc, ok := api.(*NuvoClient)
	assert.True(ok)
	assert.Equal(apiArgs.SocketPath, nc.SocketPath)

	ft := &fakeTransport{
		t:   t,
		log: tl.Logger(),
	}
	// replace the base transport
	assert.NotNil(nc.baseTransport)
	nc.baseTransport = ft

	clCa := &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: "kubernetes",
			CspDomainID: "0989d6f4-a79b-499e-837a-03188c16510d",
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			Name:              "k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680",
			ClusterIdentifier: "k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680",
		},
	}
	resp := &http.Response{
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "201 Created",
		StatusCode: http.StatusCreated,
	}
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Header["Server"] = []string{"nginx/1.12.1"}
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeRespBody))
	ft.resp = resp
	clCp := cluster.NewClusterCreateParams().WithPayload(clCa)
	clAPI := api.Cluster()
	_, err = clAPI.ClusterCreate(clCp)
	assert.Nil(err)
	if assert.NotNil(ft.req) {
		if assert.NotNil(ft.req.URL) {
			assert.Equal(fakeURL, ft.req.URL.Path)
		}
	}
	lRecs := []string{}
	var lastN uint64
	tl.Iterate(func(n uint64, rec string) {
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.Empty(lRecs)
	assert.Zero(lastN)

	// with Debug, creates a logger, check that DumpRequestOut does not fail, logs overridden URL.Host
	apiArgs.Debug = true
	apiArgs.Log = nil
	api, err = NewAPI(apiArgs)
	assert.Nil(err)
	assert.NotNil(api)
	nc, ok = api.(*NuvoClient)
	assert.True(ok)
	assert.Equal(apiArgs.SocketPath, nc.SocketPath)
	assert.NotNil(nc.Log)
	nc.Log = tl.Logger()
	assert.NotNil(nc.baseTransport)
	nc.baseTransport = ft

	clAPI = api.Cluster()
	_, err = clAPI.ClusterCreate(clCp)
	assert.Nil(err)
	if assert.NotNil(ft.req) {
		if assert.NotNil(ft.req.URL) {
			assert.Equal(fakeURL, ft.req.URL.Path)
			assert.Equal(UnixScheme, ft.req.URL.Scheme)
		}
	}
	foundLH := false
	lRecs = []string{}
	tl.Iterate(func(n uint64, rec string) {
		if strings.Contains(rec, "Host: UnixDomainSocket") {
			foundLH = true
		}
		lRecs = append(lRecs, rec)
		lastN = n
	})
	assert.True(foundLH, "Host: UnixDomainSocket not found")
}

func TestNewWebSocketDialer(t *testing.T) {
	assert := assert.New(t)

	// specify tls, no socket
	apiArgs := &APIArgs{
		TLSCACertificate:  "./ca.crt",
		TLSCertificate:    "./client.crt",
		TLSCertificateKey: "./client.key",
		TLSServerName:     "host1",
	}

	wsd, err := NewWebSocketDialer(apiArgs)
	assert.NoError(err)
	assert.NotNil(wsd)
	d := wsd.Object()
	assert.NotNil(d.TLSClientConfig)
	assert.Equal(apiArgs.Insecure, d.TLSClientConfig.InsecureSkipVerify)
	assert.Equal("host1", d.TLSClientConfig.ServerName)
	assert.Len(d.TLSClientConfig.Certificates, 1)
	assert.NotNil(d.TLSClientConfig.RootCAs)
	assert.Nil(d.NetDial)

	// ForceTLS, Insecure
	apiArgs.TLSCertificateKey = ""
	apiArgs.ForceTLS = true
	apiArgs.Insecure = true
	wsd, err = NewWebSocketDialer(apiArgs)
	assert.NoError(err)
	assert.NotNil(wsd)
	d = wsd.Object()
	assert.NotNil(d.TLSClientConfig)
	assert.Equal(apiArgs.Insecure, d.TLSClientConfig.InsecureSkipVerify)
	assert.Empty(d.TLSClientConfig.ServerName)
	assert.Nil(d.NetDial)
	apiArgs.ForceTLS = false

	// TLS bad cert
	apiArgs.TLSCertificateKey = "./badKeyFile"
	wsd, err = NewWebSocketDialer(apiArgs)
	assert.Error(err)
	assert.Regexp("no such file or directory", err)
	assert.Nil(wsd)

	// socket & TLS: socket takes priority
	apiArgs.SocketPath = "/sockPath"
	wsd, err = NewWebSocketDialer(apiArgs)
	assert.NoError(err)
	assert.NotNil(wsd)
	d = wsd.Object()
	assert.NotNil(d.NetDial)

	c, err := d.NetDial("network", "address")
	assert.Error(err)
	assert.Regexp("no such file or directory", err)
	assert.Nil(c)

	// no TLS, no socket
	apiArgs = &APIArgs{}
	wsd, err = NewWebSocketDialer(apiArgs)
	assert.NoError(err)
	assert.NotNil(wsd)
	d = wsd.Object()
	assert.Nil(d.NetDial)
}

func TestHTMLConsumer(t *testing.T) {
	assert := assert.New(t)
	consumer := HTMLConsumer()
	assert.NotNil(consumer)

	// empty body
	mErr := &models.Error{}
	assert.Equal(io.EOF, consumer.Consume(strings.NewReader(""), mErr))

	// wrong body, body is the message
	mErr = &models.Error{}
	body := "<html>"
	r := strings.NewReader(body)
	assert.NoError(consumer.Consume(r, mErr))
	assert.EqualValues(500, mErr.Code)
	assert.Equal(&body, mErr.Message)

	// empty title is ignored
	mErr = &models.Error{}
	body = "<html><title></title></html>"
	r = strings.NewReader(body)
	assert.NoError(consumer.Consume(r, mErr))
	assert.EqualValues(500, mErr.Code)
	assert.Equal(&body, mErr.Message)

	// correct form of title: code string
	mErr = &models.Error{}
	body = "<html><title> 502 Bad Gateway </title></html>"
	r = strings.NewReader(body)
	assert.NoError(consumer.Consume(r, mErr))
	assert.EqualValues(502, mErr.Code)
	assert.Equal(swag.String("502 Bad Gateway"), mErr.Message)

	// title without a code
	mErr = &models.Error{}
	body = "<html><title>Bad Gateway </title></html>"
	r = strings.NewReader(body)
	assert.NoError(consumer.Consume(r, mErr))
	assert.EqualValues(500, mErr.Code)
	assert.Equal(swag.String("Bad Gateway"), mErr.Message)
}

func TestAuthenticate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ft := &fakeTransport{
		t:   t,
		log: tl.Logger(),
	}
	client := &http.Client{
		Transport: ft,
	}
	nc := &NuvoClient{HP: "localhost:8888", httpClient: client, baseTransport: ft, scheme: "https"}
	client.Transport = nc
	nc.Log = tl.Logger()

	// success
	resp := &http.Response{
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "200 OK",
		StatusCode: http.StatusOK,
	}
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Header["Server"] = []string{"nginx/1.12.1"}
	resp.Body = ioutil.NopCloser(strings.NewReader(`{"token": "string", "exp": 12345, "iss": "Nuvoloso", "username": "foo"}`))
	ft.resp = resp
	auth, err := nc.Authenticate("foo", "bar")
	assert.NotNil(auth)
	assert.NoError(err)
	assert.Equal("POST", ft.req.Method)
	assert.Equal("/auth/login", ft.req.URL.Path)
	assert.Equal("string", nc.AuthToken)
	assert.Equal("https", ft.req.URL.Scheme)
	assert.Equal(nc.HP, ft.req.Host)
	assert.Equal("application/json", ft.req.Header.Get("Content-Type"))
	buf := new(bytes.Buffer)
	body, err := ft.req.GetBody()
	assert.NoError(err)
	buf.ReadFrom(body)
	body.Close()
	bs := buf.String()
	assert.Equal(`{"username":"foo","password":"bar"}`, bs)
	assert.Len(bs, int(ft.req.ContentLength))
	tl.Flush()

	// request error path without log
	nc.HP = "@[:-1"
	nc.Log = nil
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	if assert.Error(err) {
		_, ok := err.(*url.Error)
		assert.True(ok)
	}

	// again, but with logging
	nc.Log = tl.Logger()
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	if assert.Error(err) {
		_, ok := err.(*url.Error)
		assert.True(ok)
	}
	assert.Equal(1, tl.CountPattern("request failure"))

	// Do() failure
	nc.HP = "localhost:8888"
	nc.AuthToken = ""
	ft.failConnect = true
	ft.req = nil
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	if assert.Error(err) {
		assert.Regexp("failed connect", err.Error())
	}
	assert.Empty(nc.AuthToken)
	tl.Flush()

	// Error response, with debug
	nc.Debug = true
	ft.failConnect = false
	ft.req = nil
	resp.Status = "401 Unauthorized"
	resp.StatusCode = http.StatusUnauthorized
	resp.Header["Content-Type"] = []string{"text/plain"}
	resp.Body = ioutil.NopCloser(strings.NewReader("Invalid login or password\n"))
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	if assert.Error(err) {
		assert.Equal("Invalid login or password", err.Error())
	}
	assert.Empty(nc.AuthToken)
	assert.Equal(1, tl.CountPattern("text/plain"))
	tl.Flush()

	// nginx-style error response, with debug
	nc.Debug = true
	ft.failConnect = false
	ft.req = nil
	resp.Status = "502 Bad Gateway"
	resp.StatusCode = http.StatusUnauthorized
	resp.Header["Content-Type"] = []string{"text/html"}
	resp.Body = ioutil.NopCloser(strings.NewReader("<html>\n<head><title>502 Bad Gateway</title></head>\n<body>\n<center><h1>502 Bad Gateway</h1></center></body>\n</html>\n"))
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	assert.Regexp("502 Bad Gateway", err)
	assert.Empty(nc.AuthToken)
	assert.Equal(1, tl.CountPattern("text/html"))
	tl.Flush()

	// bad json response
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Body = ioutil.NopCloser(strings.NewReader(`{"token": "string", "exp": 12345, "iss": "Nuvoloso", "username": "foo"`))
	resp.Status = "200 OK"
	resp.StatusCode = http.StatusOK
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	if assert.Error(err) {
		assert.Equal("unexpected EOF", err.Error())
	}
	assert.Empty(nc.AuthToken)
	assert.Equal(1, tl.CountPattern("invalid response body"))
	tl.Flush()

	// cover response dump failure
	resp.Body = ft
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	assert.Error(err)
	assert.Equal(1, tl.CountPattern("DumpResponse"))
	tl.Flush()

	// works with no logger
	nc.Debug = false
	nc.Log = nil
	resp.Body = ft
	auth, err = nc.Authenticate("foo", "bar")
	assert.Nil(auth)
	assert.Error(err)
	assert.Zero(tl.CountPattern("DumpResponse"))
}

func TestValidate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ft := &fakeTransport{
		t:   t,
		log: tl.Logger(),
	}
	client := &http.Client{
		Transport: ft,
	}
	nc := &NuvoClient{HP: "localhost:8888", httpClient: client, baseTransport: ft, scheme: "https", AuthToken: "orig"}
	client.Transport = nc
	nc.Log = tl.Logger()

	// success
	resp := &http.Response{
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "200 OK",
		StatusCode: http.StatusOK,
	}
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Header["Server"] = []string{"nginx/1.12.1"}
	resp.Body = ioutil.NopCloser(strings.NewReader(`{"token": "string", "exp": 12345, "iss": "Nuvoloso", "username": "foo"}`))
	ft.resp = resp
	auth, err := nc.Validate(true)
	assert.NotNil(auth)
	assert.NoError(err)
	assert.Equal("POST", ft.req.Method)
	assert.Equal("/auth/validate", ft.req.URL.Path)
	assert.Equal("string", nc.AuthToken)
	assert.Equal("https", ft.req.URL.Scheme)
	assert.Equal("orig", ft.req.Header.Get("token"))
	assert.Equal(nc.HP, ft.req.Host)
	assert.Empty(ft.req.Header.Get("Content-Type"))
	assert.Nil(ft.req.GetBody)
	tl.Flush()

	// request error path without log
	nc.HP = "@[:-1"
	nc.Log = nil
	auth, err = nc.Validate(true)
	assert.Nil(auth)
	if assert.Error(err) {
		_, ok := err.(*url.Error)
		assert.True(ok)
	}

	// again, but with logging
	nc.Log = tl.Logger()
	auth, err = nc.Validate(true)
	assert.Nil(auth)
	if assert.Error(err) {
		_, ok := err.(*url.Error)
		assert.True(ok)
	}
	assert.Equal(1, tl.CountPattern("request failure"))

	// Do() failure, cover query param
	nc.HP = "localhost:8888"
	nc.AuthToken = "orig"
	ft.failConnect = true
	ft.req = nil
	auth, err = nc.Validate(false)
	assert.Nil(auth)
	if assert.Error(err) {
		assert.Regexp("failed connect", err.Error())
	}
	if assert.NotNil(ft.req) && assert.NotNil(ft.req.URL) {
		assert.Equal("preserve-expiry=true", ft.req.URL.RawQuery)
	}
	assert.Equal("orig", nc.Authentication().GetAuthToken())
}

func TestSetContextAccount(t *testing.T) {
	assert := assert.New(t)

	nc := &NuvoClient{}
	nc.API = new(client.Nuvoloso)
	ft := &fakeClientTransport{}
	nc.API.User = user.New(ft, strfmt.Default)
	nc.API.Account = account.New(ft, strfmt.Default)

	t.Log("accountName required")
	id, err := nc.SetContextAccount("login", "")
	assert.Empty(id)
	assert.Error(err)
	assert.Nil(ft.op)

	t.Log("account list fails")
	ft.acctResp = &account.AccountListOK{}
	ft.err = &account.AccountListDefault{Payload: &models.Error{Code: 500, Message: swag.String("database error")}}
	id, err = nc.SetContextAccount("", "account")
	assert.Empty(id)
	assert.Regexp("database error", err)

	t.Log("account list fails other error")
	ft.acctResp = &account.AccountListOK{}
	ft.err = errors.New("other error")
	id, err = nc.SetContextAccount("", "account")
	assert.Empty(id)
	assert.Equal(ft.err, err)

	t.Log("tenant not found")
	ft.acctResp = &account.AccountListOK{Payload: []*models.Account{&models.Account{}}}
	ft.err = nil
	id, err = nc.SetContextAccount("", "tenant/dep")
	assert.Empty(id)
	assert.Regexp("account.*not found$", err)
	ft.acctResp = nil

	t.Log("tenant/dep not found")
	ft.acctResp = &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountMutable: models.AccountMutable{Name: "one"},
			},
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "tid1"}},
				AccountMutable: models.AccountMutable{Name: "tenant"},
			},
		},
	}
	ft.err = nil
	id, err = nc.SetContextAccount("", "tenant/dep")
	assert.Empty(id)
	if assert.Error(err) {
		assert.Regexp("account.*not found", err.Error())
	}

	t.Log("success with account only")
	ft.acctResp.Payload = append(ft.acctResp.Payload,
		&models.Account{
			AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "aid1"}, TenantAccountID: "tid1"},
			AccountMutable: models.AccountMutable{Name: "dep"},
		})
	id, err = nc.SetContextAccount("", "tenant/dep")
	assert.NoError(err)
	assert.Equal("aid1", id)
	assert.Equal("aid1", nc.AccountID)
	assert.Empty(nc.UserID)
	ft.acctResp = nil
	nc.AccountID = ""

	t.Log("user list fails")
	ft.err = &user.UserListDefault{Payload: &models.Error{Code: 500, Message: swag.String("database error")}}
	id, err = nc.SetContextAccount("login", "account")
	assert.Empty(id)
	assert.Regexp("database error", err)

	t.Log("user list fails other error")
	ft.err = errors.New("other error")
	id, err = nc.SetContextAccount("login", "account")
	assert.Empty(id)
	assert.Equal(ft.err, err)

	t.Log("user list empty")
	ft.err = nil
	ft.userResp = &user.UserListOK{}
	id, err = nc.SetContextAccount("login", "account")
	if assert.Error(err) {
		assert.Regexp("user.*not found", err.Error())
	}

	t.Log("account not found")
	ft.userResp.Payload = []*models.User{
		&models.User{
			UserAllOf0: models.UserAllOf0{
				Meta:         &models.ObjMeta{ID: "uid1"},
				AccountRoles: []*models.UserAccountRole{},
			},
			UserMutable: models.UserMutable{
				AuthIdentifier: "login",
			},
		},
	}
	id, err = nc.SetContextAccount("login", "account")
	assert.Regexp("account.*not found.*user.*has no role", err)
	assert.Empty(nc.UserID)
	assert.Empty(nc.AccountID)

	t.Log("success")
	ft.userResp.Payload[0].AccountRoles = []*models.UserAccountRole{
		&models.UserAccountRole{
			AccountID:   "aid0",
			AccountName: "System",
		},
		&models.UserAccountRole{
			AccountID:         "aid1",
			AccountName:       "account",
			TenantAccountName: "nuvoloso",
		},
		&models.UserAccountRole{
			AccountID:   "aid2",
			AccountName: "account",
		},
	}
	id, err = nc.SetContextAccount("login", "account")
	assert.NoError(err)
	assert.Equal("aid2", id)
	assert.Equal("uid1", nc.UserID)
	assert.Equal("aid2", nc.AccountID)
	nc.AccountID, nc.UserID = "", ""

	t.Log("success with tenant/subordinate")
	id, err = nc.SetContextAccount("login", "nuvoloso/account")
	assert.NoError(err)
	assert.Equal("aid1", id)
	assert.Equal("uid1", nc.UserID)
	assert.Equal("aid1", nc.AccountID)
}

func TestAuthenticateRequest(t *testing.T) {
	assert := assert.New(t)

	r := &fakeClientRequest{}
	r.header = make(http.Header)
	reg := strfmt.NewFormats()
	nc := &NuvoClient{}

	// no header added when AuthToken, etc are empty
	assert.NoError(nc.AuthenticateRequest(r, reg))
	assert.Empty(r.header)

	// headers added
	nc.SetAuthToken("token")
	nc.UserID = "uid1"
	nc.AccountID = "aid1"
	assert.NoError(nc.AuthenticateRequest(r, reg))
	assert.Len(r.header, 2)
	v, ok := r.header["X-Auth"]
	assert.True(ok, "X-Auth header exists")
	if assert.Len(v, 1) {
		assert.Equal(v[0], "token")
	}
	v, ok = r.header["X-User"]
	assert.False(ok, "X-Auth overrides X-User")
	v, ok = r.header["X-Account"]
	assert.True(ok, "X-Account header exists")
	if assert.Len(v, 1) {
		assert.Equal(v[0], "aid1")
	}

	// userID without X-Auth
	r.header = make(http.Header)
	nc.AuthToken = ""
	assert.NoError(nc.AuthenticateRequest(r, reg))
	assert.Len(r.header, 2)
	v, ok = r.header["X-Auth"]
	assert.False(ok, "X-Auth not present")
	v, ok = r.header["X-User"]
	assert.True(ok, "X-User header exists")
	if assert.Len(v, 1) {
		assert.Equal(v[0], "uid1")
	}
	v, ok = r.header["X-Account"]
	assert.True(ok, "X-Account header exists")
	if assert.Len(v, 1) {
		assert.Equal(v[0], "aid1")
	}
}

// fakeClientRequest is a runtime.ClientRequest
type fakeClientRequest struct {
	header http.Header
}

func (r *fakeClientRequest) GetHeaderParams() http.Header {
	return r.header
}

func (r *fakeClientRequest) SetHeaderParam(name string, values ...string) error {
	r.header[name] = values
	return nil
}

func (r *fakeClientRequest) SetQueryParam(string, ...string) error {
	return nil
}

func (r *fakeClientRequest) SetFormParam(string, ...string) error {
	return nil
}

func (r *fakeClientRequest) SetPathParam(string, string) error {
	return nil
}

func (r *fakeClientRequest) GetQueryParams() url.Values {
	return url.Values{}
}

func (r *fakeClientRequest) SetFileParam(string, ...runtime.NamedReadCloser) error {
	return nil
}

func (r *fakeClientRequest) SetBodyParam(interface{}) error {
	return nil
}

func (r *fakeClientRequest) SetTimeout(time.Duration) error {
	return nil
}

func (r *fakeClientRequest) GetMethod() string {
	return "POST"
}

func (r *fakeClientRequest) GetPath() string {
	return "/path"
}

func (r *fakeClientRequest) GetBody() []byte {
	return []byte{}
}

func (r *fakeClientRequest) GetBodyParam() interface{} {
	return ""
}

func (r *fakeClientRequest) GetFileParam() map[string][]runtime.NamedReadCloser {
	return nil
}

// fakeClientTransport implements runtime.ClientTransport
type fakeClientTransport struct {
	op       *runtime.ClientOperation
	acctResp *account.AccountListOK
	userResp *user.UserListOK
	err      error
}

func (ft *fakeClientTransport) Submit(op *runtime.ClientOperation) (interface{}, error) {
	ft.op = op
	if ft.acctResp != nil {
		return ft.acctResp, ft.err
	}
	return ft.userResp, ft.err
}

// fakeTransport is a RoundTripper and a ReadCloser
type fakeTransport struct {
	t           *testing.T
	log         *logging.Logger
	resp        *http.Response
	req         *http.Request
	failConnect bool
}

func (tr *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.req = req
	if tr.failConnect {
		return nil, errors.New("failed connect")
	}
	tr.resp.Request = req
	return tr.resp, nil
}

func (tr *fakeTransport) Read(p []byte) (n int, err error) {
	return 0, errors.New("fake read error")
}

func (tr *fakeTransport) Close() error {
	return nil
}

var fakeHost = "a59825df6bf5511e7bcc9020c24c6a68-2002255389.us-west-2.elb.amazonaws.com"
var fakeURL = "/api/v1/clusters"
var fakeReqBody = `{"clusterType":"kubernetes","cspDomainId":"0989d6f4-a79b-499e-837a-03188c16510d","authorizedAccounts":null,"clusterIdentifier":"k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680","messages":null,"name":"k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680"}`
var fakeRespBody = `{"meta":{"id":"be7e66c8-420f-4af0-bbf6-a71b98a2ac24","objType":"Cluster","timeCreated":"2017-11-01T23:04:07.535Z","timeModified":"2017-11-01T23:04:07.535Z","version":1},"clusterIdentifier":"k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680","clusterType":"kubernetes","cspDomainId":"0989d6f4-a79b-499e-837a-03188c16510d","service":{"serviceState":{"heartbeatTime":"0001-01-01T00:00:00.000Z","messages":[],"state":"UNKNOWN"}},"clusterAttributes":{"ClusterIdentifier":{"kind":"STRING","value":"k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680"},"ClusterVersion":{"kind":"STRING","value":"1.7"},"CreationTimestamp":{"kind":"STRING","value":"2017-11-01T22:17:52Z"},"GitVersion":{"kind":"STRING","value":"v1.7.8"},"Platform":{"kind":"STRING","value":"linux/amd64"}},"name":"k8s-svc-uid:80ce6a80-bf52-11e7-bcc9-020c24c6a680","tags":[]}`
