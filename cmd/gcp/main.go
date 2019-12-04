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
	"io"
	"io/ioutil"
	"os"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/jessevdk/go-flags"
	"github.com/op/go-logging"
)

// Appname is set during build
var Appname string

// AppCtx contains common top-level options and state
type AppCtx struct {
	CredFlags
	OutputFormat string `short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	Csp          csp.CloudServiceProvider
	GCP          *gc.CSP
	GCPClient    *gc.Client
	Emitter
}

var appCtx = &AppCtx{}
var parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
var outputWriter io.Writer

func init() {
	outputWriter = os.Stdout
	initParser()
}

func initParser() {
	parser.ShortDescription = Appname
	parser.Usage = "[Application Options]"
	parser.LongDescription = "Nuvoloso development tool to exercise aspects of the GCP SDK"
	parser.AddCommand("validate", "Validate credentials", "Check that credentails, region and zone are valid.", &validateCmd{})
}

type validateCmd struct {
}

func (c *validateCmd) Execute(args []string) error {
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	if err := appCtx.GCPClient.Validate(nil); err != nil {
		return fmt.Errorf("GCP client validate failed: %s", err.Error())
	}
	fmt.Println("Credentials appear to be valid")
	return nil
}

func commandHandler(command flags.Commander, args []string) error {
	if command == nil {
		return nil
	}
	appCtx.CSPInit()
	return command.Execute(args)
}

func parseAndRun(args []string) error {
	parser.CommandHandler = commandHandler
	_, err := parser.ParseArgs(args)
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprint(outputWriter, err.Error())
			return nil
		}
		return fmt.Errorf("%s", err.Error())
	}
	return nil
}

func main() {
	appCtx.Emitter = &StdoutEmitter{}
	if err := parseAndRun(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s.\n", err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

// CSPInit gets the CSP object
func (ac *AppCtx) CSPInit() {
	// initialize the CSP
	if ac.Csp == nil {
		c, err := csp.NewCloudServiceProvider(gc.CSPDomainType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
			os.Exit(1)
		}
		ac.Csp = c
		gcpCsp, ok := c.(*gc.CSP)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: GCP CSP conversion failed\n")
			os.Exit(1)
		}
		ac.GCP = gcpCsp
	}
}

// CredFlags contain flags required for authentication
type CredFlags struct {
	CredFile flags.Filename `short:"K" long:"cred-file" required:"yes"`
	Zone     string         `short:"Z" long:"zone" required:"yes"`
	DebugAPI bool           `long:"debug" description:"Enable debug of the API"`
}

// ClientInit sets the GCP credential attributes
func (ac *AppCtx) ClientInit(c *CredFlags) error {
	if c.DebugAPI {
		log := logging.MustGetLogger("example")
		ac.GCP.SetDebugLogger(log)
	}
	attrs := make(map[string]models.ValueType, 4)
	cred, err := ioutil.ReadFile(string(c.CredFile))
	if err != nil {
		return err
	}
	attrs[gc.AttrCred] = models.ValueType{Kind: "SECRET", Value: string(cred)}
	attrs[gc.AttrZone] = models.ValueType{Kind: "STRING", Value: c.Zone}
	// fake a CSPDomain object
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "fakeDomain",
			},
			CspDomainType:       gc.CSPDomainType,
			CspDomainAttributes: attrs,
		},
	}
	if cl, err := ac.GCP.Client(dObj); err == nil {
		gcpCl, ok := cl.(*gc.Client)
		if !ok {
			return fmt.Errorf("GCP client conversion failed")
		}
		ac.GCPClient = gcpCl
	} else {
		return err
	}
	return nil
}
