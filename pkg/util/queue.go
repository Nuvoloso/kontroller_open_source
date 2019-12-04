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
	"reflect"
	"sync"
)

// Queue provides thread safe and type independent queuing semantics.
type Queue struct {
	mux    sync.Mutex
	elType reflect.Type
	queue  []interface{}
}

// NewQueue returns a Queue object.
// The provided sampleElement defines the type of element accepted by the queue.
// If nil then the queue has no fixed element type.
func NewQueue(sampleElement interface{}) *Queue {
	q := &Queue{}
	if sampleElement != nil {
		q.elType = reflect.TypeOf(sampleElement)
	}
	q.queue = make([]interface{}, 0, 32)
	return q
}

// Length returns the length of the queue
func (q *Queue) Length() int {
	q.mux.Lock()
	defer q.mux.Unlock()
	return len(q.queue)
}

// Type returns the type of element or nil if the queue is heterogeneous.
func (q *Queue) Type() reflect.Type {
	return q.elType
}

// Append adds multiple elements to the end of the queue. It panics if there is a type mismatch.
func (q *Queue) Append(elements []interface{}) {
	q.mux.Lock()
	defer q.mux.Unlock()
	if q.elType != nil {
		for i, el := range elements {
			if q.elType != reflect.TypeOf(el) {
				panic(fmt.Errorf("element %d type invalid", i))
			}
		}
	}
	q.queue = append(q.queue, elements...)
}

// Add adds an element to the end of the queue. It panics if there is a type mismatch.
// As a special case if the queue is homogeneous then the argument can be a slice of this type.
func (q *Queue) Add(elOrSliceOfEl interface{}) {
	q.mux.Lock()
	defer q.mux.Unlock()
	if q.elType != nil {
		vE := reflect.ValueOf(elOrSliceOfEl)
		if vE.Type() != q.elType {
			if (vE.Kind() == reflect.Slice || vE.Kind() == reflect.Array) && vE.Type().Elem() == q.elType {
				// array or slice of elements of the correct type
				for i := 0; i < vE.Len(); i++ {
					q.queue = append(q.queue, vE.Index(i).Interface())
				}
				return
			}
			panic(fmt.Errorf("invalid argument type"))
		}
	}
	q.queue = append(q.queue, elOrSliceOfEl)
}

// PeekHead returns the element at the head of the queue without dequeuing it, or nil if the queue is empty
func (q *Queue) PeekHead() interface{} {
	q.mux.Lock()
	defer q.mux.Unlock()
	if len(q.queue) > 0 {
		return q.queue[0]
	}
	return nil
}

// PopHead dequeues and returns the element at the head of the queue, or nil if the queue is empty
func (q *Queue) PopHead() interface{} {
	q.mux.Lock()
	defer q.mux.Unlock()
	if len(q.queue) > 0 {
		h := q.queue[0]
		q.popInJ(1)
		return h
	}
	return nil
}

// PopN dequeues up to N elements from the head of the queue.
// It returns the actual count dropped.
func (q *Queue) PopN(n int) int {
	q.mux.Lock()
	defer q.mux.Unlock()
	numDropped := 0
	if len(q.queue) < n {
		n = len(q.queue)
	}
	if n > 0 {
		numDropped = n
		q.popInJ(n)
	}
	return numDropped
}

// popInJ removes j elements from the head of the queue.
// It is a no-op if there are fewer than j elements in the queue.
// Invoke holding the mutex.
func (q *Queue) popInJ(j int) {
	if len(q.queue) >= j {
		// https://github.com/golang/go/wiki/SliceTricks (non-leaky cut)
		copy(q.queue[0:], q.queue[j:])
		for k, n := len(q.queue)-j, len(q.queue); k < n; k++ {
			q.queue[k] = nil
		}
		q.queue = q.queue[:len(q.queue)-j]
	}
}
