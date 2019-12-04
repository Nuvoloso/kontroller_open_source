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
	"fmt"
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

// Subject is an interface over the authentication information provided when handlers are called
type Subject interface {
	Internal() bool
	GetAccountID() string
	String() string
}

// AccessControl is the interface to support access control
type AccessControl interface {
	GetAuth(*http.Request) (Subject, *models.Error)
}

// Extractor implements the AccessControl interface
type Extractor struct{}

// GetAuth extracts authentication information from the request
func (c *Extractor) GetAuth(req *http.Request) (Subject, *models.Error) {
	a := &Info{}
	a.RemoteAddr = req.RemoteAddr
	if a.RemoteAddr == "@" {
		// The HTTP server renders the unix socket as "@" on linux, empty string on MacOS, always host:port for http and https
		a.RemoteAddr = ""
	}
	if req.TLS != nil && len(req.TLS.PeerCertificates) > 0 {
		cert := req.TLS.PeerCertificates[0]
		a.CertCN = cert.Subject.CommonName
	}
	return a, nil
}

// Info contains the validated authentication data extracted from the request.
// It also implements the Subject interface
type Info struct {
	// CertCN is the CommonName found in the client certificate (e.g. the certificate used by nginx)
	CertCN string
	// RemoteAddr is the remote client IP address (the client of nginx when nginx is in the path).
	// The empty string denotes that the client is using the trusted Unix Domain socket
	RemoteAddr string
}

// Internal returns true if the client is a trusted internal client that is not impersonating a normal user.
func (a *Info) Internal() bool {
	// TBD verify the cert is actually a one we trust
	return a.RemoteAddr == "" || a.CertCN != ""
}

// GetAccountID always returns the empty string in this implementation
func (a *Info) GetAccountID() string {
	return ""
}

// String creates a human readable version of the object
func (a *Info) String() string {
	if a.RemoteAddr != "" {
		// RemoteAddr is a string IPv4/6 address and port
		cn := "noCert!"
		if a.CertCN != "" {
			cn = "CN[" + a.CertCN + "]"
		}
		return fmt.Sprintf("%s from %s", cn, a.RemoteAddr)
	}
	return "[internal client] from unix socket"
}
