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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCookieStore(t *testing.T) {
	assert := assert.New(t)

	t.Log("case: uninitialized")
	mockCtrl := gomock.NewController(t)
	defer func() {
		mockCtrl.Finish()
		systemObj = System{}
	}()
	api := mock.NewMockDBAPI(mockCtrl)
	s := newCookieStore(api)
	assert.Zero(s.Secret())

	t.Log("case: success")
	systemObj.ID = primitive.NewObjectID()
	cs, err := s.Store(nil)
	assert.NotNil(cs)
	assert.NoError(err)
	secret := s.Secret()
	expSecret := append([]byte(secretPrefix), systemObj.ID[:]...)
	assert.Equal(expSecret, secret)

	t.Log("case: failure")
	systemObj = System{}
	s.store = nil
	api.EXPECT().MustBeReady().Return(errUnknownError)
	cs, err = s.Store(nil)
	assert.Nil(cs)
	assert.Equal(errNotReady, err)
}
