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


package gc

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/docker/go-units"
)

const localSSDSizeBytes = 375 * units.GiB // as per GC Doc, all local SSD are this size

type readDirFn func(dirName string) ([]os.FileInfo, error)

// readDirHook can be replaced during UT
var readDirHook readDirFn = ioutil.ReadDir

// LocalEphemeralDevices satisfies the EphemeralDiscoverer interface
func (cl *Client) LocalEphemeralDevices() ([]*csp.EphemeralDevice, error) {
	devList, err := readDirHook(diskDir)
	if err != nil {
		cl.csp.dbgF("Failed to list device names: %v", err)
		return nil, err
	}

	disks := map[string]*csp.EphemeralDevice{}
	for _, disk := range devList {
		if name := disk.Name(); strings.HasPrefix(name, "google-local-") {
			if i := strings.Index(name, "-part"); i == -1 {
				dev := &csp.EphemeralDevice{}
				dev.Path = filepath.Join(diskDir, name)
				dev.SizeBytes = localSSDSizeBytes
				dev.Type = csp.EphemeralTypeSSD
				dev.Initialized = true
				dev.Usable = true // for now, may change below
				disks[name] = dev
			} else {
				// not usable if it is partitioned. Note: ReadDir returns a sorted list, so partition names always sort after the base disk
				dev := disks[name[:i]]
				dev.Usable = false
			}
		}
	}
	paths := []string{} // only contains disks that pass this first filter
	for _, dev := range disks {
		if dev.Usable {
			paths = append(paths, dev.Path)
		}
	}

	// Use blkid to filter any formatted devices. Only devices that are formatted will be included in the output.
	// Note: this only works natively as root or in a privileged container with /dev:/dev mapped
	if len(paths) > 0 {
		ctx := context.Background()
		output, err := cl.csp.exec.CommandContext(ctx, "blkid", paths...).CombinedOutput()
		if err != nil {
			// blkid exits with an error if none of the devices contain a formatted filesystem, ignore the error
			cl.csp.dbgF("blkid %s error: %v", strings.Join(paths, " "), err)
		} else {
			re := regexp.MustCompile(`^` + diskDir + `/([^:]+): `)
			for _, line := range strings.Split(string(output), "\n") {
				if match := re.FindStringSubmatch(line); len(match) > 0 {
					if dev, ok := disks[string(match[1])]; ok {
						dev.Usable = false
					}
				}
			}
		}
	}

	names := util.SortedStringKeys(disks)
	ret := make([]*csp.EphemeralDevice, len(names))
	for i, name := range names {
		ret[i] = disks[name]
	}
	return ret, nil
}
