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
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-ini/ini"
	"golang.org/x/crypto/ssh/terminal"
)

func init() {
	initAuth()
}

func initAuth() {
	cmd, _ := parser.AddCommand("auth", "Authentication commands", "Authentication subcommands", &authCmd{})
	cmd.AddCommand("list", "List stored credentials", "List stored credentials.", &authListCmd{})
	cmd.AddCommand("login", "Log in", "Log into the management service on a specific host and port.", &authLoginCmd{})
	cmd.AddCommand("logout", "Log out", "Log out of the management service.", &authLogoutCmd{})
	cmd.AddCommand("show-columns", "Show Authentication table columns", "Show names of columns used in table format", &showColsCmd{columns: authHeaders})

	credFileEnv = strings.ToUpper(Appname + "_CREDENTIALS_FILE")
	homeDir, _ := os.LookupEnv(eHOME)
	defCredFile = fmt.Sprintf(credFileNameTemplate, homeDir, Appname)
	var ok bool
	if credFile, ok = os.LookupEnv(credFileEnv); !ok {
		credFile = defCredFile
	}
}

type authCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cfg *ini.File
}

// authData holds the authentication data in a form that is usable by makeRecord()
type authData struct {
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	AuthIdentifier string     `json:"authIdentifier"`
	Token          string     `json:"token"`
	Expiry         *time.Time `json:"expiry"`
}

// authHeaders record keys/headers and their description
var authHeaders = map[string]string{
	hManagementHost: "The management service host and port.",
	hAuthIdentifier: "The authentication identifier.",
	hToken:          "The authentication token.",
	hTimeExpires:    "The expiration time.",
}

var authDefaultHeaders = []string{hManagementHost, hAuthIdentifier, hTimeExpires}

func (c *authCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(authHeaders), authDefaultHeaders)
	return err
}

// makeRecord creates a map of properties
func (c *authCmd) makeRecord(o *authData) map[string]string {
	host := o.Host
	if o.Port > 0 {
		host = fmt.Sprintf("%s:%d", o.Host, o.Port)
	}
	expires := ""
	if o.Expiry != nil {
		expires = o.Expiry.Format(time.RFC3339)
		if o.Expiry.IsZero() {
			expires = "expired"
		}
	}
	return map[string]string{
		hManagementHost: host,
		hAuthIdentifier: o.AuthIdentifier,
		hToken:          o.Token,
		hTimeExpires:    expires,
	}
}

var credFileNameTemplate = "%s/.nuvoloso/%s-credentials.ini"
var credFileEnv string
var defCredFile string
var credFile string

// PrivatePerm is the FileMode that allows read and write by user only
const PrivatePerm os.FileMode = 0600

// readCredentialFile reads the credentials file if it exists and is correctly protected
func (c *authCmd) readCredentialsFile() error {
	if c.cfg != nil {
		return nil
	}
	stat, err := os.Stat(credFile)
	if err == nil && stat.Mode().Perm() != PrivatePerm {
		return errors.New("incorrect credentials file permissions")
	}
	c.cfg, err = ini.LoadSources(ini.LoadOptions{Loose: true, Insensitive: true}, credFile)
	return err
}

// writeCredentialFile (re)writes the credentials file
func (c *authCmd) writeCredentialsFile() error {
	tmpFile := credFile + ".new"
	os.Remove(tmpFile) // ignore errors
	file, err := os.OpenFile(tmpFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, PrivatePerm)
	if err != nil {
		return err
	}
	_, err = c.cfg.WriteTo(file)
	err2 := file.Close()
	if err == nil {
		err = err2
	}
	if err == nil {
		err = os.Rename(tmpFile, credFile)
	}
	return err
}

// getToken gets the current token for the given [host,port,authIdentifier] tuple
func (c *authCmd) getToken(host string, port int, authIdentifier string) (string, error) {
	if err := c.readCredentialsFile(); err != nil {
		return "", err
	}
	sectionName := fmt.Sprintf("%s,%d,%s", host, port, authIdentifier)
	section := c.cfg.Section(sectionName)
	return section.Key("token").Value(), nil
}

// setToken sets the token for a [host,port,authIdentifier] tuple, re-writing the credential file.
func (c *authCmd) setToken(host string, port int, authIdentifier string, token string) error {
	if err := c.readCredentialsFile(); err != nil {
		return err
	}
	if host == "" || strings.IndexAny(host, "],") >= 0 {
		return errors.New("invalid host")
	} else if authIdentifier == "" || strings.IndexAny(authIdentifier, "],") >= 0 {
		return errors.New("invalid authIdentifier")
	} else if port <= 0 {
		return errors.New("invalid port")
	} else if token == "" {
		return errors.New("invalid token")
	}
	sectionName := fmt.Sprintf("%s,%d,%s", host, port, authIdentifier)
	c.cfg.Section(sectionName).Key("token").SetValue(token)
	return c.writeCredentialsFile()
}

func (c *authCmd) Emit(data []*authData) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	rows := make([][]string, len(data))
	for i, o := range data {
		rec := c.makeRecord(o)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows[i] = row
	}
	return appCtx.EmitTable(c.tableCols, rows, nil)
}

type authListCmd struct {
	All            bool   `short:"a" long:"all" description:"List all stored credentials, ignoring host, port and login name"`
	Columns        string `short:"c" long:"columns" description:"Comma separated list of column names"`
	SkipValidation bool   `short:"x" long:"skip-validation" description:"Skip validating the token, which also means the TimeExpires values will be empty"`

	authCmd
	remainingArgsCatcher
}

func (c *authListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = c.readCredentialsFile(); err != nil {
		return err
	}
	res := []*authData{}
	for _, section := range c.cfg.Sections() {
		name := section.Name()
		if name == ini.DEFAULT_SECTION {
			continue
		}
		parts := strings.Split(name, ",")
		if len(parts) != 3 {
			return fmt.Errorf("corrupt credentials file, invalid section [%s]", name)
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("corrupt credentials file, invalid section [%s]", name)
		}
		host, login := parts[0], parts[2]
		if !c.All {
			if appCtx.Host != "" && !strings.EqualFold(appCtx.Host, host) {
				continue
			}
			if appCtx.LoginName != "" && !strings.EqualFold(appCtx.LoginName, login) {
				continue
			}
			if appCtx.Port != 0 && appCtx.Port != port {
				continue
			}
		}
		expires := &time.Time{}
		token := section.Key("token").Value()
		if token != "" && !c.SkipValidation {
			appCtx.API.Authentication().SetAuthToken(token)
			resp, err := appCtx.API.Authentication().Validate(false)
			if err == nil {
				*expires = time.Unix(resp.Expiry, 0)
				token = resp.Token
			} else if strings.Index(err.Error(), "5") == 0 {
				// error messages that start with "5" are communication errors, otherwise the problem is with the token
				return err
			}
		} else {
			expires = nil
		}

		row := &authData{
			Host:           host,
			Port:           port,
			AuthIdentifier: login,
			Token:          token,
			Expiry:         expires,
		}
		res = append(res, row)
	}
	appCtx.API.Authentication().SetAuthToken("") // keep appCtx from re-writing the creds file
	return c.Emit(res)
}

type passwordFn func(int) ([]byte, error)

var passwordHook passwordFn = terminal.ReadPassword

type authLoginCmd struct {
	Password       string `short:"p" long:"password" default-mask:"-" description:"Log in with this password. It is better to specify the password in the credential file or enter it interactively"`
	ForgetPassword bool   `short:"f" long:"forget-password" description:"Do not store the password in the credential file. If it was previously stored, it will be removed on success"`
	StorePassword  bool   `short:"s" long:"store-password" description:"Store the password in the credential file. This is the default behavior"`
	Columns        string `short:"c" long:"columns" description:"Comma separated list of column names"`

	authCmd
	remainingArgsCatcher
}

func (c *authLoginCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if appCtx.SocketPath != "" {
		return errors.New("login not supported with unix socket")
	} else if appCtx.NoLogin {
		return errors.New("the flag `--no-login' must not be specified")
	} else if appCtx.LoginName == "" {
		return errors.New("the required flag `--login' was not specified")
	}
	if err = c.readCredentialsFile(); err != nil {
		return err
	}
	sectionName := fmt.Sprintf("%s,%d,%s", appCtx.Host, appCtx.Port, appCtx.LoginName)
	if c.Password == "" {
		c.Password, err = util.DeObfuscate(c.cfg.Section(sectionName).Key("password").Value())
		if c.Password == "" || err != nil {
			if err != nil {
				fmt.Fprintf(outputWriter, "Ignoring invalid stored password...\n")
			}
			fmt.Fprint(outputWriter, "Password: ")
			bytePassword, err := passwordHook(int(syscall.Stdin))
			fmt.Fprintln(outputWriter)
			if err != nil {
				return err
			}
			c.Password = strings.TrimSpace(string(bytePassword))
		}
	}
	resp, err := appCtx.API.Authentication().Authenticate(appCtx.LoginName, c.Password)
	if err != nil {
		return err
	}
	if c.ForgetPassword {
		c.Password = ""
	}
	c.cfg.Section(sectionName).Key("password").SetValue(util.Obfuscate(c.Password))
	if err = c.setToken(appCtx.Host, appCtx.Port, appCtx.LoginName, resp.Token); err != nil {
		return err
	}
	expiry := time.Unix(resp.Expiry, 0)
	res := []*authData{&authData{
		Host:           appCtx.Host,
		Port:           appCtx.Port,
		AuthIdentifier: appCtx.LoginName,
		Token:          resp.Token,
		Expiry:         &expiry,
	}}
	appCtx.API.Authentication().SetAuthToken("") // keep appCtx from re-writing the creds file
	return c.Emit(res)
}

type authLogoutCmd struct {
	ForgetPassword bool `short:"f" long:"forget-password" description:"Forget any previously stored password as well. This is the default behavior"`
	StorePassword  bool `short:"s" long:"store-password" description:"Leave any previously stored password behind"`

	authCmd
	remainingArgsCatcher
}

func (c *authLogoutCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if appCtx.SocketPath != "" {
		return errors.New("logout not supported with unix socket")
	} else if appCtx.NoLogin {
		return errors.New("the flag `--no-login' must not be specified")
	} else if appCtx.LoginName == "" {
		return errors.New("the required flag `--login' was not specified")
	}
	if err = c.readCredentialsFile(); err != nil {
		return err
	}
	sectionName := fmt.Sprintf("%s,%d,%s", appCtx.Host, appCtx.Port, appCtx.LoginName)
	section, err := c.cfg.GetSection(sectionName)
	if err != nil { // err means there is no such section
		return nil
	}
	if c.StorePassword {
		section.DeleteKey("token")
	} else {
		c.cfg.DeleteSection(sectionName)
	}
	return c.writeCredentialsFile()
}
