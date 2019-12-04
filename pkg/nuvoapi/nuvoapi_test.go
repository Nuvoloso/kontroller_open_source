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

package nuvoapi

import (
	"errors"
	"fmt"
	"log"
	"net"
	"path"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	nuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/nuvo_pb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

type dsrsTest struct {
	dsr     DoSendRecv
	works   bool // Should succeed
	nuvoErr bool // Error should be nuvoErr
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func TestError(t *testing.T) {
	assert := assert.New(t)

	var e apiError
	e.What = "boo"
	e.Explanation = "hoo"
	if e.Error() != "boo: hoo" {
		t.Error("nuvoapi.Error string was: ", e.Error())
	}
	if e.LongError() != "boo: hoo, , " {
		t.Error("nuvoapi.Error string was: ", e.LongError())
	}
	e.ReqJSON = "too"
	if e.LongError() != "boo: hoo, too, " {
		t.Error("nuvoapi.Error string was: ", e.LongError())
	}
	e.ReplyJSON = "soon"
	if e.LongError() != "boo: hoo, too, soon" {
		t.Error("nuvoapi.Error string was: ", e.LongError())
	}

	errMsg := "other-package-error"
	err := fmt.Errorf("%s", errMsg)

	we := WrapError(err)
	assert.NotNil(we)
	ei, ok := we.(ErrorInt)
	assert.True(ok)
	assert.False(ei.NotInitialized())
	assert.Equal(err, ei.Raw())
	assert.Equal(errMsg, ei.Error())
	assert.Regexp(errMsg, ei.LongError())

	we = WrapFatalError(err)
	assert.NotNil(we)
	ei, ok = we.(ErrorInt)
	assert.True(ok)
	assert.True(ei.NotInitialized())
	assert.Equal(err, ei.Raw())
	assert.Equal(errMsg, ei.Error())
	assert.Regexp(errMsg, ei.LongError())

	ae := apiError{What: "BAD_ORDER", Explanation: "not initialized"}
	ei = ae
	assert.True(ei.NotInitialized())
	assert.Nil(ei.Raw())
	assert.Equal("BAD_ORDER: not initialized", ei.Error())
	assert.Equal("BAD_ORDER: not initialized, , ", ei.LongError())

	err = failed("terse", "not-so-terse", nil, nil)
	ei, ok = err.(ErrorInt)
	assert.True(ok)
	assert.False(ei.NotInitialized())
	assert.Nil(ei.Raw())
	assert.Equal("terse: not-so-terse", ei.Error())
	assert.Equal("terse: not-so-terse, , ", ei.LongError())

	err = failed("BAD_ORDER", "Not allowed before use node UUID command", nil, nil)
	ei, ok = err.(ErrorInt)
	assert.True(ok)
	assert.True(ei.NotInitialized())
	assert.Nil(ei.Raw())
	assert.Equal("BAD_ORDER: Not allowed before use node UUID command", ei.Error())
	assert.Equal("BAD_ORDER: Not allowed before use node UUID command, , ", ei.LongError())

	aErr := NewNuvoAPIError("tempErr", true)
	ei, ok = aErr.(ErrorInt)
	assert.True(ok)
	assert.NotNil(ei.Raw())
	ret := ErrorIsTemporary(aErr)
	assert.True(ret)

	aErr = NewNuvoAPIError("tempErr", false)
	ei, ok = aErr.(ErrorInt)
	assert.True(ok)
	assert.NotNil(ei.Raw())
	ret = ErrorIsTemporary(aErr)
	assert.False(ret)

	err = fmt.Errorf("some non temporary error")
	aErr = WrapError(err)
	ei, ok = aErr.(ErrorInt)
	assert.True(ok)
	assert.NotNil(ei.Raw())
	ret = ErrorIsTemporary(aErr)
	assert.False(ret)
}

func TestBuildUseDevice(t *testing.T) {
	uuid := "my uuid"
	path := "my_path"
	devType := nuvo.UseDevice_SSD
	req := buildUseDevice(uuid, path, devType)
	if req.GetMsgType() != nuvo.Cmd_USE_DEVICE_REQ {
		t.Error("msg type was: ", req.MsgType)
	}
	if len(req.UseDevice) != 1 {
		t.Error("number of use devices was: ", len(req.UseDevice))
	}
	ud := req.UseDevice[0]
	if ud.GetPath() != path {
		t.Error("path was :", ud.GetPath())
	}
	if ud.GetUuid() != uuid {
		t.Error("path was :", ud.GetUuid())
	}
	if ud.GetDevType() != nuvo.UseDevice_SSD {
		t.Error("devType was :", ud.GetDevType())
	}
	if ud.Explanation != nil {
		t.Error("had explanation :", ud.GetExplanation())
	}
	if ud.Result != nil {
		t.Error("had result :", ud.GetResult())
	}
}

func makeUseDeviceReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_USE_DEVICE_REPLY.Enum()
	reply.UseDevice[0].Result = nuvo.UseDevice_OK.Enum()
	return reply
}

func buildReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	return nil, errors.New("EOF")
}

func buildUDReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	return reply, nil
}

func buildUDReplyWrongTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	reply.MsgType = nuvo.Cmd_USE_DEVICE_REQ.Enum()
	return reply, nil
}

func buildUDReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	reply.UseDevice = reply.UseDevice[0:0]
	return reply, nil
}

func buildUDReplyNotOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	reply.UseDevice[0].Result = nuvo.UseDevice_INVALID.Enum()
	return reply, nil
}

func TestUseDevice(t *testing.T) {
	var tests = []dsrsTest{
		{buildUDReplyOkSendRecv, true, false},
		{buildUDReplyWrongTypeSendRecv, false, true},
		{buildUDReplyNoMsgSendRecv, false, true},
		{buildUDReplyNotOKSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		uuid := "my uuid"
		path := "my_path"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.UseDevice(uuid, path)
		if test.works {
			if err != nil {
				t.Error("UseDevice improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("UseDevice improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}

		for devtype := range useDeviceTypeChoices {
			uuid := "my uuid"
			path := "my_path"
			dsr := NewSendRecv(test.dsr, "sock")
			err := dsr.UseDevice(uuid, path, devtype)
			if test.works {
				if err != nil {
					t.Error("UseDevice improperly failed", test, err)
				}
			} else {
				if err == nil {
					t.Error("UseDevice improperly succeeded", test, err)
				}
				nerr, ok := err.(ErrorInt)
				if ok != test.nuvoErr {
					t.Error("wrong error type", getFunctionName(test.dsr), err)
				}
				if nerr != nil {
					nerr.LongError()
				}
			}
		}
	}

	uuid := "my uuid"
	path := "my_path"
	devtype := "AAA"
	dsr := NewSendRecv(buildUDReplyOkSendRecv, "sock")
	err := dsr.UseDevice(uuid, path, devtype)
	if err == nil {
		t.Error("UseDevice improperly succeeded", err)
	}
}

func buildUCDReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	return reply, nil
}

func buildUCDReplyWrongTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	reply.MsgType = nuvo.Cmd_USE_DEVICE_REQ.Enum()
	return reply, nil
}

func buildUCDReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	reply.UseDevice = reply.UseDevice[0:0]
	return reply, nil
}

func buildUCDReplyNotOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUseDeviceReply(req)
	reply.UseDevice[0].Result = nuvo.UseDevice_INVALID.Enum()
	return reply, nil
}

func TestUseCacheDevice(t *testing.T) {
	var tests = []dsrsTest{
		{buildUCDReplyOkSendRecv, true, false},
		{buildUCDReplyWrongTypeSendRecv, false, true},
		{buildUCDReplyNoMsgSendRecv, false, true},
		{buildUCDReplyNotOKSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		uuid := "my uuid"
		path := "my_path"
		dsr := NewSendRecv(test.dsr, "sock")
		_, _, err := dsr.UseCacheDevice(uuid, path)
		if test.works {
			if err != nil {
				t.Error("UseCacheDevice improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("UseCacheDevice improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func makeCloseDeviceReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_CLOSE_DEVICE_REPLY.Enum()
	reply.CloseDevice[0].Result = nuvo.CloseDevice_OK.Enum()
	return reply
}

func buildCDReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseDeviceReply(req)
	return reply, nil
}

func buildCDReplyWrongTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseDeviceReply(req)
	reply.MsgType = nuvo.Cmd_CLOSE_DEVICE_REQ.Enum()
	return reply, nil
}

func buildCDReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseDeviceReply(req)
	reply.CloseDevice = reply.CloseDevice[0:0]
	return reply, nil
}

func buildCDReplyNotOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseDeviceReply(req)
	reply.CloseDevice[0].Result = nuvo.CloseDevice_INVALID.Enum()
	return reply, nil
}

func TestCloseDevice(t *testing.T) {
	var tests = []dsrsTest{
		{buildCDReplyOkSendRecv, true, false},
		{buildCDReplyWrongTypeSendRecv, false, true},
		{buildCDReplyNoMsgSendRecv, false, true},
		{buildCDReplyNotOKSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		uuid := "my uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.CloseDevice(uuid)
		if test.works {
			if err != nil {
				t.Error("CloseDevice improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("CloseDevice improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildCloseDevice(t *testing.T) {
	uuid := "my uuid"
	cmd := buildCloseDevice(uuid)
	if cmd.GetMsgType() != nuvo.Cmd_CLOSE_DEVICE_REQ {
		t.Error("cmd type was: ", cmd.MsgType)
	}
	if len(cmd.CloseDevice) != 1 {
		t.Error("number of use devices was: ", len(cmd.CloseDevice))
	}
	fmt := cmd.CloseDevice[0]
	if fmt.GetUuid() != uuid {
		t.Error("uuid was :", fmt.GetUuid())
	}
	if fmt.Explanation != nil {
		t.Error("had explanation :", fmt.GetExplanation())
	}
	if fmt.Result != nil {
		t.Error("had result :", fmt.GetResult())
	}
}

func TestBuildFormatDevice(t *testing.T) {
	uuid := "my uuid"
	path := "my_path"
	ParcelSize := uint64(12345)
	cmd := buildFormatDevice(uuid, path, ParcelSize)
	if cmd.GetMsgType() != nuvo.Cmd_FORMAT_DEVICE_REQ {
		t.Error("cmd type was: ", cmd.MsgType)
	}
	if len(cmd.FormatDevice) != 1 {
		t.Error("number of use devices was: ", len(cmd.FormatDevice))
	}
	fmt := cmd.FormatDevice[0]
	if fmt.GetPath() != path {
		t.Error("path was :", fmt.GetPath())
	}
	if fmt.GetUuid() != uuid {
		t.Error("uuid was :", fmt.GetUuid())
	}
	if fmt.Explanation != nil {
		t.Error("had explanation :", fmt.GetExplanation())
	}
	if fmt.Result != nil {
		t.Error("had result :", fmt.GetResult())
	}
	if fmt.GetParcelSize() != ParcelSize {
		t.Error("parcel size was :", fmt.GetParcelSize())
	}
}

func makeFormatDeviceReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_FORMAT_DEVICE_REPLY.Enum()
	reply.FormatDevice[0].Result = nuvo.FormatDevice_OK.Enum()
	return reply
}

func buildFDReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeFormatDeviceReply(req)
	return reply, nil
}

func buildFDReplyWrongTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeFormatDeviceReply(req)
	reply.MsgType = nuvo.Cmd_FORMAT_DEVICE_REQ.Enum()
	return reply, nil
}

func buildFDReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeFormatDeviceReply(req)
	reply.FormatDevice = reply.FormatDevice[0:0]
	return reply, nil
}

func buildFDReplyNotOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeFormatDeviceReply(req)
	reply.FormatDevice[0].Result = nuvo.FormatDevice_INVALID.Enum()
	return reply, nil
}

func TestFormatDevice(t *testing.T) {
	var tests = []dsrsTest{
		{buildFDReplyOkSendRecv, true, false},
		{buildFDReplyWrongTypeSendRecv, false, true},
		{buildFDReplyNoMsgSendRecv, false, true},
		{buildFDReplyNotOKSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		uuid := "my uuid"
		path := "my_path"
		ParcelSize := uint64(123456)
		dsr := NewSendRecv(test.dsr, "sock")
		_, err := dsr.FormatDevice(uuid, path, ParcelSize)
		if test.works {
			if err != nil {
				t.Error("FormatDevice improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("FormatDevice improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildDeviceLocation(t *testing.T) {
	deviceUUID := "I'm a device"
	nodeUUID := "I'm a node"
	req := buildDeviceLocation(deviceUUID, nodeUUID)
	if req.GetMsgType() != nuvo.Cmd_DEVICE_LOCATION_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	if len(req.DeviceLocation) != 1 {
		t.Error("number of device locations: ", len(req.DeviceLocation))
	}
	dl := req.DeviceLocation[0]
	if dl.GetNode() != nodeUUID {
		t.Error("Node uuid was:", dl.GetNode())
	}
	if dl.GetDevice() != deviceUUID {
		t.Error("device uuid was:", dl.GetDevice())
	}
	if dl.Explanation != nil {
		t.Error("had explanation :", dl.GetExplanation())
	}
	if dl.Result != nil {
		t.Error("had result :", dl.GetResult())
	}
}

func makeDeviceLocationReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_DEVICE_LOCATION_REPLY.Enum()
	reply.DeviceLocation[0].Result = nuvo.DeviceLocation_OK.Enum()
	return reply
}

func buildDLReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeviceLocationReply(req)
	return reply, nil
}

func buildDLReplyWrongTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeviceLocationReply(req)
	reply.MsgType = nuvo.Cmd_DEVICE_LOCATION_REQ.Enum()
	return reply, nil
}

func buildDLReplyNoMsgTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeviceLocationReply(req)
	reply.DeviceLocation = reply.DeviceLocation[0:0]
	return reply, nil
}

func buildDLReplyNotOKTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeviceLocationReply(req)
	reply.DeviceLocation[0].Result = nuvo.DeviceLocation_INVALID.Enum()
	return reply, nil
}

func TestDeviceLocation(t *testing.T) {
	var tests = []dsrsTest{
		{buildDLReplyOKSendRecv, true, false},
		{buildReplyErrorSendRecv, false, false},
		{buildDLReplyWrongTypeSendRecv, false, true},
		{buildDLReplyNoMsgTypeSendRecv, false, true},
		{buildDLReplyNotOKTypeSendRecv, false, true},
	}
	for _, test := range tests {
		nodeUUID := "hither"
		deviceUUID := "yon"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.DeviceLocation(nodeUUID, deviceUUID)
		if test.works {
			if err != nil {
				t.Error("DeviceLocation improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("DeviceLocation improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildNodeLocation(t *testing.T) {
	nodeUUID := "I'm a node"
	ipAddr := "I'm an address"
	port := uint16(7534)
	req := buildNodeLocation(nodeUUID, ipAddr, port)
	if req.GetMsgType() != nuvo.Cmd_NODE_LOCATION_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	if len(req.NodeLocation) != 1 {
		t.Error("number of node locations: ", len(req.NodeLocation))
	}
	nl := req.NodeLocation[0]
	if nl.GetUuid() != nodeUUID {
		t.Error("Node uuid was:", nl.GetUuid())
	}
	if nl.GetIpv4Addr() != ipAddr {
		t.Error("device uuid was:", nl.GetIpv4Addr())
	}
	if nl.GetPort() != uint32(port) {
		t.Error("device uuid was:", nl.GetPort())
	}
	if nl.Explanation != nil {
		t.Error("had explanation :", nl.GetExplanation())
	}
	if nl.Result != nil {
		t.Error("had result :", nl.GetResult())
	}
}

func makeNodeLocationReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_NODE_LOCATION_REPLY.Enum()
	reply.NodeLocation[0].Result = nuvo.NodeLocation_OK.Enum()
	return reply
}

func buildNLReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeLocationReply(req)
	return reply, nil
}

func TestNodeLocationWorks(t *testing.T) {
	node := "Feist"
	ipAddr := "1.2.3.4"
	port := uint16(5690)
	dsr := NewSendRecv(buildNLReplyOKSendRecv, "sock")
	err := dsr.NodeLocation(node, ipAddr, port)
	if err != nil {
		t.Error("build didn't work", err)
	}
}

func buildNLReplyWrongTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeLocationReply(req)
	reply.MsgType = nuvo.Cmd_NODE_LOCATION_REQ.Enum()
	return reply, nil
}

func buildNLReplyNoMsgTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeLocationReply(req)
	reply.NodeLocation = reply.NodeLocation[0:0]
	return reply, nil
}

func buildNLReplyNotOKTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeLocationReply(req)
	reply.NodeLocation[0].Result = nuvo.NodeLocation_INVALID.Enum()
	return reply, nil
}

func TestNodeLocation(t *testing.T) {
	var tests = []dsrsTest{
		{buildNLReplyOKSendRecv, true, false},
		{buildReplyErrorSendRecv, false, false},
		{buildNLReplyWrongTypeSendRecv, false, true},
		{buildNLReplyNoMsgTypeSendRecv, false, true},
		{buildNLReplyNotOKTypeSendRecv, false, true},
	}
	for _, test := range tests {
		node := "Feist"
		ipAddr := "1.2.3.4"
		port := uint16(5690)
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.NodeLocation(node, ipAddr, port)
		if test.works {
			if err != nil {
				t.Error("NodeLocation improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("NodeLocation improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildNodeInitDone(t *testing.T) {
	nodeUUID := "Old McDoNode's Farm"
	clear := false
	req := buildNodeInitDone(nodeUUID, clear)
	if req.GetMsgType() != nuvo.Cmd_NODE_INIT_DONE_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	nid := req.NodeInitDone
	if nid.GetUuid() != nodeUUID {
		t.Error("Node uuid was:", nid.GetUuid())
	}
	if nid.GetClear() != clear {
		t.Error("clear was:", nid.GetClear())
	}
	if nid.Explanation != nil {
		t.Error("had explanation :", nid.GetExplanation())
	}
	if nid.Result != nil {
		t.Error("had result :", nid.GetResult())
	}
}

func makeNodeInitDoneReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_NODE_INIT_DONE_REPLY.Enum()
	reply.NodeInitDone.Result = nuvo.NodeInitDone_OK.Enum()
	return reply
}

func buildNIDReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeInitDoneReply(req)
	return reply, nil
}

func buildNIDReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeInitDoneReply(req)
	reply.MsgType = nuvo.Cmd_NODE_INIT_DONE_REQ.Enum()
	return reply, nil
}

func buildNIDReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeInitDoneReply(req)
	reply.NodeInitDone = nil
	return reply, nil
}

func buildNIDReplyNotOKTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeInitDoneReply(req)
	reply.NodeInitDone.Result = nuvo.NodeInitDone_INVALID.Enum()
	return reply, nil
}

func TestNodeInitDone(t *testing.T) {
	var tests = []dsrsTest{
		{buildNIDReplyOKSendRecv, true, false},
		{buildNIDReplyWrongReplySendRecv, false, true},
		{buildNIDReplyNoMsgSendRecv, false, true},
		{buildNIDReplyNotOKTypeSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		node := "Duck"
		clear := false
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.NodeInitDone(node, clear)
		if test.works {
			if err != nil {
				t.Error("NodeInitDone improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("NodeInitDone improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildPassThroughVolume(t *testing.T) {
	uuid := "My name is Inigo Montoya"
	path := "/dev/u/killed/my/father"
	deviceSize := uint64(666)
	req := buildOpenPassThroughVolume(uuid, path, deviceSize)
	if req.GetMsgType() != nuvo.Cmd_OPEN_PASSTHROUGH_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	spt := req.OpenPassThroughVol
	if spt == nil {
		t.Error("No OpenPassThroughVol.")
	}
	if spt.GetUuid() != uuid {
		t.Error("UUID was:", spt.GetUuid())
	}
	if spt.GetPath() != path {
		t.Error("device path was:", spt.GetPath())
	}
	if spt.GetSize() != deviceSize {
		t.Error("device size was:", spt.GetSize())
	}
	if spt.Explanation != nil {
		t.Error("had explanation :", spt.GetExplanation())
	}
	if spt.Result != nil {
		t.Error("had result :", spt.GetResult())
	}
}

func makeStartPassthroughReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_OPEN_PASSTHROUGH_REPLY.Enum()
	reply.OpenPassThroughVol.Result = nuvo.OpenPassThroughVolume_OK.Enum()
	return reply
}

func buildSPTReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeStartPassthroughReply(req)
	return reply, nil
}

func buildSPTReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeStartPassthroughReply(req)
	reply.MsgType = nuvo.Cmd_OPEN_PASSTHROUGH_REQ.Enum()
	return reply, nil
}

func buildSPTReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeStartPassthroughReply(req)
	reply.OpenPassThroughVol = nil
	return reply, nil
}

func buildSPTReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeStartPassthroughReply(req)
	reply.OpenPassThroughVol.Result = nuvo.OpenPassThroughVolume_INVALID.Enum()
	return reply, nil
}

func TestStartPassthroughVol(t *testing.T) {
	var tests = []dsrsTest{
		{buildSPTReplyOKSendRecv, true, false},
		{buildSPTReplyWrongReplySendRecv, false, true},
		{buildSPTReplyNoMsgSendRecv, false, true},
		{buildSPTReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		uuid := "Hell"
		path := "Paved with good intentions"
		deviceSize := uint64(666)
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.OpenPassThroughVolume(uuid, path, deviceSize)

		if test.works {
			if err != nil {
				t.Error("StartPassThroughVol improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("StartPassThroughVol improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildExport(t *testing.T) {
	volSeriesUUID := "My patronus is a UUID"
	pitUUID := "PiT UUID"
	exportName := "Kubiatowicz"
	writable := false
	req := buildExportLun(volSeriesUUID, pitUUID, exportName, writable)
	if req.GetMsgType() != nuvo.Cmd_EXPORT_LUN_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	spt := req.ExportLun
	if spt == nil {
		t.Error("No ExportLun.")
	}
	if spt.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("UUID was:", spt.GetVolSeriesUuid())
	}
	if spt.GetExportName() != exportName {
		t.Error("device path was:", spt.GetExportName())
	}
	if spt.GetWritable() != writable {
		t.Error("device size was:", spt.GetWritable())
	}
	if spt.Explanation != nil {
		t.Error("had explanation :", spt.GetExplanation())
	}
	if spt.Result != nil {
		t.Error("had result :", spt.GetResult())
	}
}

func makeExportLunReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_EXPORT_LUN_REPLY.Enum()
	reply.ExportLun.Result = nuvo.ExportLun_OK.Enum()
	return reply
}

func buildExportLunReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeExportLunReply(req)
	return reply, nil
}

func buildExportLunReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeExportLunReply(req)
	reply.MsgType = nuvo.Cmd_EXPORT_LUN_REQ.Enum()
	return reply, nil
}

func buildExportLunReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeExportLunReply(req)
	reply.ExportLun = nil
	return reply, nil
}

func buildExportLunReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeExportLunReply(req)
	reply.ExportLun.Result = nuvo.ExportLun_INVALID.Enum()
	return reply, nil
}

func TestExport(t *testing.T) {
	var tests = []dsrsTest{
		{buildExportLunReplyOKSendRecv, true, false},
		{buildExportLunReplyWrongReplySendRecv, false, true},
		{buildExportLunReplyNoMsgSendRecv, false, true},
		{buildExportLunReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "John Kubiatowicz"
		pitUUID := "PiT UUID"
		exportPath := "Kubi"
		writable := true
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.ExportLun(volSeriesUUID, pitUUID, exportPath, writable)

		if test.works {
			if err != nil {
				t.Error("ExportLun improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("ExportLun improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildUnExport(t *testing.T) {
	volSeriesUUID := "My patronus is a UUID"
	pitUUID := "pit UUID"
	exportName := "Kubiatowicz"
	req := buildUnexportLun(volSeriesUUID, pitUUID, exportName)
	if req.GetMsgType() != nuvo.Cmd_UNEXPORT_LUN_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	spt := req.UnexportLun
	if spt == nil {
		t.Error("No UnexportLun.")
	}
	if spt.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("UUID was:", spt.GetVolSeriesUuid())
	}
	if spt.GetExportName() != exportName {
		t.Error("device path was:", spt.GetExportName())
	}
	if spt.Explanation != nil {
		t.Error("had explanation :", spt.GetExplanation())
	}
	if spt.Result != nil {
		t.Error("had result :", spt.GetResult())
	}
}

func makeUnexportLunReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_UNEXPORT_LUN_REPLY.Enum()
	reply.UnexportLun.Result = nuvo.UnexportLun_OK.Enum()
	return reply
}

func buildUnexportLunReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUnexportLunReply(req)
	return reply, nil
}

func buildUnexportLunReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUnexportLunReply(req)
	reply.MsgType = nuvo.Cmd_EXPORT_LUN_REQ.Enum()
	return reply, nil
}

func buildUnexportLunReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUnexportLunReply(req)
	reply.UnexportLun = nil
	return reply, nil
}

func buildUnexportLunReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeUnexportLunReply(req)
	reply.UnexportLun.Result = nuvo.UnexportLun_INVALID.Enum()
	return reply, nil
}

func TestUnexport(t *testing.T) {
	var tests = []dsrsTest{
		{buildUnexportLunReplyOKSendRecv, true, false},
		{buildUnexportLunReplyWrongReplySendRecv, false, true},
		{buildUnexportLunReplyNoMsgSendRecv, false, true},
		{buildUnexportLunReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "John Kubiatowicz"
		pitUUID := "George Carlin"
		exportPath := "Kubi"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.UnexportLun(volSeriesUUID, pitUUID, exportPath)

		if test.works {
			if err != nil {
				t.Error("UnexportLun improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("UnexportLun improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildCreateParcelVol(t *testing.T) {
	volSeriesUUID := "PLease sir, might I have a volume."
	deviceUUID := "Here, if it's not too much trouble."
	parcelUUID := "Use this UUID, please."
	req := buildCreateParcelVol(volSeriesUUID, deviceUUID, parcelUUID)
	if req.GetMsgType() != nuvo.Cmd_CREATE_VOLUME_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	cpv := req.CreateVolume
	if cpv == nil {
		t.Error("No CreateVolume")
	}
	if cpv.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("Vol Series UUID was:", cpv.GetVolSeriesUuid())
	}
	if cpv.GetRootDeviceUuid() != deviceUUID {
		t.Error("Device UUID was:", cpv.GetRootDeviceUuid())
	}
	if cpv.GetRootParcelUuid() != parcelUUID {
		t.Error("Root Parcel UUID:", cpv.GetRootParcelUuid())
	}
	if cpv.Explanation != nil {
		t.Error("had explanation :", cpv.GetExplanation())
	}
	if cpv.Result != nil {
		t.Error("had result :", cpv.GetResult())
	}
}

func makeCreateParcelVolReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_CREATE_VOLUME_REPLY.Enum()
	reply.CreateVolume.Result = nuvo.CreateVolume_OK.Enum()
	return reply
}

func buildCreateParcelVolReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateParcelVolReply(req)
	return reply, nil
}

func buildCreateParcelVolReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateParcelVolReply(req)
	reply.MsgType = nuvo.Cmd_CREATE_VOLUME_REQ.Enum()
	return reply, nil
}

func buildCreateParcelVolReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateParcelVolReply(req)
	reply.CreateVolume = nil
	return reply, nil
}

func buildCreateParcelVolReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateParcelVolReply(req)
	reply.CreateVolume.Result = nuvo.CreateVolume_INVALID.Enum()
	return reply, nil
}

func buildCreateParcelVolReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateParcelVolReply(req)
	reply.CreateVolume.Result = nuvo.CreateVolume_ERROR.Enum()
	reply.CreateVolume.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestCreateParcelVol(t *testing.T) {
	var tests = []dsrsTest{
		{buildCreateParcelVolReplyOKSendRecv, true, false},
		{buildCreateParcelVolReplyWrongReplySendRecv, false, true},
		{buildCreateParcelVolReplyNoMsgSendRecv, false, true},
		{buildCreateParcelVolReplyInvalidSendRecv, false, true},
		{buildCreateParcelVolReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "Cliff Clavin"
		deviceUUID := "Cheers"
		parcelUUID := "It's a little known fact..."
		dsr := NewSendRecv(test.dsr, "sock")
		_, err := dsr.CreateParcelVol(volSeriesUUID, deviceUUID, parcelUUID)
		if test.works {
			if err != nil {
				t.Error("CreateParcelVol improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("CreateParcelVol improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildCreateLogVol(t *testing.T) {
	volSeriesUUID := "PLease sir, might I have a volume."
	deviceUUID := "Here, if it's not too much trouble."
	parcelUUID := "Use this UUID, please."
	size := uint64(1024 * 1024 * 1024)
	req := buildCreateLogVol(volSeriesUUID, deviceUUID, parcelUUID, size)
	if req.GetMsgType() != nuvo.Cmd_CREATE_VOLUME_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	clv := req.CreateVolume
	if clv == nil {
		t.Error("No CreateVolume")
	}
	if clv.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("Vol Series UUID was:", clv.GetVolSeriesUuid())
	}
	if clv.GetRootDeviceUuid() != deviceUUID {
		t.Error("Device UUID was:", clv.GetRootDeviceUuid())
	}
	if clv.GetRootParcelUuid() != parcelUUID {
		t.Error("Root Parcel UUID:", clv.GetRootParcelUuid())
	}
	if clv.GetSize() != size {
		t.Error("Size was:", clv.GetSize())
	}
	if clv.Explanation != nil {
		t.Error("had explanation :", clv.GetExplanation())
	}
	if clv.Result != nil {
		t.Error("had result :", clv.GetResult())
	}
}

func makeCreateLogVolReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_CREATE_VOLUME_REPLY.Enum()
	reply.CreateVolume.Result = nuvo.CreateVolume_OK.Enum()
	return reply
}

func buildCreateLogVolReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateLogVolReply(req)
	return reply, nil
}

func buildCreateLogVolReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateLogVolReply(req)
	reply.MsgType = nuvo.Cmd_CREATE_VOLUME_REQ.Enum()
	return reply, nil
}

func buildCreateLogVolReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateLogVolReply(req)
	reply.CreateVolume = nil
	return reply, nil
}

func buildCreateLogVolReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateLogVolReply(req)
	reply.CreateVolume.Result = nuvo.CreateVolume_INVALID.Enum()
	return reply, nil
}

func buildCreateLogVolReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreateLogVolReply(req)
	reply.CreateVolume.Result = nuvo.CreateVolume_ERROR.Enum()
	reply.CreateVolume.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestCreateLogVol(t *testing.T) {
	var tests = []dsrsTest{
		{buildCreateLogVolReplyOKSendRecv, true, false},
		{buildCreateLogVolReplyWrongReplySendRecv, false, true},
		{buildCreateLogVolReplyNoMsgSendRecv, false, true},
		{buildCreateLogVolReplyInvalidSendRecv, false, true},
		{buildCreateLogVolReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "Tests are boring"
		deviceUUID := "Very boring"
		parcelUUID := "Oh so boring"
		size := uint64(1024 * 1024)
		dsr := NewSendRecv(test.dsr, "sock")
		_, err := dsr.CreateLogVol(volSeriesUUID, deviceUUID, parcelUUID, size)
		if test.works {
			if err != nil {
				t.Error("CreateLogVol improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("CreateLogVol improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildOpenParcelVol(t *testing.T) {
	volSeriesUUID := "PLease sir, might I have my volume back?"
	deviceUUID := "It was here, if it's not too much trouble."
	parcelUUID := "It was named Spot."
	req := buildOpenVol(volSeriesUUID, deviceUUID, parcelUUID, false)
	if req.GetMsgType() != nuvo.Cmd_OPEN_VOLUME_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	opv := req.OpenVolume
	if opv == nil {
		t.Error("No OpenVolume")
	}
	if opv.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("Vol Series UUID was:", opv.GetVolSeriesUuid())
	}
	if opv.GetRootDeviceUuid() != deviceUUID {
		t.Error("Device UUID was:", opv.GetRootDeviceUuid())
	}
	if opv.GetRootParcelUuid() != parcelUUID {
		t.Error("Has root parcel uuid:", opv.GetRootParcelUuid())
	}
	if opv.Explanation != nil {
		t.Error("had explanation :", opv.GetExplanation())
	}
	if opv.Result != nil {
		t.Error("had result :", opv.GetResult())
	}
}

func makeOpenParcelVolReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_OPEN_VOLUME_REPLY.Enum()
	reply.OpenVolume.Result = nuvo.OpenVolume_OK.Enum()
	return reply
}

func buildOpenParcelVolReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeOpenParcelVolReply(req)
	return reply, nil
}

func buildOpenParcelVolReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeOpenParcelVolReply(req)
	reply.MsgType = nuvo.Cmd_OPEN_VOLUME_REQ.Enum()
	return reply, nil
}

func buildOpenParcelVolReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeOpenParcelVolReply(req)
	reply.OpenVolume = nil
	return reply, nil
}

func buildOpenParcelVolReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeOpenParcelVolReply(req)
	reply.OpenVolume.Result = nuvo.OpenVolume_INVALID.Enum()
	return reply, nil
}

func buildOpenParcelVolReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeOpenParcelVolReply(req)
	reply.OpenVolume.Result = nuvo.OpenVolume_ERROR.Enum()
	reply.OpenVolume.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestOpenParcelVol(t *testing.T) {
	var tests = []dsrsTest{
		{buildOpenParcelVolReplyOKSendRecv, true, false},
		{buildOpenParcelVolReplyWrongReplySendRecv, false, true},
		{buildOpenParcelVolReplyNoMsgSendRecv, false, true},
		{buildOpenParcelVolReplyInvalidSendRecv, false, true},
		{buildOpenParcelVolReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "Cliff Clavin"
		deviceUUID := "Cheers"
		parcelUUID := "Drugs for Norm"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.OpenVol(volSeriesUUID, deviceUUID, parcelUUID, false)
		if test.works {
			if err != nil {
				t.Error("OpenParcelVol improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("OpenParcelVol improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildDestroyVol(t *testing.T) {
	volSeriesUUID := "Please sir, might I have my volume back?"
	deviceUUID := "It was here, if it's not too much trouble."
	parcelUUID := "It was named Spot."
	req := buildDestroyVol(volSeriesUUID, deviceUUID, parcelUUID, false)
	if req.GetMsgType() != nuvo.Cmd_DESTROY_VOL_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	dv := req.DestroyVol
	if dv == nil {
		t.Error("No DestoryVol")
	}
	if dv.GetVolUuid() != volSeriesUUID {
		t.Error("Vol Series UUID was:", dv.GetVolUuid())
	}
	if dv.GetRootDeviceUuid() != deviceUUID {
		t.Error("Device UUID was:", dv.GetRootDeviceUuid())
	}
	if dv.GetRootParcelUuid() != parcelUUID {
		t.Error("Has root parcel uuid:", dv.GetRootParcelUuid())
	}
	if dv.Explanation != nil {
		t.Error("had explanation :", dv.GetExplanation())
	}
	if dv.Result != nil {
		t.Error("had result :", dv.GetResult())
	}
}

func makeDestroyVolReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_DESTROY_VOL_REPLY.Enum()
	reply.DestroyVol.Result = nuvo.DestroyVol_OK.Enum()
	return reply
}

func buildDestroyVolReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDestroyVolReply(req)
	return reply, nil
}

func buildDestroyVolReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDestroyVolReply(req)
	reply.MsgType = nuvo.Cmd_DESTROY_VOL_REQ.Enum()
	return reply, nil
}

func buildDestroyVolReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDestroyVolReply(req)
	reply.DestroyVol = nil
	return reply, nil
}

func buildDestroyVolReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDestroyVolReply(req)
	reply.DestroyVol.Result = nuvo.DestroyVol_INVALID.Enum()
	return reply, nil
}

func buildDestroyVolReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDestroyVolReply(req)
	reply.DestroyVol.Result = nuvo.DestroyVol_ERROR.Enum()
	reply.DestroyVol.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestDestroyVol(t *testing.T) {
	var tests = []dsrsTest{
		{buildDestroyVolReplyOKSendRecv, true, false},
		{buildDestroyVolReplyWrongReplySendRecv, false, true},
		{buildDestroyVolReplyNoMsgSendRecv, false, true},
		{buildDestroyVolReplyInvalidSendRecv, false, true},
		{buildDestroyVolReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "Cliff Clavin"
		deviceUUID := "Cheers"
		parcelUUID := "Drugs for Norm"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.DestroyVol(volSeriesUUID, deviceUUID, parcelUUID, false)
		if test.works {
			if err != nil {
				t.Error("DestroyVol improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("OpenParcelVol improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func makeAllocParcelsReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_ALLOC_PARCELS_REPLY.Enum()
	reply.AllocParcels.Result = nuvo.AllocParcels_OK.Enum()
	return reply
}

func buildAllocParcelsReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocParcelsReply(req)
	return reply, nil
}

func buildAllocParcelsReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocParcelsReply(req)
	reply.MsgType = nuvo.Cmd_ALLOC_PARCELS_REQ.Enum()
	return reply, nil
}

func buildAllocParcelsReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocParcelsReply(req)
	reply.AllocParcels = nil
	return reply, nil
}

func buildAllocParcelsReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocParcelsReply(req)
	reply.AllocParcels.Result = nuvo.AllocParcels_INVALID.Enum()
	return reply, nil
}

func buildAllocParcelsReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocParcelsReply(req)
	reply.AllocParcels.Result = nuvo.AllocParcels_ERROR.Enum()
	reply.AllocParcels.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestAllocParcels(t *testing.T) {
	var tests = []dsrsTest{
		{buildAllocParcelsReplyOKSendRecv, true, false},
		{buildAllocParcelsReplyWrongReplySendRecv, false, true},
		{buildAllocParcelsReplyNoMsgSendRecv, false, true},
		{buildAllocParcelsReplyInvalidSendRecv, false, true},
		{buildAllocParcelsReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "Santa"
		deviceUUID := "Sleigh"
		num := uint64(102311)
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.AllocParcels(volSeriesUUID, deviceUUID, num)
		if test.works {
			if err != nil {
				t.Error("AllocParcels improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("AllocParcels improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildAllocParcels(t *testing.T) {
	volSeriesUUID := "Santa's sleigh"
	deviceUUID := "Hermey"
	num := uint64(4324242)
	req := buildAllocParcels(volSeriesUUID, deviceUUID, num)
	ap := req.AllocParcels
	if ap == nil {
		t.Error("No AllocParcels")
	}
	if ap.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("Vol Series UUID was:", ap.GetVolSeriesUuid())
	}
	if ap.GetDeviceUuid() != deviceUUID {
		t.Error("Device UUID was:", ap.GetDeviceUuid())
	}
	if ap.GetNum() != num {
		t.Error("Number was:", ap.GetNum())
	}
	if ap.Explanation != nil {
		t.Error("had explanation :", ap.GetExplanation())
	}
	if ap.Result != nil {
		t.Error("had result :", ap.GetResult())
	}
}

func makeAllocCacheReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_ALLOC_CACHE_REPLY.Enum()
	reply.AllocCache.Result = nuvo.AllocCache_OK.Enum()
	return reply
}

func buildAllocCacheReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocCacheReply(req)
	return reply, nil
}

func buildAllocCacheReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocCacheReply(req)
	reply.MsgType = nuvo.Cmd_ALLOC_CACHE_REQ.Enum()
	return reply, nil
}

func buildAllocCacheReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocCacheReply(req)
	reply.AllocCache = nil
	return reply, nil
}

func buildAllocCacheReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocCacheReply(req)
	reply.AllocCache.Result = nuvo.AllocCache_INVALID.Enum()
	return reply, nil
}

func buildAllocCacheReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeAllocCacheReply(req)
	reply.AllocCache.Result = nuvo.AllocCache_ERROR.Enum()
	reply.AllocCache.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestAllocCache(t *testing.T) {
	var tests = []dsrsTest{
		{buildAllocCacheReplyOKSendRecv, true, false},
		{buildAllocCacheReplyWrongReplySendRecv, false, true},
		{buildAllocCacheReplyNoMsgSendRecv, false, true},
		{buildAllocCacheReplyInvalidSendRecv, false, true},
		{buildAllocCacheReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "fake-uuid"
		size := uint64(4194304)
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.AllocCache(volSeriesUUID, size)
		if test.works {
			if err != nil {
				t.Error("AllocCache improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("AllocCache improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildAllocCache(t *testing.T) {
	volSeriesUUID := "fake-uuid"
	size := uint64(4194304)
	req := buildAllocCache(volSeriesUUID, size)
	ap := req.AllocCache
	if ap == nil {
		t.Error("No AllocCache")
	}
	if ap.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("Vol Series UUID was:", ap.GetVolSeriesUuid())
	}
	if ap.GetSizeBytes() != size {
		t.Error("Size was:", ap.GetSizeBytes())
	}
	if ap.Explanation != nil {
		t.Error("had explanation :", ap.GetExplanation())
	}
	if ap.Result != nil {
		t.Error("had result :", ap.GetResult())
	}
}

func makeCloseVolReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_CLOSE_VOL_REPLY.Enum()
	reply.CloseVol.Result = nuvo.CloseVol_OK.Enum()
	return reply
}

func buildCloseVolReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseVolReply(req)
	return reply, nil
}

func buildCloseVolReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseVolReply(req)
	reply.MsgType = nuvo.Cmd_CLOSE_VOL_REQ.Enum()
	return reply, nil
}

func buildCloseVolReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseVolReply(req)
	reply.CloseVol = nil
	return reply, nil
}

func buildCloseVolReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseVolReply(req)
	reply.CloseVol.Result = nuvo.CloseVol_INVALID.Enum()
	return reply, nil
}

func buildCloseVolReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCloseVolReply(req)
	reply.CloseVol.Result = nuvo.CloseVol_ERROR.Enum()
	reply.CloseVol.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestCloseVol(t *testing.T) {
	var tests = []dsrsTest{
		{buildCloseVolReplyOKSendRecv, true, false},
		{buildCloseVolReplyWrongReplySendRecv, false, true},
		{buildCloseVolReplyNoMsgSendRecv, false, true},
		{buildCloseVolReplyInvalidSendRecv, false, true},
		{buildCloseVolReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		volSeriesUUID := "... but you can't stay here."
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.CloseVol(volSeriesUUID)
		if test.works {
			if err != nil {
				t.Error("CloseVol improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("CloseVol improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func makeGetStatsReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_GET_STATS_REPLY.Enum()
	reply.GetStats.Result = nuvo.GetStats_OK.Enum()
	return reply
}

func buildGetStatsReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetStatsReply(req)
	return reply, nil
}

func buildGetStatsReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetStatsReply(req)
	reply.MsgType = nuvo.Cmd_GET_STATS_REQ.Enum()
	return reply, nil
}

func buildGetStatsReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetStatsReply(req)
	reply.GetStats = nil
	return reply, nil
}

func buildGetStatsReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetStatsReply(req)
	reply.GetStats.Result = nuvo.GetStats_INVALID.Enum()
	return reply, nil
}

func buildGetStatsReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetStatsReply(req)
	reply.GetStats.Result = nuvo.GetStats_ERROR.Enum()
	reply.GetStats.Explanation = proto.String("Something went wrong")
	return reply, nil
}

type getStatsTest struct {
	device  bool
	read    bool
	dsr     DoSendRecv
	works   bool // Should succeed
	nuvoErr bool // Error should be nuvoErr
}

func TestGetStats(t *testing.T) {
	var tests = []getStatsTest{
		{true, true, buildGetStatsReplyOKSendRecv, true, false},
		{false, false, buildGetStatsReplyOKSendRecv, true, false},
		{true, false, buildGetStatsReplyWrongReplySendRecv, false, true},
		{false, true, buildGetStatsReplyNoMsgSendRecv, false, true},
		{false, false, buildGetStatsReplyInvalidSendRecv, false, true},
		{true, true, buildGetStatsReplyFailSendRecv, false, true},
		{true, true, buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		clear := false
		reqUUID := "Some device uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		_, err := dsr.GetStats(test.device, test.read, clear, reqUUID)
		if test.works {
			if err != nil {
				t.Error("GetStats improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("GetStats improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func makeGetVolumeStatsReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_GET_VOLUME_STATS_REPLY.Enum()
	reply.GetVolumeStats.Result = nuvo.GetVolumeStats_OK.Enum()
	return reply
}

func buildGetVolumeStatsReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetVolumeStatsReply(req)
	return reply, nil
}

func buildGetVolumeStatsWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetVolumeStatsReply(req)
	reply.MsgType = nuvo.Cmd_GET_VOLUME_STATS_REQ.Enum()
	return reply, nil
}

func buildGetVolumeStatsReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetVolumeStatsReply(req)
	reply.GetVolumeStats = nil
	return reply, nil
}

func buildGetVolumeStatsReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetVolumeStatsReply(req)
	reply.GetVolumeStats.Result = nuvo.GetVolumeStats_INVALID.Enum()
	return reply, nil
}

func buildGetVolumeStatsReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetVolumeStatsReply(req)
	reply.GetVolumeStats.Result = nuvo.GetVolumeStats_ERROR.Enum()
	reply.GetVolumeStats.Explanation = proto.String("Something went wrong")
	return reply, nil
}

type getVolumeStatsTest struct {
	dsr     DoSendRecv
	works   bool // Should succeed
	nuvoErr bool // Error should be nuvoErr
}

func TestGetVolumeStats(t *testing.T) {
	var tests = []getVolumeStatsTest{
		{buildGetVolumeStatsReplyOKSendRecv, true, false},
		{buildGetVolumeStatsWrongReplySendRecv, false, true},
		{buildGetVolumeStatsReplyNoMsgSendRecv, false, true},
		{buildGetVolumeStatsReplyInvalidSendRecv, false, true},
		{buildGetVolumeStatsReplyFailSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		clear := false
		reqUUID := "Some volume uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		_, err := dsr.GetVolumeStats(clear, reqUUID)
		if test.works {
			if err != nil {
				t.Error("GetVolumeStats improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("GetVolumeStats improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildManifest(t *testing.T) {
	volSeriesUUID := "My patronus is a UUID"
	req := buildManifest(volSeriesUUID, true)
	if req.GetMsgType() != nuvo.Cmd_MANIFEST_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	man := req.Manifest
	if man == nil {
		t.Error("No Manifest.")
	}
	if man.GetVolUuid() != volSeriesUUID {
		t.Error("UUID was:", man.GetVolUuid())
	}
	if man.GetShortReply() != true {
		t.Error("ShortReply was not true", man.GetShortReply())
	}
	if man.Explanation != nil {
		t.Error("had explanation :", man.GetExplanation())
	}
	if man.Result != nil {
		t.Error("had result :", man.GetResult())
	}
}

func makeManifestReply(req *nuvo.Cmd, shortReply bool) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_MANIFEST_REPLY.Enum()
	reply.Manifest.Result = nuvo.Manifest_OK.Enum()

	device := new(nuvo.ManifestDevice)
	deviceIndex := uint32(101)
	device.DeviceIndex = &deviceIndex
	device.DeviceUuid = proto.String("Device 1")
	targetParcels := uint32(102)
	device.TargetParcels = &targetParcels
	allocedParcels := uint32(103)
	device.AllocedParcels = &allocedParcels
	deviceClass := uint32(104)
	device.DeviceClass = &deviceClass
	parcelSize := uint64(105)
	device.ParcelSize = &parcelSize
	freeSegments := uint32(106)
	device.FreeSegments = &freeSegments
	blocksUsed := uint64(107)
	device.BlocksUsed = &blocksUsed
	reply.Manifest.Devices = append(reply.Manifest.GetDevices(), device)

	emptyDevice := new(nuvo.ManifestDevice)
	emptyDeviceIndex := uint32(201)
	emptyDevice.DeviceIndex = &emptyDeviceIndex
	emptyDevice.DeviceUuid = proto.String("Device 2")
	reply.Manifest.Devices = append(reply.Manifest.GetDevices(), emptyDevice)

	if shortReply != true {
		parcel := new(nuvo.ManifestParcel)
		parcelIndex1 := uint32(301)
		parcel.ParcelIndex = &parcelIndex1
		parcel.ParcelUuid = proto.String("Parcel 1")
		parcel.DeviceIndex = &deviceIndex
		segmentSize := uint32(302)
		parcel.SegmentSize = &segmentSize
		parcel.State = nuvo.ManifestParcel_OPEN.Enum()
		segment := new(nuvo.ManifestSegment)
		segmentBlksUsed := uint32(401)
		segment.BlksUsed = &segmentBlksUsed
		segmentAge := uint64(402)
		segment.Age = &segmentAge
		segmentReserved := true
		segment.Reserved = &segmentReserved
		segmentLogger := false
		segment.Logger = &segmentLogger
		segmentPinCnt := uint32(403)
		segment.PinCnt = &segmentPinCnt
		parcel.Segments = append(parcel.GetSegments(), segment)
		reply.Manifest.Parcels = append(reply.Manifest.GetParcels(), parcel)
		unusedParcel := new(nuvo.ManifestParcel)
		unusedParcel.State = nuvo.ManifestParcel_UNUSED.Enum()
		unusedParcel.DeviceIndex = &deviceIndex
		reply.Manifest.Parcels = append(reply.Manifest.GetParcels(), unusedParcel)
		addingdParcel := new(nuvo.ManifestParcel)
		addingdParcel.State = nuvo.ManifestParcel_ADDING.Enum()
		addingdParcel.DeviceIndex = &deviceIndex
		reply.Manifest.Parcels = append(reply.Manifest.GetParcels(), addingdParcel)
		usableParcel := new(nuvo.ManifestParcel)
		usableParcel.State = nuvo.ManifestParcel_USABLE.Enum()
		usableParcel.DeviceIndex = &deviceIndex
		reply.Manifest.Parcels = append(reply.Manifest.GetParcels(), usableParcel)
		openingParcel := new(nuvo.ManifestParcel)
		openingParcel.State = nuvo.ManifestParcel_OPENING.Enum()
		openingParcel.DeviceIndex = &deviceIndex
		reply.Manifest.Parcels = append(reply.Manifest.GetParcels(), openingParcel)
		lostParcel := new(nuvo.ManifestParcel)
		lostParcel.State = nuvo.ManifestParcel_OPENING.Enum()
		reply.Manifest.Parcels = append(reply.Manifest.GetParcels(), lostParcel)
	}

	return reply
}

type manifestTest struct {
	dsr        DoSendRecv
	shortReply bool
	works      bool // Should succeed
	nuvoErr    bool // Error should be nuvoErr
}

func buildManifestReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeManifestReply(req, false)
	return reply, nil
}

func buildManifestShortReplyOKSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeManifestReply(req, true)
	return reply, nil
}

func buildManifestReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeManifestReply(req, false)
	reply.MsgType = nuvo.Cmd_MANIFEST_REQ.Enum()
	return reply, nil
}

func buildManifestReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeManifestReply(req, false)
	reply.Manifest = nil
	return reply, nil
}

func buildManifestReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeManifestReply(req, false)
	reply.Manifest.Result = nuvo.Manifest_INVALID.Enum()
	return reply, nil
}

func buildManifestReplyFailSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeManifestReply(req, false)
	reply.Manifest.Result = nuvo.Manifest_ERROR.Enum()
	reply.Manifest.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestManifest(t *testing.T) {
	var tests = []manifestTest{
		{buildManifestReplyOKSendRecv, false, true, false},
		{buildManifestShortReplyOKSendRecv, true, true, false},
		{buildManifestReplyWrongReplySendRecv, true, false, true},
		{buildManifestReplyNoMsgSendRecv, false, false, true},
		{buildManifestReplyInvalidSendRecv, false, false, true},
		{buildManifestReplyFailSendRecv, false, false, true},
		{buildReplyErrorSendRecv, false, false, false},
	}
	for _, test := range tests {
		reqUUID := "Some device uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		devices, err := dsr.Manifest(reqUUID, test.shortReply)
		if test.works {
			if err != nil {
				t.Error("Manifest improperly failed", test, err)
			}
			if devices[0].Index != 101 ||
				devices[0].UUID != "Device 1" ||
				devices[0].ParcelSize != 105 ||
				devices[0].TargetParcels != 102 ||
				devices[0].AllocedParcels != 103 ||
				devices[0].FreeSegments != 106 ||
				devices[0].BlocksUsed != 107 ||
				devices[0].DeviceClass != 104 {
				t.Error("Device 0 wrong", devices[0])
			}
			if test.shortReply != true {
				if devices[0].Parcels[0].ParcelIndex != 301 ||
					devices[0].Parcels[0].UUID != "Parcel 1" ||
					devices[0].Parcels[0].DeviceIndex != devices[0].Index ||
					devices[0].Parcels[0].SegmentSize != 302 ||
					devices[0].Parcels[0].Segments[0].BlksUsed != 401 ||
					devices[0].Parcels[0].Segments[0].Age != 402 ||
					devices[0].Parcels[0].Segments[0].PinCnt != 403 ||
					devices[0].Parcels[0].Segments[0].Log != false ||
					devices[0].Parcels[0].Segments[0].Reserved != true {
					t.Error("Device 1, parcel 0 wrong", devices[1].Parcels[0])
				}
			}
			if devices[1].Index != 201 ||
				devices[1].UUID != "Device 2" {
				t.Error("Device 1 wrong", devices[1])
			}
		} else {
			if err == nil {
				t.Error("Manifest improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildCloselVol(t *testing.T) {
	volSeriesUUID := "You don't have to go home, but..."
	req := buildCloseVol(volSeriesUUID)
	if req.GetMsgType() != nuvo.Cmd_CLOSE_VOL_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	cv := req.CloseVol
	if cv == nil {
		t.Error("No CloseVol")
	}
	if cv.GetVolSeriesUuid() != volSeriesUUID {
		t.Error("Vol Series UUID was:", cv.GetVolSeriesUuid())
	}
	if cv.Explanation != nil {
		t.Error("had explanation :", cv.GetExplanation())
	}
	if cv.Result != nil {
		t.Error("had result :", cv.GetResult())
	}
}

func TestBuildHalt(t *testing.T) {
	req := buildHaltCommand()
	if req.GetMsgType() != nuvo.Cmd_SHUTDOWN {
		t.Error("msg type was:", req.MsgType)
	}
}

func TestHaltWorks(t *testing.T) {
	dsr := NewSendRecv(buildReplyErrorSendRecv, "sock")
	err := dsr.HaltCommand()
	if err == nil {
		t.Error("Didn't return error")
	}
}

func TestBuildUseNodeUuid(t *testing.T) {
	nodeUUID := "lymph"
	req := buildUseNodeUUID(nodeUUID)
	if req.GetMsgType() != nuvo.Cmd_USE_NODE_UUID_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	unu := req.UseNodeUuid
	if unu == nil {
		t.Error("No UseNodeUuid.")
	}
	if unu.GetUuid() != nodeUUID {
		t.Error("UUID was:", unu.GetUuid())
	}
}

func makeCapabilitiesReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_CAPABILITIES_REPLY.Enum()
	reply.Capabilities.Multifuse = proto.Bool(false)
	return reply
}

func makeUseNodeUUIDReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_USE_NODE_UUID_REPLY.Enum()
	reply.UseNodeUuid.Result = nuvo.UseNodeUuid_OK.Enum()
	return reply
}

func buildUNUReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	if req.GetMsgType() == nuvo.Cmd_CAPABILITIES_REQ {
		return makeCapabilitiesReply(req), nil
	}
	reply := makeUseNodeUUIDReply(req)
	return reply, nil
}

func buildUNUReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	if req.GetMsgType() == nuvo.Cmd_CAPABILITIES_REQ {
		return makeCapabilitiesReply(req), nil
	}
	reply := makeUseNodeUUIDReply(req)
	reply.MsgType = nuvo.Cmd_USE_NODE_UUID_REQ.Enum()
	return reply, nil
}

func buildUNUReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	if req.GetMsgType() == nuvo.Cmd_CAPABILITIES_REQ {
		return makeCapabilitiesReply(req), nil
	}
	reply := makeUseNodeUUIDReply(req)
	reply.UseNodeUuid = nil
	return reply, nil
}

func buildUNUReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	if req.GetMsgType() == nuvo.Cmd_CAPABILITIES_REQ {
		return makeCapabilitiesReply(req), nil
	}
	reply := makeUseNodeUUIDReply(req)
	reply.UseNodeUuid.Result = nuvo.UseNodeUuid_INVALID.Enum()
	reply.UseNodeUuid.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildUNUReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	if req.GetMsgType() == nuvo.Cmd_CAPABILITIES_REQ {
		return makeCapabilitiesReply(req), nil
	}
	reply := makeUseNodeUUIDReply(req)
	reply.UseNodeUuid.Result = nuvo.UseNodeUuid_ERROR.Enum()
	reply.UseNodeUuid.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildUNUReplyEOFSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	if req.GetMsgType() == nuvo.Cmd_CAPABILITIES_REQ {
		return makeCapabilitiesReply(req), nil
	}
	return nil, errors.New("EOF")
}

func TestUseNodeUuid(t *testing.T) {
	var tests = []dsrsTest{
		{buildUNUReplyOkSendRecv, true, false},
		{buildUNUReplyWrongReplySendRecv, false, true},
		{buildUNUReplyNoMsgSendRecv, false, true},
		{buildUNUReplyInvalidSendRecv, false, true},
		{buildUNUReplyErrorSendRecv, false, true},
		{buildUNUReplyEOFSendRecv, false, false},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		uuid := "my uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.UseNodeUUID(uuid)
		if test.works {
			if err != nil {
				t.Error("UseNodeUuid improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("UseNodeUuid improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildGetPitDiffs(t *testing.T) {
	vsUUID := "UUID"
	baseUUID := "base"
	incrUUID := "incr"
	req := buildGetPitDiffs(vsUUID, baseUUID, incrUUID, 0)
	if req.GetMsgType() != nuvo.Cmd_GET_PIT_DIFF_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	dp := req.GetPitDiffs
	if dp == nil {
		t.Error("No Diff")
	}
	if dp.GetVolUuid() != vsUUID {
		t.Error("Volume Series UUID was:", dp.GetVolUuid())
	}
	if dp.GetBasePitUuid() != baseUUID {
		t.Error("PiT UUID was:", dp.GetBasePitUuid())
	}
	if dp.GetIncrPitUuid() != incrUUID {
		t.Error("PiT UUID was:", dp.GetIncrPitUuid())
	}
}

func makeGetPitDiffsReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_GET_PIT_DIFF_REPLY.Enum()
	reply.GetPitDiffs.Result = nuvo.GetPitDiffs_OK.Enum()
	reply.GetPitDiffs.Diffs = append(reply.GetPitDiffs.Diffs, &nuvo.GetPitDiffs_PitDiff{})
	return reply
}

func buildGsdReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetPitDiffsReply(req)
	return reply, nil
}

func buildGsdReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetPitDiffsReply(req)
	reply.MsgType = nuvo.Cmd_GET_PIT_DIFF_REQ.Enum()
	return reply, nil
}

func buildGsdReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetPitDiffsReply(req)
	reply.GetPitDiffs = nil
	return reply, nil
}

func buildGsdReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetPitDiffsReply(req)
	reply.GetPitDiffs.Result = nuvo.GetPitDiffs_ERROR.Enum()
	reply.GetPitDiffs.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildGsdReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeGetPitDiffsReply(req)
	reply.GetPitDiffs.Result = nuvo.GetPitDiffs_ERROR.Enum()
	reply.GetPitDiffs.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestGetPitDiffs(t *testing.T) {
	var tests = []dsrsTest{
		{buildGsdReplyOkSendRecv, true, false},
		{buildGsdReplyWrongReplySendRecv, false, true},
		{buildGsdReplyNoMsgSendRecv, false, true},
		{buildGsdReplyInvalidSendRecv, false, true},
		{buildGsdReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		vuuid := "vol uuid"
		basePit := "base"
		incrPit := "incr"
		offset := uint64(0)
		dsr := NewSendRecv(test.dsr, "sock")
		_, _, err := dsr.GetPitDiffs(vuuid, basePit, incrPit, offset)
		if test.works {
			if err != nil {
				t.Error("GetPitDiffs improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("GetPitDiffs improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildCreatePit(t *testing.T) {
	vsUUID := "Your stomach"
	pitUUID := "pit"
	req := buildCreatePit(vsUUID, pitUUID)
	if req.GetMsgType() != nuvo.Cmd_CREATE_PIT_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	cp := req.CreatePit
	if cp == nil {
		t.Error("No CreatePit")
	}
	if cp.GetVolUuid() != vsUUID {
		t.Error("Volume Series UUID was:", cp.GetVolUuid())
	}
	if cp.GetPitUuid() != pitUUID {
		t.Error("PiT UUID was:", cp.GetPitUuid())
	}
}

func makeCreatePitReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_CREATE_PIT_REPLY.Enum()
	reply.CreatePit.Result = nuvo.CreatePit_OK.Enum()
	return reply
}

func buildCpReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreatePitReply(req)
	return reply, nil
}

func buildCpReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreatePitReply(req)
	reply.MsgType = nuvo.Cmd_CREATE_PIT_REQ.Enum()
	return reply, nil
}

func buildCpReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreatePitReply(req)
	reply.CreatePit = nil
	return reply, nil
}

func buildCpReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreatePitReply(req)
	reply.CreatePit.Result = nuvo.CreatePit_ERROR.Enum()
	reply.CreatePit.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildCpReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCreatePitReply(req)
	reply.CreatePit.Result = nuvo.CreatePit_ERROR.Enum()
	reply.CreatePit.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestCreatePit(t *testing.T) {
	var tests = []dsrsTest{
		{buildCpReplyOkSendRecv, true, false},
		{buildCpReplyWrongReplySendRecv, false, true},
		{buildCpReplyNoMsgSendRecv, false, true},
		{buildCpReplyInvalidSendRecv, false, true},
		{buildCpReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		vuuid := "vol uuid"
		puuid := "pit uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.CreatePit(vuuid, puuid)
		if test.works {
			if err != nil {
				t.Error("CreatePit improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("CreatePit improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildDeletePit(t *testing.T) {
	vsUUID := "Your stomach"
	pitUUID := "pit"
	req := buildDeletePit(vsUUID, pitUUID)
	if req.GetMsgType() != nuvo.Cmd_DELETE_PIT_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	dp := req.DeletePit
	if dp == nil {
		t.Error("No DeletePit")
	}
	if dp.GetVolUuid() != vsUUID {
		t.Error("Volume Series UUID was:", dp.GetVolUuid())
	}
	if dp.GetPitUuid() != pitUUID {
		t.Error("PiT UUID was:", dp.GetPitUuid())
	}
}

func makeDeletePitReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_DELETE_PIT_REPLY.Enum()
	reply.DeletePit.Result = nuvo.DeletePit_OK.Enum()
	return reply
}

func buildDpReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeletePitReply(req)
	return reply, nil
}

func buildDpReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeletePitReply(req)
	reply.MsgType = nuvo.Cmd_DELETE_PIT_REQ.Enum()
	return reply, nil
}

func buildDpReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeletePitReply(req)
	reply.DeletePit = nil
	return reply, nil
}

func buildDpReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeletePitReply(req)
	reply.DeletePit.Result = nuvo.DeletePit_ERROR.Enum()
	reply.DeletePit.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildDpReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDeletePitReply(req)
	reply.DeletePit.Result = nuvo.DeletePit_ERROR.Enum()
	reply.DeletePit.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestDeletePit(t *testing.T) {
	var tests = []dsrsTest{
		{buildDpReplyOkSendRecv, true, false},
		{buildDpReplyWrongReplySendRecv, false, true},
		{buildDpReplyNoMsgSendRecv, false, true},
		{buildDpReplyInvalidSendRecv, false, true},
		{buildDpReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		vuuid := "vol uuid"
		puuid := "pit uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.DeletePit(vuuid, puuid)
		if test.works {
			if err != nil {
				t.Error("DeletePit improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("DeletePit improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildListPits(t *testing.T) {
	vsUUID := "Your stomach"
	req := buildListPits(vsUUID)
	if req.GetMsgType() != nuvo.Cmd_LIST_PITS_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	lp := req.ListPits
	if lp == nil {
		t.Error("No ListPits")
	}
	if lp.GetVolUuid() != vsUUID {
		t.Error("Volume Series UUID was:", lp.GetVolUuid())
	}
}

func makeListPitsReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_LIST_PITS_REPLY.Enum()
	reply.ListPits.Result = nuvo.ListPits_OK.Enum()
	uuid := "MyUUID"
	reply.ListPits.Pits = append(reply.ListPits.Pits, &nuvo.ListPits_Pit{PitUuid: &uuid})
	return reply
}

func buildLpReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListPitsReply(req)
	return reply, nil
}

func buildLpReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListPitsReply(req)
	reply.MsgType = nuvo.Cmd_LIST_PITS_REQ.Enum()
	return reply, nil
}

func buildLpReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListPitsReply(req)
	reply.ListPits = nil
	return reply, nil
}

func buildLpReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListPitsReply(req)
	reply.ListPits.Result = nuvo.ListPits_ERROR.Enum()
	reply.ListPits.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildLpReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListPitsReply(req)
	reply.ListPits.Result = nuvo.ListPits_ERROR.Enum()
	reply.ListPits.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestListPits(t *testing.T) {
	var tests = []dsrsTest{
		{buildLpReplyOkSendRecv, true, false},
		{buildLpReplyWrongReplySendRecv, false, true},
		{buildLpReplyNoMsgSendRecv, false, true},
		{buildLpReplyInvalidSendRecv, false, true},
		{buildLpReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		vuuid := "vol uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		_, err := dsr.ListPits(vuuid)
		if test.works {
			if err != nil {
				t.Error("ListPits improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("ListPits improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func makeListVolsReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_LIST_VOLS_REPLY.Enum()
	reply.ListVols.Result = nuvo.ListVols_OK.Enum()
	uuid := "MyUUID"
	reply.ListVols.Vols = append(reply.ListVols.Vols, &nuvo.ListVols_Vol{VolUuid: &uuid})
	return reply
}

func buildLvReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListVolsReply(req)
	return reply, nil
}

func buildLvReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListVolsReply(req)
	reply.MsgType = nuvo.Cmd_LIST_VOLS_REQ.Enum()
	return reply, nil
}

func buildLvReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListVolsReply(req)
	reply.ListVols = nil
	return reply, nil
}

func buildLvReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListVolsReply(req)
	reply.ListVols.Result = nuvo.ListVols_ERROR.Enum()
	reply.ListVols.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildLvReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeListVolsReply(req)
	reply.ListVols.Result = nuvo.ListVols_ERROR.Enum()
	reply.ListVols.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestListVols(t *testing.T) {
	var tests = []dsrsTest{
		{buildLvReplyOkSendRecv, true, false},
		{buildLvReplyWrongReplySendRecv, false, true},
		{buildLvReplyNoMsgSendRecv, false, true},
		{buildLvReplyInvalidSendRecv, false, true},
		{buildLvReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		dsr := NewSendRecv(test.dsr, "sock")
		_, err := dsr.ListVols()
		if test.works {
			if err != nil {
				t.Error("ListVols improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("ListVols improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildPauseIo(t *testing.T) {
	vsUUID := "Your stomach"
	req := buildPauseIo(vsUUID)
	if req.GetMsgType() != nuvo.Cmd_PAUSE_IO_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	pio := req.PauseIo
	if pio == nil {
		t.Error("No PauseIo")
	}
	if pio.GetVolUuid() != vsUUID {
		t.Error("Volume Series UUID was:", pio.GetVolUuid())
	}
}

func makePauseIoReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_PAUSE_IO_REPLY.Enum()
	reply.PauseIo.Result = nuvo.PauseIo_OK.Enum()
	return reply
}

func buildPioReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makePauseIoReply(req)
	return reply, nil
}

func buildPioReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makePauseIoReply(req)
	reply.MsgType = nuvo.Cmd_PAUSE_IO_REQ.Enum()
	return reply, nil
}

func buildPioReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makePauseIoReply(req)
	reply.PauseIo = nil
	return reply, nil
}

func buildPioReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makePauseIoReply(req)
	reply.PauseIo.Result = nuvo.PauseIo_ERROR.Enum()
	reply.PauseIo.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildPioReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makePauseIoReply(req)
	reply.PauseIo.Result = nuvo.PauseIo_ERROR.Enum()
	reply.PauseIo.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestPauseIo(t *testing.T) {
	var tests = []dsrsTest{
		{buildPioReplyOkSendRecv, true, false},
		{buildPioReplyWrongReplySendRecv, false, true},
		{buildPioReplyNoMsgSendRecv, false, true},
		{buildPioReplyInvalidSendRecv, false, true},
		{buildPioReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		vuuid := "vol uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.PauseIo(vuuid)
		if test.works {
			if err != nil {
				t.Error("PauseIo improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("PauseIo improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildResumeIo(t *testing.T) {
	vsUUID := "Your stomach"
	req := buildResumeIo(vsUUID)
	if req.GetMsgType() != nuvo.Cmd_RESUME_IO_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	rio := req.ResumeIo
	if rio == nil {
		t.Error("No ResumeIo")
	}
	if rio.GetVolUuid() != vsUUID {
		t.Error("Volume Series UUID was:", rio.GetVolUuid())
	}
}

func makeResumeIoReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_RESUME_IO_REPLY.Enum()
	reply.ResumeIo.Result = nuvo.ResumeIo_OK.Enum()
	return reply
}

func buildRioReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeResumeIoReply(req)
	return reply, nil
}

func buildRioReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeResumeIoReply(req)
	reply.MsgType = nuvo.Cmd_RESUME_IO_REQ.Enum()
	return reply, nil
}

func buildRioReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeResumeIoReply(req)
	reply.ResumeIo = nil
	return reply, nil
}

func buildRioReplyInvalidSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeResumeIoReply(req)
	reply.ResumeIo.Result = nuvo.ResumeIo_ERROR.Enum()
	reply.ResumeIo.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func buildRioReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeResumeIoReply(req)
	reply.ResumeIo.Result = nuvo.ResumeIo_ERROR.Enum()
	reply.ResumeIo.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestResumeIo(t *testing.T) {
	var tests = []dsrsTest{
		{buildRioReplyOkSendRecv, true, false},
		{buildRioReplyWrongReplySendRecv, false, true},
		{buildRioReplyNoMsgSendRecv, false, true},
		{buildRioReplyInvalidSendRecv, false, true},
		{buildRioReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		vuuid := "vol uuid"
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.ResumeIo(vuuid)
		if test.works {
			if err != nil {
				t.Error("ResumeIo improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("ResumeIo improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildLogLevel(t *testing.T) {
	module := "Test module"
	level := uint32(30)
	req := buildLogLevel(module, level)
	if req.GetMsgType() != nuvo.Cmd_LOG_LEVEL_REQ {
		t.Error("msg type was: ", req.MsgType)
	}
	llio := req.LogLevel
	if llio == nil {
		t.Error("No LogLevel")
	}
	if llio.GetModuleName() != module {
		t.Error("Module was: ", llio.GetModuleName())
	}
	if llio.GetLevel() != level {
		t.Error("Log level was: ", llio.GetLevel())
	}
}

func makeLogLevelReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_LOG_LEVEL_REPLY.Enum()
	reply.LogLevel.Result = nuvo.LogLevel_OK.Enum()
	return reply
}

func buildLogLevelOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeLogLevelReply(req)
	return reply, nil
}

func buildLogLevelReplyWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeLogLevelReply(req)
	reply.MsgType = nuvo.Cmd_LOG_LEVEL_REQ.Enum()
	return reply, nil
}

func buildLogLevelReplyNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeLogLevelReply(req)
	reply.LogLevel = nil
	return reply, nil
}

func buildLogLevelReplyErrorSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeLogLevelReply(req)
	reply.LogLevel.Result = nuvo.LogLevel_BAD_ORDER.Enum()
	reply.LogLevel.Explanation = proto.String("Something went wrong")
	return reply, nil
}

func TestLogLevel(t *testing.T) {
	var tests = []dsrsTest{
		{buildLogLevelOkSendRecv, true, false},
		{buildLogLevelReplyWrongReplySendRecv, false, true},
		{buildLogLevelReplyNoMsgSendRecv, false, true},
		{buildLogLevelReplyErrorSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		module := "Test module"
		level := uint32(30)
		dsr := NewSendRecv(test.dsr, "sock")
		err := dsr.LogLevel(module, level)
		if test.works {
			if err != nil {
				t.Error("LogLevel improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("LogLevel improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildDebugTrigger(t *testing.T) {
	req := buildDebugTrigger()
	if req.GetMsgType() != nuvo.Cmd_DEBUG_TRIGGER_REQ {
		t.Error("msg type was: ", req.MsgType)
	}
	dt := req.DebugTrigger
	if dt == nil {
		t.Error("No DebugTrigger")
	}
}

func makeDebugTriggerReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_DEBUG_TRIGGER_REPLY.Enum()
	return reply
}

func buildDebugTriggerOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDebugTriggerReply(req)
	return reply, nil
}

func buildDebugTriggerWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeDebugTriggerReply(req)
	reply.MsgType = nuvo.Cmd_DEBUG_TRIGGER_REQ.Enum()
	return reply, nil
}

func TestDebugTrigger(t *testing.T) {
	var tests = []dsrsTest{
		{buildDebugTriggerOkSendRecv, true, false},
		{buildDebugTriggerWrongReplySendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		dsr := NewSendRecv(test.dsr, "sock")
		var params DebugTriggerParams
		err := dsr.DebugTrigger(params)
		if test.works {
			if err != nil {
				t.Error("DebugTrigger improperly failed", test, err)
			}
		} else {
			if err == nil {
				t.Error("DebugTrigger improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildLogSummary(t *testing.T) {
	volUUID := "UUID for Dummies"
	parcelIndex := uint32(1)
	segmentIndex := uint32(2)
	req := buildLogSummary(volUUID, parcelIndex, segmentIndex)
	if req.GetMsgType() != nuvo.Cmd_LOG_SUMMARY_REQ {
		t.Error("msg type was: ", req.MsgType)
	}
	dt := req.LogSummary
	if dt == nil {
		t.Error("No LogSummary")
	}
}

func makeLogSummaryReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_LOG_SUMMARY_REPLY.Enum()
	reply.LogSummary.VolUuid = proto.String("Status volume")
	parcelIndex := uint32(1)
	reply.LogSummary.ParcelIndex = &parcelIndex
	segmentIndex := uint32(2)
	reply.LogSummary.SegmentIndex = &segmentIndex
	magic := uint32(3)
	reply.LogSummary.Magic = &magic
	sequenceNo := uint64(4)
	reply.LogSummary.SequenceNo = &sequenceNo
	closingSequenceNo := uint64(5)
	reply.LogSummary.ClosingSequenceNo = &closingSequenceNo
	reply.LogSummary.VsUuid = proto.String("Status volume reply")

	dataEntry := new(nuvo.LogSummaryEntry)
	dataEntryType := uint32(2)
	dataEntry.LogEntryType = &dataEntryType
	dataBlockHash := uint32(201)
	dataEntry.BlockHash = &dataBlockHash
	dataEntry.Data = new(nuvo.LogSummaryEntry_Data)
	pitInfoActive := bool(true)
	dataEntry.Data.PitInfoActive = &pitInfoActive
	dataPitInfoID := uint32(202)
	dataEntry.Data.PitInfoId = &dataPitInfoID
	dataBno := uint64(203)
	dataEntry.Data.Bno = &dataBno
	dataGcParcelIndex := uint32(204)
	dataEntry.Data.GcParcelIndex = &dataGcParcelIndex
	dataGcBlockOffset := uint32(205)
	dataEntry.Data.GcBlockOffset = &dataGcBlockOffset
	reply.LogSummary.Entries = append(reply.LogSummary.GetEntries(), dataEntry)

	forkEntry := new(nuvo.LogSummaryEntry)
	forkEntryType := uint32(8)
	forkEntry.LogEntryType = &forkEntryType
	forkBlockHash := uint32(801)
	forkEntry.BlockHash = &forkBlockHash
	forkSegParcelIndex := uint32(802)
	forkEntry.Fork = new(nuvo.LogSummaryEntry_Fork)
	forkEntry.Fork.SegParcelIndex = &forkSegParcelIndex
	forkSegBlockOffset := uint32(803)
	forkEntry.Fork.SegBlockOffset = &forkSegBlockOffset
	forkSequenceNo := uint64(804)
	forkEntry.Fork.SequenceNo = &forkSequenceNo
	forkSegSubclass := uint32(805)
	forkEntry.Fork.SegSubclass = &forkSegSubclass
	reply.LogSummary.Entries = append(reply.LogSummary.GetEntries(), forkEntry)

	headerEntry := new(nuvo.LogSummaryEntry)
	headerEntryType := uint32(9)
	headerEntry.LogEntryType = &headerEntryType
	headerBlockHash := uint32(901)
	headerEntry.BlockHash = &headerBlockHash
	headerEntry.Header = new(nuvo.LogSummaryEntry_Header)
	headerSequenceNo := uint64(902)
	headerEntry.Header.SequenceNo = &headerSequenceNo
	headerSubclass := uint32(903)
	headerEntry.Header.Subclass = &headerSubclass
	reply.LogSummary.Entries = append(reply.LogSummary.GetEntries(), headerEntry)

	descEntry := new(nuvo.LogSummaryEntry)
	descEntryType := uint32(10)
	descEntry.LogEntryType = &descEntryType
	descBlockHash := uint32(1001)
	descEntry.BlockHash = &descBlockHash
	descEntry.Descriptor_ = new(nuvo.LogSummaryEntry_Descriptor)
	descCvFlag := uint32(1002)
	descEntry.Descriptor_.CvFlag = &descCvFlag
	descEntryCount := uint32(1003)
	descEntry.Descriptor_.EntryCount = &descEntryCount
	descDataBlockCount := uint32(1004)
	descEntry.Descriptor_.DataBlockCount = &descDataBlockCount
	descSequenceNumber := uint64(1005)
	descEntry.Descriptor_.SequenceNo = &descSequenceNumber
	reply.LogSummary.Entries = append(reply.LogSummary.GetEntries(), descEntry)

	snapEntry := new(nuvo.LogSummaryEntry)
	snapEntryType := uint32(11)
	snapEntry.LogEntryType = &snapEntryType
	snapEntry.Snap = new(nuvo.LogSummaryEntry_Snap)
	snapSequenceNumber := uint64(1101)
	snapEntry.Snap.SequenceNo = &snapSequenceNumber
	snapOperation := uint32(1102)
	snapEntry.Snap.Operation = &snapOperation
	reply.LogSummary.Entries = append(reply.LogSummary.GetEntries(), snapEntry)

	return reply
}

func buildLogSummaryOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeLogSummaryReply(req)
	return reply, nil
}

func buildLogSummaryWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeLogSummaryReply(req)
	reply.MsgType = nuvo.Cmd_LOG_SUMMARY_REQ.Enum()
	return reply, nil
}

func TestLogSummary(t *testing.T) {
	var tests = []dsrsTest{
		{buildLogSummaryOkSendRecv, true, false},
		{buildLogSummaryWrongReplySendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		dsr := NewSendRecv(test.dsr, "sock")
		volUUID := "UUID for Dummies"
		parcelIndex := uint32(1)
		segmentIndex := uint32(2)
		reply, err := dsr.LogSummary(volUUID, parcelIndex, segmentIndex)
		if test.works {
			if err != nil {
				t.Error("LogSummary improperly failed", test, err)
			}
			if reply.VolUUID != "Status volume reply" ||
				reply.ParcelIndex != 1 ||
				reply.SegmentIndex != 2 ||
				reply.Magic != 3 ||
				reply.SequenceNumber != 4 ||
				reply.ClosingSequenceNumber != 5 {
				t.Error("Log summary got wrong volume", test, reply)
			}
			if reply.Entries[0].EntryType != 2 ||
				reply.Entries[0].BlockHash != 201 ||
				reply.Entries[0].PitInfoActive != true ||
				reply.Entries[0].PitInfoID != 202 ||
				reply.Entries[0].BlockNumber != 203 ||
				reply.Entries[0].ParcelIndex != 204 ||
				reply.Entries[0].BlockOffset != 205 {
				t.Error("Data entry is wrong", reply)
			}
			if reply.Entries[1].EntryType != 8 ||
				reply.Entries[1].BlockHash != 801 ||
				reply.Entries[1].ParcelIndex != 802 ||
				reply.Entries[1].BlockOffset != 803 ||
				reply.Entries[1].SequenceNumber != 804 ||
				reply.Entries[1].SubClass != 805 {
				t.Error("Fork entry is wrong", reply)
			}
			if reply.Entries[2].EntryType != 9 ||
				reply.Entries[2].BlockHash != 901 ||
				reply.Entries[2].SequenceNumber != 902 ||
				reply.Entries[2].SubClass != 903 {
				t.Error("Header entry is wrong", reply)
			}
			if reply.Entries[3].EntryType != 10 ||
				reply.Entries[3].BlockHash != 1001 ||
				reply.Entries[3].CVFlag != 1002 ||
				reply.Entries[3].EntryCount != 1003 ||
				reply.Entries[3].DataBlockCount != 1004 ||
				reply.Entries[3].SequenceNumber != 1005 {
				t.Error("Desriptor entry is wrong", reply)
			}
			if reply.Entries[4].EntryType != 11 ||
				reply.Entries[4].SequenceNumber != 1101 ||
				reply.Entries[4].SnapOperation != 1102 {
				t.Error("Snap entry is wrong", reply)
			}
		} else {
			if err == nil {
				t.Error("LogSummary improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func TestBuildNodeStatus(t *testing.T) {
	req := buildNodeStatus()
	if req.GetMsgType() != nuvo.Cmd_NODE_STATUS_REQ {
		t.Error("msg type was:", req.MsgType)
	}
	rio := req.NodeStatus
	if rio == nil {
		t.Error("No NodeStatus")
	}
}

func makeNodeStatusReply(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_NODE_STATUS_REPLY.Enum()
	reply.NodeStatus.NodeUuid = proto.String("Node UUID, fool!")
	reply.NodeStatus.GitBuildHash = proto.String("Hash Hash Hash")
	var dataClass = new(nuvo.DataClassSpace)
	class := uint32(1)
	dataClass.Class = &class
	blocksUsed := uint64(2)
	dataClass.BlocksUsed = &blocksUsed
	blocksAllocated := uint64(3)
	dataClass.BlocksAllocated = &blocksAllocated
	blocksTotal := uint64(4)
	dataClass.BlocksTotal = &blocksTotal
	var volStatus = new(nuvo.VolStatus)
	volStatus.DataClassSpace = append(volStatus.GetDataClassSpace(), dataClass)
	volStatus.VolUuid = proto.String("Vol id or something")
	reply.NodeStatus.Volumes = append(reply.NodeStatus.GetVolumes(), volStatus)

	return reply
}

func buildNodeStatusReplyOkSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeStatusReply(req)
	return reply, nil
}

func buildNodeStatusWrongReplySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeStatusReply(req)
	reply.MsgType = nuvo.Cmd_NODE_STATUS_REQ.Enum()
	return reply, nil
}

func buildNodeStatusNoMsgSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeNodeStatusReply(req)
	reply.NodeStatus = nil
	return reply, nil
}

func TestNodeStatus(t *testing.T) {
	var tests = []dsrsTest{
		{buildNodeStatusReplyOkSendRecv, true, false},
		{buildNodeStatusWrongReplySendRecv, false, true},
		{buildNodeStatusNoMsgSendRecv, false, true},
		{buildReplyErrorSendRecv, false, false},
	}
	for _, test := range tests {
		dsr := NewSendRecv(test.dsr, "sock")
		reply, err := dsr.NodeStatus()
		if test.works {
			if err != nil {
				t.Error("NodeStatus improperly failed", test, err)
			}
			if reply.NodeUUID != "Node UUID, fool!" {
				t.Error("Got wrong uuid", test, err, reply.NodeUUID)
			}
			if reply.GitBuildHash != "Hash Hash Hash" {
				t.Error("Got wrong uuid", test, err, reply.GitBuildHash)
			}
			if reply.Volumes[0].VolUUID != "Vol id or something" {
				t.Error("Got wrong voluuid", test, err, reply)
			}
			if reply.Volumes[0].ClassSpace[0].Class != 1 ||
				reply.Volumes[0].ClassSpace[0].BlocksUsed != 2 ||
				reply.Volumes[0].ClassSpace[0].BlocksAllocated != 3 ||
				reply.Volumes[0].ClassSpace[0].BlocksTotal != 4 {
				t.Error("Got wrong class data", test, err, reply)
			}
		} else {
			if err == nil {
				t.Error("NodeStatus improperly succeeded", test, err)
			}
			nerr, ok := err.(ErrorInt)
			if ok != test.nuvoErr {
				t.Error("wrong error type", getFunctionName(test.dsr), err)
			}
			if nerr != nil {
				nerr.LongError()
			}
		}
	}
}

func mockFuseEcho(c, quit chan int, socket string, bytesToReturn int) {
	l, err := net.Listen("unix", socket)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	c <- 0

	done := false
	for !done {
		select {
		case <-quit:
			done = true
			break
		default:
			l.(*net.UnixListener).SetDeadline(time.Now().Add(time.Millisecond))
			fd, err := l.Accept()
			if err != nil {
				if oe, ok := err.(*net.OpError); ok {
					if !oe.Timeout() || !oe.Temporary() {
						log.Fatal("accept error:", err)
					}
				} else {
					log.Fatal("accept error:", err)
				}
			} else {
				buf := make([]byte, 512)
				nr, err := fd.Read(buf)
				if err == nil {
					if nr < bytesToReturn {
						bytesToReturn = nr
					}
					data := buf[0:bytesToReturn]
					_, err = fd.Write(data)
					if err != nil {
						log.Fatal("Write: ", err)
					}
				}
				fd.Close()
			}
		}
	}
	l.Close()
	c <- 0
}

var nagOS = false

func mockRSREcho(wg *sync.WaitGroup, t *testing.T, dsr *DoSendRecver, req *nuvo.Cmd, bytesToReturn int, succeed bool, socket string, timeout bool) {
	defer wg.Done()
	var reply *nuvo.Cmd
	var err error
	if dsr != nil {
		reply, err = dsr.doSendRecv(socket, req)
	} else {
		if timeout {
			reply, err = RealSendRecvTimeout(socket, req, 20, 5)
		} else {
			reply, err = RealSendRecv(socket, req)
		}
	}
	if succeed {
		if reply == nil || err != nil {
			if oe, ok := err.(*net.OpError); ok {
				if !oe.Temporary() {
					if runtime.GOOS == "darwin" {
						nagOS = true
					} else {
						t.Error("SendRecv did not succeed", bytesToReturn, err)
					}
				} else if !timeout {
					t.Error("SendRecv improperly timed out", bytesToReturn, err)
				}
			} else {
				t.Error("SendRecv did not succeed", bytesToReturn, err)
			}
		}
	} else {
		if err == nil {
			t.Error("SendRecv succeeded", bytesToReturn, err)
		}
		t.Log(err)
		if dsr != nil {
			ei, ok := err.(ErrorInt)
			if !ok {
				t.Error("Wrapped SendRecv did not return ErrorInt")
			}
			if !ei.NotInitialized() { // artifact of the UT that it returns a fatal error
				t.Error("Wrapped SendRecv did not return NotInitialized() => true")
			}
		}
	}
}

func testRSRint(t *testing.T, dsr *DoSendRecver, req *nuvo.Cmd, bytesToReturn int, succeed bool, serverRunning bool, num int, timeout bool) {
	socket := "/tmp/test_socket"
	var c = make(chan int)
	var quit = make(chan int)
	if serverRunning {
		go mockFuseEcho(c, quit, socket, bytesToReturn)
		<-c
	}

	var wg sync.WaitGroup
	for i := 0; i < num; i++ {
		wg.Add(1)
		go mockRSREcho(&wg, t, dsr, req, bytesToReturn, succeed, socket, true)
	}

	wg.Wait()
	if serverRunning {
		quit <- 0
		<-c
	}
}

func TestRealSendRecv(t *testing.T) {
	uuid := "my uuid"
	path := "my_path"
	devType := nuvo.UseDevice_HDD
	req := buildUseDevice(uuid, path, devType)
	data, err := proto.Marshal(req)
	if err != nil {
		log.Panic("marshaling error: ", err)
	}
	length := len(data) + 4
	testRSRint(t, nil, req, length, true, true, 1, false)
	testRSRint(t, nil, req, 6, false, true, 1, false)       // illegal tag 0 (wire type 0)
	testRSRint(t, nil, req, 3, false, true, 1, false)       // less than 4 bytes in reply
	testRSRint(t, nil, req, length, false, false, 1, false) // dial unix /tmp/test_socket: connect: no such file or directory
	testRSRint(t, nil, req, length, true, true, 500, false) // Many requests, don't timeout
	testRSRint(t, nil, req, length, true, true, 500, true)  // Many requests, do timeout
	if nagOS {
		fmt.Printf("If you were running on our target OS you wouldn't see this.\n")
	}
}

func TestNewNuvoVM(t *testing.T) {
	nvm := NewNuvoVM("sock")
	dsr, ok := nvm.(*DoSendRecver) // returns a wrapped send receive
	if !ok {
		t.Errorf("NewNuvoVM does not return DoSendRecv")
	}
	if dsr.socket != "sock" {
		t.Errorf("NewNuvoVM did not save the socket")
	}
	// unabashedly adapted from TestRealSendRecv
	uuid := "my uuid"
	path := "my_path"
	devType := nuvo.UseDevice_SSD
	req := buildUseDevice(uuid, path, devType)
	data, err := proto.Marshal(req)
	if err != nil {
		log.Panic("marshaling error: ", err)
	}
	length := len(data) + 4
	testRSRint(t, dsr, req, length, true, true, 1, false)
	testRSRint(t, dsr, req, 6, false, true, 1, false)       // illegal tag 0 (wire type 0)
	testRSRint(t, dsr, req, 3, false, true, 1, false)       // less than 4 bytes in reply
	testRSRint(t, dsr, req, length, false, false, 1, false) // dial unix /tmp/test_socket: connect: no such file or directory
}

func TestNewNuvoVMWithNotInitializedAlerter(t *testing.T) {
	assert := assert.New(t)
	cb := &fakeCB{}
	nvm := NewNuvoVMWithNotInitializedAlerter("sock", cb)
	dsr, ok := nvm.(*DoSendRecver) // returns a wrapped send receive
	if !ok {
		t.Errorf("NewNuvoVM does not return DoSendRecv")
	}
	if dsr.socket != "sock" {
		t.Errorf("NewNuvoVM did not save the socket")
	}
	// unabashedly adapted from TestRealSendRecv
	uuid := "my uuid"
	path := "my_path"
	devType := nuvo.UseDevice_SSD
	req := buildUseDevice(uuid, path, devType)
	data, err := proto.Marshal(req)
	if err != nil {
		log.Panic("marshaling error: ", err)
	}
	length := len(data) + 4
	testRSRint(t, dsr, req, length, true, true, 1, false)
	assert.Equal(0, cb.Called)
	testRSRint(t, dsr, req, 6, false, true, 1, false) // illegal tag 0 (wire type 0)
	assert.Equal(1, cb.Called)
	testRSRint(t, dsr, req, 3, false, true, 1, false) // less than 4 bytes in reply
	assert.Equal(2, cb.Called)
	testRSRint(t, dsr, req, length, false, false, 1, false) // dial unix /tmp/test_socket: connect: no such file or directory
	assert.Equal(3, cb.Called)
}

type fakeCB struct {
	Called int
}

func (cb *fakeCB) NuvoNotInitializedError(err error) {
	cb.Called++
}

func TestLunPath(t *testing.T) {

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(nil, "sock")
	if path.Join("A", "B") != dsr.LunPath("A", "B") {
		t.Error("LunPath failed")
	}
}

func TestLunPathMultiTrue(t *testing.T) {
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	var cap Capabilities
	cap.Multifuse = true
	nodeCapabilities["sock"] = cap

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(nil, "sock")
	if path.Join("A", "B", "vol") != dsr.LunPath("A", "B") {
		t.Error("LunPath failed")
	}
}

func TestLunPathMultiFalse(t *testing.T) {
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	var cap Capabilities
	cap.Multifuse = false
	nodeCapabilities["sock"] = cap

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(nil, "sock")
	if path.Join("A", "B") != dsr.LunPath("A", "B") {
		t.Error("LunPath failed")
	}
}

func TestLunPathMultiPanic(t *testing.T) {
	assert := assert.New(t)
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(nil, "sock")
	assert.Panics(func() { dsr.LunPath("A", "B") })
}

func makeCapabilitiesReplyMulti(req *nuvo.Cmd) *nuvo.Cmd {
	data, _ := proto.Marshal(req)
	var reply = new(nuvo.Cmd)
	_ = proto.Unmarshal(data, reply)
	reply.MsgType = nuvo.Cmd_CAPABILITIES_REPLY.Enum()
	return reply
}

func makeCapabilitiesReplyMultiFalseSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCapabilitiesReplyMulti(req)
	reply.Capabilities.Multifuse = proto.Bool(false)
	return reply, nil
}

func TestCapabilitiesMultiFalse(t *testing.T) {
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(makeCapabilitiesReplyMultiFalseSendRecv, "sock")
	dsr.GetCapabilities()

	if nodeCapabilities["sock"].Multifuse != false {
		t.Error("Capabilities returned wrong multifuse")
	}
}

func makeCapabilitiesReplyMultiTrueSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCapabilitiesReplyMulti(req)
	reply.Capabilities.Multifuse = proto.Bool(true)
	return reply, nil
}

func TestCapabilitiesMultiTrue(t *testing.T) {
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(makeCapabilitiesReplyMultiTrueSendRecv, "sock")
	dsr.GetCapabilities()

	if nodeCapabilities["sock"].Multifuse != true {
		t.Error("Capabilities returned wrong multifuse")
	}
}

func makeCapabilitiesReplyWrongTypeSendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCapabilitiesReplyMulti(req)
	reply.MsgType = nuvo.Cmd_CAPABILITIES_REQ.Enum()
	return reply, nil
}

func TestCapabilitiesWrongType(t *testing.T) {
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(makeCapabilitiesReplyWrongTypeSendRecv, "sock")
	_, err := dsr.GetCapabilities()
	if err == nil {
		t.Error("Capabilities should have failed")
	}
}

func makeCapabilitiesReplyMissingBodySendRecv(socket string, req *nuvo.Cmd) (*nuvo.Cmd, error) {
	reply := makeCapabilitiesReplyMulti(req)
	reply.Capabilities = nil
	return reply, nil
}

func TestCapabilitiesMissingBody(t *testing.T) {
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(makeCapabilitiesReplyMissingBodySendRecv, "sock")
	_, err := dsr.GetCapabilities()
	if err == nil {
		t.Error("Capabilities should have failed")
	}
}

func TestCapabilitiesSendRcvError(t *testing.T) {
	defer func() {
		nodeCapabilities = nil
	}()
	nodeCapabilities = make(map[string]Capabilities)

	// Don't need a routine because this never sends an actual protobuf message.
	dsr := NewSendRecv(buildReplyErrorSendRecv, "sock")
	_, err := dsr.GetCapabilities()
	if err == nil {
		t.Error("Capabilities should have failed")
	}
}
