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
	"fmt"
	"os"

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	flags "github.com/jessevdk/go-flags"
	uuid "github.com/satori/go.uuid"
)

func printLongError(err error) {
	if nerr, ok := err.(nuvoapi.ErrorInt); ok {
		if len(nuvoCmd.Verbose) > 0 {
			fmt.Printf("%s\n", nerr.LongError())
		}
	}
}

type nuvoCmdStruct struct {
	Verbose []bool `short:"v" long:"verbose" description:"Verbose information"`
	Socket  string `short:"s" long:"socket" description:"Name of control socket" default:"/tmp/nuvo_api.socket" value-name:"API_SOCKET"`
}

var nuvoCmd nuvoCmdStruct

type useDeviceCmdStruct struct {
	UUID    string `short:"u" long:"device-uuid" description:"UUID of the device" required:"true" value-name:"UUID"`
	Device  string `short:"d" long:"device" description:"Path to device" required:"true" value-name:"PATH"`
	DevType string `short:"t" long:"type" choice:"SSD" choice:"HDD" description:"Type of device" required:"false" value-name:"TYPE"`
}

var useDeviceCmd useDeviceCmdStruct

func (x *useDeviceCmdStruct) Execute(args []string) error {
	var err error
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("use-device: device uuid: \"%s\", device: \"%s\", device_type: \"%s\"\n", x.UUID, x.Device, x.DevType)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	if x.DevType == "" {
		err = nuvoAPI.UseDevice(x.UUID, x.Device)
	} else {
		err = nuvoAPI.UseDevice(x.UUID, x.Device, x.DevType)
	}
	printLongError(err)
	return err
}

type useCacheDeviceCmdStruct struct {
	UUID   string `short:"u" long:"device-uuid" description:"UUID of the device" required:"true" value-name:"UUID"`
	Device string `short:"d" long:"device" description:"Path to device" required:"true" value-name:"PATH"`
}

var useCacheDeviceCmd useCacheDeviceCmdStruct

func (x *useCacheDeviceCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("use-cache-device: device uuid: \"%s\", device: \"%s\"\n", x.UUID, x.Device)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	size, unitSize, err := nuvoAPI.UseCacheDevice(x.UUID, x.Device)
	if err != nil {
		printLongError(err)
	} else {
		fmt.Printf("%d %d", size, unitSize)
	}
	return err
}

type closeDeviceCmdStruct struct {
	UUID string `short:"u" long:"device-uuid" description:"UUID of the device" required:"true" value-name:"UUID"`
}

var closeDeviceCmd closeDeviceCmdStruct

func (x *closeDeviceCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("close-device: device uuid: \"%s\"\n", x.UUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.CloseDevice(x.UUID)
	printLongError(err)
	return err
}

type formatDeviceCmdStruct struct {
	UUID       string `short:"u" long:"device-uuid" description:"UUID of the device" required:"true" value-name:"UUID"`
	Device     string `short:"d" long:"device" description:"Path to device" required:"true" value-name:"PATH"`
	ParcelSize uint64 `short:"p" long:"parcel-size" description:"Size of parcels in bytes" required:"true" value-name:"SIZE"`
}

var formatDeviceCmd formatDeviceCmdStruct

func (x *formatDeviceCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("format-device: device uuid: \"%s\", device: \"%s\", parcel-size: %d\n",
			x.UUID, x.Device, x.ParcelSize)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	_, err := nuvoAPI.FormatDevice(x.UUID, x.Device, x.ParcelSize)
	printLongError(err)
	return err
}

type deviceLocationCmdStruct struct {
	DeviceUUID string `short:"d" long:"device-uuid" description:"UUID of the device" required:"true" value-name:"DEVICE_UUID"`
	NodeUUID   string `short:"n" long:"node-uuid" description:"UUID of the node" required:"true" value-name:"NODE_UUID"`
}

var deviceLocationCmd deviceLocationCmdStruct

func (x *deviceLocationCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("device-location: device uuid: \"%s\", node uuid: \"%s\"\n",
			x.DeviceUUID, x.NodeUUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.DeviceLocation(x.DeviceUUID, x.NodeUUID)
	printLongError(err)
	return err
}

type nodeLocationCmdStruct struct {
	NodeUUID string `short:"n" long:"node-uuid" description:"UUID of the node" required:"true" value-name:"UUID"`
	Ipv4Addr string `short:"i" long:"ipv4-addr" description:"IP address or name" required:"true" value-name:"ADDRESS"`
	Port     uint16 `short:"p" long:"port" description:"Port to which to connect" required:"true" value-name:"PORT"`
}

var nodeLocationCmd nodeLocationCmdStruct

func (x *nodeLocationCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("node-location: uuid: \"%s\" address: \"%s\", port: %d\n",
			x.NodeUUID, x.Ipv4Addr, x.Port)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.NodeLocation(x.NodeUUID, x.Ipv4Addr, x.Port)
	printLongError(err)
	return err
}

type nodeInitDoneCmdStruct struct {
	NodeUUID string `short:"n" long:"node-uuid" description:"UUID of the node" value-name:"UUID"`
	Clear    bool   `short:"c" long:"clear" description:"Clear the node init complete flag"`
}

var nodeInitDoneCmd nodeInitDoneCmdStruct

func (x *nodeInitDoneCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("node-init-done: uuid: \"%s\" clear: %t \n", x.NodeUUID, x.Clear)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.NodeInitDone(x.NodeUUID, x.Clear)
	printLongError(err)
	return err
}

type passThroughVolCmdStruct struct {
	Name     string `short:"n" long:"name" description:"Name of the export" required:"true" value-name:"NAME"`
	UUID     string `short:"u" long:"vol-series-uuid" description:"Vol series uuid" value-name:"VOL_UUID" optional:""`
	Device   string `short:"d" long:"device" description:"Path to device" required:"true" value-name:"DEVICE"`
	Size     uint64 `short:"s" long:"size" description:"Size of volume in bytes" required:"true" value-name:"SIZE"`
	Create   bool   `short:"c" long:"create" description:"Create a file with given device name"`
	Readonly bool   `short:"r" long:"readonly" description:"Whether the lun should be readonly"`
}

var passThroughVolCmd passThroughVolCmdStruct

func (x *passThroughVolCmdStruct) Execute(args []string) error {
	var vsUUID string
	if x.UUID != "" {
		vsUUID = x.UUID
	} else {
		vsUUID = uuid.NewV4().String()
		fmt.Printf("UUID not specified.  Using: \"%s\"\n", vsUUID)
	}
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("passthrough-volume: name: \"%s\" device: \"%s\", device-size: %d\n",
			x.Name, x.Device, x.Size)
	}
	if x.Create {
		if len(nuvoCmd.Verbose) > 0 {
			fmt.Printf("creating backing device: \"%s\", device-size: %d\n",
				x.Device, x.Size)
		}
		_, err := os.Create(x.Device)
		if err != nil {
			return err
		}
		err = os.Truncate(x.Device, int64(x.Size))
		if err != nil {
			return err
		}
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.OpenPassThroughVolume(vsUUID, x.Device, x.Size)
	if err != nil {
		printLongError(err)
		return err
	}
	err = nuvoAPI.ExportLun(vsUUID, "", x.Name, !x.Readonly)
	printLongError(err)
	return err
}

type haltCmdStruct struct {
}

var haltCmd haltCmdStruct

func (x *haltCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {

	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.HaltCommand()
	printLongError(err)
	return err
}

type exportCmdStruct struct {
	VolSeries string `short:"v" long:"vol-series" description:"UUID of volume series to be Exported" required:"true" value-name:"VOL_UUID"`
	Pit       string `short:"p" long:"pit" description:"UUID of PiT to be Exported" required:"false" value-name:"PIT_UUID"`
	Export    string `short:"e" long:"export-name" description:"Name to export." required:"true" value-name:"EXPORT"`
	Readonly  bool   `short:"r" long:"readonly" description:"Whether the lun should be readonly"`
}

var exportCmd exportCmdStruct

func (x *exportCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("export: \"%s\" pit: \"%s\" export-name: \"%s\", readonly: %t\n",
			x.VolSeries, x.Pit, x.Export, x.Readonly)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.ExportLun(x.VolSeries, x.Pit, x.Export, !x.Readonly)
	if err != nil {
		printLongError(err)
	}
	return err
}

type unexportCmdStruct struct {
	VolSeries string `short:"v" long:"vol-series" description:"UUID of volume series to be unexported" required:"true" value-name:"VOL_UUID"`
	Pit       string `short:"p" long:"pit" description:"UUID of PiT to be Exported" required:"false" value-name:"PIT_UUID"`
	Export    string `short:"e" long:"export-name" description:"Exported named to unexport." required:"true" value-name:"EXPORT"`
}

var unexportCmd unexportCmdStruct

func (x *unexportCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("unexport: \"%s\" pit: \"%s\" export-name: \"%s\"\n",
			x.VolSeries, x.Pit, x.Export)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.UnexportLun(x.VolSeries, x.Pit, x.Export)
	if err != nil {
		printLongError(err)
	}
	return err
}

type createParcelVolCmdStruct struct {
	VolSeries  string `short:"v" long:"vol-series" description:"UUID of volume series to create" required:"true" value-name:"VOL_UUID"`
	RootDevice string `short:"d" long:"root-device" description:"UUID of device to hold root parcel" required:"true" value-name:"ROOT_DEV_UUID"`
	RootParcel string `short:"p" long:"root-parcel" description:"UUID to assign to the root parcel" required:"true" value-name:"ROOT_PARCEL_UUID"`
}

var createParcelVolCmd createParcelVolCmdStruct

func (x *createParcelVolCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("create parcel vol: \"%s\" root-device: \"%s\" root-parcel: \"%s\"\n",
			x.VolSeries, x.RootDevice, x.RootParcel)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	rootParcel, err := nuvoAPI.CreateParcelVol(x.VolSeries, x.RootDevice, x.RootParcel)
	if err != nil {
		printLongError(err)
	} else {
		fmt.Printf("%s", rootParcel)
	}
	return err
}

type createLogVolCmdStruct struct {
	VolSeries  string `short:"v" long:"vol-series" description:"UUID of volume series to create" required:"true" value-name:"VOL_UUID"`
	RootDevice string `short:"d" long:"root-device" description:"UUID of device to hold root parcel" required:"true" value-name:"ROOT_DEV_UUID"`
	RootParcel string `short:"p" long:"root-parcel" description:"UUID to assign to the root parcel" required:"true" value-name:"ROOT_PARCEL_UUID"`
	Size       uint64 `short:"s" long:"size" description:"Size of root lun in bytes" required:"true" value-name:"SIZE"`
}

var createLogVolCmd createLogVolCmdStruct

func (x *createLogVolCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("create log vol: \"%s\", root-device: \"%s\", root-parcel: \"%s\", size: %d\n",
			x.VolSeries, x.RootDevice, x.RootParcel, x.Size)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	rootParcel, err := nuvoAPI.CreateLogVol(x.VolSeries, x.RootDevice, x.RootParcel, x.Size)
	if err != nil {
		printLongError(err)
	} else {
		fmt.Printf("%s", rootParcel)
	}
	return err
}

type openParcelVolCmdStruct struct {
	VolSeries  string `short:"v" long:"vol-series" description:"UUID of volume series to be opened" required:"true" value-name:"VOL_UUID"`
	RootDevice string `short:"d" long:"root-device" description:"UUID of device holding the root parcel" required:"true" value-name:"ROOT_DEV_UUID"`
	RootParcel string `short:"p" long:"root-parcel" description:"UUID of the root parcel" required:"true" value-name:"ROOT_PARCEL_UUID"`
}

var openParcelVolCmd openParcelVolCmdStruct

func (x *openParcelVolCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("open parcel vol: \"%s\" root-device: \"%s\" root-parcel: \"%s\"\n",
			x.VolSeries, x.RootDevice, x.RootParcel)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.OpenVol(x.VolSeries, x.RootDevice, x.RootParcel, false)
	if err != nil {
		printLongError(err)
	}
	return err
}

type openLogVolCmdStruct struct {
	VolSeries  string `short:"v" long:"vol-series" description:"UUID of volume series to be opened" required:"true" value-name:"VOL_UUID"`
	RootDevice string `short:"d" long:"root-device" description:"UUID of device holding the root parcel" required:"true" value-name:"ROOT_DEV_UUID"`
	RootParcel string `short:"p" long:"root-parcel" description:"UUID of the root parcel" required:"true" value-name:"ROOT_PARCEL_UUID"`
}

var openLogVolCmd openLogVolCmdStruct

func (x *openLogVolCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("open log vol: \"%s\" root-device: \"%s\" root-parcel: \"%s\"\n",
			x.VolSeries, x.RootDevice, x.RootParcel)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.OpenVol(x.VolSeries, x.RootDevice, x.RootParcel, true)
	if err != nil {
		printLongError(err)
	}
	return err
}

type destroyParcelVolCmdStruct struct {
	VolSeries  string `short:"v" long:"vol-series" description:"UUID of volume series to be destroyed" required:"true" value-name:"VOL_UUID"`
	RootDevice string `short:"d" long:"root-device" description:"UUID of device holding the root parcel" required:"true" value-name:"ROOT_DEV_UUID"`
	RootParcel string `short:"p" long:"root-parcel" description:"UUID of the root parcel" required:"true" value-name:"ROOT_PARCEL_UUID"`
}

var destroyParcelVolCmd destroyParcelVolCmdStruct

func (x *destroyParcelVolCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("destroy vol: \"%s\" root-device: \"%s\" root-parcel: \"%s\"\n",
			x.VolSeries, x.RootDevice, x.RootParcel)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.DestroyVol(x.VolSeries, x.RootDevice, x.RootParcel, false)
	if err != nil {
		printLongError(err)
	}
	return err
}

type destroyLogVolCmdStruct struct {
	VolSeries  string `short:"v" long:"vol-series" description:"UUID of volume series to be destroyed" required:"true" value-name:"VOL_UUID"`
	RootDevice string `short:"d" long:"root-device" description:"UUID of device holding the root parcel" required:"true" value-name:"ROOT_DEV_UUID"`
	RootParcel string `short:"p" long:"root-parcel" description:"UUID of the root parcel" required:"true" value-name:"ROOT_PARCEL_UUID"`
}

var destroyLogVolCmd destroyLogVolCmdStruct

func (x *destroyLogVolCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("destroy vol: \"%s\" root-device: \"%s\" root-parcel: \"%s\"\n",
			x.VolSeries, x.RootDevice, x.RootParcel)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.DestroyVol(x.VolSeries, x.RootDevice, x.RootParcel, true)
	if err != nil {
		printLongError(err)
	}
	return err
}

type allocParcelsCmdStruct struct {
	VolSeries string `short:"v" long:"vol-series" description:"UUID of volume series to be allocated parcels" required:"true" value-name:"VOL_UUID"`
	Device    string `short:"d" long:"device-uuid" description:"UUID of device to alloc parcels from" required:"true" value-name:"DEV_UUID"`
	Num       uint64 `short:"n" long:"number" description:"Number of parcels to allocate" required:"true" value-name:"NUMBER"`
}

var allocParcelsCmd allocParcelsCmdStruct

func (x *allocParcelsCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("allocate parcels to vol: \"%s\" device: \"%s\" num: %d\n",
			x.VolSeries, x.Device, x.Num)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.AllocParcels(x.VolSeries, x.Device, x.Num)
	if err != nil {
		printLongError(err)
	}
	return err
}

type allocCacheCmdStruct struct {
	VolSeries string `short:"v" long:"vol-series" description:"UUID of volume series to be allocated cache" required:"true" value-name:"VOL_UUID"`
	Size      uint64 `short:"s" long:"number" description:"Amount of cache to allocate in bytes" required:"true" value-name:"NUMBER"`
}

var allocCacheCmd allocCacheCmdStruct

func (x *allocCacheCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("allocate cache to vol: \"%s\" size: %d\n",
			x.VolSeries, x.Size)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.AllocCache(x.VolSeries, x.Size)
	if err != nil {
		printLongError(err)
	}
	return err
}

type closeVolCmdStruct struct {
	VolSeries string `short:"v" long:"vol-series" description:"UUID of volume series to be closed" required:"true" value-name:"VOL_UUID"`
}

var closeVolCmd closeVolCmdStruct

func (x *closeVolCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("close vol: \"%s\"\n", x.VolSeries)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.CloseVol(x.VolSeries)
	if err != nil {
		printLongError(err)
	}
	return err
}

type getStatsCmdStruct struct {
	VolUUID    string `short:"v" long:"volume-uuid" description:"UUID of volume series" required:"false" value-name:"VOL_UUID"`
	DevUUID    string `short:"d" long:"device-uuid" description:"UUID of device" required:"false" value-name:"DEV_UUID"`
	Read       bool   `short:"r" long:"read" description:"Get the read stats"`
	Write      bool   `short:"w" long:"write" description:"Get the write stats"`
	Clear      bool   `short:"c" long:"clear" description:"Clear the stats"`
	Histograms bool   `short:"p" long:"print-histograms" description:"Print histograms"`
}

var getStatsCmd getStatsCmdStruct

func (x *getStatsCmdStruct) Execute(args []string) error {
	if (x.VolUUID == "" && x.DevUUID == "") != (x.VolUUID != "" && x.DevUUID != "") {
		return fmt.Errorf("Exactly one of VOL_UUID and DEV_UUID must be set")
	}
	if x.Read == x.Write {
		return fmt.Errorf("Exactly one of Read and Write must be set")
	}
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("get stats: \"%s\" \"%s\" %t %t %t \n", x.VolUUID, x.DevUUID, x.Read, x.Write, x.Clear)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	var uuid string
	var device bool
	if x.DevUUID != "" {
		device = true
		uuid = x.DevUUID
	} else {
		device = false
		uuid = x.VolUUID
	}
	stats, err := nuvoAPI.GetStats(device, x.Read, x.Clear, uuid)
	if err != nil {
		printLongError(err)
	} else if x.Histograms {
		fmt.Printf("Count, %d, Size, %d, Latency, %f, Std_dev, %f, SeriesUUID, %s",
			stats.Count, stats.SizeTotal, stats.LatencyMean, stats.LatencyStdev, stats.SeriesUUID)
		fmt.Printf(", Sizes")
		for _, v := range stats.SizeHist {
			fmt.Printf(", %d", v)
		}
		fmt.Printf(", Latencies")
		for _, v := range stats.LatencyHist {
			fmt.Printf(", %d", v)
		}
		fmt.Printf("\n")
	} else {
		fmt.Printf("Count: %d\n", stats.Count)
		fmt.Printf("Size (total bytes): %d\n", stats.SizeTotal)
		fmt.Printf("Latency (mean): %f\n", stats.LatencyMean)
		fmt.Printf("Latency (std dev): %f\n", stats.LatencyStdev)
		fmt.Printf("SeriesUUID: %s\n", stats.SeriesUUID)
	}

	return err
}

type getVolStatsCmdStruct struct {
	VolUUID string `short:"v" long:"volume-uuid" description:"UUID of volume series" required:"true" value-name:"VOL_UUID"`
	Clear   bool   `short:"c" long:"clear" description:"Clear the stats"`
}

var getVolStatsCmd getVolStatsCmdStruct

func (x *getVolStatsCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("get stats: \"%s\". %t \n", x.VolUUID, x.Clear)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)

	stats, err := nuvoAPI.GetVolumeStats(x.Clear, x.VolUUID)
	if err != nil {
		printLongError(err)
	} else {
		fmt.Printf("IO Stats            READ,WRITE\n")
		fmt.Printf("Count:              %d,%d\n", stats.IOReads.Count, stats.IOWrites.Count)
		fmt.Printf("Size (total bytes): %d,%d\n", stats.IOReads.SizeTotal, stats.IOWrites.SizeTotal)
		fmt.Printf("Latency (mean):     %f,%f\n", stats.IOReads.LatencyMean, stats.IOWrites.LatencyMean)
		fmt.Printf("Latency (std dev):  %f,%f\n", stats.IOReads.LatencyStdev, stats.IOWrites.LatencyStdev)
		fmt.Printf("SeriesUUID:         %s,%s\n", stats.IOReads.SeriesUUID, stats.IOWrites.SeriesUUID)
		fmt.Printf("Cache Stats            USER,METADATA\n")
		fmt.Printf("IO Read Count:         %d,%d\n", stats.CacheUser.IOReadTotal, stats.CacheMetadata.IOReadTotal)
		fmt.Printf("Cache Read Count:      %d,%d\n", stats.CacheUser.CacheIOReadLineTotalCount, stats.CacheMetadata.CacheIOReadLineTotalCount)
		fmt.Printf("Cache Read Hit Count:  %d,%d\n", stats.CacheUser.CacheIOReadLineHitCount, stats.CacheMetadata.CacheIOReadLineHitCount)
		fmt.Printf("Cache Read Miss Count: %d,%d\n", stats.CacheUser.CacheIOReadLineMissCount, stats.CacheMetadata.CacheIOReadLineMissCount)
		fmt.Printf("IO Write Count:        %d,%d\n", stats.CacheUser.IOWriteTotal, stats.CacheMetadata.IOWriteTotal)
		fmt.Printf("Cache Write Count:     %d,%d\n", stats.CacheUser.CacheIOWriteLineTotalCount, stats.CacheMetadata.CacheIOWriteLineTotalCount)
	}

	return err
}

type getManifestCmdStruct struct {
	VolUUID   string `short:"v" long:"volume-uuid" description:"UUID of volume series" required:"false" value-name:"VOL_UUID"`
	ImageName string `short:"f" long:"file-name" description:"file name to save image" required:"false" value-name:"IMAGE_NAME" default:"manifest.png"`
	Short     bool   `short:"s" long:"short" descripton:"just device info"`
}

var getManifestCmd getManifestCmdStruct

func (x *getManifestCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("manifest: \"%s\" \n", x.VolUUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	devices, err := nuvoAPI.Manifest(x.VolUUID, x.Short)
	if err != nil {
		printLongError(err)
	} else {
		for _, device := range devices {
			fmt.Printf("device index %d, UUID:  \"%s\", parcel_size %d, target_parcels %d (%d alloced) free_segs %d, bllks_used %d\n",
				device.Index, device.UUID, device.ParcelSize, device.TargetParcels, device.AllocedParcels, device.FreeSegments, device.BlocksUsed)
			if !x.Short {
				for _, parcel := range device.Parcels {
					fmt.Printf("    parcel index %d, UUID:  \"%s\", state: %s segment_size: %d \n",
						parcel.ParcelIndex, parcel.UUID, parcel.State, parcel.SegmentSize)
					for i, segment := range parcel.Segments {
						fmt.Printf("        segment %2d, blks used %4d, age %6d, log/gc %5t, pin_cnt %5d, reserved %5t, Fullness %6.4f\n",
							i, segment.BlksUsed, segment.Age, segment.Log, segment.PinCnt, segment.Reserved, segment.Fullness)
					}
				}
			}
		}
		drawDevices(devices, len(devices)*410, 1000, x.ImageName, x.Short)
	}

	return err
}

type useNodeUUIDCmdStruct struct {
	UUID string `short:"u" long:"node-uuid" description:"UUID of node" required:"true" value-name:"NODE_UUID"`
}

var useNodeUUIDCmd useNodeUUIDCmdStruct

func (x *useNodeUUIDCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("node uuid: \"%s\"\n", x.UUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.UseNodeUUID(x.UUID)
	if err != nil {
		printLongError(err)
	}
	return err
}

type createPitCmdStruct struct {
	VolUUID string `short:"v" long:"vol-uuid" description:"UUID of volume" required:"true" value-name:"VOLUME_UUID"`
	PitUUID string `short:"p" long:"pit-uuid" description:"desired PiT UUID" required:"true" value-name:"PIT_UUID"`
}

var createPitCmd createPitCmdStruct

func (x *createPitCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("vol uuid: \"%s\" pit uuid: \"%s\"\n", x.VolUUID, x.PitUUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.CreatePit(x.VolUUID, x.PitUUID)
	if err != nil {
		printLongError(err)
	}
	return err
}

type deletePitCmdStruct struct {
	VolUUID string `short:"v" long:"vol-uuid" description:"UUID of volume" required:"true" value-name:"VOLUME_UUID"`
	PitUUID string `short:"p" long:"pit-uuid" description:"PiT UUID" required:"true" value-name:"PIT_UUID"`
}

var deletePitCmd deletePitCmdStruct

func (x *deletePitCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("vol uuid: \"%s\" pit uuid: \"%s\"\n", x.VolUUID, x.PitUUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.DeletePit(x.VolUUID, x.PitUUID)
	if err != nil {
		printLongError(err)
	}
	return err
}

type listPitsCmdStruct struct {
	VolUUID string `short:"v" long:"vol-uuid" description:"UUID of volume" required:"true" value-name:"VOLUME_UUID"`
}

var listPitsCmd listPitsCmdStruct

func (x *listPitsCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("vol uuid: \"%s\"\n", x.VolUUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	pits, err := nuvoAPI.ListPits(x.VolUUID)
	if err != nil {
		printLongError(err)
	} else {
		fmt.Printf("%d PiTs in Volume Series %s\n", len(pits), x.VolUUID)
		for _, p := range pits {
			fmt.Printf("%s\n", p)
		}
	}
	return err
}

type listVolsCmdStruct struct {
}

var listVolsCmd listVolsCmdStruct

func (x *listVolsCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("list Vol series objects")
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	vols, err := nuvoAPI.ListVols()
	if err != nil {
		printLongError(err)
	} else {
		for _, v := range vols {
			fmt.Printf("%s\n", v)
		}
	}
	return err
}

type pauseIoCmdStruct struct {
	VolUUID string `short:"v" long:"vol-uuid" description:"UUID of volume" required:"true" value-name:"NODE_UUID"`
}

var pauseIoCmd pauseIoCmdStruct

func (x *pauseIoCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("vol uuid: \"%s\"\n", x.VolUUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.PauseIo(x.VolUUID)
	if err != nil {
		printLongError(err)
	}
	return err
}

type resumeIoCmdStruct struct {
	VolUUID string `short:"v" long:"vol-uuid" description:"UUID of volume" required:"true" value-name:"NODE_UUID"`
}

var resumeIoCmd resumeIoCmdStruct

func (x *resumeIoCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("vol uuid: \"%s\"\n", x.VolUUID)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.ResumeIo(x.VolUUID)
	if err != nil {
		printLongError(err)
	}
	return err
}

type logLevelCmdStruct struct {
	ModuleName string `short:"m" long:"module-name" description:"Name of module" required:"true" value-name:"MODULE_NAME"`
	Level      uint32 `short:"l" long:"log-level" description:"Log level for this module" required:"true" value-name:"LOG_LEVEL"`
}

var logLevelCmd logLevelCmdStruct

func (x *logLevelCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("log level module: \"%s\", level: %d\n",
			x.ModuleName, x.Level)
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	err := nuvoAPI.LogLevel(x.ModuleName, x.Level)
	if err != nil {
		printLongError(err)
	}
	return err
}

type nodeStatusCmdStruct struct {
	Node      bool   `short:"n" long:"node" description:"Node uuid"`
	BuildInfo bool   `short:"b" long:"build-info" description:"Git build hash"`
	Space     bool   `short:"s" long:"space" description:"Space information for open volumes"`
	Used      bool   `short:"u" long:"used" description:"just print used blocks"`
	Allocated bool   `short:"a" long:"allocated" description:"just print allocated blocks"`
	Total     bool   `short:"t" long:"total" description:"just print the total blocks"`
	VolUUID   string `short:"v" long:"vol-uuid" description:"UUID of volume to print stats" value-name:"VOLUME_UUID" default:""`
	DataClass uint32 `short:"d" long:"data-class" description:"Class to print for single block mode" value-name:"DATA_CLASS"`
}

var nodeStatusCmd nodeStatusCmdStruct

func (x *nodeStatusCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("node status\n")
	}
	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)
	status, err := nuvoAPI.NodeStatus()
	if err != nil {
		printLongError(err)
	}
	if x.Node {
		fmt.Printf("%s\n", status.NodeUUID)
	}
	if x.Used || x.Total || x.Allocated {
		for _, vol := range status.Volumes {
			if x.VolUUID != vol.VolUUID {
				continue
			}
			for _, class := range vol.ClassSpace {
				if x.DataClass != class.Class {
					continue
				}
				if x.Allocated {
					fmt.Printf("%d\n", class.BlocksAllocated)
				} else if x.Used {
					fmt.Printf("%d\n", class.BlocksUsed)
				} else if x.Total {
					fmt.Printf("%d\n", class.BlocksTotal)
				}
			}
		}
	} else if x.Space {
		for _, vol := range status.Volumes {
			if x.VolUUID != "" && (x.VolUUID != vol.VolUUID) {
				continue
			} else {
				fmt.Printf("Volume UUID:  \"%s\"\n", vol.VolUUID)
				for _, class := range vol.ClassSpace {
					fmt.Printf("\tclass: %d, used: %d, allocated: %d, total: %d\n",
						class.Class, class.BlocksUsed, class.BlocksAllocated, class.BlocksTotal)
				}
			}
		}
	}
	if x.BuildInfo {
		fmt.Printf("%s\n", status.GitBuildHash)
	}
	return err
}

type debugTriggerCmdStruct struct {
	Trigger          string `long:"trigger" description:"Trigger string" default:""`
	NodeUUID         string `long:"node" description:"Node uuid" default:""`
	VolUUID          string `long:"volume" description:"Volume uuid" default:""`
	DevUUID          string `long:"device" description:"Device uuid" default:""`
	ParcelIndex      uint32 `long:"parcel-index" decription:"Parcel index"`
	SegmentIndex     uint32 `long:"segment-index" decription:"Segment index"`
	InjectErrorType  uint32 `long:"error-type" decription:"Fault Injection Error Type"`
	InjectReturnCode int32  `long:"return-code" decription:"Fault Injection Error Code"`
	InjectRepeatCnt  int32  `long:"repeat-cnt" decription:"Number of times to repeat the error. Values <= 0 mean infinite"`
	InjectSkipCnt    int32  `long:"skip-cnt" decription:"Number of times to wait before injecting error"`
	Multiuse1        uint64 `long:"multi-use1" decription:"64bit uint just for you"`
	Multiuse2        uint64 `long:"multi-use2" decription:"64bit uint just for you"`
	Multiuse3        uint64 `long:"multi-use3" decription:"64bit uint just for you"`
}

var debugTriggerCmd debugTriggerCmdStruct

func (x *debugTriggerCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("debug trigger\n")
	}

	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)

	var params nuvoapi.DebugTriggerParams
	params.Trigger = x.Trigger
	params.NodeUUID = x.NodeUUID
	params.VolUUID = x.VolUUID
	params.DevUUID = x.DevUUID
	params.ParcelIndex = x.ParcelIndex
	params.SegmentIndex = x.SegmentIndex
	params.InjectErrorType = x.InjectErrorType
	params.InjectReturnCode = x.InjectReturnCode
	params.InjectRepeatCnt = x.InjectRepeatCnt
	params.InjectSkipCnt = x.InjectSkipCnt
	params.Multiuse1 = x.Multiuse1
	params.Multiuse2 = x.Multiuse2
	params.Multiuse3 = x.Multiuse3

	err := nuvoAPI.DebugTrigger(params)
	if err != nil {
		printLongError(err)
	}
	return err
}

type logSummaryCmdStruct struct {
	VolUUID      string `short:"v" long:"volume" description:"Volume uuid" required:"true"`
	ParcelIndex  uint32 `short:"p" long:"parcel-index" decription:"Parcel index" required:"true"`
	SegmentIndex uint32 `short:"s" long:"segment-index" decription:"Segment index" required:"true"`
}

var logSummaryCmd logSummaryCmdStruct

func (x *logSummaryCmdStruct) Execute(args []string) error {
	if len(nuvoCmd.Verbose) > 0 {
		fmt.Printf("log summary\n")
	}

	nuvoAPI := nuvoapi.NewNuvoVM(nuvoCmd.Socket)

	result, err := nuvoAPI.LogSummary(x.VolUUID, x.ParcelIndex, x.SegmentIndex)
	if err != nil {
		printLongError(err)
	}
	fmt.Printf("volume: \"%s\", parcel %d, segment %d, magic %d, seq_no %d, closing_seq_no %d\n",
		result.VolUUID, result.ParcelIndex, result.SegmentIndex, result.Magic, result.SequenceNumber, result.ClosingSequenceNumber)

	for i, entry := range result.Entries {
		switch {
		case entry.EntryType >= 2 && entry.EntryType <= 7:
			var entryType string
			switch entry.EntryType {
			case 2:
				entryType = "DATA"
			case 3:
				entryType = "MAP0"
			case 4:
				entryType = "MAP1"
			case 5:
				entryType = "MAP2"
			case 6:
				entryType = "MAP3"
			case 7:
				entryType = "MAP4"
			default:
				entryType = "ERROR"
			}
			// Data or map
			fmt.Printf("\tEntry %d, type %s, hash %d, PitActive %t, PitID %d, bno %d, gc parcel %d, gc offset %d\n",
				i, entryType, entry.BlockHash, entry.PitInfoActive, entry.PitInfoID, entry.BlockNumber,
				entry.ParcelIndex, entry.BlockOffset)
		case entry.EntryType == 8:
			// Fork
			fmt.Printf("\tEntry %d, type FORK, hash %d, parcel index %d, block offset %d, seq_no %d, sublcass %d\n",
				i, entry.BlockHash, entry.ParcelIndex, entry.BlockOffset, entry.SequenceNumber, entry.SubClass)
		case entry.EntryType == 9:
			// Header
			fmt.Printf("\tEntry %d, type HEADER, hash %d, seq_no %d, sublcass %d\n",
				i, entry.BlockHash, entry.SequenceNumber, entry.SubClass)
		case entry.EntryType == 10:
			// Descriptor
			fmt.Printf("\tEntry %d, type DESCRIPTOR, hash %d, seq_no %d, CVFlag %d, entry count %d, data block count %d\n",
				i, entry.BlockHash, entry.SequenceNumber, entry.CVFlag, entry.EntryCount, entry.DataBlockCount)
		case entry.EntryType == 11:
			// Snap
			fmt.Printf("\tEntry %d, type SNAP, operation %d, seq_no %d\n",
				i, entry.SnapOperation, entry.SequenceNumber)
		default:
		}
	}
	return err
}

func main() {
	var parser = flags.NewParser(&nuvoCmd, flags.Default)

	parser.AddCommand("use-device",
		"Use a device",
		"The use-device command tells the volume manager to start using a device along with its type.",
		&useDeviceCmd)
	parser.AddCommand("use-cache-device",
		"Use a device as cache",
		"The use-cache-device command tells the volume manager to start using a device as cache for local volumes",
		&useCacheDeviceCmd)
	parser.AddCommand("close-device",
		"Close a device",
		"The close-device command tells the volume manager to close a device.",
		&closeDeviceCmd)
	parser.AddCommand("format-device",
		"Format a device",
		"The format-device command tells the volume manager to format a device.",
		&formatDeviceCmd)
	parser.AddCommand("device-location",
		"Set which node a device is on",
		"The device-location command tells the volume manager what node has a device.",
		&deviceLocationCmd)
	parser.AddCommand("node-location",
		"Set how to find a node",
		"The node-location command tells the volume manager the ip address and port of a node.",
		&nodeLocationCmd)
	parser.AddCommand("node-init-done",
		"Initial node configuration is complete",
		"The node-init-done command tells the volume manager that the initial node configuration is complete.",
		&nodeInitDoneCmd)
	parser.AddCommand("passthrough-volume",
		"Start a passthrough volume",
		"The passthrough-volume command starts a volume series that passes IO directly to a local device.",
		&passThroughVolCmd)
	parser.AddCommand("export",
		"Export a lun for a loaded volume or point in time.",
		"The export command exports a loaded volume series.",
		&exportCmd)
	parser.AddCommand("unexport",
		"Unexport an exported lun on a volume or point in time",
		"The export command unexports a loaded volume series.",
		&unexportCmd)
	parser.AddCommand("halt",
		"Halt the volume manager",
		"The halt command shuts down the volume manager.",
		&haltCmd)
	parser.AddCommand("create-parcel-volume",
		"Create a volume using parcels",
		"The create-parcel-volume command creates a volume and returns a root parcel.",
		&createParcelVolCmd)
	parser.AddCommand("create-volume",
		"Create a Nuvoloso volume",
		"The create-volume command creates a volume and returns a root parcel.",
		&createLogVolCmd)
	parser.AddCommand("open-parcel-volume",
		"Open a parcel volume",
		"The open-parcel-volume command opens a volume.",
		&openParcelVolCmd)
	parser.AddCommand("open-volume",
		"Open a Nuvoloso volume",
		"The open-volume command opens a volume.",
		&openLogVolCmd)
	parser.AddCommand("destroy-parcel-volume",
		"Destroy a parcel volume",
		"The destroy-parcel-volume command destroys a volume.",
		&destroyParcelVolCmd)
	parser.AddCommand("destroy-volume",
		"Destroy a Nuvoloso volume",
		"The destroy-volume command destroys a volume.",
		&destroyLogVolCmd)
	parser.AddCommand("list-vols",
		"List Vols",
		"List the Vol series objects for this node",
		&listVolsCmd)
	parser.AddCommand("alloc-parcels",
		"Allocate parcels to a volume",
		"The alloc-parcels command allocates parcels to a volume.",
		&allocParcelsCmd)
	parser.AddCommand("alloc-cache",
		"Allocate cache to a volume",
		"The alloc-cache command allocates a specified amount of cache to a volume.",
		&allocCacheCmd)
	parser.AddCommand("close-volume",
		"Close a volume",
		"The close-vol command closes a volume.",
		&closeVolCmd)
	parser.AddCommand("get-stats",
		"Get stats from a volume or device",
		"The get-stats command gets read or write stats from a device or volume and optionally clears them.",
		&getStatsCmd)
	parser.AddCommand("get-vol-stats",
		"Get stats from a volume",
		"The get-stats command gets read, write and cache stats from a volume and optionally clears them.",
		&getVolStatsCmd)
	parser.AddCommand("manifest",
		"Get the manifest",
		"Does stuff",
		&getManifestCmd)
	parser.AddCommand("use-node-uuid",
		"Set the node UUID",
		"The set-node_uuid command sets the UUID for the node which enables the node.",
		&useNodeUUIDCmd)
	parser.AddCommand("create-pit",
		"Capture a point in time (PiT)",
		"Capture the state of a volume as a Point-in-time.",
		&createPitCmd)
	parser.AddCommand("delete-pit",
		"Delete a PiT",
		"Delete the previously created Point-in-time.",
		&deletePitCmd)
	parser.AddCommand("list-pits",
		"List Pits",
		"List the previously created Points-in-time on a volume.",
		&listPitsCmd)
	parser.AddCommand("pause-io",
		"Pause I/O on a volume.",
		"Pause I/O on a volume so that a snapshot can be captured.",
		&pauseIoCmd)
	parser.AddCommand("resume-io",
		"Resume I/O on a volume.",
		"Resume I/O on a volume that had previously been paused.",
		&resumeIoCmd)
	parser.AddCommand("log-level",
		"Set module log level.",
		"Set the log level on the named module.",
		&logLevelCmd)
	parser.AddCommand("node-status",
		"Get the node status.",
		"Get the node uuid, vol space usage and other status.",
		&nodeStatusCmd)
	parser.AddCommand("debug-trigger",
		"Trigger a debug routine.",
		"Trigger a debug routine with optional parameters.",
		&debugTriggerCmd)
	parser.AddCommand("log-summary",
		"Print the summary of a log segment.",
		"Print the summary of a log segment.",
		&logSummaryCmd)
	parser.AddCommand("garbage-collect",
		"Do garbage collection.",
		"Do garbage collection.",
		&getGarbageCollectCmd)
	parser.AddCommand("lun-path",
		"Get the lun path",
		"Get the lun path",
		&getLunPathCmd)
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
