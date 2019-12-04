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
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/jessevdk/go-flags"
	"github.com/op/go-logging"
)

// Sidecar constants
const (
	AdminDBName           = "admin"
	DefaultClusterDNSName = "cluster.local"

	EnvClusterDNSName = "CLUSTER_DNS_NAME"
	EnvReplicaSetName = "MONGODB_RS_NAME"
)

// Build information passed in via ldflags
var (
	BuildID   string
	BuildTime string
	BuildHost string
	BuildJob  string
	Appname   string
)

// Ini file name template (a var for testing purposes)
var iniFileNameTemplate = "/etc/nuvoloso/%s.ini"

// Ops is the application interface
type Ops interface {
	CheckReplicaSet() error
	Init() error
	InitiateReplicaSet() error
	LogStart()
	Monitor()
	ParseArgs() error
	SetupLogging()
}

// mainContext contains context information for this package
type mainContext struct {
	MongoArgs      mongodb.Args `group:"Mongo Options" namespace:"mongo"`
	ClusterDNSName string       // set from env
	Namespace      string       // set from env
	PodIP          string       // set from env
	PodName        string       // set from env
	ReplicaSetName string       // set from env
	ServiceVersion string
	invocationArgs string
	ops            Ops
	stopMonitor    chan struct{}
	sigChan        chan os.Signal
	ClientTimeout  int `long:"client-timeout" description:"Specify the timeout in seconds for client operations" default:"25"`
	clusterClient  cluster.Client
	db             mongodb.DBAPI
	log            *logging.Logger

	SSLSkipVerify bool           `short:"k" long:"ssl-skip-verify" description:"Allow unverified mongo DB service certificate or host name"`
	CACert        flags.Filename `long:"ca-cert" description:"The CA certificate to use to validate the mongo service for secure connections"`
	ClientCert    flags.Filename `long:"client-cert" description:"The certificate to use for secure connections"`
	ClientKey     flags.Filename `long:"client-key" description:"The private key to use for secure connections"`

	LogLevel string         `long:"log-level" description:"Specify the minimum logging level" default:"DEBUG" choice:"DEBUG" choice:"INFO" choice:"WARNING" choice:"ERROR"`
	Version  bool           `long:"version" short:"V" description:"Shows the build version and time created" no-ini:"1" json:"-"`
	WriteIni flags.Filename `long:"write-config" description:"Specify the name of a file to which to write the current configuration" no-ini:"1" json:"-"`
}

type clusterClientFn func(clusterType string) (cluster.Client, error)
type dbAPIFn func(args *mongodb.Args) mongodb.DBAPI
type exitFn func(int)

var clusterClientHook clusterClientFn = cluster.NewClient
var dbAPIHook dbAPIFn = mongodb.NewDBAPI
var exitHook exitFn = os.Exit

// ParseArgs will parse the INI file and command line args.
// It calls os.Exit(0) if --help, --version or --write-config are specified
func (app *mainContext) ParseArgs() error {
	iniFile := fmt.Sprintf(iniFileNameTemplate, Appname)
	parser := flags.NewParser(app, flags.Default)
	parser.ShortDescription = Appname
	parser.LongDescription = "Nuvoloso MongoDB replica set initialization sidecar.\n\n" +
		"The sidecar reads its configuration options from " + iniFile + " on startup. " +
		"Command line flags will override the file values. " +
		"Additional required configuration must be provided in the environment; see the corresponding documentation.\n\n" +
		"In this sidecar, the --mongo.url is a bootstrap URL and must always refer only to the mongod in the same pod as the sidecar, not the replica set. " +
		"The --mongo.db must refer to the database containing Nuvoloso configuration data (for future use)."

	if e := flags.NewIniParser(parser).ParseFile(iniFile); e != nil {
		if !os.IsNotExist(e) {
			return e
		}
		app.log.Info("Configuration file", iniFile, "not found")
	} else {
		app.log.Info("Read configuration defaults from", iniFile)
	}

	if _, err := parser.Parse(); err != nil {
		code := 1
		if fe, ok := err.(*flags.Error); ok {
			if fe.Type == flags.ErrHelp {
				code = 0
			}
		}
		exitHook(code)
	}
	if app.Version {
		fmt.Println("Build ID:", BuildID)
		fmt.Println("Build date:", BuildTime)
		fmt.Println("Build host:", BuildHost)
		if BuildJob != "" {
			fmt.Println("Build job:", BuildJob)
		}
		exitHook(0)
	}
	if app.WriteIni != "" {
		iniP := flags.NewIniParser(parser)
		iniP.WriteFile(string(app.WriteIni), flags.IniIncludeComments|flags.IniIncludeDefaults|flags.IniCommentDefaults)
		fmt.Println("Wrote configuration to", app.WriteIni)
		exitHook(0)
	}
	return nil
}

// SetupLogging initializes logging for the application
func (app *mainContext) SetupLogging() {
	logLevel, err := logging.LogLevel(app.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %s", app.LogLevel)
		exitHook(1)
	}
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	formatter := logging.MustStringFormatter("%{time} %{level:.1s} %{shortfile} %{message}")
	formatted := logging.NewBackendFormatter(backend, formatter)
	leveled := logging.AddModuleLevel(formatted)
	leveled.SetLevel(logLevel, "")
	logging.SetBackend(leveled)

	app.log = logging.MustGetLogger("")
}

func (app *mainContext) Init() error {
	app.ClusterDNSName = os.Getenv(EnvClusterDNSName)
	if app.ClusterDNSName == "" {
		app.ClusterDNSName = DefaultClusterDNSName
	}
	app.Namespace = os.Getenv(cluster.NuvoEnvPodNamespace)
	app.PodIP = os.Getenv(cluster.NuvoEnvPodIP)
	app.PodName = os.Getenv(cluster.NuvoEnvPodName)
	app.ReplicaSetName = os.Getenv(EnvReplicaSetName)
	if app.Namespace == "" || app.PodIP == "" || app.PodName == "" || app.ReplicaSetName == "" {
		return fmt.Errorf("One or more of %s, %s, %s or %s is not set", EnvReplicaSetName, cluster.NuvoEnvPodNamespace, cluster.NuvoEnvPodIP, cluster.NuvoEnvPodName)
	}
	var err error
	if app.clusterClient, err = clusterClientHook(cluster.K8sClusterType); err != nil {
		return err
	}
	app.clusterClient.SetTimeout(app.ClientTimeout)

	app.MongoArgs.Log = app.log
	app.MongoArgs.AppName = Appname
	app.MongoArgs.DirectConnect = true
	app.MongoArgs.TLSCACertificate = string(app.CACert)
	app.MongoArgs.TLSCertificate = string(app.ClientCert)
	app.MongoArgs.TLSCertificateKey = string(app.ClientKey)
	app.db = dbAPIHook(&app.MongoArgs)
	if err = app.db.Connect(nil); err != nil {
		return err
	}

	app.stopMonitor = make(chan struct{})
	go func() {
		app.sigChan = make(chan os.Signal, 1)
		signal.Notify(app.sigChan, os.Interrupt, os.Signal(syscall.SIGTERM))
		<-app.sigChan
		app.log.Infof("%s shutting down", Appname)

		app.db.Terminate()
		close(app.stopMonitor)
	}()

	return nil
}

func (app *mainContext) LogStart() {
	buildContext := BuildJob
	if buildContext == "" {
		buildContext = BuildHost
	}
	app.ServiceVersion = BuildID + " " + BuildTime + " " + buildContext
	if b, err := json.MarshalIndent(app, "", "    "); err == nil {
		// TBD also log periodically if additional logging is added
		app.invocationArgs = string(b)
		app.log.Info(app.invocationArgs)
	}
}

var app = &mainContext{}

func init() {
	app.ops = app // self-reference
}

func main() {
	app.LogLevel = "DEBUG" // for bootstrap
	app.ops.SetupLogging()
	err := app.ops.ParseArgs()
	if err == nil {
		app.ops.SetupLogging() // again
		err = app.ops.Init()
	}
	if err == nil {
		app.ops.LogStart()
		// checkReplicaSet will initialize the replica set if needed
		err = app.ops.CheckReplicaSet()
	}
	if err == nil {
		app.ops.Monitor()
	}
	if err != nil {
		app.log.Critical(err)
		exitHook(1)
	}
}
