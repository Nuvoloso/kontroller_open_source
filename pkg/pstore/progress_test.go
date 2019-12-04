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


package pstore

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestProgressTimer(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	ctx := context.Background()

	ops, err := NewController(&ControllerArgs{Log: tl.Logger()})
	assert.NoError(err)
	assert.NotNil(ops)
	c, ok := ops.(*Controller)
	assert.True(ok)

	// create a progress file
	pf, err := ioutil.TempFile("", "progress*.json")
	assert.NoError(err)
	defer os.Remove(pf.Name())
	tl.Logger().Info("pfn: %s", pf.Name())
	tl.Flush()

	// use the default callback for testing
	pl := &progressLogger{
		id:     "fakeProgressLogger",
		logger: tl.Logger(),
	}
	ph := &SnapshotRestoreResult{} // a progressHolder object
	pt := c.progressStart(ctx, ph, pl, pf.Name())
	assert.NotNil(pt)
	assert.NotNil(pt.ticker)

	// stop the timer (it is tested elsewhere)
	pt.Stop()
	assert.Nil(pt.ticker)

	// write data to the progress file
	ph.Stats = CopyStats{
		BytesChanged:     100,
		BytesTransferred: 101,
		BytesUnchanged:   102,
		Size:             103,
		OffsetProgress:   104,
	}
	pfData, err := json.Marshal(ph)
	assert.NoError(err)
	_, err = pf.Write(pfData)
	assert.NoError(err)
	pf.Close()

	// file should be found
	pt.Beep(ctx)
	assert.True(tl.CountPattern("fakeProgressLogger") == 1)
	assert.NotEqual(&time.Time{}, pt.prevTimeStamp)
	tl.Flush()

	// same file on next pass - should be ignored
	pt.Beep(ctx)
	assert.True(tl.CountPattern("fakeProgressLogger") == 0)

	// file is garbled
	// Beep is sensitive to file system time granularity so re-create the file until the TS changes
	pFi, err := os.Stat(pt.progressFileName)
	assert.NoError(err)
	for {
		err = ioutil.WriteFile(pf.Name(), []byte("bad json"), 0600)
		assert.NoError(err)
		nFi, err := os.Stat(pf.Name())
		assert.NoError(err)
		if !nFi.ModTime().Equal(pFi.ModTime()) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	pt.Beep(ctx)
	assert.True(tl.CountPattern("fakeProgressLogger") == 0)

	// file not found
	os.Remove(pf.Name())
	pt.Beep(ctx)
	assert.True(tl.CountPattern("fakeProgressLogger") == 0)
}

func TestProgressReport(t *testing.T) {
	assert := assert.New(t)

	cs := &CopyStats{
		BytesChanged:     100,
		BytesTransferred: 101,
		BytesUnchanged:   102,
		OffsetProgress:   200,
		Size:             1000,
	}

	csPR := cs.progressReport()
	assert.NotNil(csPR)
	assert.Equal(cs.BytesTransferred, csPR.TransferredBytes)
	assert.Equal(cs.Size, csPR.TotalBytes)
	assert.Equal(cs.OffsetProgress, csPR.OffsetBytes)
	pc := float64(cs.OffsetProgress) * 100 / float64(cs.Size)
	assert.Equal(pc, csPR.PercentComplete)

	cs.OffsetProgress = 0
	csPR = cs.progressReport()
	assert.NotNil(csPR)
	assert.Equal(cs.BytesTransferred, csPR.TransferredBytes)
	assert.Equal(cs.Size, csPR.TotalBytes)
	assert.Equal(cs.OffsetProgress, csPR.OffsetBytes)
	pc = float64(cs.BytesTransferred) * 100 / float64(cs.Size)
	assert.Equal(pc, csPR.PercentComplete)

	sbr := &SnapshotBackupResult{
		Stats: *cs,
	}
	sbrPR := sbr.progressReport()
	assert.Equal(csPR, sbrPR)

	srr := &SnapshotRestoreResult{
		Stats: *cs,
	}
	srrPR := srr.progressReport()
	assert.Equal(csPR, srrPR)
}
