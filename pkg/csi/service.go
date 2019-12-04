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
	"encoding/json"
	"net"
	"os"
	"sync"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"google.golang.org/grpc"
)

// csiService is a service that listens to requests from the sidecars
type csiService struct {
	HandlerArgs
	// handler interfaces
	NodeOps       NodeHandlerOps
	ControllerOps ControllerHandlerOps
	Node          *models.Node
	Cluster       *models.Cluster
	// internal args
	mux      sync.Mutex
	ops      csiServiceOps
	listener net.Listener
	server   grpcServer
}

type csiServiceOps interface {
	RegisterIdentityServer()
	RegisterNodeServer()
	RegisterControllerServer()
}

type grpcServer interface {
	Serve(net.Listener) error
	Stop()
}

// Start starts the handler service on a goroutine
func (h *csiService) Start() error {
	h.mux.Lock()
	defer h.mux.Unlock()
	if h.ops == nil {
		if err := h.init(); err != nil {
			return err
		}
	}
	h.ops.RegisterIdentityServer()
	if h.NodeOps != nil {
		h.ops.RegisterNodeServer()
	} else {
		h.ops.RegisterControllerServer()
	}
	go util.PanicLogger(h.Log, func() { h.server.Serve(h.listener) })
	return nil
}

// Stop terminates the handler service.
// It could be called before Start() returns.
// It must block until the service terminates.
func (h *csiService) Stop() {
	h.mux.Lock()
	defer h.mux.Unlock()
	if h.server != nil {
		h.server.Stop()
	}
}

// init performs basic initialization of the service
func (h *csiService) init() error {
	var err error
	h.ops = h
	os.Remove(h.Socket)
	l, err := net.Listen("unix", h.Socket)
	if err != nil {
		return err
	}
	h.listener = l
	h.server = grpc.NewServer()
	return nil
}

func (h *csiService) dump(obj interface{}) string {
	rc := ""
	if bytes, err := json.Marshal(obj); err == nil {
		rc = string(bytes)
	}
	return rc
}
