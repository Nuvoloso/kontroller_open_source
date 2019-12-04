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


package endpoint

// This test is disabled (by putting a "_" on the front of the name)
// so that it will not generate a large about of cloud traffic regularly.
// If you would like to run this test, it can be re-enabled by renaming the
// test to remove "_" from the beginning of the name.
//  mv _catalog_test.go catalog_test.go
//
// This test may require additional cloud service setup to work. Try it
// and the errors should guide you into setting it up properly.

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/stats"
)

var domain = "catalog_test_domain"

func readGoogleCred(fn string) (string, error) {

	credFileName := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	f, err := os.Open(credFileName)
	if err != nil {
		return "", errors.New("Cred File Open: " + err.Error())
	}
	defer f.Close()

	credBytes, err := ioutil.ReadAll(f)
	if err != nil {
		return "", errors.New("Cred File Read: " + err.Error())
	}

	return string(credBytes), nil
}

func makeArgs(epType string, domain string, clear bool) (Arg, error) {

	var passPhrase string
	if !clear {
		passPhrase = "This is my pass phrase"
	}

	epa := Arg{
		Type:    epType,
		Purpose: "Manipulation",
	}

	switch epType {
	case TypeAzure:
		bucketName := os.Getenv("AZURE_BUCKET")
		if bucketName == "" {
			return Arg{}, errors.New("AZURE_BUCKET not in environment")
		}
		epa.Args.Azure = AzArgs{
			BucketName: bucketName,
			Domain:     domain,
			PassPhrase: passPhrase,
		}
	case TypeDir:
		epa.Args.Dir = DirArgs{
			Directory:  "/tmp/dirtestdir",
			Domain:     domain,
			PassPhrase: passPhrase,
		}
	case TypeGoogle:
		credString, err := readGoogleCred(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
		if err != nil {
			return Arg{}, errors.New("Google Cred File: " + err.Error())
		}
		bucketName := os.Getenv("GOOGLE_BUCKET")
		if bucketName == "" {
			return Arg{}, errors.New("GOOGLE_BUCKET not in environment")
		}
		epa.Args.Google = GcArgs{
			BucketName: bucketName,
			Domain:     domain,
			PassPhrase: passPhrase,
			Cred:       credString,
		}
	case TypeAWS:
		bucketName := os.Getenv("AWS_BUCKET")
		if bucketName == "" {
			return Arg{}, errors.New("AWS_BUCKET not in environment")
		}
		region := os.Getenv("AWS_REGION")
		if region == "" {
			return Arg{}, errors.New("AWS_REGION not in environment")
		}
		accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
		if accessKeyID == "" {
			return Arg{}, errors.New("AWS_ACCESS_KEY_ID not in environment")
		}
		secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		if secretAccessKey == "" {
			return Arg{}, errors.New("AWS_SECRET_ACCESS_KEY not in environment")
		}

		epa.Args.S3 = S3Args{
			BucketName:      bucketName,
			Region:          region,
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			Domain:          domain,
			PassPhrase:      passPhrase,
		}
	case TypeFile:
		epa.Args.File = FileArgs{
			FileName:      "Bogus",
			DestPreZeroed: true,
		}
	default:
		return epa, errors.New("Illegal Endpoint Type")
	}

	return epa, nil
}

func TestCatalog(t *testing.T) {
	epTypes := []string{TypeDir, TypeAWS, TypeAzure, TypeGoogle}
	domains := []string{"Domain", ""}
	var mystats stats.Stats

	for _, epType := range epTypes {
		for j := 0; j < 2; j++ {

			var clear bool
			if j == 1 {
				clear = false
			} else {
				clear = true
			}

			for _, myDomain := range domains {
				fmt.Printf("Type: %s Domain: %s clear: %v                  
", epType, myDomain, clear)
				epa, err := makeArgs(epType, myDomain, clear)
				if err != nil {
					t.Error(epType + ": " + err.Error())
					continue
				}

				ep, err := SetupEndpoint(&epa, &mystats, 1)

				if myDomain == "" {
					if err == nil {
						t.Error(epType + ": " + "setup endpoint should fail with no domain")
					}
					continue
				}

				if err != nil {
					t.Error(epType + ": " + "Can't setup endpoint: " + err.Error())
				}

				data := []byte{1, 2, 3, 4}
				objNames := []string{"Object", "Object1"}

				for _, obj := range objNames {
					ep.StoreData(obj, data)

					buf := make([]byte, len(data))
					n, err := ep.RetreiveData(obj, buf[:])
					if err != nil {
						t.Error(epType + ": " + "RetreivedData Failed: " + err.Error())
					}
					if n != len(data) {
						t.Error(epType + ": " + "Retreived data size doesn't match written size")
					}
					for i := 0; i < n; i++ {
						if data[i] != buf[i] {
							t.Error(epType + ": " + "Retreived clear data doesn't match written data")
						}
					}
				}

				var names []string
				name, ctx, err := ep.GetListNext(nil)
				if err != nil {
					t.Error("First GetListNext: " + err.Error())
				}
				for ctx != nil {
					names = append(names, name)
					name, ctx, err = ep.GetListNext(ctx)
					if err != nil {
						t.Error("First GetListNext: " + err.Error())
					}
				}

				for _, expectedName := range objNames {
					found := false
					for _, n := range names {
						if n == expectedName {
							found = true
						}
					}
					if found == false {
						t.Error(epType + ": " + "GetListNext didn't return the expected object name")
					}
				}
				type myStruct struct {
					Field1 int
					Field2 bool
					Field3 string
				}

				ms := myStruct{Field1: 2, Field2: true, Field3: "A String"}
				err = PutJSONCatalog(ep, "myStruct.JSON", &ms)
				if err != nil {
					t.Error(epType + ": " + "putJSONCatalog " + err.Error())
				}

				ms2 := myStruct{}
				err = GetJSONCatalog(ep, "myStruct.JSON", &ms2)
				if err != nil {
					t.Error(epType + ": " + "getJSONCatalog " + err.Error())
				}
				if ms != ms2 {
					t.Error(epType + ": Put/GetJSONCatalog mismatch")
				}
				ep.Done(true)
			}
		}
	}
}
