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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/ws"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/gorilla/websocket"
	"github.com/op/go-logging"
	"golang.org/x/net/html"
)

// APIArgs specifies the arguments to create a Nuvoloso client API
type APIArgs struct {
	Host              string
	Port              int
	TLSCertificate    string
	TLSCertificateKey string
	TLSCACertificate  string // not required but passed on
	TLSServerName     string // not required but passed on
	ForceTLS          bool   // use TLS even if Certificate and Key are not specified
	Insecure          bool
	SocketPath        string
	LoginName         string
	Password          string
	Debug             bool            // emits to the log or stderr
	Log               *logging.Logger // optional; DEBUG/ERROR levels
}

// AuthIdentityKey is a context key for adding an Identity context value
type AuthIdentityKey struct{}

// NuvoClient contains the real implementation of the API
type NuvoClient struct {
	API           *client.Nuvoloso
	HP            string
	SocketPath    string
	AuthToken     string
	UserID        string
	AccountID     string
	Debug         bool
	Log           *logging.Logger
	baseTransport http.RoundTripper
	httpClient    *http.Client
	scheme        string
}

// NewAPI returns the real implementation of the Nuvoloso client API
func NewAPI(args *APIArgs) (API, error) {
	nc := &NuvoClient{
		Debug: args.Debug,
		Log:   args.Log,
	}
	if args.Host == "" && args.SocketPath == "" {
		return nil, errors.New("missing host name or socket path")
	}
	var httpClient *http.Client
	tc := client.DefaultTransportConfig()
	switch {
	case args.SocketPath != "":
		httpClient = NewUnixClient(args.SocketPath, nil)
		nc.SocketPath = args.SocketPath
		nc.scheme = "unix"
	case args.TLSCertificate != "" && args.TLSCertificateKey != "" || args.ForceTLS:
		tlsClientOpts := httptransport.TLSClientOptions{
			Certificate: args.TLSCertificate,
			Key:         args.TLSCertificateKey,
			CA:          args.TLSCACertificate,
			ServerName:  args.TLSServerName,
		}
		if args.ForceTLS && (args.TLSCertificate == "" || args.TLSCertificateKey == "") {
			tlsClientOpts.Certificate, tlsClientOpts.Key = "", ""
		}
		if args.Insecure {
			tlsClientOpts.InsecureSkipVerify = true
			tlsClientOpts.ServerName = "" // if ServerName is set, InsecureSkipVerify is ignored
		}
		var err error
		httpClient, err = httptransport.TLSClient(tlsClientOpts)
		if err != nil {
			return nil, err
		}
		port := 443
		if args.Port > 0 {
			port = args.Port
		}
		nc.HP = fmt.Sprintf("%s:%d", args.Host, port)
		nc.scheme = "https"
	case args.TLSCertificate != "" || args.TLSCertificateKey != "":
		return nil, errors.New("TLS certificate and TLS key must be specified together")
	default:
		httpClient = &http.Client{
			Transport: http.DefaultTransport,
		}
		port := 80
		if args.Port > 0 {
			port = args.Port
		}
		nc.HP = fmt.Sprintf("%s:%d", args.Host, port)
		nc.scheme = "http"
	}
	if nc.Debug && nc.Log == nil {
		nc.SetupDebugLogging()
	}
	// wrap the client transport to handle updating auth token and optional debug logging
	nc.baseTransport = httpClient.Transport
	httpClient.Transport = nc
	nc.httpClient = httpClient
	runtime := httptransport.NewWithClient(nc.HP, tc.BasePath, []string{nc.scheme}, httpClient)
	runtime.Consumers[HTMLContentType] = HTMLConsumer()
	runtime.DefaultAuthentication = nc
	nc.API = client.New(runtime, strfmt.Default)
	return nc, nil
}

// SetupDebugLogging initializes debug logging to stderr
func (nc *NuvoClient) SetupDebugLogging() {
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	formatted := logging.NewBackendFormatter(backend, logging.DefaultFormatter)
	leveled := logging.AddModuleLevel(formatted)
	leveled.SetLevel(logging.DEBUG, "")
	nc.Log = logging.MustGetLogger("")
	nc.Log.SetBackend(leveled)
}

// RoundTrip is a wrapper around the transport to log the request and response, add identity headers and handle auth token updates
func (nc *NuvoClient) RoundTrip(r *http.Request) (*http.Response, error) {
	// use identity information from the context if present, overrides any identity set by SetContextAccount()
	if cnv := r.Context().Value(AuthIdentityKey{}); cnv != nil {
		identity := cnv.(*models.Identity) // or panic
		if nc.Debug {
			nc.Log.Debugf("RoundTrip: account: %s user: %s", identity.AccountID, identity.UserID)
		}
		if identity.AccountID != "" {
			r.Header.Set(com.AccountHeader, string(identity.AccountID))
		} else {
			r.Header.Del(com.AccountHeader)
		}
		if identity.UserID != "" {
			r.Header.Set(com.UserHeader, string(identity.UserID))
		} else {
			r.Header.Del(com.UserHeader)
		}
	}

	if nc.Debug {
		// DumpRequestOut cannot handle unix scheme or empty URL.Host
		reqSend := r
		if r.URL != nil && r.URL.Scheme == "unix" {
			reqSend = new(http.Request)
			*reqSend = *r
			reqSend.URL = new(url.URL)
			*reqSend.URL = *r.URL
			reqSend.URL.Scheme = "http"
			reqSend.URL.Host = "UnixDomainSocket"
		}
		dumpBody := true
		if reqSend.URL != nil && reqSend.URL.Path == "/auth/login" {
			// body contains sensitive infomation, don't dump it
			dumpBody = false
		}
		b, err := httputil.DumpRequestOut(reqSend, dumpBody)
		if err != nil {
			nc.Log.Errorf("DumpRequestOut: %s", err.Error())
			return nil, err
		}
		nc.Log.Debug(string(b))
		if reqSend != r {
			r.Body = reqSend.Body // in case DumpRequestOut replaces Body
		}
	}
	resp, err := nc.baseTransport.RoundTrip(r)
	if err != nil {
		return resp, err
	}
	if nc.Debug {
		b, err := httputil.DumpResponse(resp, true)
		if err != nil {
			nc.Log.Errorf("DumpResponse: %s", err.Error())
			return nil, err
		}
		nc.Log.Debug(string(b))
	}
	if token := resp.Header.Get("X-Auth"); token != "" {
		nc.AuthToken = token
	}
	return resp, nil
}

// HTMLContentType is the Context-Type for HTML body
const HTMLContentType = "text/html"

// HTMLConsumer creates a new HTML consumer tailored to handling errors returned by nginx and transforming them into JSON of models.Error.
// nginx error message body contain a title with the message like <title>502 Bad Gateway</title>
func HTMLConsumer() runtime.Consumer {
	return runtime.ConsumerFunc(func(reader io.Reader, data interface{}) error {
		var buf bytes.Buffer
		tee := io.TeeReader(reader, &buf)
		doc := html.NewTokenizer(tee)
		inTitle := false
		message := ""
		for message == "" {
			switch tt := doc.Next(); tt {
			case html.ErrorToken:
				if doc.Err() == io.EOF {
					byteMessage, _ := ioutil.ReadAll(&buf)
					message = string(byteMessage)
				}
				if message == "" {
					return doc.Err()
				}
			case html.StartTagToken:
				tn, _ := doc.TagName()
				if string(tn) == "title" {
					inTitle = true
				}
			case html.EndTagToken:
				inTitle = false
			case html.TextToken:
				if inTitle {
					message = string(doc.Text())
				}
			}
		}
		message = strings.TrimSpace(message)
		n := 0
		for n < len(message) && message[n] >= '0' && message[n] <= '9' {
			n++
		}
		mErr := &models.Error{Code: http.StatusInternalServerError, Message: &message}
		if n > 0 {
			code, _ := strconv.Atoi(message[0:n]) // cannot fail
			mErr.Code = int32(code)
		}
		buf.Reset()
		enc := json.NewEncoder(&buf)
		enc.Encode(mErr) // cannot fail
		dec := json.NewDecoder(&buf)
		dec.UseNumber() // preserve number formats
		return dec.Decode(data)
	})
}

// NewWebSocketDialer configures a WebSocket dialer with TLS options if present.
// If the SocketPath is specified then a NetDial function is added.
func NewWebSocketDialer(args *APIArgs) (ws.Dialer, error) {
	d := &websocket.Dialer{}
	if args.SocketPath != "" {
		d.NetDial = func(network, addr string) (net.Conn, error) {
			return net.Dial(UnixScheme, args.SocketPath)
		}
	} else if args.TLSCertificate != "" || args.ForceTLS {
		tlsClientOpts := httptransport.TLSClientOptions{
			Certificate: args.TLSCertificate,
			Key:         args.TLSCertificateKey,
			CA:          args.TLSCACertificate,
			ServerName:  args.TLSServerName,
		}
		if args.ForceTLS && (tlsClientOpts.Certificate == "" || tlsClientOpts.Key == "") {
			tlsClientOpts.Certificate, tlsClientOpts.Key = "", ""
		}
		if args.Insecure {
			tlsClientOpts.InsecureSkipVerify = true
			tlsClientOpts.ServerName = "" // if ServerName is set, InsecureSkipVerify is ignored
		}
		tlsC, err := httptransport.TLSClientAuth(tlsClientOpts)
		if err != nil {
			return nil, err
		}
		d.TLSClientConfig = tlsC
	}
	return ws.WrapDialer(d), nil
}

// LoginReq is the body of the /auth/login request
type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResp is the body of a successful /auth/login or /auth/validate response
type AuthResp struct {
	Token    string `json:"token"`
	Expiry   int64  `json:"exp"`
	Issuer   string `json:"iss"`
	Username string `json:"username"`
}

// Authenticate contacts the auth service to get the authToken
// TBD use another swagger spec for this service
func (nc *NuvoClient) Authenticate(loginName, password string) (*AuthResp, error) {
	loginReq := &LoginReq{
		Username: loginName,
		Password: password,
	}
	buf, _ := json.Marshal(loginReq)
	body := bytes.NewReader(buf)
	url := fmt.Sprintf("%s://%s/auth/login", nc.scheme, nc.HP)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		if nc.Log != nil {
			nc.Log.Errorf("Authenticate request failure: %s", err.Error())
		}
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return nc.callAuth(req)
}

// GetAuthToken gets the current auth token, empty string if not set
func (nc *NuvoClient) GetAuthToken() string {
	return nc.AuthToken
}

// SetAuthToken sets a new auth token but does not validate it automatically
func (nc *NuvoClient) SetAuthToken(newToken string) {
	nc.AuthToken = newToken
}

// Validate contacts the auth service to validate the current AuthToken
// TBD use another swagger spec for this service
func (nc *NuvoClient) Validate(updateExpiry bool) (*AuthResp, error) {
	url := fmt.Sprintf("%s://%s/auth/validate", nc.scheme, nc.HP)
	if !updateExpiry {
		url += "?preserve-expiry=true"
	}
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		if nc.Log != nil {
			nc.Log.Errorf("Validate request failure: %s", err.Error())
		}
		return nil, err
	}
	req.Header.Set("token", nc.AuthToken)
	return nc.callAuth(req)
}

// callAuth sends the request to the auth service and parses its response, setting the AuthToken on success
func (nc *NuvoClient) callAuth(req *http.Request) (*AuthResp, error) {
	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// auth service sends error responses as plain text string in the body, otherwise just use http.Status
		if strings.Index(resp.Header.Get("Content-Type"), "text/plain") == 0 {
			buf := new(bytes.Buffer)
			buf.ReadFrom(resp.Body)
			resp.Body.Close()
			return nil, errors.New(strings.TrimSpace(buf.String()))
		}
		return nil, errors.New(resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	auth := &AuthResp{}
	err = dec.Decode(auth)
	resp.Body.Close()
	if err != nil {
		if nc.Log != nil {
			nc.Log.Errorf("Auth invalid response body: %s", err.Error())
		}
		return nil, err
	}
	nc.AuthToken = auth.Token
	return auth, nil
}

// SetContextAccount sets the AccountID and optionally UserID to use for subsequent operations.
// The accountName may be of the form "tenantName/subordinateName". The accountID is returned.
func (nc *NuvoClient) SetContextAccount(authIdentifier, accountName string) (string, error) {
	if accountName == "" {
		return "", errors.New("non-empty accountName required")
	}
	name, tenantName := accountName, ""
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		tenantName, name = name[:idx], name[idx+1:]
	}
	if authIdentifier == "" {
		res, err := nc.API.Account.AccountList(account.NewAccountListParams())
		if err != nil {
			if e, ok := err.(*account.AccountListDefault); ok && e.Payload.Message != nil {
				err = errors.New(*e.Payload.Message)
			}
			return "", err
		}
		tenantID := models.ObjIDMutable("")
		if tenantName != "" {
			for _, a := range res.Payload {
				if a.Name == models.ObjName(tenantName) && a.TenantAccountID == "" {
					tenantID = models.ObjIDMutable(a.Meta.ID)
					break
				}
			}
		}
		if tenantName == "" || tenantID != "" {
			for _, a := range res.Payload {
				if a.Name == models.ObjName(name) && a.TenantAccountID == tenantID {
					nc.AccountID = string(a.Meta.ID)
					return string(a.Meta.ID), nil
				}
			}
		}
		return "", fmt.Errorf("account '%s' not found", accountName)
	}
	res, err := nc.API.User.UserList(user.NewUserListParams().WithAuthIdentifier(&authIdentifier))
	if err != nil {
		if e, ok := err.(*user.UserListDefault); ok && e.Payload.Message != nil {
			err = errors.New(*e.Payload.Message)
		}
		return "", err
	} else if len(res.Payload) != 1 {
		return "", fmt.Errorf("user '%s' not found", authIdentifier)
	}

	for _, a := range res.Payload[0].AccountRoles {
		if a.AccountName == name && a.TenantAccountName == tenantName {
			nc.UserID = string(res.Payload[0].Meta.ID)
			nc.AccountID = string(a.AccountID)
			return nc.AccountID, nil
		}
	}
	return "", fmt.Errorf("account '%s' not found or user '%s' has no role in the account", accountName, authIdentifier)
}

// runtime.ClientAuthInfoWriter methods

// AuthenticateRequest adds authentication headers as needed
func (nc *NuvoClient) AuthenticateRequest(r runtime.ClientRequest, _ strfmt.Registry) error {
	if nc.AuthToken != "" {
		r.SetHeaderParam(com.AuthHeader, nc.AuthToken)
	} else if nc.UserID != "" {
		r.SetHeaderParam(com.UserHeader, nc.UserID)
	}
	if nc.AccountID != "" {
		r.SetHeaderParam(com.AccountHeader, nc.AccountID)
	}
	return nil
}

// Interface methods

// Account returns the real client
func (nc *NuvoClient) Account() AccountClient {
	return nc.API.Account
}

// ApplicationGroup returns the real client
func (nc *NuvoClient) ApplicationGroup() ApplicationGroupClient {
	return nc.API.ApplicationGroup
}

// AuditLog returns the real implementation (self reference)
func (nc *NuvoClient) AuditLog() AuditLogClient {
	return nc.API.AuditLog
}

// Authentication returns the real implementation (self reference)
func (nc *NuvoClient) Authentication() AuthenticationAPI {
	return nc
}

// Cluster returns the real client
func (nc *NuvoClient) Cluster() ClusterClient {
	return nc.API.Cluster
}

// ConsistencyGroup returns the real client
func (nc *NuvoClient) ConsistencyGroup() ConsistencyGroupClient {
	return nc.API.ConsistencyGroup
}

// CspCredential returns the real client
func (nc *NuvoClient) CspCredential() CSPCredentialClient {
	return nc.API.CspCredential
}

// CspDomain returns the real client
func (nc *NuvoClient) CspDomain() CSPDomainClient {
	return nc.API.CspDomain
}

// CspStorageType returns the real client
func (nc *NuvoClient) CspStorageType() CSPStorageTypeClient {
	return nc.API.CspStorageType
}

// Metrics returns the real client
func (nc *NuvoClient) Metrics() MetricsClient {
	return nc.API.Metrics
}

// Node returns the real client
func (nc *NuvoClient) Node() NodeClient {
	return nc.API.Node
}

// Pool returns the real client
func (nc *NuvoClient) Pool() PoolClient {
	return nc.API.Pool
}

// ProtectionDomain returns the real client
func (nc *NuvoClient) ProtectionDomain() ProtectionDomainClient {
	return nc.API.ProtectionDomain
}

// Role returns the real client
func (nc *NuvoClient) Role() RoleClient {
	return nc.API.Role
}

// ServiceDebug returns the real client
func (nc *NuvoClient) ServiceDebug() DebugClient {
	return nc.API.ServiceDebug
}

// ServicePlanAllocation returns the real client
func (nc *NuvoClient) ServicePlanAllocation() ServicePlanAllocationClient {
	return nc.API.ServicePlanAllocation
}

// ServicePlan returns the real client
func (nc *NuvoClient) ServicePlan() ServicePlanClient {
	return nc.API.ServicePlan
}

// Slo returns the real client
func (nc *NuvoClient) Slo() SLOClient {
	return nc.API.Slo
}

// Snapshot returns the real client
func (nc *NuvoClient) Snapshot() SnapshotClient {
	return nc.API.Snapshot
}

// Storage returns the real client
func (nc *NuvoClient) Storage() StorageClient {
	return nc.API.Storage
}

// StorageFormula returns the real client
func (nc *NuvoClient) StorageFormula() StorageFormulaClient {
	return nc.API.StorageFormula
}

// StorageRequest returns the real client
func (nc *NuvoClient) StorageRequest() StorageRequestClient {
	return nc.API.StorageRequest
}

// System returns the real client
func (nc *NuvoClient) System() SystemClient {
	return nc.API.System
}

// Task returns the real client
func (nc *NuvoClient) Task() TaskClient {
	return nc.API.Task
}

// User returns the real client
func (nc *NuvoClient) User() UserClient {
	return nc.API.User
}

// VolumeSeries returns the real client
func (nc *NuvoClient) VolumeSeries() VolumeSeriesClient {
	return nc.API.VolumeSeries
}

// VolumeSeriesRequest returns the real client
func (nc *NuvoClient) VolumeSeriesRequest() VolumeSeriesRequestClient {
	return nc.API.VolumeSeriesRequest
}

// Watchers returns the real client
func (nc *NuvoClient) Watchers() WatchersClient {
	return nc.API.Watchers
}

// Transport returns the real client
func (nc *NuvoClient) Transport() runtime.ClientTransport {
	return nc.API.Transport
}
