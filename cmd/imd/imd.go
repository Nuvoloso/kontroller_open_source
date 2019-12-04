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
	"os"
	"sort"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	_ "github.com/Nuvoloso/kontroller/pkg/csp/aws"
	_ "github.com/Nuvoloso/kontroller/pkg/csp/azure"
	_ "github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/jessevdk/go-flags"
)

// Build information passed in via ld flags
var (
	BuildID   string
	BuildTime string
	BuildHost string
	BuildJob  string
	Appname   string
)

type appCtx struct {
	CSPDomainType string `short:"D" long:"domain-type" description:"Specify the CSP domain type" choice:"AWS" choice:"Azure" choice:"GCP" required:"yes"`
	TimeoutSecs   int    `short:"t" long:"timeout-seconds" description:"Specify the timeout in seconds" default:"5"`
}

func main() {
	appCtx := &appCtx{}
	parser := flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	parser.ShortDescription = Appname
	parser.LongDescription = "Instance meta-data discovery tool."

	_, err := parser.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}

	c, err := csp.NewCloudServiceProvider(models.CspDomainTypeMutable(appCtx.CSPDomainType))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
	c.LocalInstanceMetadataSetTimeout(appCtx.TimeoutSecs)
	imd, err := c.LocalInstanceMetadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
	names := make([]string, 0, len(imd))
	for n := range imd {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("%s: %s\n", n, imd[n])
	}
	os.Exit(0)
}
