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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
)

// K8sWatcher is used to watch kubernetes objects for changes
type K8sWatcher struct {
	url             string //"/api/v1/persistentvolumes"
	resourceVersion string
	ops             K8sWatcherOps
	c               *K8s
	ioReadCloser    io.ReadCloser
	isOpen          bool
	mux             sync.Mutex
	stoppedChan     chan struct{}
	log             *logging.Logger
	client          *http.Client
}

// K8sWatcherOps describes the operations that the watcher implements
type K8sWatcherOps interface {
	WatchEvent(ctx context.Context, object interface{})
	WatchObject() interface{}
}

// Start starts the watcher
func (w *K8sWatcher) Start(ctx context.Context) error {
	var err error
	var shp string
	shp, err = w.c.K8sGetSchemeHostPort()
	if err == nil {
		watchVersion := fmt.Sprintf("&watch=1&resourceVersion=%s", w.resourceVersion)
		p := shp + w.url + watchVersion
		var req *http.Request
		req, err = http.NewRequest("GET", p, nil)
		if err == nil {
			var res *http.Response
			res, err = w.c.k8sClientDo(ctx, req, w.client)
			if err == nil {
				if util.Contains(k8sSuccessCodes, res.StatusCode) {
					w.stoppedChan = make(chan struct{})
					w.isOpen = true
					w.ioReadCloser = res.Body
					go w.monitorBody(ctx)
				} else {
					var body []byte
					body, err = ioutil.ReadAll(res.Body)
					if err == nil {
						var status K8sStatus
						if e := json.Unmarshal(body, &status); e == nil {
							return &k8sError{Message: status.Message, Reason: status.Reason, Code: status.Code}
						}
						return &k8sError{Message: string(body), Reason: StatusReasonUnknown, Code: res.StatusCode}
					}
				}
			}
		}
	}
	return err
}

func (w *K8sWatcher) monitorBody(ctx context.Context) {
	w.log.Infof("starting k8s watcher: %s", w.url)
	if w.ioReadCloser != nil {
		dec := json.NewDecoder(w.ioReadCloser)
		for {
			obj := w.ops.WatchObject()
			if err := dec.Decode(obj); err != nil {
				w.log.Errorf("unable to decode k8s watcher event %s: %s", w.url, err.Error())
				break
			}
			w.ops.WatchEvent(ctx, obj)
		}
	}
	w.log.Infof("stopping k8s watcher: %s", w.url)
	w.mux.Lock()
	if w.IsActive() {
		w.ioReadCloser.Close()
		w.isOpen = false
	}
	w.mux.Unlock()
	close(w.stoppedChan)
}

// Stop terminates the watcher
func (w *K8sWatcher) Stop() {
	w.log.Infof("stopping k8s watcher: %s", w.url)
	w.mux.Lock()
	if w.IsActive() {
		w.ioReadCloser.Close()
		w.isOpen = false
	}
	w.mux.Unlock()
	<-w.stoppedChan
}

// IsActive returns true if the watcher is active
func (w *K8sWatcher) IsActive() bool {
	return w.isOpen
}
