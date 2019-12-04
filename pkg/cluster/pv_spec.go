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
	"fmt"
	TT "text/template"

	logging "github.com/op/go-logging"
)

type pvTemplateProcessor interface {
	ExecuteTemplate(ctx context.Context, args *PVSpecArgs) (*PVSpec, error)
}

var pvTProc pvTemplateProcessor

func getPVTemplateProcessor(ctx context.Context, log *logging.Logger) (pvTemplateProcessor, error) {
	var err error
	if pvTProc == nil {
		k8sMutex.Lock()
		defer k8sMutex.Unlock()
		if pvTProc == nil {
			pvTProc, err = k8sNewPVTemplateProcessor(ctx, log)
			if err != nil {
				err = fmt.Errorf("failed to create k8s pv template processor: %s", err.Error())
			}
		}
	}
	return pvTProc, err
}

const (
	pvSpecDefaultTemplateDir = "/opt/nuvoloso/lib/deploy"
)

var pvSpecTemplateDir = pvSpecDefaultTemplateDir

type pvSpecTemplate struct {
	log *logging.Logger
	tt  *TT.Template
}

// k8sNewPVTemplateProcessor returns a new template processor to process persisten volume yaml templates
// It loads all the files matching pv*.yaml from the pvSpecTemplateDir.
func k8sNewPVTemplateProcessor(ctx context.Context, log *logging.Logger) (*pvSpecTemplate, error) {
	tt, err := TT.ParseGlob(pvSpecTemplateDir + "/pv*.yaml.tmpl")
	if err != nil {
		return nil, err
	}
	return &pvSpecTemplate{tt: tt, log: log}, nil
}

// findTemplate returns a template that satisfies the selection criteria of the arguments
func (tf *pvSpecTemplate) findTemplate(args *PVSpecArgs) *TT.Template {
	names := []string{
		"pv-" + args.ClusterVersion + ".yaml.tmpl",
		"pv.yaml.tmpl",
	}
	for _, n := range names {
		if t := tf.tt.Lookup(n); t != nil {
			return t
		}
	}
	return nil
}

// ExecuteTemplate finds and executes the suitable template
func (tf *pvSpecTemplate) ExecuteTemplate(ctx context.Context, args *PVSpecArgs) (*PVSpec, error) {
	if args == nil || args.AccountID == "" || args.Capacity == "" || args.FsType == "" ||
		args.SystemID == "" || args.VolumeID == "" || args.ClusterVersion == "" {
		return nil, fmt.Errorf("invalid arguments")
	}
	tmpl := tf.findTemplate(args)
	if tmpl == nil {
		return nil, fmt.Errorf("could not find appropriate deployment template for %s", args.ClusterVersion)
	}
	b := &bytes.Buffer{}
	if err := tmpl.Execute(b, args); err != nil {
		return nil, err
	}
	return &PVSpec{
		Format: "yaml",
		PvSpec: b.String(),
	}, nil
}
