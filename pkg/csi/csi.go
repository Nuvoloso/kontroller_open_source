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


package csi

import (
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
	logging "github.com/op/go-logging"
)

// Constants related to CSI
const (
	CsiK8sPodNamespaceKey = "csi.storage.k8s.io/pod.namespace"
	CsiPodNameKey      = "csi.storage.k8s.io/pod.name"
)

// Handler contains common internal methods
type Handler interface {
	// Start starts the handler service on a goroutine.
	Start() error
	// Stop terminates the handler service.
	// It could be called before Start() returns.
	// It must block until the service terminates.
	Stop()
}

// HandlerArgs contains common arguments
type HandlerArgs struct {
	// Socket is the pathname of the unix domain socket for the handler.
	// The handler responds to requests from the sidecars over this socket.
	Socket        string
	Version       string
	ClusterClient cluster.Client
	Log           *logging.Logger
}

// ErrInvalidArgs is returned if arguments are not valid
var ErrInvalidArgs = fmt.Errorf("invalid arguments")

// Validate checks correctness
func (ha *HandlerArgs) Validate() error {
	if ha.Socket == "" || ha.Log == nil {
		return ErrInvalidArgs
	}
	return nil
}
