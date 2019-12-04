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
	"strconv"

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/fogleman/gg"
)

type rgb struct {
	r float64
	g float64
	b float64
}

const (
	parcelPadding  = 4.0
	segmentSpacing = 2.0
	parcelSpacing  = 4.0
	devicePadding  = 3.0
	deviceSpacing  = 4.0
	volumePadding  = 4.0
	textPadding    = 5.0

	segmentSize    = 30.0
	segmentsAcross = 12.0
)

var textColor = rgb{0, 0, 0}
var parcelColor = rgb{0.6, 0.6, 0.6}
var deviceColor = rgb{0.8, 0.8, 0.8}
var backgroundColor = rgb{1.0, 1.0, 1.0}

func rgbSegment(fullness float64) rgb {
	var low rgb
	var high rgb
	var t float64
	if fullness < 0 {
		fullness = 0
	}
	if fullness > 1 {
		fullness = 1
	}
	if fullness <= 0.25 {
		low.r = 0xe4 / float64(0xff)
		high.r = 0xff / float64(0xff)
		low.g = 0xff / float64(0xff)
		high.g = 0xe8 / float64(0xff)
		low.b = 0x7a / float64(0xff)
		high.b = 0x1a / float64(0xff)
		t = fullness / 0.25
	} else if fullness <= 0.5 {
		low.r = 0xff / float64(0xff)
		high.r = 0xff / float64(0xff)
		low.g = 0xe8 / float64(0xff)
		high.g = 0xbd / float64(0xff)
		low.b = 0x1a / float64(0xff)
		high.b = 0x00 / float64(0xff)
		t = (fullness - 0.25) / 0.25
	} else if fullness <= 0.75 {
		low.r = 0xff / float64(0xff)
		high.r = 0xff / float64(0xff)
		low.g = 0xbd / float64(0xff)
		high.g = 0xa0 / float64(0xff)
		low.b = 0x00 / float64(0xff)
		high.b = 0x00 / float64(0xff)
		t = (fullness - 0.5) / 0.25
	} else {
		low.r = 0xff / float64(0xff)
		high.r = 0xfc / float64(0xff)
		low.g = 0xa0 / float64(0xff)
		high.g = 0x7f / float64(0xff)
		low.b = 0x00 / float64(0xff)
		high.b = 0x00 / float64(0xff)
		t = (fullness - 0.75) / 0.25
	}
	return rgb{low.r*(1-t) + high.r*t,
		low.g*(1-t) + high.g*t,
		low.b*(1-t) + high.b*t}
}

func drawSegment(dc *gg.Context, seg nuvoapi.Segment) {
	segRGB := rgbSegment(seg.Fullness)
	dc.SetRGB(segRGB.r, segRGB.g, segRGB.b)
	dc.DrawRectangle(-segmentSize/2, -segmentSize/2, segmentSize, segmentSize)
	dc.Fill()
	dc.SetRGB(textColor.r, textColor.g, textColor.b)
	var s string
	if seg.Log {
		s = "U"
	} else if seg.Reserved {
		s = "R"
	} else if seg.Age > 0 {
		s = strconv.FormatInt(int64(seg.RelativeAge*10), 10)
	} else {
		s = ""
	}
	dc.DrawStringAnchored(s, 0, 0, 0.5, 0.5)
}

func parcelWidth() float64 {
	return 2*parcelPadding + segmentSize*segmentsAcross + (segmentsAcross-1)*segmentSpacing
}

func drawParcel(dc *gg.Context, parcel nuvoapi.Parcel, draw bool) float64 {
	stepX := segmentSize + segmentSpacing
	startX := parcelPadding + segmentSize/2

	stepY := segmentSize + segmentSpacing
	startY := parcelPadding + segmentSize/2

	maxY := (len(parcel.Segments) - 1) / segmentsAcross
	stringY := startY + float64(maxY+1)*stepY + 1

	s := fmt.Sprintf("Idx: %d, state: %s, seg_size: %dKB", parcel.ParcelIndex, parcel.State, parcel.SegmentSize/1024)
	_, stringH := dc.MeasureString(s)

	h := stringY + stringH

	if draw == true {
		dc.SetRGB(parcelColor.r, parcelColor.g, parcelColor.b)
		dc.DrawRectangle(0, 0, parcelWidth(), h)
		dc.Fill()

		for i, segment := range parcel.Segments {
			x := i % segmentsAcross
			y := i / segmentsAcross
			dc.Push()
			dc.Translate(startX+float64(x)*stepX, startY+float64(y)*stepY)
			drawSegment(dc, segment)
			dc.Pop()
		}
		dc.SetRGB(textColor.r, textColor.g, textColor.b)
		dc.DrawString(s, 4, stringY)
	}
	return h
}

func deviceWidth() float64 {
	return parcelWidth() + 2*devicePadding
}

func drawDevice(dc *gg.Context, device nuvoapi.Device, draw bool, short bool) float64 {
	s1 := fmt.Sprintf("Parcel size: %dKB", device.ParcelSize/1024)
	s2 := fmt.Sprintf("Target parcels: %d (%d used)", device.TargetParcels, device.AllocedParcels)
	s3 := fmt.Sprintf("Free segments: %d", device.FreeSegments)
	s4 := fmt.Sprintf("Blocks used: %d", device.BlocksUsed)
	_, stringH1 := dc.MeasureString(s1)
	_, stringH2 := dc.MeasureString(s2)
	_, stringH3 := dc.MeasureString(s3)
	_, stringH4 := dc.MeasureString(s4)
	calcH := stringH1 + stringH2 + stringH3 + stringH4 + 4*textPadding

	if draw {
		dc.SetRGB(deviceColor.r, deviceColor.g, deviceColor.b)
		dc.DrawRectangle(0, 0, deviceWidth(), drawDevice(dc, device, false, short))
		dc.Fill()
		dc.SetRGB(0, 0, 0)
		dc.Push()
		dc.Translate(0, stringH1+textPadding)
		dc.DrawString(s1, 4, 0)
		dc.Translate(0, stringH2+textPadding)
		dc.DrawString(s2, 4, 0)
		dc.Translate(0, stringH3+textPadding)
		dc.DrawString(s3, 4, 0)
		dc.Translate(0, stringH4+textPadding)
		dc.DrawString(s4, 4, 0)
	}
	calcH += devicePadding
	if draw {
		dc.Translate(devicePadding, devicePadding)
	}
	if !short {
		for _, parcel := range device.Parcels {
			parcelH := drawParcel(dc, parcel, draw)
			calcH += parcelH + parcelPadding
			if draw {
				dc.Translate(0, parcelH+parcelPadding)
			}
		}
	}
	if draw {
		dc.Pop()
	}
	return calcH
}

func drawDevices(devices []nuvoapi.Device, minW, minH int, name string, short bool) {
	// pick width and height based on something.
	dc := gg.NewContext(minW, minH)
	numDevices := len(devices)
	if numDevices < 4 {
		numDevices = 4
	}
	maxW := float64(minW)
	w := float64(numDevices) * (deviceWidth() + deviceSpacing)
	if w > maxW {
		maxW = w
	}
	maxH := float64(minH)
	for _, device := range devices {
		h := drawDevice(dc, device, false, short)
		if h > maxH {
			maxH = h
		}
	}
	dc = gg.NewContext(int(maxW), int(maxH))
	dc.LoadFontFace("/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf", 16)
	dc.SetRGB(backgroundColor.r, backgroundColor.g, backgroundColor.b)
	dc.Clear()
	dc.Translate(volumePadding, volumePadding)
	for _, device := range devices {
		drawDevice(dc, device, true, short)
		dc.Translate(deviceWidth()+deviceSpacing, 0)
	}
	tmpName := name + ".tmp"
	dc.SavePNG(tmpName)
	os.Rename(tmpName, name)
}
