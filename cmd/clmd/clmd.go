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
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/jessevdk/go-flags"
	"github.com/op/go-logging"
)

// Build information passed in via ld flags
var (
	BuildID   string
	BuildTime string
	BuildHost string
	BuildJob  string
	Appname   string
)

// AppCtx contains common top-level options and state
type AppCtx struct {
	OutputFormat string `short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	K8sClient    cluster.Client
	ClusterType  string `short:"C" long:"cluster-type" description:"Specify the cluster type" required:"yes" default:"kubernetes"`
	TimeoutSecs  int    `short:"t" long:"timeout-seconds" description:"Specify the timeout in seconds" default:"5"`
	Verbose      bool   `short:"v" long:"verbose" description:"Enable debugging"`
	ctx          context.Context
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
	parser.LongDescription = "Nuvoloso development tool to exercise kubernetes client calls"
}

func commandHandler(command flags.Commander, args []string) error {
	if command == nil {
		return nil
	}
	cl, err := cluster.NewClient(appCtx.ClusterType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
	log := logging.MustGetLogger(Appname)
	if appCtx.Verbose {
		logging.SetLevel(logging.DEBUG, Appname)
	} else {
		logging.SetLevel(logging.INFO, Appname)
	}
	cl.SetDebugLogger(log)
	cl.SetTimeout(appCtx.TimeoutSecs)
	appCtx.K8sClient = cl
	return command.Execute(args)
}

func parseAndRun(args []string) error {
	parser.CommandHandler = commandHandler
	_, err := parser.ParseArgs(args)
	if err != nil {
		return fmt.Errorf("%s", err.Error())
	}
	return nil
}

func main() {
	appCtx.Emitter = &StdoutEmitter{}
	appCtx.ctx = context.Background()
	if err := parseAndRun(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}
