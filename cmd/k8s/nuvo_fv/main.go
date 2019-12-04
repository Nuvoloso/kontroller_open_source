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
	"os"
	"path/filepath"
	"strings"

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

// Ini file related variables
var iniFileNameTemplate = "%s/%s.ini"
var iniFileEnv string
var kpvPathTemplate = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/nuvoloso.com~%s"
var defIniFile string
var iniFile string
var iniErr error

var appCtx = &AppCtx{}
var parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
var outputWriter io.Writer
var logWriter io.Writer

func init() {
	iniFileEnv = strings.ToUpper(Appname + "_CONFIG_FILE")
	outputWriter = os.Stdout
	logWriter = os.Stderr
	initParser()
}

func initParser() {
	parser.ShortDescription = Appname
	parser.Usage = "[Application Options]"
	parser.LongDescription = "Nuvoloso Flexvolume driver for Kubernetes. " +
		"Commands are of the form 'action arguments...'.\n" +
		"\n" +
		"The program initializes itself from an INI file specified by the " +
		iniFileEnv + " environment variable or else from " +
		fmt.Sprintf(iniFileNameTemplate, driverDir(), Appname) + ". " +
		"The format of the file is as follows:\n\n" +
		" [Application Options]\n" +
		" SocketPath = unixSocketPath\n" +
		"\n" +
		"All properties are optional and correspond to the program argument long flag names."

	parser.AddCommand("version", "Show version", "Show build version information.", &versionCmd{})
	parser.AddCommand("help", "Show program or command usage", "Shows program usage if no argument specified otherwise it displays help for the command specified.", &helpCmd{})
}

type execFn func() (string, error)

var appExecHook execFn = os.Executable

// Return the directory containing the executing driver, or the built-in default
func driverDir() string {
	ex, err := appExecHook()
	if err != nil {
		return fmt.Sprintf(kpvPathTemplate, Appname)
	}
	return filepath.Dir(ex)
}

type versionCmd struct{}

func (c *versionCmd) Execute(args []string) error {
	buildContext := BuildJob
	if buildContext == "" {
		buildContext = BuildHost
	}
	s := fmt.Sprintf("Version [%s %s %s]", BuildID, BuildTime, buildContext)
	PrintDriverOutput(DriverStatusSuccess, s)
	return nil
}

type helpCmd struct {
	Positional struct {
		CommandSpecifier string `positional-arg-name:"COMMAND"`
	} `positional-args:"yes"`
}

func (c *helpCmd) Execute(args []string) error {
	parser.Command.Active = nil // want global help by default
	if c.Positional.CommandSpecifier != "" {
		cmd := parser.Find(c.Positional.CommandSpecifier)
		if cmd != nil {
			parser.Command.Active = cmd
		} else {
			fmt.Fprintf(outputWriter, "Command %q not found!\n", c.Positional.CommandSpecifier)
		}
	}
	parser.WriteHelp(outputWriter)
	return nil
}

func commandHandler(command flags.Commander, args []string) error {
	if command == nil {
		return nil
	}
	// directly invoke commands that don't need the API
	if hc, ok := command.(*helpCmd); ok {
		return hc.Execute(args)
	}
	if vc, ok := command.(*versionCmd); ok {
		return vc.Execute(args)
	}
	if ic, ok := command.(*initCmd); ok {
		return ic.Execute(args)
	}
	err := appCtx.Init()
	if iniErr != nil {
		if err == nil {
			err = iniErr
		} else {
			appCtx.Log.Error(iniErr.Error())
		}
	}
	if err == nil {
		err = command.Execute(args)
	}
	if err != nil {
		PrintDriverOutput(DriverStatusFailure, err.Error())
	}
	return nil
}

func parseAndRun(args []string) error {
	defIniFile = fmt.Sprintf(iniFileNameTemplate, driverDir(), Appname)
	var ok bool
	iniFile, ok = os.LookupEnv(iniFileEnv)
	if !ok {
		iniFile = defIniFile
	}
	if e := flags.NewIniParser(parser).ParseFile(iniFile); e != nil {
		if !os.IsNotExist(e) {
			// iniErr will be processed for those commands that require the ini file, see commandHandler
			iniErr = e
		}
	}
	parser.CommandHandler = commandHandler
	_, err := parser.ParseArgs(args)
	return err
}

func main() {
	if err := parseAndRun(os.Args[1:]); err != nil {
		if flagErr, ok := err.(*flags.Error); ok && flagErr.Type == flags.ErrHelp {
			fmt.Fprintln(outputWriter, flagErr.Message)
		} else {
			PrintDriverOutput(DriverStatusNotSupported, err.Error())
		}
	}
	// flexvolume driver exit status is not significant
}
