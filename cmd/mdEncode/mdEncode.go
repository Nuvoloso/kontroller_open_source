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
	"flag"
	"fmt"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
)

var mdFileName string

func init() {
	flag.StringVar(&mdFileName, "f", "/tmp/mdfile", "Metadatafile")
}

func main() {
	flag.Parse()

	keybuf := [32]byte{
		0, 1, 2, 3, 4, 5, 6, 7,
		0, 1, 2, 3, 4, 5, 6, 7,
		0, 1, 2, 3, 4, 5, 6, 7,
		0, 1, 2, 3, 4, 5, 6, 7,
	}
	keyslice := keybuf[:]
	keyP := &keyslice

	mdReader, err := metadata.NewMetaReader(mdFileName)
	if err != nil {
		fmt.Printf("Reader: %s\n", err.Error())
		return
	}

	fmt.Println(metadata.Header())
	mde := metadata.NextEntry(mdReader, nil)

	for mde.Offset() != metadata.Infinity {
		fmt.Printf("%s\n", mde.Encode(keyP))
		mde = metadata.NextEntry(mdReader, nil)
	}
}
