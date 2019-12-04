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


// Package nuvoapi - API control of nuvo volume manager
//
// The nuvoapi package is the go control interface into the
// nuvo volume manager.  The underlying interface is simple.
// The volume manager exposes a unix socket.  Commands to the
// volume manager are formatted using protobufs defined in
// "nuvo.proto"  This package takes method calls, formats the
// protobufs and sends the command.  It returns apiError which
// is an extension of error.
//
// To send commands first create a DoSendRecver using
// NewSendRecv.  This takes the function that sends and
// receive as well as a string with a path to the control
// socket.
package nuvoapi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"path"
	"sync"
	"time"

	nuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/nuvo_pb"
	"github.com/docker/go-units"
	"github.com/golang/protobuf/proto"
)

// Constants related to the API. They may replaced by API calls in the future
const (
	DefaultSegmentSizeBytes          int64  = 4 * units.MiB
	DefaultDeviceFormatOverheadBytes int64  = 16 * units.MiB
	DeviceTypeSSD                    string = "SSD"
	DeviceTypeHDD                    string = "HDD"
)

// ErrorInt extends the error interface and is returned by all operations
type ErrorInt interface {
	error
	LongError() string
	NotInitialized() bool
	Raw() error
}

/*
StatsIO is stats for devices or volumes
*/
type StatsIO struct {
	Count          uint64
	SizeTotal      uint64
	LatencyMean    float64
	LatencyStdev   float64
	LatencySubBits uint64
	SizeHist       []uint64
	LatencyHist    []uint64
	SeriesUUID     string
}

/*
StatsCache are cache stats for a volume
*/
type StatsCache struct {
	IOReadTotal                uint64
	CacheIOReadLineTotalCount  uint64
	CacheIOReadLineHitCount    uint64
	CacheIOReadLineMissCount   uint64
	IOWriteTotal               uint64
	CacheIOWriteLineTotalCount uint64
}

/*
StatsCombinedVolume are combined stats for a volume
*/
type StatsCombinedVolume struct {
	IOReads       StatsIO
	IOWrites      StatsIO
	CacheUser     StatsCache
	CacheMetadata StatsCache
}

/*
Segment contains usage information about a segment
*/
type Segment struct {
	BlksUsed uint32
	Age      uint64
	PinCnt   uint32
	Log      bool
	Reserved bool

	Fullness    float64
	RelativeAge float64
}

/*
Parcel contains usage information about a parcel and its segments.
*/
type Parcel struct {
	State       string
	ParcelIndex uint32
	DeviceIndex uint32
	UUID        string
	SegmentSize uint32
	Segments    []Segment
}

/*
Device contains usage information about a device and its parcels.
*/
type Device struct {
	Index          uint32
	UUID           string
	ParcelSize     uint64
	TargetParcels  uint32
	AllocedParcels uint32
	FreeSegments   uint32
	DeviceClass    uint32
	BlocksUsed     uint64
	BlockSize      uint32
	Parcels        []Parcel
}

/*
Diff is a single difference between two Points in Time
*/
type Diff struct {
	Start  uint64
	Length uint64
	Dirty  bool
}

/*
VolClassSpace is the space status of a volume
*/
type VolClassSpace struct {
	Class           uint32
	BlocksUsed      uint64
	BlocksAllocated uint64
	BlocksTotal     uint64
	// Should have some way to explicitly ask for space?
}

/*
VolStatus is the status of a single volume.
*/
type VolStatus struct {
	VolUUID    string
	ClassSpace []VolClassSpace
}

/*
NodeStatusReport is the status of the node
*/
type NodeStatusReport struct {
	NodeUUID     string
	GitBuildHash string
	Volumes      []VolStatus
}

/*
DebugTriggerParams is the struct to pass in trigger parameters
*/
type DebugTriggerParams struct {
	Trigger          string
	NodeUUID         string
	VolUUID          string
	DevUUID          string
	ParcelIndex      uint32
	SegmentIndex     uint32
	InjectErrorType  uint32
	InjectReturnCode int32
	InjectRepeatCnt  int32
	InjectSkipCnt    int32
	Multiuse1        uint64
	Multiuse2        uint64
	Multiuse3        uint64
}

/*
LogEntry is the struct to return a log entry.  It is flat instead of
a union because go.
*/
type LogEntry struct {
	EntryType      uint32
	BlockHash      uint32
	SequenceNumber uint64
	SubClass       uint32
	PitInfoActive  bool
	PitInfoID      uint32
	BlockNumber    uint64
	ParcelIndex    uint32
	BlockOffset    uint32
	CVFlag         uint32
	EntryCount     uint32
	DataBlockCount uint32
	SnapOperation  uint32
}

/*
LogSummaryResults is the struct to pass back summary of a segment log
*/
type LogSummaryResults struct {
	VolUUID               string
	ParcelIndex           uint32
	SegmentIndex          uint32
	Magic                 uint32
	SequenceNumber        uint64
	ClosingSequenceNumber uint64
	Entries               []LogEntry
}

/*
Capabilities stores the capabilites of a node
*/
type Capabilities struct {
	Multifuse bool
}

var nodeCapabilities map[string]Capabilities
var capabilitiesMutex sync.Mutex

// apiError implements ErrorInt.
type apiError struct {
	lostState   bool  // true if failed, restarting or needs initialization
	err         error // raw error
	What        string
	Explanation string
	ReqJSON     string
	ReplyJSON   string
}

// Error satisfies the error interface. It returns What happened and Why.
func (e apiError) Error() string {
	if e.What == "" {
		return e.Explanation
	}
	return fmt.Sprintf("%v: %v", e.What, e.Explanation)
}

// LongError satisfies the ErrorInt interface. It returns What happened, Why and the JSON request and reply if available.
func (e apiError) LongError() string {
	return fmt.Sprintf("%v: %v, %v, %v", e.What, e.Explanation, e.ReqJSON, e.ReplyJSON)
}

// NotInitialized satisfies the ErrorInt interface. It returns true if nuvo failed, is restarting or needs initialization.
func (e apiError) NotInitialized() bool {
	return e.lostState || e.What == "BAD_ORDER"
}

// Raw satisfies the ErrorInt interface. It returns the base (non-nuvo) error if any.
func (e apiError) Raw() error {
	return e.err
}

// WrapFatalError recasts an error and returns a new error with NotInitialized() returning true.
// The underlying error is available through the Raw() interface.
// Exposed for external UTs.
func WrapFatalError(err error) error {
	return apiError{Explanation: err.Error(), err: err, lostState: true}
}

// WrapError recasts an error and returns a new error with NotInitialized() returning false.
// The underlying error is available through the Raw() interface.
// Exposed for external UTs.
func WrapError(err error) error {
	return apiError{Explanation: err.Error(), err: err}
}

// ErrorIsTemporary returns a bool if the error is a temporary net error
func ErrorIsTemporary(err error) bool {
	if nerr, ok := err.(ErrorInt); ok {
		if oe, ok := nerr.Raw().(*net.OpError); ok {
			if oe.Temporary() {
				return true
			}
		}
	}
	return false
}

// NewNuvoAPIError creates a NuvoApi error to be used for testing purposes
func NewNuvoAPIError(message string, temp bool) error {
	return WrapError(&net.OpError{Op: "set", Net: "pipe", Source: nil, Addr: nil, Err: newFakeError(message, temp)})
}

type fakeError struct {
	message string
	temp    bool
}

func (f fakeError) Error() string {
	return f.message
}
func newFakeError(msg string, t bool) error {
	return &fakeError{message: msg, temp: t}
}

type temporary interface {
	Temporary() bool
}

func (f fakeError) Temporary() bool {
	if f.temp {
		return true
	}
	return false
}

/*
When we get a failure, store away the json of the protobufs, if any,
to print out later.
*/
func failed(terseStr string, errStr string, req *nuvo.Cmd, reply *nuvo.Cmd) error {
	var e apiError
	e.What = terseStr
	e.Explanation = errStr
	if req != nil {
		j, _ := json.Marshal(req) // succeed or panic
		e.ReqJSON = string(j[:])
	}
	if reply != nil {
		j, _ := json.Marshal(reply) // succeed or panic
		e.ReplyJSON = string(j[:])
	}
	return e
}

// NuvoVM is the main interface.  Call NewSendRecv to get a DoSendRecver which
// implements the NuvoVM interface.  All of the errors may be underlying errors
// or apiError.  This could be cleaned up a bit, but it is serviceable.
// Each of these return nil on success or error on failure.
// The error returned by the operations is also an ErrorInt interface.
type NuvoVM interface {
	UseDevice(uuid string, path string, deviceType ...string) error
	UseCacheDevice(uuid string, path string) (uint64, uint64, error)
	CloseDevice(uuid string) error
	FormatDevice(uuid string, path string, ParcelSize uint64) (uint64, error)
	DeviceLocation(deviceUUID string, nodeUUID string) error
	NodeLocation(nodeUUID string, ipAddr string, port uint16) error
	NodeInitDone(nodeUUID string, clear bool) error
	OpenPassThroughVolume(name string, device string, deviceSize uint64) error
	ExportLun(volSeriesUUID string, pitUUID string, exportName string, writable bool) error
	LunPath(nuvoDir string, exportName string) string
	UnexportLun(volSeriesUUID string, pitUUID string, exportName string) error
	HaltCommand() error
	CreateParcelVol(volSeriesUUID string, rootDeviceUUID string, rootParcelUUID string) (string, error)
	CreateLogVol(volSeriesUUID string, rootDeviceUUID string, rootParcelUUID string, size uint64) (string, error)
	OpenVol(volSeriesUUID string, rootDeviceUUID string, rootParcelUUID string, logVol bool) error
	ListVols() ([]string, error)
	AllocParcels(volSeriesUUID string, deviceUUID string, num uint64) error
	CloseVol(volSeriesUUID string) error
	GetStats(device bool, read bool, clear bool, reqUUID string) (*StatsIO, error)
	GetVolumeStats(clear bool, reqUUID string) (*StatsCombinedVolume, error)
	DestroyVol(volSeriesUUID string, rootDeviceUUID string, rootParcelUUID string, logVol bool) error
	UseNodeUUID(nodeUUID string) error
	GetCapabilities() (*Capabilities, error)
	Manifest(volSeriesUUID string, short bool) ([]Device, error)
	CreatePit(volSeriesUUID string, pitUUID string) error
	DeletePit(volSeriesUUID string, PitUUID string) error
	ListPits(volSeriesUUID string) ([]string, error)
	PauseIo(volSeriesUUID string) error
	ResumeIo(volSeriesUUID string) error
	LogLevel(moduleName string, level uint32) error
	NodeStatus() (NodeStatusReport, error)
	GetPitDiffs(volSeriesUUID string, basePit string, incrPit string, off uint64) ([]Diff, uint64, error)
	DebugTrigger(params DebugTriggerParams) error
	LogSummary(volSeriesUUID string, parcelIndex uint32, segmentIndex uint32) (*LogSummaryResults, error)
	AllocCache(volSeriesUUID string, sizeBytes uint64) error
}

// DoSendRecv is the prototype of the routine that sends commands and gets replies.
// socket holds the path to the Unix control socket.
//
// cmd is a protobuf command as in nuvo.pb.go (generated from nuvo.proto)
type DoSendRecv func(socket string, cmd *nuvo.Cmd) (*nuvo.Cmd, error)

// DoSendRecver is a structure to hold both the worker sender/receiver and
// the string socket.  Having the worker function here allows us to
// substitute in a different handler for testing.
type DoSendRecver struct {
	doSendRecv DoSendRecv
	socket     string
}

// I do not remember what this is or why.
// Dave assures me it says that DoSendRecver implements the interface.
var _ = NuvoVM(&DoSendRecver{})

// NotInitializedAlerter provides a callback interface for NotInitialized() errors.
type NotInitializedAlerter interface {
	NuvoNotInitializedError(err error)
}

// NewNuvoVMWithNotInitializedAlerter returns a NuvoVM interface for external use.
// It provides an optional callback for errors where NotInitialized() is true.
func NewNuvoVMWithNotInitializedAlerter(socket string, cb NotInitializedAlerter) NuvoVM {
	wrappedSendRecv := func(socket string, cmd *nuvo.Cmd) (*nuvo.Cmd, error) {
		res, err := RealSendRecv(socket, cmd)
		if err != nil {
			e, ok := err.(apiError)
			if !ok {
				err = WrapFatalError(err)
				e, _ = err.(apiError)
			}
			if cb != nil && e.NotInitialized() {
				cb.NuvoNotInitializedError(err)
			}
		}
		return res, err
	}
	return NewSendRecv(wrappedSendRecv, socket)
}

// NewNuvoVM returns a NuvoVM interface for external use.
func NewNuvoVM(socket string) NuvoVM {
	return NewNuvoVMWithNotInitializedAlerter(socket, nil)
}

// NewSendRecv takes a worker routine and a socket to talk to and returns the
// DoSendRecver that implements the DoSendRecver interface.
func NewSendRecv(dsr DoSendRecv, socket string) *DoSendRecver {
	return &DoSendRecver{
		doSendRecv: dsr,
		socket:     socket,
	}
}

// RealSendRecvTimeout is the core workhorse.  I have it exported right now so it can
// be passed into NewSendRecv (which allows alternate versions to be passed in
// for testing) Packs up the protobuf, opens the socket. Sends it over the socket,
// receives the replay and returns it.
//
// This returns *nuvo.Cmd, nil on success.
// This returns nil, error on failure
func RealSendRecvTimeout(socket string, cmd *nuvo.Cmd, milliseconds, maxStepMilliseconds int32) (*nuvo.Cmd, error) {
	data, err := proto.Marshal(cmd)
	if err != nil {
		//log.Panic("marshaling error: ", err)
		return nil, err
	}
	var length uint32
	length = uint32(len(data))
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.LittleEndian, length)
	if err != nil {
		//log.Panic("binary.Write failed", err)
		return nil, err
	}
	buf.Write([]byte(data))

	// Try opening the control socket. If fail, sleep
	// for 0-100 ms.  Double retry max each time until we get to
	// 1s.  Keep trying until past timeout_milliseconds.
	rand.Seed(time.Now().UTC().UnixNano())
	retryMillisecondsMax := int32(100)
	retryDurationLeft := time.Duration(milliseconds) * time.Millisecond
	var c net.Conn
	for retryDurationLeft > 0 {
		c, err = net.Dial("unix", socket)
		if err == nil {
			defer c.Close()
			break
		} else if oe, ok := err.(*net.OpError); ok {
			if !(oe.Temporary()) {
				return nil, err
			}
			duration := time.Duration(int64(time.Millisecond) * int64(rand.Int31n(retryMillisecondsMax)))
			if duration > retryDurationLeft {
				duration = retryDurationLeft
				retryDurationLeft = 0
			} else {
				retryDurationLeft -= duration
			}
			time.Sleep(duration)
			retryMillisecondsMax *= 2
			if retryMillisecondsMax > maxStepMilliseconds {
				retryMillisecondsMax = maxStepMilliseconds
			}
		} else {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	_, err = c.Write(buf.Bytes())
	if err != nil {
		return nil, err
	}

	lengthBuf := make([]byte, 4)
	n, err := c.Read(lengthBuf)
	if err != nil {
		return nil, err
	} else if n != 4 {
		return nil, errors.New("less than 4 bytes in reply")
	}
	var readLen uint32
	err = binary.Read(bytes.NewReader(lengthBuf), binary.LittleEndian, &readLen)
	if err != nil {
		//log.Panic("binary.Read failed:", err)
		return nil, err
	}
	readBuf := make([]byte, readLen)
	n, err = c.Read(readBuf[:])

	var reply = new(nuvo.Cmd)
	err = proto.Unmarshal(readBuf[:], reply)
	if err != nil {
		log.Print("unmarshaling error: ", err)
		return nil, err
	}

	return reply, nil
}

// RealSendRecv is the core workhorse.  I have it exported right now so it can
// be passed into NewSendRecv (which allows alternate versions to be passed in
// for testing) Packs up the protobuf, opens the socket. Sends it over the socket,
// receives the replay and returns it.
//
// This returns *nuvo.Cmd, nil on success.
// This returns nil, error on failure
func RealSendRecv(socket string, cmd *nuvo.Cmd) (*nuvo.Cmd, error) {
	return RealSendRecvTimeout(socket, cmd, 5000, 1000)
}

func buildUseDevice(uuid string, path string, deviceType nuvo.UseDevice_DevType) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_USE_DEVICE_REQ
	p.MsgType = &mt
	ud := new(nuvo.UseDevice)
	ud.Path = proto.String(path)
	ud.Uuid = proto.String(uuid)
	ud.DevType = &deviceType
	p.UseDevice = append(p.UseDevice, ud)
	return p
}

// UseDevice allows for either SSD or HDD to be specified as a device type
var useDeviceTypeChoices = map[string]nuvo.UseDevice_DevType{DeviceTypeSSD: nuvo.UseDevice_SSD, DeviceTypeHDD: nuvo.UseDevice_HDD}

// UseDevice tells the volume manager to use a device.
// This takes strings holding a uuid and a path.  Validation that
// the uuid is really a UUID is the responsibility of the volume
// manager.
//
//Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) UseDevice(uuid string, path string, deviceTypeChoice ...string) error {
	var devType nuvo.UseDevice_DevType
	if len(deviceTypeChoice) >= 1 {
		if _, valid := useDeviceTypeChoices[deviceTypeChoice[0]]; !valid {
			return failed("Api Error", "Invalid device type", nil, nil)
		}
		devType = useDeviceTypeChoices[deviceTypeChoice[0]]
	} else {
		devType = nuvo.UseDevice_SSD
	}
	req := buildUseDevice(uuid, path, devType)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_USE_DEVICE_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if len(reply.UseDevice) != 1 {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.UseDevice[0].GetResult() != nuvo.UseDevice_OK {
		return failed(reply.UseDevice[0].GetResult().String(),
			reply.UseDevice[0].GetExplanation(), req, reply)
	}
	return nil
}

// UseCacheDevice tells the volume manager to use a device.
// This takes strings holding a uuid and a path.  Validation that
// the uuid is really a UUID is the responsibility of the volume
// manager. The usable size and cacheUnitSizeBytes are returned on success.
// The cacheUnitSizeBytes applies to the entire cache, not just the new device. It can change as each new cache device is added.
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) UseCacheDevice(uuid string, path string) (uint64, uint64, error) {
	req := buildUseDevice(uuid, path, nuvo.UseDevice_EPH)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return 0, 0, err
	}
	if *reply.MsgType != nuvo.Cmd_USE_DEVICE_REPLY {
		return 0, 0, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if len(reply.UseDevice) != 1 {
		return 0, 0, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.UseDevice[0].GetResult() != nuvo.UseDevice_OK {
		return 0, 0, failed(reply.UseDevice[0].GetResult().String(),
			reply.UseDevice[0].GetExplanation(), req, reply)
	}
	return reply.UseDevice[0].GetSize(), reply.UseDevice[0].GetAllocSize(), nil
}

func buildCloseDevice(uuid string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_CLOSE_DEVICE_REQ
	p.MsgType = &mt
	fd := new(nuvo.CloseDevice)
	fd.Uuid = proto.String(uuid)
	p.CloseDevice = append(p.CloseDevice, fd)
	return p
}

// CloseDevice tells the volume manager to close a device.
// This takes a string uuid for the device to close.
// Validation that parameters make sense is the responsibility
// of the volume manager.
//
// Returns:
//  - 0 on success
//  - Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) CloseDevice(uuid string) error {
	req := buildCloseDevice(uuid)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_CLOSE_DEVICE_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if len(reply.CloseDevice) != 1 {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.CloseDevice[0].GetResult() != nuvo.CloseDevice_OK {
		return failed(reply.CloseDevice[0].GetResult().String(),
			reply.CloseDevice[0].GetExplanation(), req, reply)
	}
	return nil
}

func buildFormatDevice(uuid string, path string, ParcelSize uint64) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_FORMAT_DEVICE_REQ
	p.MsgType = &mt
	fd := new(nuvo.FormatDevice)
	fd.Path = proto.String(path)
	fd.Uuid = proto.String(uuid)
	fd.ParcelSize = proto.Uint64(ParcelSize)
	p.FormatDevice = append(p.FormatDevice, fd)
	return p
}

// FormatDevice tells the volume manager to format a device.
// This takes strings holding a uuid and a path and the parcel size.
// Validation that parameters make sense is the responsibility
// of the volume manager.
//
// Returns:
//  - Total parcel count on success, or 0 otherwise
//  - Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) FormatDevice(uuid string, path string, ParcelSize uint64) (uint64, error) {
	req := buildFormatDevice(uuid, path, ParcelSize)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return 0, err
	}
	if *reply.MsgType != nuvo.Cmd_FORMAT_DEVICE_REPLY {
		return 0, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if len(reply.FormatDevice) != 1 {
		return 0, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.FormatDevice[0].GetResult() != nuvo.FormatDevice_OK {
		return 0, failed(reply.FormatDevice[0].GetResult().String(),
			reply.FormatDevice[0].GetExplanation(), req, reply)
	}
	return 0, nil // TBD: provide the real total parcel count
}

func buildDeviceLocation(deviceUUID string, nodeUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_DEVICE_LOCATION_REQ
	p.MsgType = &mt
	dl := new(nuvo.DeviceLocation)
	dl.Node = proto.String(nodeUUID)
	dl.Device = proto.String(deviceUUID)
	p.DeviceLocation = append(p.DeviceLocation, dl)
	return p
}

// DeviceLocation tells the volume manager where a device is.
// This takes strings holding the device uuid and the node uuid.
// Validation that parameters make sense is the responsibility
// of the volume manager.
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) DeviceLocation(deviceUUID string, nodeUUID string) error {
	req := buildDeviceLocation(deviceUUID, nodeUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_DEVICE_LOCATION_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if len(reply.DeviceLocation) != 1 {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.DeviceLocation[0].GetResult() != nuvo.DeviceLocation_OK {
		return failed(reply.DeviceLocation[0].GetResult().String(),
			reply.DeviceLocation[0].GetExplanation(), req, reply)
	}
	return nil
}

func buildNodeLocation(nodeUUID string, ipAddr string, port uint16) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_NODE_LOCATION_REQ
	p.MsgType = &mt
	nl := new(nuvo.NodeLocation)
	nl.Uuid = proto.String(nodeUUID)
	nl.Ipv4Addr = proto.String(ipAddr)
	nl.Port = proto.Uint32(uint32(port))
	p.NodeLocation = append(p.NodeLocation, nl)
	return p
}

// NodeLocation tells the volume manager where a node is.
// This takes strings holding a uuid the ip address and port.
// Validation that parameters make sense is the responsibility
// of the volume manager.
//
// The ipAddr can be a dotted address (e.g. "127.0.0.1")
// or a network name.
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) NodeLocation(nodeUUID string, ipAddr string, port uint16) error {
	req := buildNodeLocation(nodeUUID, ipAddr, port)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_NODE_LOCATION_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if len(reply.NodeLocation) != 1 {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.NodeLocation[0].GetResult() != nuvo.NodeLocation_OK {
		return failed(reply.NodeLocation[0].GetResult().String(),
			reply.NodeLocation[0].GetExplanation(), req, reply)
	}
	return nil
}

func buildNodeInitDone(nodeUUID string, clear bool) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_NODE_INIT_DONE_REQ
	p.MsgType = &mt
	nid := new(nuvo.NodeInitDone)
	nid.Uuid = proto.String(nodeUUID)
	nid.Clear = proto.Bool(clear)
	p.NodeInitDone = nid
	return p
}

// NodeInitDone tells the volume manager when node initialization is complete.
// This takes a string containing the uuid of the initialized node
// Validation that parameters make sense is the responsibility of the
// volume manager.
//
// Might return an error or an apiError
func (dsr *DoSendRecver) NodeInitDone(nodeUUID string, clear bool) error {
	req := buildNodeInitDone(nodeUUID, clear)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_NODE_INIT_DONE_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.NodeInitDone == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.NodeInitDone.GetResult() != nuvo.NodeInitDone_OK {
		return failed(reply.NodeInitDone.GetResult().String(),
			reply.NodeInitDone.GetExplanation(), req, reply)
	}
	return nil
}

func buildOpenPassThroughVolume(uuid string, device string, deviceSize uint64) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_OPEN_PASSTHROUGH_REQ
	p.MsgType = &mt
	p.OpenPassThroughVol = new(nuvo.OpenPassThroughVolume)
	p.OpenPassThroughVol.Uuid = proto.String(uuid)
	p.OpenPassThroughVol.Path = proto.String(device)
	p.OpenPassThroughVol.Size = proto.Uint64(deviceSize)
	return p
}

// OpenPassThroughVolume tells the volume manager to open a passthrough volume
// Takes strings for the uuid of the volume (since there is no label)
// and the path to the device.  The deviceSize is what size the device
// should be.  Validation that parameters make sense is the responsibility
// of the volume manager.  But don't be surprised if the volume
// manager is unhappy if deviceSize is not a multiple of 4096.
//
// This does not export a lun.
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) OpenPassThroughVolume(uuid string, device string, deviceSize uint64) error {
	req := buildOpenPassThroughVolume(uuid, device, deviceSize)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_OPEN_PASSTHROUGH_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.OpenPassThroughVol == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.OpenPassThroughVol.GetResult() != nuvo.OpenPassThroughVolume_OK {
		return failed(reply.OpenPassThroughVol.GetResult().String(),
			reply.OpenPassThroughVol.GetExplanation(), req, reply)
	}
	return nil
}

func buildHaltCommand() *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_SHUTDOWN
	p.MsgType = &mt
	return p
}

// HaltCommand halts the volume manager.
// It does it's best to cleanly shutdown, including unmounting the fuse directory
// and deleting the command socket.  This command is unique in that we expect to
// NOT get a reply.
func (dsr *DoSendRecver) HaltCommand() error {
	req := buildHaltCommand()
	_, err := dsr.doSendRecv(dsr.socket, req)
	return err
}

func buildExportLun(volSeriesUUID string, pitUUID string, exportName string, writable bool) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_EXPORT_LUN_REQ
	p.MsgType = &mt
	p.ExportLun = new(nuvo.ExportLun)
	p.ExportLun.VolSeriesUuid = proto.String(volSeriesUUID)
	if pitUUID != "" {
		p.ExportLun.PitUuid = proto.String(pitUUID)
	}
	p.ExportLun.ExportName = proto.String(exportName)
	p.ExportLun.Writable = proto.Bool(writable)
	return p
}

// ExportLun tells the volume manager to export a lun from a volume series.
// The volume series specified by volSeriesUUID must already have
// been opened.  The LUN will be exported as exportName.  It can
// be writeable or readonly, depending on the value of writable.
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) ExportLun(volSeriesUUID string, pitUUID string, exportName string, writable bool) error {
	req := buildExportLun(volSeriesUUID, pitUUID, exportName, writable)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_EXPORT_LUN_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.ExportLun == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.ExportLun.GetResult() != nuvo.ExportLun_OK {
		return failed(reply.ExportLun.GetResult().String(),
			reply.ExportLun.GetExplanation(), req, reply)
	}
	return nil
}

// LunPath takes the nuvodir and exportName and returns the path to the lun
func (dsr *DoSendRecver) LunPath(nuvoDir string, exportName string) string {
	capabilitiesMutex.Lock()
	cap, prs := nodeCapabilities[dsr.socket]
	capabilitiesMutex.Unlock()
	if prs == false {
		panic(fmt.Sprintf("LunPath called before Capabilities: %s", dsr.socket))
	}
	if cap.Multifuse {
		return path.Join(nuvoDir, exportName, "vol")
	}
	return path.Join(nuvoDir, exportName)
}

func buildUnexportLun(volSeriesUUID string, pitUUID string, exportName string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_UNEXPORT_LUN_REQ
	p.MsgType = &mt
	p.UnexportLun = new(nuvo.UnexportLun)
	p.UnexportLun.VolSeriesUuid = proto.String(volSeriesUUID)
	if pitUUID != "" {
		p.UnexportLun.PitUuid = proto.String(pitUUID)
	}
	p.UnexportLun.ExportName = proto.String(exportName)
	return p
}

// UnexportLun tells the volume manager to unexport a lun that has been exported.
// the volSeriesUUID and exportName are redundant, but eventually
// will not be when we have snapshots.
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) UnexportLun(volSeriesUUID string, pitUUID string, exportName string) error {
	req := buildUnexportLun(volSeriesUUID, pitUUID, exportName)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_UNEXPORT_LUN_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.UnexportLun == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.UnexportLun.GetResult() != nuvo.UnexportLun_OK {
		return failed(reply.ExportLun.GetResult().String(),
			reply.ExportLun.GetExplanation(), req, reply)
	}
	return nil
}

func buildCreateParcelVol(volUUID string, rootDevice string, rootParcel string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_CREATE_VOLUME_REQ
	p.MsgType = &mt
	p.CreateVolume = new(nuvo.CreateVolume)
	p.CreateVolume.VolSeriesUuid = proto.String(volUUID)
	p.CreateVolume.RootDeviceUuid = proto.String(rootDevice)
	p.CreateVolume.RootParcelUuid = proto.String(rootParcel)
	p.CreateVolume.LogVolume = proto.Bool(false)
	return p
}

// CreateParcelVol tells the volume manager to create a parcel volume
// with volSeriesUUID and root parcel on rootDeviceUUID
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) CreateParcelVol(volSeriesUUID string, rootDeviceUUID string, rootParcel string) (string, error) {
	req := buildCreateParcelVol(volSeriesUUID, rootDeviceUUID, rootParcel)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return "", err
	}
	if *reply.MsgType != nuvo.Cmd_CREATE_VOLUME_REPLY {
		return "", failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.CreateVolume == nil {
		return "", failed("Api Error", "Improper reply", req, reply)
	}
	if reply.CreateVolume.GetResult() != nuvo.CreateVolume_OK {
		return "", failed(reply.CreateVolume.GetResult().String(),
			reply.CreateVolume.GetExplanation(), req, reply)
	}
	return reply.CreateVolume.GetRootParcelUuid(), nil
}

func buildCreateLogVol(volUUID string, rootDevice string, rootParcel string, size uint64) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_CREATE_VOLUME_REQ
	p.MsgType = &mt
	p.CreateVolume = new(nuvo.CreateVolume)
	p.CreateVolume.VolSeriesUuid = proto.String(volUUID)
	p.CreateVolume.RootDeviceUuid = proto.String(rootDevice)
	p.CreateVolume.RootParcelUuid = proto.String(rootParcel)
	p.CreateVolume.LogVolume = proto.Bool(true)
	p.CreateVolume.Size = proto.Uint64(size)
	return p
}

// CreateLogVol tells the volume manager to create a log volume
// with volSeriesUUID and root parcel on rootDeviceUUID
// Size is the nominal size of the active LUN.
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) CreateLogVol(volSeriesUUID string, rootDeviceUUID string, rootParcel string, size uint64) (string, error) {
	req := buildCreateLogVol(volSeriesUUID, rootDeviceUUID, rootParcel, size)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return "", err
	}
	if *reply.MsgType != nuvo.Cmd_CREATE_VOLUME_REPLY {
		return "", failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.CreateVolume == nil {
		return "", failed("Api Error", "Improper reply", req, reply)
	}
	if reply.CreateVolume.GetResult() != nuvo.CreateVolume_OK {
		return "", failed(reply.CreateVolume.GetResult().String(),
			reply.CreateVolume.GetExplanation(), req, reply)
	}
	return reply.CreateVolume.GetRootParcelUuid(), nil
}

func buildOpenVol(volUUID string, rootDevice string, rootParcel string, logVol bool) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_OPEN_VOLUME_REQ
	p.MsgType = &mt
	p.OpenVolume = new(nuvo.OpenVolume)
	p.OpenVolume.VolSeriesUuid = proto.String(volUUID)
	p.OpenVolume.RootDeviceUuid = proto.String(rootDevice)
	p.OpenVolume.RootParcelUuid = proto.String(rootParcel)
	p.OpenVolume.LogVolume = proto.Bool(logVol)
	return p
}

// OpenVol tells the volume manager to open a volume
// with volSeriesUUID and root parcel rootParcelUUID on rootDeviceUUID
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) OpenVol(volSeriesUUID string, rootDeviceUUID string, rootParcelUUID string, logVol bool) error {
	req := buildOpenVol(volSeriesUUID, rootDeviceUUID, rootParcelUUID, logVol)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_OPEN_VOLUME_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.OpenVolume == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.OpenVolume.GetResult() != nuvo.OpenVolume_OK {
		return failed(reply.OpenVolume.GetResult().String(),
			reply.OpenVolume.GetExplanation(), req, reply)
	}
	return nil
}

func buildDestroyVol(volUUID string, rootDevice string, rootParcel string, logVol bool) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_DESTROY_VOL_REQ
	p.MsgType = &mt
	p.DestroyVol = new(nuvo.DestroyVol)
	p.DestroyVol.VolUuid = proto.String(volUUID)
	p.DestroyVol.RootDeviceUuid = proto.String(rootDevice)
	p.DestroyVol.RootParcelUuid = proto.String(rootParcel)
	p.DestroyVol.LogVolume = proto.Bool(logVol)
	return p
}

// DestroyVol tells the volume manager to destroy a volume
// with volSeriesUUID and root parcel rootParcelUUID on rootDeviceUUID
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) DestroyVol(volSeriesUUID string, rootDeviceUUID string, rootParcelUUID string, logVol bool) error {
	req := buildDestroyVol(volSeriesUUID, rootDeviceUUID, rootParcelUUID, logVol)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_DESTROY_VOL_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.DestroyVol == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.DestroyVol.GetResult() != nuvo.DestroyVol_OK {
		return failed(reply.DestroyVol.GetResult().String(),
			reply.DestroyVol.GetExplanation(), req, reply)
	}
	return nil
}

func buildAllocParcels(volUUID string, deviceUUID string, num uint64) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_ALLOC_PARCELS_REQ
	p.MsgType = &mt
	p.AllocParcels = new(nuvo.AllocParcels)
	p.AllocParcels.VolSeriesUuid = proto.String(volUUID)
	p.AllocParcels.DeviceUuid = proto.String(deviceUUID)
	p.AllocParcels.Num = proto.Uint64(num)
	return p
}

// AllocParcels tells the volume manager to allocate parcels
// from device deviceUUID for volume volSeriesUUID
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) AllocParcels(volSeriesUUID string, deviceUUID string, num uint64) error {
	req := buildAllocParcels(volSeriesUUID, deviceUUID, num)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_ALLOC_PARCELS_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.AllocParcels == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.AllocParcels.GetResult() != nuvo.AllocParcels_OK {
		return failed(reply.AllocParcels.GetResult().String(),
			reply.AllocParcels.GetExplanation(), req, reply)
	}
	return nil
}

func buildAllocCache(volUUID string, sizeBytes uint64) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_ALLOC_CACHE_REQ
	p.MsgType = &mt
	p.AllocCache = new(nuvo.AllocCache)
	p.AllocCache.VolSeriesUuid = proto.String(volUUID)
	p.AllocCache.SizeBytes = proto.Uint64(sizeBytes)
	return p
}

// AllocCache tells the volume manager to allocate size bytes cache
// to the volume volSeriesUUID
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) AllocCache(volSeriesUUID string, sizeBytes uint64) error {
	req := buildAllocCache(volSeriesUUID, sizeBytes)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_ALLOC_CACHE_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.AllocCache == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.AllocCache.GetResult() != nuvo.AllocCache_OK {
		return failed(reply.AllocCache.GetResult().String(),
			reply.AllocCache.GetExplanation(), req, reply)
	}
	return nil
}

func buildCloseVol(volUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_CLOSE_VOL_REQ
	p.MsgType = &mt
	p.CloseVol = new(nuvo.CloseVol)
	p.CloseVol.VolSeriesUuid = proto.String(volUUID)
	return p
}

// CloseVol tells the volume manager to close volume volSeriesUUID
//
// Might return an error (such as a socket error) or an apiError
func (dsr *DoSendRecver) CloseVol(volSeriesUUID string) error {
	req := buildCloseVol(volSeriesUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_CLOSE_VOL_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.CloseVol == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.CloseVol.GetResult() != nuvo.CloseVol_OK {
		return failed(reply.CloseVol.GetResult().String(),
			reply.CloseVol.GetExplanation(), req, reply)
	}
	return nil
}

func buildGetStats(device bool, read bool, clear bool, reqUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_GET_STATS_REQ
	p.MsgType = &mt
	p.GetStats = new(nuvo.GetStats)
	if device {
		t := nuvo.GetStats_Type(nuvo.GetStats_DEVICE)
		p.GetStats.Type = &t
	} else {
		t := nuvo.GetStats_Type(nuvo.GetStats_VOLUME)
		p.GetStats.Type = &t
	}
	if read {
		rw := nuvo.GetStats_ReadWrite(nuvo.GetStats_READ)
		p.GetStats.Rw = &rw
	} else {
		rw := nuvo.GetStats_ReadWrite(nuvo.GetStats_WRITE)
		p.GetStats.Rw = &rw
	}
	clr := clear
	p.GetStats.Clear = &clr
	p.GetStats.Uuid = proto.String(reqUUID)
	return p
}

// GetStats gets stats for a device or volume
func (dsr *DoSendRecver) GetStats(device bool, read bool, clear bool, reqUUID string) (*StatsIO, error) {
	req := buildGetStats(device, read, clear, reqUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, err
	}
	if *reply.MsgType != nuvo.Cmd_GET_STATS_REPLY {
		return nil, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.GetStats == nil {
		return nil, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.GetStats.GetResult() != nuvo.GetStats_OK {
		return nil, failed(reply.GetStats.GetResult().String(),
			reply.GetStats.GetExplanation(), req, reply) /// UGH!
	}
	var stats StatsIO
	stats.Count = reply.GetStats.Stats.GetCount()
	stats.SizeTotal = reply.GetStats.Stats.GetSizeTotal()
	stats.LatencyMean = reply.GetStats.Stats.GetLatencyMean()
	stats.LatencyStdev = reply.GetStats.Stats.GetLatencyStdev()
	stats.LatencySubBits = reply.GetStats.Stats.GetLatencySubBits()
	stats.SizeHist = reply.GetStats.Stats.GetSizeHist()
	stats.LatencyHist = reply.GetStats.Stats.GetLatencyHist()
	stats.SeriesUUID = reply.GetStats.Stats.GetSeriesUuid()

	return &stats, nil
}

func buildGetVolumeStats(clear bool, reqUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_GET_VOLUME_STATS_REQ
	p.MsgType = &mt
	p.GetVolumeStats = new(nuvo.GetVolumeStats)
	clr := clear
	p.GetVolumeStats.Clear = &clr
	p.GetVolumeStats.Uuid = proto.String(reqUUID)
	return p
}

// GetVolumeStats gets stats for a volume
func (dsr *DoSendRecver) GetVolumeStats(clear bool, reqUUID string) (*StatsCombinedVolume, error) {
	req := buildGetVolumeStats(clear, reqUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, err
	}
	if *reply.MsgType != nuvo.Cmd_GET_VOLUME_STATS_REPLY {
		return nil, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.GetVolumeStats == nil {
		return nil, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.GetVolumeStats.GetResult() != nuvo.GetVolumeStats_OK {
		return nil, failed(reply.GetVolumeStats.GetResult().String(),
			reply.GetVolumeStats.GetExplanation(), req, reply) /// UGH!
	}
	var stats StatsCombinedVolume

	stats.IOReads.Count = reply.GetVolumeStats.GetReadStats().GetCount()
	stats.IOReads.SizeTotal = reply.GetVolumeStats.GetReadStats().GetSizeTotal()
	stats.IOReads.LatencyMean = reply.GetVolumeStats.GetReadStats().GetLatencyMean()
	stats.IOReads.LatencyStdev = reply.GetVolumeStats.GetReadStats().GetLatencyStdev()
	stats.IOReads.LatencySubBits = reply.GetVolumeStats.GetReadStats().GetLatencySubBits()
	stats.IOReads.SizeHist = reply.GetVolumeStats.GetReadStats().GetSizeHist()
	stats.IOReads.LatencyHist = reply.GetVolumeStats.GetReadStats().GetLatencyHist()
	stats.IOReads.SeriesUUID = reply.GetVolumeStats.GetReadStats().GetSeriesUuid()

	stats.IOWrites.Count = reply.GetVolumeStats.GetWriteStats().GetCount()
	stats.IOWrites.SizeTotal = reply.GetVolumeStats.GetWriteStats().GetSizeTotal()
	stats.IOWrites.LatencyMean = reply.GetVolumeStats.GetWriteStats().GetLatencyMean()
	stats.IOWrites.LatencyStdev = reply.GetVolumeStats.GetWriteStats().GetLatencyStdev()
	stats.IOWrites.LatencySubBits = reply.GetVolumeStats.GetWriteStats().GetLatencySubBits()
	stats.IOWrites.SizeHist = reply.GetVolumeStats.GetWriteStats().GetSizeHist()
	stats.IOWrites.LatencyHist = reply.GetVolumeStats.GetWriteStats().GetLatencyHist()
	stats.IOWrites.SeriesUUID = reply.GetVolumeStats.GetWriteStats().GetSeriesUuid()

	stats.CacheUser.IOReadTotal = reply.GetVolumeStats.GetCacheStatsUser().GetIoReadTotal()
	stats.CacheUser.CacheIOReadLineTotalCount = reply.GetVolumeStats.GetCacheStatsUser().GetCacheIoReadLineTotalCount()
	stats.CacheUser.CacheIOReadLineHitCount = reply.GetVolumeStats.GetCacheStatsUser().GetCacheIoReadLineHitCount()
	stats.CacheUser.CacheIOReadLineMissCount = reply.GetVolumeStats.GetCacheStatsUser().GetCacheIoReadLineMissCount()
	stats.CacheUser.IOWriteTotal = reply.GetVolumeStats.GetCacheStatsUser().GetIoWriteTotal()
	stats.CacheUser.CacheIOWriteLineTotalCount = reply.GetVolumeStats.GetCacheStatsUser().GetCacheIoWriteLineTotalCount()

	stats.CacheMetadata.IOReadTotal = reply.GetVolumeStats.GetCacheStatsMetadata().GetIoReadTotal()
	stats.CacheMetadata.CacheIOReadLineTotalCount = reply.GetVolumeStats.GetCacheStatsMetadata().GetCacheIoReadLineTotalCount()
	stats.CacheMetadata.CacheIOReadLineHitCount = reply.GetVolumeStats.GetCacheStatsMetadata().GetCacheIoReadLineHitCount()
	stats.CacheMetadata.CacheIOReadLineMissCount = reply.GetVolumeStats.GetCacheStatsMetadata().GetCacheIoReadLineMissCount()
	stats.CacheMetadata.IOWriteTotal = reply.GetVolumeStats.GetCacheStatsMetadata().GetIoWriteTotal()
	stats.CacheMetadata.CacheIOWriteLineTotalCount = reply.GetVolumeStats.GetCacheStatsMetadata().GetCacheIoWriteLineTotalCount()

	return &stats, nil
}

func buildManifest(volUUID string, short bool) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mfst := nuvo.Cmd_MANIFEST_REQ
	p.MsgType = &mfst
	p.Manifest = new(nuvo.Manifest)
	p.Manifest.VolUuid = proto.String(volUUID)
	p.Manifest.ShortReply = proto.Bool(short)
	return p
}

// Manifest gets current state of a manifest for a volume
func (dsr *DoSendRecver) Manifest(volUUID string, short bool) ([]Device, error) {
	req := buildManifest(volUUID, short)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, err
	}
	if *reply.MsgType != nuvo.Cmd_MANIFEST_REPLY {
		return nil, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.Manifest == nil {
		return nil, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.Manifest.GetResult() != nuvo.Manifest_OK {
		return nil, failed(reply.Manifest.GetResult().String(),
			reply.Manifest.GetExplanation(), req, reply)
	}
	protoParcels := reply.Manifest.GetParcels()
	protoDevs := reply.Manifest.GetDevices()

	// compute the min and max non-zero age
	minAge := uint64(math.MaxUint64)
	maxAge := uint64(0)
	for _, protoParcel := range protoParcels {
		for _, protoSegment := range protoParcel.Segments {

			if protoSegment.GetAge() > 0 && protoSegment.GetAge() < minAge {
				minAge = protoSegment.GetAge()
			}
			if maxAge < protoSegment.GetAge() {
				maxAge = protoSegment.GetAge()
			}
		}
	}
	if maxAge == minAge {
		maxAge++
	}

	devices := make([]Device, len(protoDevs))
	for i, protoDev := range protoDevs {
		devices[i].Index = protoDev.GetDeviceIndex()
		devices[i].UUID = protoDev.GetDeviceUuid()
		devices[i].ParcelSize = protoDev.GetParcelSize()
		devices[i].TargetParcels = protoDev.GetTargetParcels()
		devices[i].AllocedParcels = protoDev.GetAllocedParcels()
		devices[i].FreeSegments = protoDev.GetFreeSegments()
		devices[i].AllocedParcels = protoDev.GetAllocedParcels()
		devices[i].DeviceClass = protoDev.GetDeviceClass()
		devices[i].BlocksUsed = protoDev.GetBlocksUsed()
		devices[i].BlockSize = 4096
		devices[i].Parcels = make([]Parcel, protoDev.GetAllocedParcels())
		if len(devices[i].Parcels) == 0 {
			continue
		}
		devI := 0
		for _, protoParcel := range protoParcels {
			if protoParcel.GetDeviceIndex() != devices[i].Index {
				continue
			}
			devices[i].Parcels[devI].ParcelIndex = protoParcel.GetParcelIndex()
			devices[i].Parcels[devI].DeviceIndex = protoParcel.GetDeviceIndex()
			devices[i].Parcels[devI].UUID = protoParcel.GetParcelUuid()
			devices[i].Parcels[devI].SegmentSize = protoParcel.GetSegmentSize()
			devices[i].Parcels[devI].Segments = make([]Segment, len(protoParcel.Segments))
			for sI, protoSegment := range protoParcel.Segments {
				devices[i].Parcels[devI].Segments[sI].BlksUsed = protoSegment.GetBlksUsed()
				devices[i].Parcels[devI].Segments[sI].Age = protoSegment.GetAge()
				devices[i].Parcels[devI].Segments[sI].Log = protoSegment.GetLogger()
				devices[i].Parcels[devI].Segments[sI].PinCnt = protoSegment.GetPinCnt()
				devices[i].Parcels[devI].Segments[sI].Reserved = protoSegment.GetReserved()
				devices[i].Parcels[devI].Segments[sI].Fullness =
					float64(protoSegment.GetBlksUsed()*4096) / float64(devices[i].Parcels[devI].SegmentSize)
				if protoSegment.GetAge() != 0 {
					devices[i].Parcels[devI].Segments[sI].RelativeAge =
						(float64(maxAge - protoSegment.GetAge())) / float64(maxAge-minAge)
				}
			}
			switch protoParcel.GetState() {
			case nuvo.ManifestParcel_UNUSED:
				devices[i].Parcels[devI].State = "UNUSED"
			case nuvo.ManifestParcel_ADDING:
				devices[i].Parcels[devI].State = "ADDING"
			case nuvo.ManifestParcel_USABLE:
				devices[i].Parcels[devI].State = "USABLE"
			case nuvo.ManifestParcel_OPENING:
				devices[i].Parcels[devI].State = "OPENING"
			case nuvo.ManifestParcel_OPEN:
				devices[i].Parcels[devI].State = "OPEN"
			}
			devI++
		}
	}

	return devices, nil
}

func buildUseNodeUUID(uuid string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_USE_NODE_UUID_REQ
	p.MsgType = &mt
	unu := new(nuvo.UseNodeUuid)
	unu.Uuid = proto.String(uuid)
	p.UseNodeUuid = unu
	return p
}

// UseNodeUUID Tells the node to use a particular node UUID
func (dsr *DoSendRecver) UseNodeUUID(uuid string) error {
	_, err := dsr.GetCapabilities()
	if err != nil {
		return err
	}
	req := buildUseNodeUUID(uuid)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_USE_NODE_UUID_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.UseNodeUuid == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.UseNodeUuid.GetResult() != nuvo.UseNodeUuid_OK {
		return failed(reply.UseNodeUuid.GetResult().String(),
			reply.UseNodeUuid.GetExplanation(), req, reply)
	}
	return nil
}

func buildCapabilities() *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_CAPABILITIES_REQ
	p.MsgType = &mt
	cap := new(nuvo.Capabilities)
	p.Capabilities = cap
	return p
}

// GetCapabilities gets the node capabilities
func (dsr *DoSendRecver) GetCapabilities() (*Capabilities, error) {
	req := buildCapabilities()
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, err
	}
	if *reply.MsgType != nuvo.Cmd_CAPABILITIES_REPLY {
		return nil, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.Capabilities == nil {
		return nil, failed("Api Error", "Improper reply", req, reply)
	}
	var cap Capabilities
	cap.Multifuse = reply.Capabilities.GetMultifuse()
	capabilitiesMutex.Lock()
	if nodeCapabilities == nil {
		nodeCapabilities = make(map[string]Capabilities)
	}
	nodeCapabilities[dsr.socket] = cap
	capabilitiesMutex.Unlock()
	return &cap, nil
}

func buildGetPitDiffs(uuid string, base string, incr string, offset uint64) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_GET_PIT_DIFF_REQ
	p.MsgType = &mt
	gpd := new(nuvo.GetPitDiffs)
	gpd.VolUuid = proto.String(uuid)
	gpd.BasePitUuid = proto.String(base)
	gpd.IncrPitUuid = proto.String(incr)
	gpd.Offset = proto.Uint64(offset)
	p.GetPitDiffs = gpd
	return p
}

// GetPitDiffs Get differences between two snapshots
func (dsr *DoSendRecver) GetPitDiffs(uuid string, baseUUID string, incrUUID string, offset uint64) ([]Diff, uint64, error) {
	req := buildGetPitDiffs(uuid, baseUUID, incrUUID, offset)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, 0, err
	}
	if *reply.MsgType != nuvo.Cmd_GET_PIT_DIFF_REPLY {
		return nil, 0, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.GetPitDiffs == nil {
		return nil, 0, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.GetPitDiffs.GetResult() != nuvo.GetPitDiffs_OK {
		return nil, 0, failed(reply.GetPitDiffs.GetResult().String(),
			reply.GetPitDiffs.GetExplanation(), req, reply)
	}
	diffs := reply.GetPitDiffs.GetDiffs()

	var retDiffs []Diff
	for i := 0; i < len(diffs); i++ {
		diff := Diff{Start: diffs[i].GetOffset(),
			Length: diffs[i].GetLength(),
			Dirty:  diffs[i].GetDirty()}
		retDiffs = append(retDiffs, diff)
	}
	progress := reply.GetPitDiffs.GetOffset()
	return retDiffs, progress, nil
}

func buildCreatePit(volUUID string, pitUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_CREATE_PIT_REQ
	p.MsgType = &mt
	cp := new(nuvo.CreatePit)
	cp.VolUuid = proto.String(volUUID)
	cp.PitUuid = proto.String(pitUUID)
	p.CreatePit = cp
	return p
}

// CreatePit create a PiT on a particular volume
func (dsr *DoSendRecver) CreatePit(volUUID string, pitUUID string) error {
	req := buildCreatePit(volUUID, pitUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_CREATE_PIT_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.CreatePit == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.CreatePit.GetResult() != nuvo.CreatePit_OK {
		return failed(reply.CreatePit.GetResult().String(),
			reply.CreatePit.GetExplanation(), req, reply)
	}
	return nil
}

func buildDeletePit(volUUID string, pitUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_DELETE_PIT_REQ
	p.MsgType = &mt
	dp := new(nuvo.DeletePit)
	dp.VolUuid = proto.String(volUUID)
	dp.PitUuid = proto.String(pitUUID)
	p.DeletePit = dp
	return p
}

// DeletePit Delete a PiT on a particular volume
func (dsr *DoSendRecver) DeletePit(volUUID string, pitUUID string) error {
	req := buildDeletePit(volUUID, pitUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_DELETE_PIT_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.DeletePit == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.DeletePit.GetResult() != nuvo.DeletePit_OK {
		return failed(reply.DeletePit.GetResult().String(),
			reply.DeletePit.GetExplanation(), req, reply)
	}
	return nil
}

func buildListPits(volUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_LIST_PITS_REQ
	p.MsgType = &mt
	lp := new(nuvo.ListPits)
	lp.VolUuid = proto.String(volUUID)
	p.ListPits = lp
	return p
}

// ListPits List the PiTs on a particular volume
func (dsr *DoSendRecver) ListPits(volUUID string) ([]string, error) {
	req := buildListPits(volUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, err
	}
	if *reply.MsgType != nuvo.Cmd_LIST_PITS_REPLY {
		return nil, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.ListPits == nil {
		return nil, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.ListPits.GetResult() != nuvo.ListPits_OK {
		return nil, failed(reply.ListPits.GetResult().String(),
			reply.ListPits.GetExplanation(), req, reply)
	}

	pits := reply.ListPits.GetPits()
	var uuids []string

	for _, p := range pits {
		uuids = append(uuids, *p.PitUuid)
	}

	return uuids, nil
}

func buildListVols() *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_LIST_VOLS_REQ
	p.MsgType = &mt
	lp := new(nuvo.ListVols)
	p.ListVols = lp
	return p
}

// ListVols List the Vols on this node
func (dsr *DoSendRecver) ListVols() ([]string, error) {
	req := buildListVols()
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, err
	}
	if *reply.MsgType != nuvo.Cmd_LIST_VOLS_REPLY {
		return nil, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.ListVols == nil {
		return nil, failed("Api Error", "Improper reply", req, reply)
	}
	if reply.ListVols.GetResult() != nuvo.ListVols_OK {
		return nil, failed(reply.ListVols.GetResult().String(),
			reply.ListVols.GetExplanation(), req, reply)
	}

	vols := reply.ListVols.GetVols()
	var uuids []string

	for _, p := range vols {
		uuids = append(uuids, *p.VolUuid)
	}

	return uuids, nil
}

func buildPauseIo(volUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_PAUSE_IO_REQ
	p.MsgType = &mt
	pio := new(nuvo.PauseIo)
	pio.VolUuid = proto.String(volUUID)
	p.PauseIo = pio
	return p
}

// PauseIo Pause I/O on a particular volume
func (dsr *DoSendRecver) PauseIo(volUUID string) error {
	req := buildPauseIo(volUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_PAUSE_IO_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.PauseIo == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.PauseIo.GetResult() != nuvo.PauseIo_OK {
		return failed(reply.PauseIo.GetResult().String(),
			reply.PauseIo.GetExplanation(), req, reply)
	}
	return nil
}

func buildResumeIo(volUUID string) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_RESUME_IO_REQ
	p.MsgType = &mt
	rio := new(nuvo.ResumeIo)
	rio.VolUuid = proto.String(volUUID)
	p.ResumeIo = rio
	return p
}

// ResumeIo ResumePause I/O on a particular volume
func (dsr *DoSendRecver) ResumeIo(volUUID string) error {
	req := buildResumeIo(volUUID)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_RESUME_IO_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.ResumeIo == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.ResumeIo.GetResult() != nuvo.ResumeIo_OK {
		return failed(reply.ResumeIo.GetResult().String(),
			reply.ResumeIo.GetExplanation(), req, reply)
	}
	return nil
}

func buildLogLevel(moduleName string, level uint32) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_LOG_LEVEL_REQ
	p.MsgType = &mt
	ll := new(nuvo.LogLevel)
	ll.ModuleName = proto.String(moduleName)
	ll.Level = proto.Uint32(level)
	p.LogLevel = ll
	return p
}

// LogLevel Set the log level on a module
func (dsr *DoSendRecver) LogLevel(moduleName string, level uint32) error {
	req := buildLogLevel(moduleName, level)
	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_LOG_LEVEL_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.LogLevel == nil {
		return failed("Api Error", "Improper reply", req, reply)
	}
	if reply.LogLevel.GetResult() != nuvo.LogLevel_OK {
		return failed(reply.LogLevel.GetResult().String(),
			reply.LogLevel.GetExplanation(), req, reply)
	}
	return nil
}

func buildNodeStatus() *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_NODE_STATUS_REQ
	p.MsgType = &mt
	vs := new(nuvo.NodeStatus)
	p.NodeStatus = vs
	return p
}

// NodeStatus gets status for node
func (dsr *DoSendRecver) NodeStatus() (NodeStatusReport, error) {
	req := buildNodeStatus()
	var node NodeStatusReport

	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return node, err
	}
	if *reply.MsgType != nuvo.Cmd_NODE_STATUS_REPLY {
		return node, failed("Api Error", "Wrong message reply type", req, reply)
	}
	if reply.NodeStatus == nil {
		return node, failed("Api Error", "Improper reply", req, reply)
	}

	node.NodeUUID = reply.NodeStatus.GetNodeUuid()
	node.GitBuildHash = reply.NodeStatus.GetGitBuildHash()
	protoVols := reply.NodeStatus.GetVolumes()
	node.Volumes = make([]VolStatus, len(protoVols))
	for i, protoVol := range protoVols {
		node.Volumes[i].VolUUID = protoVol.GetVolUuid()
		volClasses := protoVol.GetDataClassSpace()
		node.Volumes[i].ClassSpace = make([]VolClassSpace, len(volClasses))
		for j, volClass := range volClasses {
			node.Volumes[i].ClassSpace[j].Class = volClass.GetClass()
			node.Volumes[i].ClassSpace[j].BlocksUsed = volClass.GetBlocksUsed()
			node.Volumes[i].ClassSpace[j].BlocksAllocated = volClass.GetBlocksAllocated()
			node.Volumes[i].ClassSpace[j].BlocksTotal = volClass.GetBlocksTotal()
		}
	}
	return node, nil
}

func buildDebugTrigger() *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_DEBUG_TRIGGER_REQ
	p.MsgType = &mt
	dt := new(nuvo.DebugTrigger)
	p.DebugTrigger = dt
	return p
}

// DebugTrigger gets status for node
func (dsr *DoSendRecver) DebugTrigger(params DebugTriggerParams) error {
	req := buildDebugTrigger()
	req.DebugTrigger.Trigger = proto.String(params.Trigger)
	req.DebugTrigger.NodeUuid = proto.String(params.NodeUUID)
	req.DebugTrigger.VolUuid = proto.String(params.VolUUID)
	req.DebugTrigger.DevUuid = proto.String(params.DevUUID)
	req.DebugTrigger.ParcelIndex = proto.Uint32(params.ParcelIndex)
	req.DebugTrigger.SegmentIndex = proto.Uint32(params.SegmentIndex)
	req.DebugTrigger.InjectErrorType = proto.Uint32(params.InjectErrorType)
	req.DebugTrigger.InjectReturnCode = proto.Int32(params.InjectReturnCode)
	req.DebugTrigger.InjectRepeatCnt = proto.Int32(params.InjectRepeatCnt)
	req.DebugTrigger.InjectSkipCnt = proto.Int32(params.InjectSkipCnt)
	req.DebugTrigger.Multiuse1 = proto.Uint64(params.Multiuse1)
	req.DebugTrigger.Multiuse2 = proto.Uint64(params.Multiuse2)
	req.DebugTrigger.Multiuse3 = proto.Uint64(params.Multiuse3)

	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return err
	}
	if *reply.MsgType != nuvo.Cmd_DEBUG_TRIGGER_REPLY {
		return failed("Api Error", "Wrong message reply type", req, reply)
	}
	return nil
}

func buildLogSummary(volUUID string, parcelIndex uint32, segmentIndex uint32) *nuvo.Cmd {
	p := new(nuvo.Cmd)
	mt := nuvo.Cmd_LOG_SUMMARY_REQ
	p.MsgType = &mt
	ls := new(nuvo.LogSummary)
	ls.VolUuid = proto.String(volUUID)
	ls.ParcelIndex = proto.Uint32(parcelIndex)
	ls.SegmentIndex = proto.Uint32(segmentIndex)
	p.LogSummary = ls
	return p
}

// LogSummary gets the log summary for a segment
func (dsr *DoSendRecver) LogSummary(volUUID string, parcelIndex uint32, segmentIndex uint32) (*LogSummaryResults, error) {
	req := buildLogSummary(volUUID, parcelIndex, segmentIndex)

	reply, err := dsr.doSendRecv(dsr.socket, req)
	if err != nil {
		return nil, err
	}
	if *reply.MsgType != nuvo.Cmd_LOG_SUMMARY_REPLY {
		return nil, failed("Api Error", "Wrong message reply type", req, reply)
	}

	var result LogSummaryResults
	result.VolUUID = reply.LogSummary.GetVsUuid()
	result.ParcelIndex = reply.LogSummary.GetParcelIndex()
	result.SegmentIndex = reply.LogSummary.GetSegmentIndex()
	result.Magic = reply.LogSummary.GetMagic()
	result.SequenceNumber = reply.LogSummary.GetSequenceNo()
	result.ClosingSequenceNumber = reply.LogSummary.GetClosingSequenceNo()

	protoEntries := reply.LogSummary.GetEntries()
	result.Entries = make([]LogEntry, len(protoEntries))
	for i, protoEntry := range protoEntries {
		result.Entries[i].EntryType = protoEntry.GetLogEntryType()
		result.Entries[i].BlockHash = protoEntry.GetBlockHash()
		switch {
		case result.Entries[i].EntryType >= 2 && result.Entries[i].EntryType <= 7:
			// Data or map
			result.Entries[i].PitInfoActive = protoEntry.GetData().GetPitInfoActive()
			result.Entries[i].PitInfoID = protoEntry.GetData().GetPitInfoId()
			result.Entries[i].BlockNumber = protoEntry.GetData().GetBno()
			result.Entries[i].ParcelIndex = protoEntry.GetData().GetGcParcelIndex()
			result.Entries[i].BlockOffset = protoEntry.GetData().GetGcBlockOffset()
		case result.Entries[i].EntryType == 8:
			// Fork
			result.Entries[i].ParcelIndex = protoEntry.GetFork().GetSegParcelIndex()
			result.Entries[i].BlockOffset = protoEntry.GetFork().GetSegBlockOffset()
			result.Entries[i].SequenceNumber = protoEntry.GetFork().GetSequenceNo()
			result.Entries[i].SubClass = protoEntry.GetFork().GetSegSubclass()
		case result.Entries[i].EntryType == 9:
			// Header
			result.Entries[i].SequenceNumber = protoEntry.GetHeader().GetSequenceNo()
			result.Entries[i].SubClass = protoEntry.GetHeader().GetSubclass()
		case result.Entries[i].EntryType == 10:
			// Descriptor
			result.Entries[i].CVFlag = protoEntry.GetDescriptor_().GetCvFlag()
			result.Entries[i].EntryCount = protoEntry.GetDescriptor_().GetEntryCount()
			result.Entries[i].DataBlockCount = protoEntry.GetDescriptor_().GetDataBlockCount()
			result.Entries[i].SequenceNumber = protoEntry.GetDescriptor_().GetSequenceNo()
		case result.Entries[i].EntryType == 11:
			// Snap
			result.Entries[i].SequenceNumber = protoEntry.GetSnap().GetSequenceNo()
			result.Entries[i].SnapOperation = protoEntry.GetSnap().GetOperation()
		default:
		}
	}

	return &result, nil
}
