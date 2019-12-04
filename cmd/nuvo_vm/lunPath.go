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

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
)

type getLunPathStruct struct {
	NuvoDir    string `short:"d" long:"nuvo-dir" description:"Nuvo directory" required:"true" value-name:"NUVO_DIR"`
	ExportName string `short:"e" long:"export-name" description:"Export name" required:"true" value-name:"EXPORT_NAME"`
}

var getLunPathCmd getLunPathStruct

func (x *getLunPathStruct) Execute(args []string) error {
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	_, err := nuvoAPI.GetCapabilities()
	if err != nil {
		return nil
	}
	nuvoAPI = nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	lunPath := nuvoAPI.LunPath(x.NuvoDir, x.ExportName)
	fmt.Println(lunPath)
	return nil
}
