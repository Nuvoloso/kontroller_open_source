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

// vsIoMLimitBuffering drops elements from the VolumeSeries I/O pipeline queues
// taking into consideration the state of each stage.
// Note: this should be called only when a pipeline stage encounters an error or
// when the pipeline capacity is consistently exceeded for a period of time.
func (c *Component) vsIoMLimitBuffering() int {
	c.mux.Lock()
	defer c.mux.Unlock()
	return c.vsIoMLimitBufferingInLock()
}

func (c *Component) vsIoMLimitBufferingInLock() int {
	numDropped := 0
	var wSafe, pSafe bool
	excess := c.vsIoMPipelineCapacity()
	if excess > 0 && c.vIOW.InError {
		n := c.vIOW.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from inactive VolumeSeries I/O Metric writer queue", n)
			numDropped += n
			pSafe = true
		}
		excess = c.vsIoMPipelineCapacity()
	}
	if excess > 0 && c.vIOP.InError {
		n := c.vIOP.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from inactive VolumeSeries I/O Metric processor queue", n)
			numDropped += n
			wSafe = true
		}
		excess = c.vsIoMPipelineCapacity()
	}
	if excess > 0 && !wSafe {
		n := c.vIOW.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from active VolumeSeries I/O Metric writer queue", n)
			numDropped += n
		}
		excess = c.vsIoMPipelineCapacity()
	}
	if excess > 0 && !pSafe {
		n := c.vIOP.Queue.PopN(excess)
		if n > 0 {
			c.Log.Warningf("Dropped %d records from active VolumeSeries I/O Metric processor queue", n)
			numDropped += n
		}
	}
	c.numVIODropped += numDropped
	return numDropped
}

// vsIoMPipelineCapacity returns the available (-ve) or overconsumed (+ve) capacity
func (c *Component) vsIoMPipelineCapacity() int {
	return c.vIOP.Queue.Length() + c.vIOW.Queue.Length() - c.VolumeIOMaxBuffered
}

// vsIoMLimitCapacityWithBurstTolerance should be called periodically.
// It applies buffer limits if continuous violation of permitted capacity exceeds a threshold.
func (c *Component) vsIoMLimitCapacityWithBurstTolerance() int {
	c.mux.Lock()
	defer c.mux.Unlock()
	n := 0
	if c.vsIoMPipelineCapacity() > 0 {
		c.numVIOCapacityExceeded++
	} else {
		c.numVIOCapacityExceeded = 0
	}
	if c.numVIOCapacityExceeded > c.VolumeIOBurstTolerance {
		c.Log.Warning("VolumeSeries I/O Metric pipeline capacity tolerance threshold exceeded")
		n = c.vsIoMLimitBufferingInLock()
		if n > 0 {
			c.numVIOCapacityExceeded = 0
		}
	}
	return n
}
