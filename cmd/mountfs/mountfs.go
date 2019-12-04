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
	"fmt"
	"os"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/mount"
	"github.com/jessevdk/go-flags"
	logging "github.com/op/go-logging"
)

// Build information passed in via ld flags
var (
	BuildID   string
	BuildTime string
	BuildHost string
	BuildJob  string
	Appname   string
)

type appCtx struct {
	FsType      string `short:"t" long:"fs-type" description:"File system type during mount." default:"ext4"`
	QueryMount  bool   `short:"q" long:"query-mount" description:"Check if a mount is done. Requires source and target paths in that order."`
	DoMount     bool   `short:"m" long:"mount" description:"Perform a mount operation. Requires source and target paths in that order."`
	DoUnmount   bool   `short:"u" long:"unmount" description:"Perform an unmount operation. Requires target path."`
	ReadOnly    bool   `short:"r" long:"read-only" description:"Disable write operations. Optional for query-mount/mount."`
	TimeoutSecs int    `long:"timeout" description:"Specify the command timeout in seconds." default:"300"`
}

func (a *appCtx) Usage() string {
	return fmt.Sprintf("[-q MountArgs | -m MountArgs | -u UnmountArgs]\n\n"+
		"1. Query:\n\t%s -q [-t fsType] [-r] [--timeout Seconds] Source Target\n"+
		"2. Mount:\n\t%s -m [-t fsType] [-r] [--timeout Seconds] Source Target\n"+
		"3. Unmount:\n\t%s -u [--timeout Seconds] Target\n", Appname, Appname, Appname)
}

func main() {
	appCtx := &appCtx{}
	parser := flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	parser.Usage = appCtx.Usage()
	parser.ShortDescription = Appname
	parser.LongDescription = "File system over Nuvo volume test tool."

	args, err := parser.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
	if appCtx.QueryMount && appCtx.DoMount && appCtx.DoUnmount {
		fmt.Fprintf(os.Stderr, "Error: Only one of query-mount, mount or unmount must be specified\n")
		os.Exit(1)
	}
	if !(appCtx.QueryMount || appCtx.DoMount || appCtx.DoUnmount) {
		fmt.Fprintf(os.Stderr, "Error: query-mount, mount or unmount must be specified\n")
		os.Exit(1)
	}
	log := logging.MustGetLogger("mountfs")
	ma := &mount.MounterArgs{
		Log: log,
	}
	mounter, _ := mount.New(ma)
	if appCtx.QueryMount || appCtx.DoMount {
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "Error: query-mount and mount require source and target paths\n")
			os.Exit(1)
		}
		opts := []string{}
		if appCtx.ReadOnly {
			opts = []string{"ro"}
		}
		fma := &mount.FilesystemMountArgs{
			LogPrefix: Appname,
			Source:    args[0],
			Target:    args[1],
			FsType:    appCtx.FsType,
			Options:   opts,
			Deadline:  time.Now().Add(time.Duration(appCtx.TimeoutSecs) * time.Second),
		}
		var err error
		var op string
		if appCtx.QueryMount {
			op = "query-mount"
			var isMounted bool
			isMounted, err = mounter.IsFilesystemMounted(context.Background(), fma)
			if err == nil {
				fmt.Printf("IsMounted: %v\n", isMounted)
			}
		} else {
			op = "mount"
			err = mounter.MountFilesystem(context.Background(), fma)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s failed: %v\n", op, err)
			os.Exit(1)
		}
	}
	if appCtx.DoUnmount {
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "Error: Unmount requires target path\n")
			os.Exit(1)
		}
		fua := &mount.FilesystemUnmountArgs{
			LogPrefix: Appname,
			Target:    args[0],
			Deadline:  time.Now().Add(time.Duration(appCtx.TimeoutSecs) * time.Second),
		}
		if err := mounter.UnmountFilesystem(context.Background(), fua); err != nil {
			fmt.Fprintf(os.Stderr, "Error: unmount failed: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(0)
}
