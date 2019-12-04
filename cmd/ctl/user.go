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
	"sort"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

func init() {
	initUser()
}

const interactivePassword = "\x00\xff\x00\xff\x00\xff\x00\xff\x00\xff\x00"

func initUser() {
	cmd, _ := parser.AddCommand("user", "User object commands", "User object subcommands", &userCmd{})
	cmd.Aliases = []string{"users"}
	cmd.AddCommand("create", "Create a user", "Create a User object.", &userCreateCmd{})
	cmd.AddCommand("delete", "Delete a user", "Delete a User object.", &userDeleteCmd{})
	cmd.AddCommand("list", "List users", "List or search for User objects.", &userListCmd{})
	c, _ := cmd.AddCommand("modify", "Modify a user", "Modify a User object.", &userModifyCmd{})
	// replace the optional value with something that very difficult to type
	c.FindOptionByLongName("password").OptionalValue = []string{interactivePassword}
	cmd.AddCommand("show-columns", "Show user table columns", "Show names of columns used in table format.", &showColsCmd{columns: userHeaders})
}

type userCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

// user record keys/headers and their description
var userHeaders = map[string]string{
	hID:             dID,
	hTimeCreated:    dTimeCreated,
	hTimeModified:   dTimeModified,
	hVersion:        dVersion,
	hAuthIdentifier: "The identifier known to the authentication system.",
	hDisabled:       "Indicates if the user is disabled.",
	hProfile:        "User profile.",
}

var userDefaultHeaders = []string{hAuthIdentifier, hDisabled}

// makeRecord creates a map of properties
func (c *userCmd) makeRecord(o *models.User) map[string]string {
	profile := []string{}
	for k, v := range o.Profile {
		profile = append(profile, k+":"+v.Value)
	}
	sort.Strings(profile)
	return map[string]string{
		hID:             string(o.Meta.ID),
		hTimeCreated:    time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:   time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:        fmt.Sprintf("%d", o.Meta.Version),
		hAuthIdentifier: o.AuthIdentifier,
		hDisabled:       fmt.Sprintf("%v", o.Disabled),
		hProfile:        strings.Join(profile, "\n"),
	}
}

func (c *userCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(userHeaders), userDefaultHeaders)
	return err
}

func (c *userCmd) Emit(data []*models.User) error {
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

func (c *userCmd) list(params *user.UserListParams) ([]*models.User, error) {
	if params == nil {
		params = user.NewUserListParams()
	}
	res, err := appCtx.API.User().UserList(params)
	if err != nil {
		if e, ok := err.(*user.UserListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type userCreateCmd struct {
	AuthIdentifier string            `short:"I" long:"auth-identifier" description:"An authentication identifier" required:"yes"`
	Disabled       bool              `long:"disabled" description:"The user is disabled if true"`
	Password       string            `short:"p" long:"password" default-mask:"-" description:"Password for the new user. If not specified, you will be prompted to enter it interactively (recommended)"`
	Profile        map[string]string `short:"P" long:"profile" description:"A profile property name:value pair. Repeat as necessary. The property named 'userName' contains the user display name"`
	Columns        string            `short:"c" long:"columns" description:"Comma separated list of column names"`

	userCmd
	remainingArgsCatcher
}

func (c *userCreateCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := user.NewUserCreateParams().WithPayload(&models.User{})
	params.Payload.AuthIdentifier = c.AuthIdentifier
	params.Payload.Disabled = c.Disabled
	if c.Password == "" {
		pw, err := paranoidPasswordPrompt("Password")
		if err != nil {
			return err
		}
		c.Password = pw
	}
	params.Payload.Password = c.Password
	params.Payload.Profile = map[string]models.ValueType{}
	for k, v := range c.Profile {
		params.Payload.Profile[k] = models.ValueType{Kind: "STRING", Value: v}
	}
	res, err := appCtx.API.User().UserCreate(params)
	if err != nil {
		if e, ok := err.(*user.UserCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.User{res.Payload})
}

type userDeleteCmd struct {
	AuthIdentifier string `short:"I" long:"auth-identifier" description:"An authentication identifier" required:"yes"`
	Confirm        bool   `long:"confirm" description:"Confirm the deletion of the user"`

	userCmd
	remainingArgsCatcher
}

func (c *userDeleteCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the '%s' user", c.AuthIdentifier)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	lRes, err := c.list(user.NewUserListParams().WithAuthIdentifier(&c.AuthIdentifier))
	if err != nil {
		return err
	} else if len(lRes) != 1 {
		return fmt.Errorf("User '%s' not found", c.AuthIdentifier)
	}
	dParams := user.NewUserDeleteParams().WithID(string(lRes[0].Meta.ID))
	if _, err := appCtx.API.User().UserDelete(dParams); err != nil {
		if e, ok := err.(*user.UserDeleteDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type userListCmd struct {
	Name           string `short:"n" long:"name" description:"A user name pattern"`
	AuthIdentifier string `short:"I" long:"auth-identifier" description:"An authentication identifier"`
	Columns        string `short:"c" long:"columns" description:"Comma separated list of column names"`

	userCmd
	remainingArgsCatcher
}

func (c *userListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := user.NewUserListParams()
	params.AuthIdentifier = &c.AuthIdentifier
	params.UserNamePattern = &c.Name
	var res []*models.User
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}

type userModifyCmd struct {
	AuthIdentifier    string            `short:"I" long:"auth-identifier" description:"The authentication identifier of the User object to be modified. If not specified, the --login is used"`
	NewAuthIdentifier string            `short:"N" long:"new-auth-identifier" description:"The new authentication identifier for the object"`
	Disable           bool              `long:"disable" description:"The user is disabled if set"`
	Enable            bool              `long:"enable" description:"The user is enabled if set"`
	Password          string            `short:"p" long:"password" optional:"1" optional-value:"interactive" default-mask:"-" description:"New password for the user. If specified without a value, you will be prompted to enter it interactively (recommended)"`
	Profile           map[string]string `short:"P" long:"profile" description:"A profile property name:value pair. Repeat as necessary. The property named 'userName' contains the user display name"`
	ProfileAction     string            `long:"profile-action" description:"Specifies how to process profile properties" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Version           int32             `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns           string            `short:"c" long:"columns" description:"Comma separated list of column names"`

	userCmd
	remainingArgsCatcher
}

func (c *userModifyCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	if c.AuthIdentifier == "" {
		c.AuthIdentifier = appCtx.LoginName
		if c.AuthIdentifier == "" {
			return fmt.Errorf("either --login or --auth-identifier is required")
		}
	}
	lRes, err := c.list(user.NewUserListParams().WithAuthIdentifier(&c.AuthIdentifier))
	if err != nil {
		return err
	} else if len(lRes) != 1 {
		return fmt.Errorf("User '%s' not found", c.AuthIdentifier)
	}
	nChg := 0
	uParams := user.NewUserUpdateParams().WithPayload(&models.UserMutable{})
	if c.NewAuthIdentifier != "" {
		uParams.Payload.AuthIdentifier = c.NewAuthIdentifier
		uParams.Set = append(uParams.Set, "authIdentifier")
		nChg++
	}
	if c.Disable || c.Enable {
		if c.Disable && c.Enable {
			return fmt.Errorf("do not specify 'enable' and 'disable' together")
		}
		uParams.Payload.Disabled = c.Disable
		uParams.Set = append(uParams.Set, "disabled")
		nChg++
	}
	if c.Password == interactivePassword {
		c.Password, err = paranoidPasswordPrompt("New password")
		if err != nil {
			return err
		}
	}
	if c.Password != "" {
		uParams.Payload.Password = c.Password
		uParams.Set = append(uParams.Set, "password")
		nChg++
	}
	if len(c.Profile) != 0 || c.ProfileAction == "SET" {
		uParams.Payload.Profile = map[string]models.ValueType{}
		for k, v := range c.Profile {
			uParams.Payload.Profile[k] = models.ValueType{Kind: "STRING", Value: v}
		}
		switch c.ProfileAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "profile")
		case "SET":
			uParams.Set = append(uParams.Set, "profile")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "profile")
		}
		nChg++
	}
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	uParams.ID = string(lRes[0].Meta.ID)
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *user.UserUpdateOK
	if res, err = appCtx.API.User().UserUpdate(uParams); err != nil {
		if e, ok := err.(*user.UserUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.User{res.Payload})
}
