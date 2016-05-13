// Copyright 2016 CoreOS, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package stop implements a pattern for shutting down a group of processes.
package stop

import (
	"sync"
)

// AlreadyDone is a constant used to express that a process has already been
// stopped.
var AlreadyDone <-chan struct{}

func init() {
	// Create the value of AlreadyDone.
	closeMe := make(chan struct{})
	close(closeMe)
	AlreadyDone = closeMe
}

// Stoppable represents any process that can be stopped.
type Stoppable interface {
	Stop() <-chan struct{}
}

// Group represents many stoppable processes that can all be stopped at once.
type Group struct {
	stoppables     []StopperFunc
	stoppablesLock sync.Mutex
}

// NewGroup allocates a new group of processes that will be stopped together.
func NewGroup() *Group {
	return &Group{
		stoppables: make([]StopperFunc, 0),
	}
}

// StopperFunc is an alternative to implementing the Stoppable interface.
type StopperFunc func() <-chan struct{}

// Add inserts a stoppable process into a Group.
func (cg *Group) Add(toAdd Stoppable) {
	cg.stoppablesLock.Lock()
	defer cg.stoppablesLock.Unlock()

	cg.stoppables = append(cg.stoppables, toAdd.Stop)
}

// AddFunc inserts a callback into a Group.
func (cg *Group) AddFunc(toAddFunc StopperFunc) {
	cg.stoppablesLock.Lock()
	defer cg.stoppablesLock.Unlock()

	cg.stoppables = append(cg.stoppables, toAddFunc)
}

// Stop calls the stop method on all of the processes in a Group and waits for
// them to complete.
func (cg *Group) Stop() <-chan struct{} {
	cg.stoppablesLock.Lock()
	defer cg.stoppablesLock.Unlock()

	whenDone := make(chan struct{})

	waitChannels := make([]<-chan struct{}, 0, len(cg.stoppables))
	for _, toStop := range cg.stoppables {
		waitFor := toStop()
		if waitFor == nil {
			panic("Someone returned a nil chan from Stop")
		}
		waitChannels = append(waitChannels, waitFor)
	}

	cg.stoppables = make([]StopperFunc, 0)

	go func() {
		for _, waitForMe := range waitChannels {
			<-waitForMe
		}
		close(whenDone)
	}()

	return whenDone
}
