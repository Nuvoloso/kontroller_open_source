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
	"errors"
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
)

type getGarbageCollectStruct struct {
	VolUUID string `short:"v" long:"volume-uuid" description:"UUID of volume series" required:"false" value-name:"VOL_UUID"`
	Policy  string `short:"p" long:"policy" choice:"oldest" choice:"youngest" choice:"fullest" choice:"emptiest" choice:"all"`
}

var getGarbageCollectCmd getGarbageCollectStruct

func segmentCollectable(a nuvoapi.Segment) bool {
	return (a.Reserved == false && a.Log == false && a.Age != 0)
}

// Test if segment a is a better choice than segment b
type segmentBetter func(a nuvoapi.Segment, b nuvoapi.Segment) bool

func segmentOlder(a nuvoapi.Segment, b nuvoapi.Segment) bool {
	return a.Age < b.Age
}

func segmentYounger(a nuvoapi.Segment, b nuvoapi.Segment) bool {
	return a.Age > b.Age
}

func segmentFuller(a nuvoapi.Segment, b nuvoapi.Segment) bool {
	return a.Fullness > b.Fullness
}

func segmentEmptier(a nuvoapi.Segment, b nuvoapi.Segment) bool {
	return a.Fullness < b.Fullness
}

func manifestBest(devices []nuvoapi.Device, test segmentBetter) (uint32, uint32, error) {
	foundSegment := false
	var bestSegment nuvoapi.Segment
	var bestSegIndex uint32
	var bestParcelIndex uint32
	for _, device := range devices {
		for _, parcel := range device.Parcels {
			for s, segment := range parcel.Segments {
				if segmentCollectable(segment) == false {
					continue
				}
				if !foundSegment || test(segment, bestSegment) {
					foundSegment = true
					bestSegment = segment
					bestParcelIndex = parcel.ParcelIndex
					bestSegIndex = uint32(s)
				}
			}
		}
	}
	if foundSegment == false {
		return 0, 0, errors.New("No segment to clean")
	}
	return bestParcelIndex, bestSegIndex, nil
}

func gcSegment(socket string, volUUID string, parcelIndex uint32, segmentIndex uint32) error {
	nuvoAPI := nuvoapi.NewNuvoVM(socket)

	var params nuvoapi.DebugTriggerParams
	params.Trigger = "gc_segment"
	params.VolUUID = volUUID
	params.ParcelIndex = parcelIndex
	params.SegmentIndex = segmentIndex
	fmt.Printf("Attempting to gc parcel %2d segment %2d\n", parcelIndex, segmentIndex)
	return nuvoAPI.DebugTrigger(params)
}

func gcBest(socket string, volUUID string, devices []nuvoapi.Device, test segmentBetter) error {
	parcel, segment, err := manifestBest(devices, test)
	if err != nil {
		return err
	}
	return gcSegment(socket, volUUID, parcel, segment)
}

func gcAll(socket string, volUUID string, devices []nuvoapi.Device) error {
	for _, device := range devices {
		for _, parcel := range device.Parcels {
			for s, segment := range parcel.Segments {
				if segmentCollectable(segment) == false {
					continue
				}
				err := gcSegment(socket, volUUID, parcel.ParcelIndex, uint32(s))
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (x *getGarbageCollectStruct) Execute(args []string) error {
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	devices, err := nuvoAPI.Manifest(x.VolUUID, false)
	if err != nil {
		printLongError(err)
		return err
	}
	switch {
	case x.Policy == "oldest":
		return gcBest(nuvoCmd.Socket, x.VolUUID, devices, segmentOlder)
	case x.Policy == "youngest":
		return gcBest(nuvoCmd.Socket, x.VolUUID, devices, segmentYounger)
	case x.Policy == "fullest":
		return gcBest(nuvoCmd.Socket, x.VolUUID, devices, segmentFuller)
	case x.Policy == "emptiest":
		return gcBest(nuvoCmd.Socket, x.VolUUID, devices, segmentEmptier)
	case x.Policy == "all":
		return gcAll(nuvoCmd.Socket, x.VolUUID, devices)
	}

	return err
}
