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


package main

import (
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_debug"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

func init() {
	initDebug()
}

func initDebug() {
	parser.AddCommand("debug", "Execute debug operation", "Execute debug operation", &debugCmd{})
}

type debugCmd struct {
	Stack bool `short:"S" long:"stack" description:"Service to dump all stack traces to the log"`
}

func (c *debugCmd) Execute(args []string) error {
	var err error
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := service_debug.NewDebugPostParams()
	params.Payload = &models.DebugSettings{
		Stack: swag.Bool(c.Stack),
	}
	_, err = appCtx.API.ServiceDebug().DebugPost(params)
	if err != nil {
		if e, ok := err.(*service_debug.DebugPostDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}
