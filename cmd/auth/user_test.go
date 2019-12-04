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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

func TestFindUser(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	userObj := bson.M{
		"authidentifier": "fake",
		"password":       "invalid",
	}

	t.Log("case: not ready")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errUnknownError)
	obj, err := findUser(nil, api, LoginParams{})
	assert.Nil(obj)
	assert.Equal(errNotReady, err)
	mockCtrl.Finish()

	t.Log("case: not found")
	ctx := context.WithValue(context.Background(), struct{}{}, 24)
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db := mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr := mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(mongo.ErrNoDocuments)
	api.EXPECT().ErrorCode(mongo.ErrNoDocuments).Return(mongodb.ECKeyNotFound)
	obj, err = findUser(ctx, api, LoginParams{"fake", "not used"})
	assert.Nil(obj)
	assert.Equal(errInvalidUser, err)
	mockCtrl.Finish()

	t.Log("case: db error")
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(context.DeadlineExceeded)
	api.EXPECT().ErrorCode(context.DeadlineExceeded).Return(mongodb.ECInterrupted)
	api.EXPECT().Logger().Return(l)
	obj, err = findUser(ctx, api, LoginParams{"fake", "not used"})
	assert.Nil(obj)
	assert.Equal(errNotReady, err)
	mockCtrl.Finish()

	t.Log("case: CompareHashAndPassword fails")
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, userObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	api.EXPECT().Logger().Return(l)
	obj, err = findUser(ctx, api, LoginParams{"fake", "fake"})
	assert.Nil(obj)
	assert.Error(err)
	assert.NotEqual(errInvalidUser, err)
	assert.NotEqual(errDisabledUser, err)
	mockCtrl.Finish()

	t.Log("case: password mismatch")
	pw, err := bcrypt.GenerateFromPassword([]byte("fake"), 0)
	if !assert.NoError(err) {
		assert.FailNow("bcrypt.GenerateFromPassword must not fail")
	}
	userObj["password"] = string(pw)
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, userObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	obj, err = findUser(ctx, api, LoginParams{"fake", "wrong"})
	assert.Nil(obj)
	assert.Equal(errInvalidUser, err)
	mockCtrl.Finish()

	t.Log("case: disabled")
	userObj["disabled"] = true
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, userObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	obj, err = findUser(ctx, api, LoginParams{"fake", "fake"})
	assert.Nil(obj)
	assert.Equal(errDisabledUser, err)
	mockCtrl.Finish()

	t.Log("case: success")
	userObj["disabled"] = false
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, userObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	obj, err = findUser(ctx, api, LoginParams{"fake", "fake"})
	if assert.NotNil(obj) {
		assert.Equal("fake", obj.AuthIdentifier)
		assert.False(obj.Disabled)
		assert.Equal(string(pw), obj.Password)
	}
	assert.NoError(err)
}
