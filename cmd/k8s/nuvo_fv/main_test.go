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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

type testCmd struct {
	called bool
}

func (c *testCmd) Execute(args []string) error {
	c.called = true
	return nil
}

func TestInvocation(t *testing.T) {
	assert := assert.New(t)
	savedArgs := os.Args
	savedParser := parser
	savedOutput := outputWriter
	savedLogger := logWriter
	defer func() {
		os.Args = savedArgs
		logWriter = savedLogger
		outputWriter = savedOutput
		parser = savedParser
	}()

	// save some values
	origIniFileTemp := iniFileNameTemplate
	defer func() {
		iniFileNameTemplate = origIniFileTemp
	}()

	// invalid command, default ini file non-existent
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	iniFileNameTemplate = "./%s-%s.ini"
	err := parseAndRun([]string{"foo-command"})
	assert.Error(err)
	assert.Regexp("foo-command", err.Error())
	assert.Equal(fmt.Sprintf(iniFileNameTemplate, driverDir(), Appname), defIniFile)
	assert.Equal(iniFile, defIniFile)

	// invalid command, non-existent ini file from environment
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	os.Setenv(iniFileEnv, "fooBar")
	err = parseAndRun([]string{"foo-command"})
	assert.Error(err)
	assert.Regexp("foo-command", err.Error())
	assert.Equal(iniFile, "fooBar")

	// invalid command, bad ini file from environment
	os.Setenv(iniFileEnv, "./testbad.ini")
	err = parseAndRun([]string{"foo-command"})
	if assert.Error(err) {
		assert.Regexp("foo-command", err.Error())
	}
	if assert.Error(iniErr) { // deferred, but primary error supercedes
		assert.Regexp("Not an INI file", iniErr.Error())
	}
	iniErr = nil // reset
	assert.Equal(iniFile, "./testbad.ini")

	// good command, bad ini file, ini error deferred
	var b bytes.Buffer
	outputWriter = &b
	initParser()
	tCmd := &testCmd{}
	appCtx.NuvoMountDir = "/mnt" // test FindNuvoMountDir separately
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	os.Setenv(iniFileEnv, "./testbad.ini")
	err = parseAndRun([]string{"testCommand"})
	assert.NoError(err)
	if assert.Error(iniErr) {
		assert.Regexp("Not an INI file", iniErr.Error())
	}
	assert.True(b.Len() > 0)
	assert.Regexp("status.*Failure.*message.*not yet configured.*Trace Log:.*Not an INI file", b.String())
	iniErr = nil // reset

	// good command, bad ini file, ini error deferred and app context init succeeds
	b.Reset()
	outputWriter = &b
	logWriter = os.Stderr
	initParser()
	tCmd = &testCmd{}
	appCtx.ClusterID = "id"
	appCtx.NodeID = "id"
	appCtx.NodeIdentifier = "i-d"
	appCtx.SystemID = "id"
	appCtx.NuvoMountDir = "/mnt" // test FindNuvoMountDir separately
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	os.Setenv(iniFileEnv, "./testbad.ini")
	err = parseAndRun([]string{"testCommand"})
	assert.NoError(err)
	if assert.Error(iniErr) {
		assert.Regexp("Not an INI file", iniErr.Error())
	}
	assert.True(b.Len() > 0)
	assert.Regexp("status.*Failure.*message.*Not an INI file", b.String())
	assert.NotRegexp("Trace Log", b.String())
	iniErr = nil       // reset
	appCtx = &AppCtx{} // reset

	// test command, good ini file from environment
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	appCtx.NuvoMountDir = "/mnt" // test FindNuvoMountDir separately
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	os.Setenv(iniFileEnv, "./testgood.ini")
	err = parseAndRun([]string{"testCommand"})
	assert.NoError(err)
	assert.Equal(iniFile, "./testgood.ini")
	assert.NotNil(appCtx.api)
	assert.NotNil(appCtx.oCrud)
	assert.True(tCmd.called)

	// test command, no socket
	b.Reset()
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	appCtx.api = nil
	appCtx.oCrud = nil
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	os.Unsetenv(iniFileEnv)
	err = parseAndRun([]string{"--socket-path=", "testCommand"})
	assert.NoError(err)
	assert.True(b.Len() > 0)
	assert.Regexp("status.*Failure.*message.*socket path", b.String())
	assert.Nil(appCtx.api)
	assert.False(tCmd.called)

	// test invocation - no command
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	os.Setenv(iniFileEnv, "./testgood.ini")
	err = parseAndRun([]string{})
	assert.Error(err)
	assert.Regexp("Please specify one command of", err.Error())

	// commandHandler low-level test
	err = commandHandler(nil, []string{})
	assert.NoError(err)

	// driverDir low-level test
	appExecHook = func() (string, error) {
		return "", errors.New("no path")
	}
	Appname = "nuvo_fv"
	assert.Equal(fmt.Sprintf(kpvPathTemplate, Appname), driverDir())
	appExecHook = os.Executable

	// main prints help
	b.Reset()
	outputWriter = &b
	os.Args = []string{"driver", "-h"}
	main()
	assert.True(b.Len() > 0)
	assert.Regexp("Nuvoloso Flexvolume driver", b.String())

	// main unsupported command
	b.Reset()
	outputWriter = &b
	logWriter = bytes.NewBufferString("fake log message\nand another")
	os.Args = []string{"driver", "not"}
	main()
	assert.True(b.Len() > 0)
	assert.Regexp("status.*Not supported.*nTrace Log.*fake log", b.String())
}

func TestVersionCmd(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		outputWriter = os.Stdout
		parser = savedParser
	}()

	tBuildID := "testBuildID"
	tBuildTime := "testBuildTime"
	tBuildHost := "testBuildHost"
	BuildID = tBuildID
	BuildTime = tBuildTime
	BuildHost = tBuildHost

	// version command
	var b bytes.Buffer
	outputWriter = &b
	BuildID = "1"
	BuildTime = "Z"
	BuildJob = "k:1"
	BuildHost = "host"
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err := parseAndRun([]string{"version"})
	assert.NoError(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	o := &map[string]interface{}{}
	err = json.Unmarshal(b.Bytes(), o)
	assert.NoError(err)
	if assert.Contains(*o, "status") {
		assert.Equal("Success", (*o)["status"])
	}
	if assert.Contains(*o, "message") {
		v := (*o)["message"]
		s, ok := v.(string)
		if assert.True(ok) {
			assert.Regexp("Version .1 Z k:1", s)
		}
	}

	// version with host
	b.Reset()
	BuildJob = ""
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err = parseAndRun([]string{"version"})
	assert.NoError(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	o = &map[string]interface{}{}
	err = json.Unmarshal(b.Bytes(), o)
	assert.NoError(err)
	if assert.Contains(*o, "status") {
		assert.Equal("Success", (*o)["status"])
	}
	if assert.Contains(*o, "message") {
		v := (*o)["message"]
		s, ok := v.(string)
		if assert.True(ok) {
			assert.Regexp("Version .1 Z host", s)
		}
	}

	// version command (unknown flag)
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err = parseAndRun([]string{"version", "--foo-flag"})
	assert.Error(err)
	assert.Regexp("foo-flag", err.Error())
}

func TestHelpCmd(t *testing.T) {
	assert := assert.New(t)
	// Note: once help is called the state of the parser gets "frozen".
	// Hence do not use the main parser for these tests.
	savedParser := parser
	defer func() {
		outputWriter = os.Stdout
		parser = savedParser
	}()

	// help command (top level)
	var b bytes.Buffer
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err := parseAndRun([]string{"help"})
	assert.NoError(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("Nuvoloso Flexvolume driver", b.String())

	// invalid command
	b = bytes.Buffer{}
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err = parseAndRun([]string{"help", "fooCmd"})
	assert.NoError(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("Nuvoloso Flexvolume driver", b.String())

	// help command (version)
	b = bytes.Buffer{}
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err = parseAndRun([]string{"help", "version"})
	assert.NoError(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("build version information", b.String())
}

func TestInitCmd(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		outputWriter = os.Stdout
		parser = savedParser
	}()

	// verify Init command is called without initializing app context
	var b bytes.Buffer
	outputWriter = &b
	appCtx = &AppCtx{}
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initInit()
	err := parseAndRun([]string{"init"})
	assert.NoError(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	o := &map[string]interface{}{}
	err = json.Unmarshal(b.Bytes(), o)
	assert.NoError(err)
	if assert.Contains(*o, "status") {
		assert.Equal("Success", (*o)["status"])
	}
	assert.Nil(appCtx.Log)
}
