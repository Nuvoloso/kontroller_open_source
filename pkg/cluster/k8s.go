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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
	"gopkg.in/yaml.v2"
)

// K8s is the kubernetes client concrete type
type K8s struct {
	timeout                time.Duration
	debugLog               *logging.Logger
	env                    *K8sEnv
	transport              *http.Transport
	httpClient             *http.Client
	shp                    string
	conditionTimeMap       map[ConditionStatus]time.Time
	conditionRefreshPeriod time.Duration
	mux                    sync.Mutex
}

// K8sEnv contains the pod environment data relating to k8s
type K8sEnv struct {
	Host      string
	Port      int
	Token     string
	CaCert    []byte
	Namespace string
	PodIP     string
	PodName   string
}

// K8sAPIVer is used to decode the result of /version
type K8sAPIVer struct {
	Major, Minor, GitVersion, Platform string
}

// K8sAPIObjMetadata is used to decode object metadata
type K8sAPIObjMetadata struct {
	Name, Namespace, UID, ResourceVersion, CreationTimestamp string
}

// K8sAPIService is used to decode the result of /api/v1/namespaces/default/services/kubernetes
type K8sAPIService struct {
	Metadata K8sAPIObjMetadata
}

// k8sMutex is available for internal use in this package
var k8sMutex sync.Mutex

// K8s constants
const (
	K8sClusterType                 = "kubernetes"
	K8sDefaultSupportedVersion     = "v1.14.6"
	K8sSupportedVersionsConstraint = ">= 1.13.0"
	K8sEnvServerHost               = "KUBERNETES_SERVICE_HOST"
	K8sEnvServerPort               = "KUBERNETES_SERVICE_PORT"
	K8sSASecretPath                = "/var/run/secrets/kubernetes.io/serviceaccount"
	K8sDefaultTimeoutSecs          = 30
	K8sAPIVersion                  = "v1"
	K8sConditionRefreshPeriod      = "1m"            //1 minutes
	NuvoEnvPodIP                   = "NUVO_POD_IP"   // set in managed cluster service pod environments from downward api data
	NuvoEnvPodName                 = "NUVO_POD_NAME" // set in managed cluster service pod environments from downward api data
	NuvoEnvPodNamespace            = "NUVO_POD_NAMESPACE"
	NuvoEnvPodUID                  = "NUVO_POD_UID"
	NuvoEnvNodeName                = "NUVO_NODE_NAME"
	K8sPvLabelType                 = "nuvoloso-volume"
	K8sPvNamePrefix                = "nuvoloso-volume-"
	K8sScNamePrefix                = "nuvoloso-"
	K8sTypeLabelFlex               = "nuvo-vol" // for backward compatibility, should be deprecated
	K8sClusterIdentifierPrefix     = "k8s-svc-uid:"
	K8sDefaultK8sServiceURL        = "/api/v1/namespaces/default/services/kubernetes"
)

var k8sSASecretPath = K8sSASecretPath

var k8sSuccessCodes = []int{http.StatusOK, http.StatusCreated, http.StatusAccepted}

func init() {
	clusterRegistry[K8sClusterType] = &K8s{}
}

// Type is part of the Client interface
func (c *K8s) Type() string {
	return K8sClusterType
}

// SetTimeout is part of the Client interface
func (c *K8s) SetTimeout(secs int) {
	c.timeout, _ = time.ParseDuration(fmt.Sprintf("%ds", secs))
}

// SetDebugLogger is part of the Client interface
func (c *K8s) SetDebugLogger(log *logging.Logger) {
	c.debugLog = log
}

func (c *K8s) dbg(fmt string, args ...interface{}) {
	if c.debugLog != nil {
		c.debugLog.Debugf(fmt, args...)
	}
}

// MetaData is part of the Client interface
func (c *K8s) MetaData(ctx context.Context) (map[string]string, error) {
	md := make(map[string]string)
	var ver K8sAPIVer
	var svc K8sAPIService
	err := c.K8sClientGetJSON(ctx, "/version", &ver)
	if err == nil {
		err = c.K8sClientGetJSON(ctx, K8sDefaultK8sServiceURL, &svc)
	}
	if err != nil {
		return nil, err
	}
	md[CMDVersion] = ver.Major + "." + ver.Minor
	md[CMDIdentifier] = K8sClusterIdentifierPrefix + svc.Metadata.UID
	md[CMDServiceIP] = c.env.PodIP
	md[CMDServiceLocator] = c.env.PodName
	md["GitVersion"] = ver.GitVersion
	md["Platform"] = ver.Platform
	md["CreationTimestamp"] = svc.Metadata.CreationTimestamp
	return md, nil
}

// ValidateClusterObjID is part of the Client interface,
// it parses provided data string to extract the cluster identifier
func (c *K8s) ValidateClusterObjID(ctx context.Context, data, format string) (string, error) {
	var service *K8sService
	if strings.EqualFold("json", format) {
		err := json.Unmarshal([]byte(data), &service)
		if err != nil {
			switch e := err.(type) {
			case *json.UnmarshalTypeError:
				c.dbg("%s ⇒ unmarshal err %s", data, e.Error())
			}
			return "", err
		}
	} else if strings.EqualFold("yaml", format) {
		err := yaml.Unmarshal([]byte(data), &service)
		if err != nil {
			return "", err
		}
	} else {
		return "", fmt.Errorf("unsupported format type")
	}
	var invalidProperty, invalidValue, expectedValue string
	if service.ObjectMeta.UID == "" {
		return "", fmt.Errorf("uid not found")
	}
	if !strings.EqualFold(service.Kind, "Service") {
		invalidProperty = "kind"
		invalidValue = service.Kind
		expectedValue = "Service"
	}
	if !strings.EqualFold(service.SelfLink, K8sDefaultK8sServiceURL) {
		invalidProperty = "selfLink"
		invalidValue = service.SelfLink
		expectedValue = K8sDefaultK8sServiceURL
	}
	if invalidProperty != "" {
		return "", fmt.Errorf("invalid identity object ([%s] expected: %s, actual: %s)", invalidProperty, expectedValue, invalidValue)
	}
	return K8sClusterIdentifierPrefix + service.ObjectMeta.UID, nil
}

// GetService returns the Kubernetes Service given
func (c *K8s) GetService(ctx context.Context, name, namespace string) (*Service, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/services/%s", namespace, name)
	var service *K8sService
	err := c.K8sClientGetJSON(ctx, path, &service)
	if err != nil {
		return nil, err
	}
	serviceRet := &Service{
		Name:      name,
		Namespace: namespace,
	}
	if service.Spec.Type == ServiceTypeLoadBalancer {
		for _, ingress := range service.Status.LoadBalancer.Ingress {
			if ingress.Hostname != "" {
				serviceRet.Hostname = ingress.Hostname
			} else if ingress.IP != "" {
				serviceRet.Hostname = ingress.IP
			}
		}
	}
	return serviceRet, nil
}

func (c *K8s) loadEnvData() error {
	if c.env != nil {
		return nil
	}
	e := &K8sEnv{}
	e.Host = os.Getenv(K8sEnvServerHost)
	p := os.Getenv(K8sEnvServerPort)
	pip := os.Getenv(NuvoEnvPodIP)
	pn := os.Getenv(NuvoEnvPodName)
	if e.Host == "" || p == "" || pip == "" || pn == "" {
		return fmt.Errorf("%s, %s, %s or %s not set", K8sEnvServerHost, K8sEnvServerPort, NuvoEnvPodIP, NuvoEnvPodName)
	}
	pNum, err := strconv.Atoi(p)
	if err != nil || pNum <= 0 {
		return fmt.Errorf("invalid %s value", K8sEnvServerPort)
	}
	e.Port = pNum
	e.PodIP = pip
	e.PodName = pn
	loadFile := func(fName string) []byte {
		var b []byte
		b, err = ioutil.ReadFile(path.Join(k8sSASecretPath, fName))
		return b
	}
	e.Token = "Bearer " + string(loadFile("token"))
	if err == nil {
		e.CaCert = loadFile("ca.crt")
	}
	if err == nil {
		e.Namespace = string(loadFile("namespace"))
	}
	if err != nil {
		return err
	}
	c.env = e
	return nil
}

// makeClient sets the timeout and returns a client
func (c *K8s) makeClient(timeout time.Duration) (*http.Client, error) {
	if err := c.loadEnvData(); err != nil {
		return nil, err
	}
	cfg := &tls.Config{}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(c.env.CaCert)
	cfg.RootCAs = caCertPool
	c.transport = &http.Transport{TLSClientConfig: cfg}
	return &http.Client{
		Transport: c.transport,
		Timeout:   timeout,
	}, nil
}

// k8sClientDo makes an authenticated call to the Kubernetes service
// All calls insert the bearer token.
// If the Accept header is not set it will be set to application/json.
func (c *K8s) k8sClientDo(ctx context.Context, req *http.Request, client *http.Client) (*http.Response, error) {
	if client == nil {
		return nil, fmt.Errorf("invalid client")
	}
	req.Header.Set("Authorization", c.env.Token)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	req = req.WithContext(ctx)
	return client.Do(req)
}

// K8sClientDo makes an authenticated call to the Kubernetes service
// All calls insert the bearer token.
// If the Accept header is not set it will be set to application/json.
func (c *K8s) K8sClientDo(ctx context.Context, req *http.Request) (*http.Response, error) {
	var err error
	if c.httpClient == nil {
		c.mux.Lock()
		if c.httpClient == nil {
			if c.timeout == 0 {
				c.SetTimeout(K8sDefaultTimeoutSecs)
			}
			c.httpClient, err = c.makeClient(c.timeout)
		}
		c.mux.Unlock()
		if err != nil {
			return nil, err
		}
	}
	return c.k8sClientDo(ctx, req, c.httpClient)
}

// K8sGetSchemeHostPort returns a string with scheme://host:port
func (c *K8s) K8sGetSchemeHostPort() (string, error) {
	if c.shp != "" {
		return c.shp, nil
	}
	if err := c.loadEnvData(); err != nil {
		return "", err
	}
	c.shp = fmt.Sprintf("https://%s:%d", c.env.Host, c.env.Port)
	return c.shp, nil
}

// K8sClientGetJSON makes a GET call for an application/json response and
// unmarshals the result to the provided object
func (c *K8s) K8sClientGetJSON(ctx context.Context, path string, v interface{}) error {
	var err error
	var shp string
	shp, err = c.K8sGetSchemeHostPort()
	if err == nil {
		p := shp + path
		c.dbg("GET %s", p)
		var req *http.Request
		req, err = http.NewRequest("GET", p, nil)
		if err == nil {
			var res *http.Response
			res, err = c.K8sClientDo(ctx, req)
			if err == nil {
				var body []byte
				body, err = ioutil.ReadAll(res.Body)
				if c.debugLog != nil {
					c.dbg("%s ⇒ body: %s\n", path, body)
				}
				if !util.Contains(k8sSuccessCodes, res.StatusCode) {
					var status K8sStatus
					if e := json.Unmarshal(body, &status); e == nil {
						return &k8sError{Message: status.Message, Reason: status.Reason, Code: status.Code}
					}
					return &k8sError{Message: string(body), Reason: StatusReasonUnknown, Code: res.StatusCode}
				}
				if err == nil {
					err = json.Unmarshal(body, v)
					if err != nil {
						switch e := err.(type) {
						case *json.UnmarshalTypeError:
							c.dbg("%s ⇒ unmarshal err %s", path, e.Error())
						}
					}
				}
			}
		}
	}
	return err
}

// K8sClientPostJSON makes a POST call for an application/json response and
// unmarshals the result to the provided object
func (c *K8s) K8sClientPostJSON(ctx context.Context, path string, inBody []byte, v interface{}) error {
	shp, err := c.K8sGetSchemeHostPort()
	if err == nil {
		p := shp + path
		c.dbg("POST %s", p)
		var req *http.Request
		req, err = http.NewRequest("POST", p, bytes.NewBuffer(inBody))
		if err == nil {
			var res *http.Response
			if req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}
			c.dbg("POST %s", path)
			c.dbg("Request Body: %s", inBody)
			res, err = c.K8sClientDo(ctx, req)
			if err == nil {
				var body []byte
				body, err = ioutil.ReadAll(res.Body)
				if c.debugLog != nil {
					c.dbg("%s ⇒ body: %s\n", path, body)
				}
				if !util.Contains(k8sSuccessCodes, res.StatusCode) {
					var status K8sStatus
					if e := json.Unmarshal(body, &status); e == nil {
						return &k8sError{Message: status.Message, Reason: status.Reason, Code: status.Code}
					}
					return &k8sError{Message: string(body), Reason: StatusReasonUnknown, Code: res.StatusCode}
				}
				if err == nil {
					err = json.Unmarshal(body, v)
					if err != nil {
						switch e := err.(type) {
						case *json.UnmarshalTypeError:
							c.dbg("%s ⇒ unmarshal err %s", path, e.Error())
						}
					}
				}
			}
		}
	}
	return err
}

// K8sClientPutJSON makes a PUT call for an application/json response and
// unmarshals the result to the provided object
func (c *K8s) K8sClientPutJSON(ctx context.Context, path string, inBody []byte, v interface{}) error {
	shp, err := c.K8sGetSchemeHostPort()
	if err == nil {
		p := shp + path
		c.dbg("PUT %s", p)
		var req *http.Request
		req, err = http.NewRequest("PUT", p, bytes.NewBuffer(inBody))
		if err == nil {
			var res *http.Response
			if req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}
			c.dbg("PUT %s", path)
			c.dbg("Request Body: %s", inBody)
			res, err = c.K8sClientDo(ctx, req)
			if err == nil {
				var body []byte
				body, err = ioutil.ReadAll(res.Body)
				if c.debugLog != nil {
					c.dbg("%s ⇒ body: %s\n", path, body)
				}
				if !util.Contains(k8sSuccessCodes, res.StatusCode) {
					var status K8sStatus
					if e := json.Unmarshal(body, &status); e == nil {
						return &k8sError{Message: status.Message, Reason: status.Reason, Code: status.Code}
					}
					return &k8sError{Message: string(body), Reason: StatusReasonUnknown, Code: res.StatusCode}
				}
				if err == nil {
					err = json.Unmarshal(body, v)
					if err != nil {
						switch e := err.(type) {
						case *json.UnmarshalTypeError:
							c.dbg("%s ⇒ unmarshal err %s", path, e.Error())
						}
					}
				}
			}
		}
	}
	return err
}

// K8sClientDeleteJSON makes a PUT call for an application/json response and
// unmarshals the result to the provided object
func (c *K8s) K8sClientDeleteJSON(ctx context.Context, path string, v interface{}) error {
	shp, err := c.K8sGetSchemeHostPort()
	if err == nil {
		p := shp + path
		c.dbg("DELETE %s", p)
		var req *http.Request
		req, err = http.NewRequest("DELETE", p, nil)
		if err == nil {
			var res *http.Response
			if req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}
			c.dbg("DELETE %s", path)
			res, err = c.K8sClientDo(ctx, req)
			if err == nil {
				var body []byte
				body, err = ioutil.ReadAll(res.Body)
				if c.debugLog != nil {
					c.dbg("%s ⇒ body: %s\n", path, body)
				}
				if !util.Contains(k8sSuccessCodes, res.StatusCode) {
					var status K8sStatus
					if e := json.Unmarshal(body, &status); e == nil {
						return &k8sError{Message: status.Message, Reason: status.Reason, Code: status.Code}
					}
					return &k8sError{Message: string(body), Reason: StatusReasonUnknown, Code: res.StatusCode}
				}
				if err == nil {
					err = json.Unmarshal(body, v)
					if err != nil {
						switch e := err.(type) {
						case *json.UnmarshalTypeError:
							c.dbg("%s ⇒ unmarshal err %s", path, e.Error())
						}
					}
				}
			}
		}
	}
	return err
}

// GetPVSpec is part of the K8sPersistentVolume interface
func (c *K8s) GetPVSpec(ctx context.Context, args *PVSpecArgs) (*PVSpec, error) {
	c.pvSpecFillDefaultArgs(ctx, args)
	if args == nil {
		return nil, fmt.Errorf("invalid arguments")
	}
	tp, err := getPVTemplateProcessor(ctx, c.debugLog)
	if err != nil {
		return nil, err
	}
	return tp.ExecuteTemplate(ctx, args)
}

func (c *K8s) pvSpecFillDefaultArgs(ctx context.Context, args *PVSpecArgs) {
	if args == nil {
		return
	}
	if args.ClusterVersion == "" {
		args.ClusterVersion = PVSpecDefaultVersion
	}
	if args.FsType == "" {
		args.FsType = PVSpecDefaultFsType
	}
	return
}

type k8sError struct {
	Message string // message
	Reason  StatusReason
	Code    int
}

var _ = Error(&k8sError{})

func (e *k8sError) Error() string {
	return e.Message
}

func (e *k8sError) ObjectExists() bool {
	return e.Reason == StatusReasonAlreadyExists
}

func (e *k8sError) PVIsBound() bool {
	return e.Message == ErrPvIsBoundOnDelete.Message
}

func (e *k8sError) NotFound() bool {
	return e.Reason == StatusReasonNotFound
}

// NewK8sError returns an error for an Error with IsRetryable() true
func NewK8sError(msg string, reason StatusReason) error {
	return &k8sError{Message: msg, Reason: reason}
}

// ErrPvIsBoundOnDelete is the error raised when deleteding a bound PV
var ErrPvIsBoundOnDelete = &k8sError{
	Message: "Persistent Volume scheduled for deletion is still bound",
	Reason:  StatusReasonConflict,
	Code:    http.StatusConflict,
}

// VolID2K8sPvName generates a pv name from a nuvo volume ID
func VolID2K8sPvName(volumeID string) string {
	return fmt.Sprintf("%s%s", K8sPvNamePrefix, volumeID)
}

// K8sPvName2VolID gets the volume ID from a k8s pv name
func K8sPvName2VolID(pvName string) string {
	if !strings.HasPrefix(pvName, K8sPvNamePrefix) {
		return ""
	}
	var volumeID string
	fmt.Sscanf(pvName, K8sPvNamePrefix+"%s", &volumeID)
	return volumeID
}

var k8sValidNameRe = regexp.MustCompile(`[ -]+`)

// K8sMakeValidName takes a string input and returns a kubernetes specific valid name
func K8sMakeValidName(inName string) string {
	return strings.ToLower(k8sValidNameRe.ReplaceAllString(inName, "-"))
}
