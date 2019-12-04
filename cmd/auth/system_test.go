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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var errUnknownError = errors.New("unknown error")

func TestGetSystemID(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: not ready")
	mockCtrl := gomock.NewController(t)
	defer func() {
		mockCtrl.Finish()
		systemObj = System{}
	}()
	api := mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errUnknownError)
	id, err := getSystemID(nil, api)
	assert.Nil(id)
	assert.Equal(errNotReady, err)
	mockCtrl.Finish()

	t.Log("case: Decode error")
	ctx := context.WithValue(context.Background(), struct{}{}, 42)
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db := mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("system").Return(col)
	sr := mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(errUnknownError)
	api.EXPECT().ErrorCode(errUnknownError).Return(mongodb.ECInterrupted)
	api.EXPECT().Logger().Return(l)
	id, err = getSystemID(ctx, api)
	assert.Nil(id)
	assert.Equal(errNotReady, err)

	t.Log("case: success")
	systemObj = System{}
	expID := primitive.NewObjectID()
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("system").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, bson.M{"_id": expID})).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	id, err = getSystemID(ctx, api)
	assert.Equal(expID[:], id)
	assert.NoError(err)

	t.Log("case: success cached")
	assert.False(systemObj.ID.IsZero())
	id, err = getSystemID(ctx, api)
	assert.Equal(expID[:], id)
	assert.NoError(err)
}

type decodeMatcher struct {
	buf []byte
}

// newMockDecodeMatcher constructs a matcher for the mongo Decode() argument.
// The m parameter is any type that can be passed to bson.Marshal(), eg bson.M or a struct pointer.
// m may also be nil to skip attempting to use the value in Matches, for use in error conditions.
func newMockDecodeMatcher(t *testing.T, m interface{}) gomock.Matcher {
	if m == nil {
		return &decodeMatcher{}
	}
	buf, err := bson.Marshal(m)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "bson.Marshal should not fail")
	}
	return &decodeMatcher{buf: buf}
}

func (m *decodeMatcher) Matches(x interface{}) bool {
	if len(m.buf) == 0 { // nothing to copy, used to cover Decode returning an error, destination object must not be nil
		return x != nil
	}
	if x != nil {
		return bson.Unmarshal(m.buf, x) == nil
	}
	return false
}

func (m *decodeMatcher) String() string {
	if m.buf != nil {
		return "decoder matches object"
	}
	return "decoder matches nil"
}
