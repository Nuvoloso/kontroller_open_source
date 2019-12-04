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
	"os"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/util"
	logging "github.com/op/go-logging"
)

type progressLogger struct {
	id     string
	logger *logging.Logger
}

func (lpr progressLogger) CopyProgress(ctx context.Context, cpr CopyProgressReport) {
	lpr.logger.Infof("%s: %v", lpr.id, cpr)
}

// progressTimer is returned from ProgressStart
type progressTimer struct {
	cntrl            *Controller          // The controller
	progressFileName string               // the progress file name
	holder           progressHolder       // Holder of the progress
	reporter         CopyProgressReporter // receiver of progress
	ticker           *util.RoundingTicker // timing mechanism
	prevTimeStamp    time.Time            // last time stamp on the progress file
}

func (c *Controller) progressStart(ctx context.Context, res progressHolder, reporter CopyProgressReporter, pfn string) *progressTimer {
	pt := &progressTimer{
		cntrl:            c,
		progressFileName: pfn,
		holder:           res,
		reporter:         reporter,
	}
	rta := &util.RoundingTickerArgs{
		Log:    c.Log,
		Period: time.Second * 17, // unround numbers reduce timing issues
	}
	pt.ticker, _ = util.NewRoundingTicker(rta)
	pt.ticker.Start(pt)
	return pt
}

func (pt *progressTimer) Stop() {
	pt.ticker.Stop()
	pt.ticker = nil
}

// Beep is the function called regularly to report the progress
func (pt *progressTimer) Beep(ctx context.Context) {
	fi, err := os.Stat(pt.progressFileName)
	if err != nil {
		return // File not available yet
	}
	timeStamp := fi.ModTime()
	if timeStamp.Equal(pt.prevTimeStamp) {
		return // File hasn't changed
	}
	err = pt.cntrl.ReadResult(pt.progressFileName, pt.holder)
	if err != nil {
		return // File in unreadable state
	}
	cpr := pt.holder.progressReport()
	pt.reporter.CopyProgress(ctx, *cpr)
	pt.prevTimeStamp = timeStamp
}
