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


package metrics

// sIoMLimitBuffering drops elements from the Storage I/O pipeline queues
// taking into consideration the state of each stage.
// Note: this should be called only when a pipeline stage encounters an error or
// when the pipeline capacity is consistently exceeded for a period of time.
func (c *Component) sIoMLimitBuffering() int {
	c.mux.Lock()
	defer c.mux.Unlock()
	return c.sIoMLimitBufferingInLock()
}

func (c *Component) sIoMLimitBufferingInLock() int {
	numDropped := 0
	var wSafe, pSafe bool
	excess := c.sIoMPipelineCapacity()
	if excess > 0 && c.sIOW.InError {
		n := c.sIOW.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from inactive Storage I/O Metric writer queue", n)
			numDropped += n
			pSafe = true
		}
		excess = c.sIoMPipelineCapacity()
	}
	if excess > 0 && c.sIOP.InError {
		n := c.sIOP.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from inactive Storage I/O Metric processor queue", n)
			numDropped += n
			wSafe = true
		}
		excess = c.sIoMPipelineCapacity()
	}
	if excess > 0 && !wSafe {
		n := c.sIOW.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from active Storage I/O Metric writer queue", n)
			numDropped += n
		}
		excess = c.sIoMPipelineCapacity()
	}
	if excess > 0 && !pSafe {
		n := c.sIOP.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from active Storage I/O Metric processor queue", n)
			numDropped += n
		}
	}
	c.numSIODropped += numDropped
	return numDropped
}

// sIoMPipelineCapacity returns the available (-ve) or overconsumed (+ve) capacity
func (c *Component) sIoMPipelineCapacity() int {
	return c.sIOP.Queue.Length() + c.sIOW.Queue.Length() - c.StorageIOMaxBuffered
}

// sIoMLimitCapacityWithBurstTolerance should be called periodically.
// It applies buffer limits if continuous violation of permitted capacity exceeds a threshold.
func (c *Component) sIoMLimitCapacityWithBurstTolerance() int {
	c.mux.Lock()
	defer c.mux.Unlock()
	n := 0
	if c.sIoMPipelineCapacity() > 0 {
		c.numSIOCapacityExceeded++
	} else {
		c.numSIOCapacityExceeded = 0
	}
	if c.numSIOCapacityExceeded > c.StorageIOBurstTolerance {
		c.Log.Warning("Storage I/O Metric pipeline capacity tolerance threshold exceeded")
		n = c.sIoMLimitBufferingInLock()
		if n > 0 {
			c.numSIOCapacityExceeded = 0
		}
	}
	return n
}
