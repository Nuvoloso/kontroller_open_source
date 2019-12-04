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
	"net/http"
	"os"
	"strings"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/ws"
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
var iniFileNameTemplate = "%s/.nuvoloso/%s.config"
var iniFileEnv string
var defIniFile string
var iniFile string

// AppCtx contains common top-level options and state
type AppCtx struct {
	Host          string         `long:"host" description:"The management service host" default:"localhost"`
	Port          int            `long:"port" description:"The management service port number" default:"443"`
	Verbose       []bool         `short:"v" long:"verbose" description:"Show debug information"`
	SSLSkipVerify bool           `short:"k" long:"ssl-skip-verify" description:"Allow unverified management service certificate or host name"`
	CACert        flags.Filename `long:"ca-cert" description:"The CA certificate to use to validate the management service for secure connections"`
	ClientCert    flags.Filename `long:"client-cert" description:"The certificate to use for secure connections"`
	ClientKey     flags.Filename `long:"client-key" description:"The private key to use for secure connections"`
	UseSSL        bool           `long:"ssl" description:"Use SSL/TLS (https) even if a certificate and key are not specified"`
	SSLServerName string         `long:"ssl-server-name" description:"The actual server name of the management service SSL certificate, defaults to the --host value"`
	NoLogin       bool           `long:"no-login" description:"Do not prompt for credentials even if they are not specified"`
	LoginName     string         `long:"login" description:"Log in or select stored credentials for this identifier, usually an e-mail address"`
	Account       string         `short:"A" long:"account" description:"The account context in which the command executes. The form 'tenant/subordinate' identifies a specific subordinate account of a tenant account"`
	SocketPath    string         `long:"socket-path" description:"The management service unix domain socket"`
	OutputFormat  string         `short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`

	watcher Watcher
	osExec  OSExecutor

	apiArgs *mgmtclient.APIArgs
	API     mgmtclient.API
	Dialer  ws.Dialer
	Emitter
	hostArg      bool
	portArg      bool
	socketArg    bool
	noLoginArg   bool
	loginNameArg bool
	AccountID    string
	token        string
}

func (c *AppCtx) processFlagOverrides() {
	// command line host:port overrides INI file socketPath
	if (c.hostArg || c.portArg) && !c.socketArg {
		c.SocketPath = ""
	}
	// If the login name is not specified on the command line, then
	// - command line NoLogin overrides INI file LoginName
	// - NoLogin in INI file overrides INI file LoginName
	if !c.loginNameArg && (c.noLoginArg || c.NoLogin) {
		c.LoginName = ""
	}
}

// InitAPI initializes the API from ini and command line flags
func (c *AppCtx) InitAPI() error {
	// initialize the API
	var err error
	if c.API == nil {
		c.processFlagOverrides()
		c.apiArgs = &mgmtclient.APIArgs{
			Host:              c.Host,
			Port:              c.Port,
			TLSCACertificate:  string(c.CACert),
			TLSCertificate:    string(c.ClientCert),
			TLSCertificateKey: string(c.ClientKey),
			TLSServerName:     c.SSLServerName,
			ForceTLS:          c.UseSSL,
			Insecure:          c.SSLSkipVerify,
			SocketPath:        c.SocketPath,
			Debug:             len(c.Verbose) > 0,
		}
		c.API, err = mgmtclient.NewAPI(c.apiArgs)
		if err == nil && c.SocketPath == "" {
			ac := &authCmd{}
			c.token, err = ac.getToken(c.Host, c.Port, c.LoginName)
			if err == nil {
				c.API.Authentication().SetAuthToken(c.token)
			}
		}
	}
	return err
}

// InitContextAccount will initialize the context account after InitAPI has been called
func (c *AppCtx) InitContextAccount() error {
	var err error
	if c.Account != "" && c.AccountID == "" {
		c.AccountID, err = c.API.Authentication().SetContextAccount(c.LoginName, c.Account)
	}
	return err
}

// InitCrudeDialer will establish a dialer after InitAPI called
func (c *AppCtx) InitCrudeDialer() error {
	if c.Dialer != nil {
		return nil
	}
	var err error
	c.Dialer, err = mgmtclient.NewWebSocketDialer(c.apiArgs)
	return err
}

// MakeCrudeURL constructs the CRUD Event client URL after InitAPI called
func (c *AppCtx) MakeCrudeURL(id string) string {
	return crude.ClientURL(c.apiArgs, id)
}

// MakeAuthHeaders constructs the authentication headers after InitAPI called
func (c *AppCtx) MakeAuthHeaders() http.Header {
	requestHeader := http.Header{}
	if token := appCtx.API.Authentication().GetAuthToken(); token != "" {
		requestHeader.Set(com.AuthHeader, token)
	}
	if c.AccountID != "" {
		requestHeader.Set(com.AccountHeader, c.AccountID)
	}
	return requestHeader
}

// PersistToken is called before successful exit after InitAPI to persist the updated auth token.
// No action is taken if the token is unchanged, was initially empty or empty at the end
func (c *AppCtx) PersistToken() error {
	if appCtx.token != "" {
		token := appCtx.API.Authentication().GetAuthToken()
		if token != "" && token != appCtx.token {
			ac := &authCmd{}
			return ac.setToken(c.Host, c.Port, c.LoginName, token)
		}
	}
	return nil
}

var appCtx = &AppCtx{}
var parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
var interactiveReader io.Reader
var outputWriter io.Writer
var debugWriter io.Writer
var flippedFirstTwoArgs bool

func init() {
	iniFileEnv = strings.ToUpper(Appname + "_CONFIG_FILE")
	interactiveReader = os.Stdin
	outputWriter = os.Stdout
	debugWriter = os.Stderr
	appCtx.watcher = appCtx // self-reference
	appCtx.osExec = appCtx  // self-reference
	initParser()
}

func initParser() {
	flippedFirstTwoArgs = false
	parser.ShortDescription = Appname
	parser.Usage = "[Application Options]"
	parser.LongDescription = "Nuvoloso management command line interface. " +
		"Most commands are of the form 'object action arguments...' but " +
		"it is also possible to invoke the command in the form 'action object arguments...'.\n" +
		"\n" +
		"The program initializes itself from an INI file specified by the " +
		iniFileEnv + " environment variable or else from " +
		fmt.Sprintf(iniFileNameTemplate, "$"+eHOME, Appname) + ". " +
		"The format of the file is as follows:\n\n" +
		" [Application Options]\n" +
		" Host = managementServiceHostName\n" +
		" Port = managementServicePortNumber\n" +
		" ClientCert = clientCertificateFile\n" +
		" ClientKey = clientCertificateKeyFile\n" +
		"\n" +
		"All properties are optional and correspond to the program argument long flag names.\n" +
		"\n" +
		"Credentials are stored in a separate INI file specified by the " + credFileEnv + " environment variable or else from " +
		fmt.Sprintf(credFileNameTemplate, "$"+eHOME, Appname) + ". " +
		"The mode of this file must be 0600 (read-write owner only). " +
		"Each section of this file contains the credentials for a single management server, identified by host, port and login name. " +
		"The format of each section is as follows:\n\n" +
		" [hostNameOrAddr,portNumber,loginName]\n" +
		" password = storedPassword\n" +
		" token = storedToken\n" +
		"\n" +
		"All properties are optional. " +
		"No attempt is made to normalize the hostNameOrAddr, so 'localhost' and '127.0.0.1' are considered to be different. " +
		"See 'nvctl auth -h' for more information regarding credentials."

	parser.AddCommand("version", "Show version", "Show build version information.", &versionCmd{})
	parser.AddCommand("help", "Show program or command/subcommand usage", "Shows the program usage if no argument specified otherwise it displays help for the command or subcommand specified.", &helpCmd{})
	c, _ := parser.AddCommand("whoami", "Show invocation identities", "Show invocation login and account identities.", &whoAmICmd{})
	c.Aliases = []string{"id"}
}

type versionCmd struct{}

func (c *versionCmd) Execute(args []string) error {
	data := struct{ BuildID, BuildTime, BuildJob, BuildHost string }{
		BuildID,
		BuildTime,
		BuildJob,
		BuildHost,
	}
	switch appCtx.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	return appCtx.EmitTable([]string{"Build ID", "Build Date", "Build Job", "Build Host"},
		[][]string{{data.BuildID, data.BuildTime, data.BuildJob, data.BuildHost}}, nil)
}

type helpCmd struct {
	Positional struct {
		CommandSpecifier []string `positional-arg-name:"Command"`
	} `positional-args:"yes"`
}

func (c *helpCmd) Execute(args []string) error {
	parser.Command.Active = nil // want global help by default
	if len(c.Positional.CommandSpecifier) > 0 {
		cName := c.Positional.CommandSpecifier[0]
		cmd := parser.Find(cName)
		if cmd != nil {
			if len(c.Positional.CommandSpecifier) > 1 {
				scName := c.Positional.CommandSpecifier[1]
				sc := cmd.Find(scName)
				if sc != nil {
					cmd = sc
					// adjust the name to show the parent command - subtly different from "cmd subcmd -h" when showing options
					cmd.Name = cName + " " + scName
				} else {
					fmt.Fprintf(outputWriter, "Subcommand \"%s %s\" not found!\n", cName, scName)
				}
			}
			parser.Command.Active = cmd
		} else {
			fmt.Fprintf(outputWriter, "Command \"%s\" not found!\n", cName)
		}
	}
	parser.WriteHelp(outputWriter)
	return nil
}

type whoAmICmd struct{}

func (c *whoAmICmd) Execute(args []string) error {
	if appCtx.LoginName == "" && appCtx.Account == "" && appCtx.ClientCert != "" && appCtx.ClientKey != "" {
		fmt.Fprintln(outputWriter, "Internal user")
		return nil
	}
	var err error
	fmt.Fprintln(outputWriter, "LoginName:", appCtx.LoginName)
	fmt.Fprintln(outputWriter, "  Account:", appCtx.Account)
	if appCtx.LoginName == "" && appCtx.Account == "" {
		err = fmt.Errorf("Specify Account and LoginName in the [Application Options] section of the config file or use the -A and --login flags")
	} else if appCtx.LoginName == "" {
		err = fmt.Errorf("Specify LoginName in the [Application Options] section of the config file or the --login flag")
	} else if appCtx.Account == "" {
		err = fmt.Errorf("Specify Account in the [Application Options] section of the config file or the -A flag")
	}
	return err
}

func commandHandler(command flags.Commander, args []string) error {
	if command == nil {
		return nil
	}
	// directly invoke commands that don't need the API
	// if there are many such commands we could use reflection and a list of types that are exempt
	if hc, ok := command.(*helpCmd); ok {
		return hc.Execute(args)
	}
	if vc, ok := command.(*versionCmd); ok {
		return vc.Execute(args)
	}
	// initialize the API
	var err error
	if err = appCtx.InitAPI(); err == nil {
		if err = command.Execute(args); err == nil {
			err = appCtx.PersistToken()
		}
	}
	return err
}

// remember which transport/auth args were set in the command line using a separate parser
func checkArgsForSpecialFlagOverrides(args []string) {
	// no defaults in this abstraction of AppCtx
	type CommAuthFlags struct {
		Host       string `long:"host"`
		Port       int    `long:"port"`
		SocketPath string `long:"socket-path"`
		NoLogin    bool   `long:"no-login"`
		LoginName  string `long:"login"`
	}
	tac := &CommAuthFlags{}
	var tp = flags.NewParser(tac, (flags.Default&^flags.PrintErrors)|flags.IgnoreUnknown)
	_, e := tp.ParseArgs(args)
	if e == nil {
		if tac.Host != "" {
			appCtx.hostArg = true
		}
		if tac.Port != 0 {
			appCtx.portArg = true
		}
		if tac.SocketPath != "" {
			appCtx.socketArg = true
		}
		if tac.NoLogin == true {
			appCtx.noLoginArg = true
		}
		if tac.LoginName != "" {
			appCtx.loginNameArg = true
		}
	}
}

func parseAndRun(args []string) error {
	homeDir, ok := os.LookupEnv(eHOME)
	if !ok {
		homeDir = "/"
	}
	checkArgsForSpecialFlagOverrides(args)
	defIniFile = fmt.Sprintf(iniFileNameTemplate, homeDir, Appname)
	iniFile, ok = os.LookupEnv(iniFileEnv)
	if !ok {
		iniFile = defIniFile
	}
	if e := flags.NewIniParser(parser).ParseFile(iniFile); e != nil {
		if !os.IsNotExist(e) {
			return fmt.Errorf("%s: INI file error: %s", iniFile, e)
		}
	}
	parser.CommandHandler = commandHandler
	_, err := parser.ParseArgs(args)
	if err != nil && !flippedFirstTwoArgs {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrCommandRequired {
			return fmt.Errorf("%s followed by an action; it is also possible to specify the action before the object", e.Message)
		}
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrUnknownCommand && len(args) >= 2 {
			verb := args[0]
			noun := args[1]
			if cmd := parser.Find(noun); cmd != nil {
				if subcmd := cmd.Find(verb); subcmd != nil {
					args[0] = noun
					args[1] = verb
					flippedFirstTwoArgs = true
					return parseAndRun(args)
				}
			}
		}
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprint(outputWriter, err.Error())
			return nil
		}
		return fmt.Errorf("%s", err.Error())
	}
	return nil
}

type exitFn func(int)

var exitHook exitFn = os.Exit

func main() {
	appCtx.Emitter = &StdoutEmitter{}
	if err := parseAndRun(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s.\n", err.Error())
		exitHook(1)
	}
}
