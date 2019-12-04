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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvofv"
	"github.com/op/go-logging"
)

// Commander is an interface over the exec pkg API
type Commander interface {
	CommandContext(context.Context, string, ...string) Cmd
}

// Cmd is an interface over the exec.Cmd object
type Cmd interface {
	CombinedOutput() ([]byte, error)
}

// AppCtx contains common top-level options and state
type AppCtx struct {
	nuvofv.AppArgs

	Log          *logging.Logger
	api          mgmtclient.API
	ctx          context.Context
	exec         Commander
	oCrud        crud.Ops
	NuvoMountDir string
}

// Init initializes the AppCtx
// Logging is initialized even on error
func (app *AppCtx) Init() error {
	app.SetupLogging()
	if app.ClusterID == "" || app.NodeID == "" || app.NodeIdentifier == "" || app.SystemID == "" {
		return errors.New("system properties are not yet configured by agentd - try again later")
	}
	app.ctx = context.Background()
	apiArgs := &mgmtclient.APIArgs{
		SocketPath: app.SocketPath,
		Debug:      len(app.Verbose) > 0,
		Log:        app.Log,
	}
	var err error
	app.api, err = mgmtclient.NewAPI(apiArgs)
	if err != nil {
		return err
	}
	app.oCrud = crud.NewClient(app.api, app.Log)
	// if blocks are present to support UT
	if app.exec == nil {
		app.exec = app // self-reference
	}
	if app.NuvoMountDir == "" {
		return app.FindNuvoMountDir()
	}
	return nil
}

// SetupLogging initializes logging for the application
func (app *AppCtx) SetupLogging() {
	logLevel := logging.WARNING
	if len(app.Verbose) > 0 {
		logLevel = logging.DEBUG
	}
	// flexvolume driver output should be JSON only, capture log in a buffer and include output in the JSON result message
	if logWriter == os.Stderr {
		logWriter = &bytes.Buffer{}
	}
	backend := logging.NewLogBackend(logWriter, "", 0)

	formatter := logging.MustStringFormatter("%{time} %{level:.1s} %{shortfile} %{message}")
	formatted := logging.NewBackendFormatter(backend, formatter)
	leveled := logging.AddModuleLevel(formatted)
	leveled.SetLevel(logLevel, "")
	logging.SetBackend(leveled)

	app.Log = logging.MustGetLogger("")
}

// FindNuvoMountDir checks that the nuvo FUSE head is present
func (app *AppCtx) FindNuvoMountDir() error {
	cmd := app.exec.CommandContext(app.ctx, "mount")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	// must screen-scrape, mount does not have an option to output JSON
	re := regexp.MustCompile("^.+ on (.+) type fuse.nuvo .*")
	for _, line := range strings.Split(string(output), "\n") {
		if match := re.FindStringSubmatch(line); len(match) > 0 {
			app.NuvoMountDir = match[1]
			app.Log.Debugf("nuvo is mounted at dir [%s]", app.NuvoMountDir)
			return nil
		}
	}
	return fmt.Errorf("nuvo mount point not found, check that the nuvo service/container is running")
}

// IsMounted checks if the mountDir (not the device) is mounted, returns the device if so
func (app *AppCtx) IsMounted(mountDir string) (string, error) {
	cmd := app.exec.CommandContext(app.ctx, "mount")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	// must screen-scrape, mount does not have an option to output JSON
	re := regexp.MustCompile("^(.+) on (.+) type .*")
	for _, line := range strings.Split(string(output), "\n") {
		if match := re.FindStringSubmatch(line); len(match) > 0 {
			if match[2] == mountDir {
				dev := match[1]
				app.Log.Debugf("mount-dir [%s] is mounted in [%s]", mountDir, dev)
				return dev, nil
			}
		}
	}
	return "", nil
}

// CommandContext creates a new Cmd, note that nil ctx is not allowed by exec.CommandContext
func (app *AppCtx) CommandContext(ctx context.Context, name string, arg ...string) Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
