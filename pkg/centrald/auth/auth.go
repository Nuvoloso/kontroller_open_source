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


package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	app "github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/op/go-logging"
)

// Config contains the configuration arguments and related properties for authentication and authorization
type Config struct {
	Disabled      bool   `long:"disabled" description:"Disable authentication validation, all requests are treated as internal requests. Development use only" hidden:"true"`
	NoAuthOk      bool   `long:"allow-unauthenticated" description:"Allow clients that do not authenticate via cert or token. Development use only" hidden:"true"`
	Host          string `long:"host" description:"The hostname of the authentication service" default:"localhost"`
	Port          int16  `long:"port" description:"The port number of the authentication service" default:"5555"`
	UseSSL        bool   `long:"ssl" description:"Use SSL (https) to communicate with the authentication service"`
	SSLServerName string `long:"ssl-server-name" description:"The actual server name of the authentication service SSL certificate"`
	DebugClient   bool   `no-flag:"1"`

	// copied from service flags on startup
	TLSCertificate    string `no-flag:"1"`
	TLSCertificateKey string `no-flag:"1"`
	TLSCACertificate  string `no-flag:"1"`

	client *http.Client
	mux    sync.Mutex
	Log    *logging.Logger
	DS     app.DataStore
}

// InfoKey is a context key using a struct to guarantee uniqueness
type InfoKey struct{}

// Info contains the validated authentication data extracted from the request
type Info struct {
	// AuthIdentifier is the authentication subsystem identifier extracted from the auth token in the request header
	// or the identifier of the UserID from the X-User header.
	// The empty string denotes a trusted "internal" user authenticated by CertCN, ClientDN+ClientVerified or use of unix socket
	AuthIdentifier string
	// AccountID is the validated Account object ID found in the request header
	AccountID string
	// AccountName is the name of the validated Account, if AccountID is set
	AccountName string
	// TenantAccountID is the tenant account ID for the AccountID, if AccountID has a tenant account
	TenantAccountID string
	// UserID is the validated User object ID for AuthIdentifier or the validated proxy user ID if specified in the request header
	UserID string
	// CertCN is the CommonName found in the client certificate (e.g. the certificate used by nginx)
	CertCN string
	// ClientDN is the subject (client) Distinguished Name nginx extracted from the remote client
	ClientDN string
	// ClientCertVerified is set when nginx was able to verify the client certificate
	ClientCertVerified bool
	// Token is the X-Auth token value updated by the authentication service or empty if no X-Auth token is present
	Token string
	// RemoteAddr is the remote client IP address (the client of nginx when nginx is in the path).
	// The empty string denotes that the client is using the trusted Unix Domain socket
	RemoteAddr string
	// RoleObj contains the role capabilities; nil for a trusted internal client that does not pass the X-Account and X-User headers
	RoleObj *models.Role
}

// WatcherAuth functions

// Internal returns true if the client is a trusted internal client that is not impersonating a normal user. Internal clients have nil RoleObj
func (a *Info) Internal() bool {
	return a.RoleObj == nil
}

// GetAccountID returns the account ID or the empty string
func (a *Info) GetAccountID() string {
	return a.AccountID
}

// String creates a human readable version of the Info, eg for logging
func (a *Info) String() string {
	id := []string{}
	from := "unix socket"
	if a.RemoteAddr != "" {
		// RemoteAddr is a string IPv4/6 address and port
		from = a.RemoteAddr
		cn := "noCert!"
		if a.CertCN != "" {
			cn = "CN[" + a.CertCN + "]"
		}
		id = append(id, cn)
		// headers added by nginx when it verifies a client cert on our behalf
		if a.ClientDN != "" {
			dn := "clientDN[" + a.ClientDN + "]"
			if a.ClientCertVerified {
				dn += "(V)"
			}
			id = append(id, dn)
		}
	}
	if a.UserID != "" {
		id = append(id, "U["+a.AuthIdentifier+","+a.UserID+"]")
	}
	if a.AccountID != "" {
		role := ""
		if a.RoleObj != nil {
			role = "," + string(a.RoleObj.Name)
		}
		id = append(id, "A["+a.AccountName+","+a.AccountID+role+"]")
	}
	if len(id) == 0 {
		id = append(id, "[internal client]")
	}
	return fmt.Sprintf("%s from %s", strings.Join(id, " "), from)
}

// functions used in centrald

// InternalOK checks if the auth.Info identifies the caller as an internal trusted client,
// eg using the trusted unix socket or a trusted certificate.
func (a *Info) InternalOK() error {
	if !a.Internal() {
		return app.ErrorUnauthorizedOrForbidden
	}
	return nil
}

// CapOK checks if the auth.Info includes the specified capability
// for the specified accountIDs (any accountID if the list is empty) or is a trusted internal client.
func (a *Info) CapOK(capName string, accountIDs ...models.ObjIDMutable) error {
	if !a.Internal() {
		if !a.RoleObj.Capabilities[capName] {
			return app.ErrorUnauthorizedOrForbidden
		} else if len(accountIDs) > 0 && !util.Contains(accountIDs, models.ObjIDMutable(a.AccountID)) {
			return app.ErrorUnauthorizedOrForbidden
		}
	}
	return nil
}

// UserOK checks if the auth.Info identifies the specified user or is a trusted internal client.
func (a *Info) UserOK(userID string) error {
	if !a.Internal() && a.UserID != userID {
		return app.ErrorUnauthorizedOrForbidden
	}
	return nil
}

// unknownRole is used when a client authenticates but provides no account context from which a role can be derived
var unknownRole = models.Role{
	RoleMutable: models.RoleMutable{
		Capabilities: map[string]bool{},
		Name:         "Unknown",
	},
}

// NewInfo constructs a new, validated Info object from the request.
// On error, the partially constructed Info object is also returned for logging purposes
func (ctx *Config) NewInfo(r *http.Request) (*Info, error) {
	auth := &Info{}
	auth.RemoteAddr = r.RemoteAddr
	if auth.RemoteAddr == "@" {
		// The HTTP server renders the unix socket as "@" on linux, empty string on MacOS, always host:port for http and https
		auth.RemoteAddr = ""
	}
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		auth.CertCN = cert.Subject.CommonName
	}
	if value := r.Header.Get("X-Ssl-Client-Dn"); value != "" {
		// headers added by nginx when it verifies a client cert on our behalf
		auth.ClientDN = value
		if r.Header.Get("X-Ssl-Client-Verify") == "SUCCESS" {
			auth.ClientCertVerified = true
		}
	}
	if value := r.Header.Get("X-Real-IP"); value != "" {
		// X-Real-IP is added by nginx
		// TBD only trust this and similar headers when peer cert is that of nginx (or webgui?)
		auth.RemoteAddr = value
	}
	// client is trusted (like internal user) in these cases, reset below if X-Auth is present and validated
	// TBD only trust certain client certs (eg agentd, clusterd) even if from our CA
	trusted := ctx.NoAuthOk || ctx.Disabled || auth.ClientCertVerified || auth.RemoteAddr == ""

	var accountObj *models.Account
	var userObj *models.User
	var err error
	if value := r.Header.Get("X-Auth"); value != "" {
		if ctx.Disabled {
			auth.Token = value
		} else {
			resp, err := ctx.Validate(value)
			if err != nil {
				return auth, err
			}
			var users []*models.User
			if users, err = ctx.DS.OpsUser().List(r.Context(), user.UserListParams{AuthIdentifier: &resp.Username}); err != nil {
				return auth, err
			} else if len(users) != 1 {
				return auth, errors.New("user not found")
			}
			userObj = users[0]
			auth.Token = resp.Token
			trusted = false
		}
	} else if !trusted {
		// TBD ClientCertVerified: only if the cert is for a trusted service like clusterd
		return auth, errors.New("missing auth token")
	}
	if value := r.Header.Get(com.UserHeader); value != "" {
		if !trusted {
			return auth, errors.New("not a trusted client")
		}
		if userObj, err = ctx.DS.OpsUser().Fetch(r.Context(), value); err != nil {
			if err == app.ErrorNotFound {
				err = errors.New("user not found")
			}
			return auth, err
		}
	}
	if value := r.Header.Get(com.AccountHeader); value != "" {
		// if we get here, client is either trusted internal or specified X-Auth user
		if accountObj, err = ctx.DS.OpsAccount().Fetch(r.Context(), value); err != nil {
			if err == app.ErrorNotFound {
				err = errors.New("account not found")
			}
			return auth, err
		}
	}
	if userObj != nil {
		if userObj.Disabled {
			return auth, errors.New("user is disabled")
		}
		if accountObj == nil && !trusted {
			// X-Account not present so role is unknown
			auth.RoleObj = &unknownRole
		}
		auth.AuthIdentifier = userObj.AuthIdentifier
		auth.UserID = string(userObj.Meta.ID)
	}
	if accountObj != nil {
		if accountObj.Disabled {
			return auth, errors.New("account is disabled")
		}
		if userObj != nil {
			role, ok := accountObj.UserRoles[string(userObj.Meta.ID)]
			if !ok {
				return auth, errors.New("user is not associated with account")
			}
			if role.Disabled {
				return auth, errors.New("user is disabled in the account")
			}
			// if trusted, user and account are for audit logging but not access control other than sanity checks above
			if !trusted {
				if auth.RoleObj, err = ctx.DS.OpsRole().Fetch(string(role.RoleID)); err != nil {
					if err == app.ErrorNotFound {
						err = errors.New("role not found")
					}
					return auth, err
				}
			}
		}
		auth.AccountID = string(accountObj.Meta.ID)
		auth.AccountName = string(accountObj.Name)
		auth.TenantAccountID = string(accountObj.TenantAccountID)
	}
	return auth, nil
}

// ValidateResp is the body of a successful /auth/validate response
type ValidateResp struct {
	Token    string `json:"token"`
	Expiry   int64  `json:"exp"`
	Issuer   string `json:"iss"`
	Username string `json:"username"`
}

// based on http.DefaultTransport but provides additional TLS information
var clientTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}).DialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

var authTransport http.RoundTripper = clientTransport

func (ctx *Config) createHTTPClient() error {
	ctx.mux.Lock()
	defer ctx.mux.Unlock()
	if ctx.client != nil {
		return nil
	}
	if ctx.UseSSL && clientTransport.TLSClientConfig == nil {
		tlsClientOpts := httptransport.TLSClientOptions{
			Certificate: ctx.TLSCertificate,
			Key:         ctx.TLSCertificateKey,
			CA:          ctx.TLSCACertificate,
			ServerName:  ctx.SSLServerName,
		}
		cfg, err := httptransport.TLSClientAuth(tlsClientOpts)
		if err != nil {
			return err
		}
		clientTransport.TLSClientConfig = cfg
	}
	ctx.client = &http.Client{Transport: authTransport}
	return nil
}

// Validate uses the auth service to validate the provided jwt token
func (ctx *Config) Validate(token string) (*ValidateResp, error) {
	if ctx.client == nil {
		if err := ctx.createHTTPClient(); err != nil {
			return nil, err
		}
	}
	scheme := "http"
	if ctx.UseSSL {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/auth/validate", scheme, ctx.Host, ctx.Port)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		ctx.Log.Errorf("Validate request failure: %s", err.Error())
		return nil, err
	}
	req.Header.Set("token", token)
	if ctx.DebugClient {
		b, _ := httputil.DumpRequestOut(req, false)
		ctx.Log.Debug(string(b))
	}
	resp, err := ctx.client.Do(req)
	if err != nil {
		ctx.Log.Errorf("Validate transport failure: %s", err.Error())
		return nil, err
	}
	if ctx.DebugClient {
		b, e2 := httputil.DumpResponse(resp, true) // OK to dump body, no sensitive information
		if e2 != nil {
			ctx.Log.Errorf("DumpResponse: %s", e2.Error())
			return nil, e2
		}
		ctx.Log.Debug(string(b))
	}
	if resp.StatusCode != http.StatusOK {
		// auth service sends error responses as a string in the body
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		resp.Body.Close()
		err = errors.New(strings.TrimSpace(buf.String()))
		return nil, err
	}
	dec := json.NewDecoder(resp.Body)
	auth := &ValidateResp{}
	err = dec.Decode(auth)
	resp.Body.Close()
	if err != nil {
		ctx.Log.Errorf("Validate invalid response body: %s", err.Error())
		return nil, err
	}
	return auth, nil
}
