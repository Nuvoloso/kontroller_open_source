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
	"sync"
)

// CriticalSectionGuard supports fair access to a critical section of code (e.g. to implement a Monitor)
type CriticalSectionGuard struct {
	mux    sync.Mutex
	cond   *sync.Cond
	closed bool
	queue  []string
	Used   int // count of lockers (set on leave)
}

// CriticalSectionTicket must be used to signal departure from the critical section
type CriticalSectionTicket struct {
	csg *CriticalSectionGuard
	id  string
}

// NewCriticalSectionGuard returns an initialized CriticalSectionGuard object
func NewCriticalSectionGuard() *CriticalSectionGuard {
	csg := &CriticalSectionGuard{}
	csg.cond = sync.NewCond(&csg.mux)
	csg.queue = make([]string, 0, 10)
	return csg
}

// Enter blocks the invoker until the critical section can be entered.
// It can fail for various reasons.
func (csg *CriticalSectionGuard) Enter(id string) (*CriticalSectionTicket, error) {
	csg.mux.Lock()
	defer csg.mux.Unlock()
	if Contains(csg.queue, id) {
		return nil, fmt.Errorf("already in critical section")
	}
	if csg.closed {
		return nil, fmt.Errorf("critical section closed")
	}
	csg.queue = append(csg.queue, id)
	for !csg.closed && csg.queue[0] != id {
		csg.cond.Wait()
	}
	if csg.closed {
		return nil, fmt.Errorf("critical section closed")
	}
	return &CriticalSectionTicket{csg: csg, id: id}, nil
}

// Drain cancels all waiters and waits for the critical section to empty.
func (csg *CriticalSectionGuard) Drain() {
	csg.mux.Lock()
	defer csg.mux.Unlock()
	csg.closed = true
	if len(csg.queue) > 1 {
		csg.queue = csg.queue[0:1]
	}
	csg.cond.Broadcast()
	for len(csg.queue) != 0 {
		csg.cond.Wait()
	}
}

// Status returns the current content of the critical section queue at the invocation point in time.
// The head of the queue holds the critical section.
func (csg *CriticalSectionGuard) Status() []string {
	csg.mux.Lock()
	defer csg.mux.Unlock()
	q := make([]string, len(csg.queue))
	copy(q, csg.queue)
	return q
}

// Leave is used to exit the critical section. It can only be used once.
func (cst *CriticalSectionTicket) Leave() {
	csg := cst.csg
	csg.mux.Lock()
	defer csg.mux.Unlock()
	if len(csg.queue) > 0 && csg.queue[0] == cst.id {
		csg.queue = csg.queue[1:]
		csg.cond.Broadcast()
		csg.Used++
	}
	// invalidate the ticket
	cst.id = ""
	cst.csg = nil
}
