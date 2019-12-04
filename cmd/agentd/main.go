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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	app "github.com/Nuvoloso/kontroller/pkg/agentd"
	csi "github.com/Nuvoloso/kontroller/pkg/agentd/csi"
	_ "github.com/Nuvoloso/kontroller/pkg/agentd/handlers"
	_ "github.com/Nuvoloso/kontroller/pkg/agentd/heartbeat"
	_ "github.com/Nuvoloso/kontroller/pkg/agentd/metrics"
	appSReq "github.com/Nuvoloso/kontroller/pkg/agentd/sreq"
	appVReq "github.com/Nuvoloso/kontroller/pkg/agentd/vreq"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	_ "github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/util"
	loads "github.com/go-openapi/loads"
	"github.com/go-openapi/swag"
	flags "github.com/jessevdk/go-flags"
	logging "github.com/op/go-logging"
)

// Build information passed in via ld flags
var (
	BuildID   string
	BuildTime string
	BuildHost string
	BuildJob  string
	Appname   string
)

// Ini file name template
const iniFileNameTemplate = "/etc/nuvoloso/%s.ini"

// mainContext contains context information for handlers in this package
type mainContext struct {
	AppName string          // application name
	Log     *logging.Logger `json:"-"` // default logger
	AppArgs app.AppArgs
	AppCtx  *app.AppCtx `no-flag:"1" json:"-"` // backend context

	VReqArgs appVReq.Args `group:"VolumeSeriesRequest Options" namespace:"vreq"`
	SReqArgs appSReq.Args `group:"StorageRequest Options" namespace:"sreq"`
	CsiArgs  csi.Args     `group:"CSI Options" namespace:"csi"`

	Permissive bool `long:"permissive" description:"Allow unverified clients when using TLS. Development use only" hidden:"true" json:"-"`
	Version    bool `long:"version" short:"V" description:"Shows the build version and time created" no-ini:"1" json:"-"`
	// default log level set in option
	LogLevel string         `long:"log-level" description:"Specify the minimum logging level" default:"DEBUG" choice:"DEBUG" choice:"INFO" choice:"WARNING" choice:"ERROR"`
	WriteIni flags.Filename `long:"write-config" description:"Specify the name of a file to which to write the current configuration." no-ini:"1" json:"-"`
}

func main() {
	mCtx := &mainContext{AppName: Appname}
	mCtx.LogLevel = "DEBUG" // for bootstrap
	mCtx.SetupLogging()

	swaggerSpec, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err)
		os.Exit(1)
	}

	api := operations.NewNuvolosoAPI(swaggerSpec)
	api.Logger = mCtx.Log.Infof
	api.ServerShutdown = mCtx.serverShutdown
	nuvoAPIVersion := swaggerSpec.Spec().Info.Version

	server := restapi.NewServer(api)
	defer server.Shutdown()
	server.ConfigureServerCallback = mCtx.configureServerCallback
	server.ConfigureTLSCallback = mCtx.configureTLSCallback

	iniFile := fmt.Sprintf(iniFileNameTemplate, Appname)
	parser := flags.NewParser(mCtx, flags.Default)
	parser.ShortDescription = mCtx.AppName
	parser.LongDescription = "The per-node agent Nuvoloso management service.\n\n" +
		"The service reads its configuration options from " + iniFile + " on startup. " +
		"Command line flags will override the file values."

	// add additional flags
	api.CommandLineOptionsGroups = append(api.CommandLineOptionsGroups, swag.CommandLineOptionsGroup{
		ShortDescription: "Service Options",
		LongDescription:  "",
		Options:          server,
	})

	for _, optsGroup := range api.CommandLineOptionsGroups {
		_, err := parser.AddGroup(optsGroup.ShortDescription, optsGroup.LongDescription, optsGroup.Options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s", err)
			os.Exit(1)
		}
	}

	if e := flags.NewIniParser(parser).ParseFile(iniFile); e != nil {
		if !os.IsNotExist(e) {
			fmt.Fprintf(os.Stderr, "Error: %s", e)
			os.Exit(1)
		}
		mCtx.Log.Info("Configuration file", iniFile, "not found")
	} else {
		mCtx.Log.Info("Read configuration defaults from", iniFile)
	}

	if _, err := parser.Parse(); err != nil {
		code := 1
		if fe, ok := err.(*flags.Error); ok {
			if fe.Type == flags.ErrHelp {
				code = 0
			}
		}
		os.Exit(code)
	}
	if mCtx.Version {
		fmt.Println("Build ID:", BuildID)
		fmt.Println("Build date:", BuildTime)
		fmt.Println("Build host:", BuildHost)
		if BuildJob != "" {
			fmt.Println("Build job:", BuildJob)
		}
		fmt.Println("NuvoAPI version:", nuvoAPIVersion)
		os.Exit(0)
	}
	if mCtx.WriteIni != "" {
		iniP := flags.NewIniParser(parser)
		iniP.WriteFile(string(mCtx.WriteIni), flags.IniIncludeComments|flags.IniIncludeDefaults|flags.IniCommentDefaults)
		fmt.Println("Wrote configuration to", mCtx.WriteIni)
		os.Exit(0)
	}
	mCtx.SetupLogging() // again

	// initialize the back end and install handlers
	evMArgs := &crude.ManagerArgs{ // TBD: may contain flag args in future
		WSKeepAlive: false,
		Log:         mCtx.Log,
	}
	evM := crude.NewManager(evMArgs)
	appVReq.ComponentInit(&mCtx.VReqArgs)
	appSReq.ComponentInit(&mCtx.SReqArgs)
	csi.ComponentInit(&mCtx.CsiArgs)
	mCtx.AppArgs.CrudeOps = evM
	mCtx.AppArgs.API = api
	buildContext := BuildJob
	if buildContext == "" {
		buildContext = BuildHost
	}
	mCtx.AppArgs.ServiceVersion = BuildID + " " + BuildTime + " " + buildContext
	mCtx.AppArgs.NuvoAPIVersion = nuvoAPIVersion
	mCtx.AppArgs.Server = server
	mCtx.AppArgs.Log = mCtx.Log
	// display the invocation flags before starting (use `json:"-"` to suppress fields)
	if b, err := json.MarshalIndent(mCtx, "", "    "); err == nil {
		mCtx.AppArgs.InvocationArgs = string(b)
		mCtx.Log.Info(mCtx.AppArgs.InvocationArgs)
	}
	mCtx.AppCtx = app.AppInit(&mCtx.AppArgs)

	// Setup middleware if any, otherwise do nothing
	server.SetHandler(mCtx.outermostMiddleware(
		api.Serve(func(next http.Handler) http.Handler {
			return util.NumberRequestMiddleware(mCtx.peepAtClientCertMiddleware(mCtx.loggingHandler(mCtx.innermostMiddleware(next))))
		})))

	// in case previous instance crashed, clean up any leftover unix socket
	if server.SocketPath != "" && util.Contains(server.EnabledListeners, "unix") {
		if stat, err := os.Stat(string(server.SocketPath)); err == nil {
			if stat.Mode()&os.ModeSocket != 0 {
				mCtx.Log.Warningf("Removing stale unix socket %s", string(server.SocketPath))
				if err = os.Remove(string(server.SocketPath)); err != nil {
					mCtx.Log.Fatal(err)
				}
			}
		}
	}

	// start the backend app
	mCtx.Log.Infof("Starting %s (%s)", mCtx.AppName, mCtx.AppArgs.ServiceVersion)
	mCtx.AppCtx.Start()

	if err := server.Serve(); err != nil {
		mCtx.Log.Fatal(err)
	}
}

// SetupLogging initializes logging for the application
func (mCtx *mainContext) SetupLogging() {
	logLevel, err := logging.LogLevel(mCtx.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %s", mCtx.LogLevel)
		os.Exit(1)
	}
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	formatter := logging.MustStringFormatter("%{time} %{level:.1s} %{shortfile} %{message}")
	formatted := logging.NewBackendFormatter(backend, formatter)
	leveled := logging.AddModuleLevel(formatted)
	leveled.SetLevel(logLevel, "")
	logging.SetBackend(leveled)

	mCtx.Log = logging.MustGetLogger("")
}

// configureServerCallback on the server object
func (mCtx *mainContext) configureServerCallback(s *http.Server, scheme, addr string) {
	mCtx.Log.Infof("ConfigureServerCallback %s %s", scheme, addr)
}

// configureTLSCallback on the server object
func (mCtx *mainContext) configureTLSCallback(t *tls.Config) {
	mCtx.Log.Debug("ConfigureTLSCallback")
	if mCtx.Permissive {
		mCtx.Log.Debug("Permitting unverifiable clients")
		t.ClientAuth = tls.VerifyClientCertIfGiven
	}
}

// serverShutdown callback
func (mCtx *mainContext) serverShutdown() {
	mCtx.Log.Infof("Service has shut down")
	mCtx.AppCtx.Stop()
}
