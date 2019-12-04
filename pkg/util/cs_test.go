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


package util

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCriticalSectionGuard(t *testing.T) {
	assert := assert.New(t)

	csg := NewCriticalSectionGuard()
	assert.NotNil(csg.cond)
	assert.NotNil(csg.queue)
	assert.Empty(csg.queue)
	assert.False(csg.closed)

	// simple usage - no contention
	id1 := "id1"
	cst1, err := csg.Enter(id1)
	assert.NoError(err)
	assert.Len(csg.queue, 1)
	assert.Equal(id1, csg.queue[0])
	assert.Equal(csg.queue, csg.Status())
	assert.NotNil(cst1)
	assert.Equal(csg, cst1.csg)
	assert.Equal(id1, cst1.id)

	cst2, err := csg.Enter(id1)
	assert.Error(err)
	assert.Regexp("already in critical section", err)
	assert.Nil(cst2)
	assert.Equal(0, csg.Used)

	cst1.Leave()
	assert.Len(csg.queue, 0)
	assert.Nil(cst1.csg)
	assert.Empty(cst1.id)
	assert.Panics(func() { cst1.Leave() })
	assert.Equal(1, csg.Used)

	// contention
	contender := func(id string, ts *time.Time, q *[]string, e *error) {
		cst, err := csg.Enter(id)
		if err != nil {
			t.Logf("%s: Enter(): %s", id, err.Error())
			*e = fmt.Errorf("%s: %s", id, err.Error())
			return
		}
		t.Logf("** %s: %v", id, csg.queue)
		*q = append([]string{}, csg.queue...)
		*ts = time.Now()
		time.Sleep(1 * time.Millisecond)
		cst.Leave()
	}
	cst1, err = csg.Enter(id1) // hold the CS
	var ts2, ts3 time.Time
	var e2, e3 error
	var q2, q3 []string
	go contender("id2", &ts2, &q2, &e2)
	for len(csg.queue) != 2 {
		time.Sleep(1 * time.Millisecond)
	}
	assert.Equal([]string{"id1", "id2"}, csg.queue)
	assert.Equal(csg.queue, csg.Status())
	go contender("id3", &ts3, &q3, &e3)
	for len(csg.queue) != 3 {
		time.Sleep(1 * time.Millisecond)
	}
	assert.Equal([]string{"id1", "id2", "id3"}, csg.queue)
	assert.Equal(csg.queue, csg.Status())
	cst1.Leave() // exit cs
	for len(csg.queue) != 0 {
		time.Sleep(1 * time.Millisecond)
	}
	assert.NotEqual(ts2, ts3)
	assert.True(ts2.Before(ts3))
	assert.Equal([]string{"id2", "id3"}, q2)
	assert.Equal([]string{"id3"}, q3)
	assert.NoError(e2)
	assert.NoError(e3)

	// Drain
	assert.False(csg.closed)
	cst1, err = csg.Enter(id1) // hold the CS
	go contender("id2", &ts2, &q2, &e2)
	go contender("id3", &ts3, &q3, &e3)
	for len(csg.queue) != 3 {
		time.Sleep(1 * time.Millisecond)
	}
	var drained bool
	go func() {
		csg.Drain()
		drained = true
	}()
	for len(csg.queue) > 1 {
		time.Sleep(30 * time.Millisecond)
	}
	assert.True(csg.closed)
	assert.Equal([]string{"id1"}, csg.queue)
	assert.False(drained)
	assert.Error(e2)
	assert.Regexp("id2.*critical section closed", e2)
	assert.Error(e3)
	assert.Regexp("id3.*critical section closed", e3)
	cst1.Leave()
	for !drained {
		time.Sleep(1 * time.Millisecond)
	}
	_, err = csg.Enter(id1)
	assert.Error(err)
	assert.Regexp("critical section closed", err)
}
