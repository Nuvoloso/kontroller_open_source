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
	"errors"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/crypto/bcrypt"
)

const userCollection = "user"

// User object, only the fields required in this service are decoded
type User struct {
	AuthIdentifier string
	Disabled       bool
	Password       string
}

var errInvalidUser = errors.New("Incorrect username or password")
var errDisabledUser = errors.New("User is disabled, contact an administrator")

// findUser looks up the user, validates the password and checks if the user is disabled. The object is returned on success
func findUser(ctx context.Context, api mongodb.DBAPI, params LoginParams) (*User, error) {
	if api.MustBeReady() != nil {
		return nil, errNotReady
	}
	obj := &User{}
	if err := api.Client().Database(api.DBName()).Collection(userCollection).FindOne(ctx, bson.D{{Key: "authidentifier", Value: params.Username}}).Decode(obj); err != nil {
		if api.ErrorCode(err) == mongodb.ECKeyNotFound {
			return nil, errInvalidUser
		}
		api.Logger().Error("FindOne", err)
		return nil, errNotReady
	}
	api.ErrorCode(nil)
	if err := bcrypt.CompareHashAndPassword([]byte(obj.Password), []byte(strings.TrimSpace(params.Password))); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return nil, errInvalidUser
		}
		api.Logger().Error("CompareHashAndPassword", err)
		return nil, err
	}
	if obj.Disabled {
		return nil, errDisabledUser
	}
	return obj, nil
}
