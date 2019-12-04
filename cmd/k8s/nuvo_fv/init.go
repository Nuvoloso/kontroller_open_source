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

func init() {
	initInit()
}

func initInit() {
	parser.AddCommand("init", "Init command", "Initializes the driver", &initCmd{})
}

type initCmd struct{}

func (c *initCmd) Execute(args []string) error {
	// This command is expected (by kubelet) to always succeed. It does not depend on the app context
	o := NewDriverOutput()
	o.Status = DriverStatusSuccess
	// do not support the attachable interface methods
	o.Capabilities = map[string]interface{}{"attach": false}
	o.Print()
	return nil
}
