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


package gcsdk

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

func TestAPI(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	serviceAccount := `{ "type": "service_account", "project_id": "test-proj-1" }`
	api := New()
	assert.NotNil(api)

	cs, err := api.NewComputeService(ctx, option.WithCredentialsJSON([]byte("{")))
	assert.Nil(cs)
	assert.Regexp("unexpected end of JSON input", err)

	cs, err = api.NewComputeService(ctx, option.WithCredentialsJSON([]byte(serviceAccount)))
	assert.NotNil(cs)
	assert.NoError(err)
	_, ok := cs.(*service)
	assert.True(ok)

	d := cs.Disks()
	assert.NotNil(d)
	_, ok = d.(*disksService)
	assert.True(ok)

	dDel := d.Delete("project", "zone", "disk-1")
	assert.NotNil(dDel)

	ctx, cancelFn := context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(dDel, dDel.Context(ctx))

	// cannot actually test success, just test failure
	op, err := dDel.Do()
	assert.Nil(op)
	assert.Regexp("context deadline exceeded", err)

	dGet := d.Get("project", "zone", "disk-1")
	assert.NotNil(dGet)

	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(dGet, dGet.Context(ctx))

	// cannot actually test success, just test failure
	disk, err := dGet.Do()
	assert.Nil(disk)
	assert.Regexp("context deadline exceeded", err)

	dInsert := d.Insert("project", "zone", &compute.Disk{})
	assert.NotNil(dInsert)

	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(dInsert, dInsert.Context(ctx))

	// cannot actually test success, just test failure
	op, err = dInsert.Do()
	assert.Nil(op)
	assert.Regexp("context deadline exceeded", err)

	dList := d.List("project", "zone")
	assert.NotNil(dList)
	assert.Equal(dList, dList.Filter("name = vid"))

	// cannot actually test success, just test failure
	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	err = dList.Pages(ctx, nil)
	assert.Regexp("context deadline exceeded", err)

	dSL := d.SetLabels("project", "zone", "disk-1", &compute.ZoneSetLabelsRequest{})
	assert.NotNil(dSL)

	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(dSL, dSL.Context(ctx))

	// cannot actually test success, just test failure
	op, err = dSL.Do()
	assert.Nil(op)
	assert.Regexp("context deadline exceeded", err)

	i := cs.Instances()
	assert.NotNil(i)
	_, ok = i.(*instancesService)
	assert.True(ok)

	iAttach := i.AttachDisk("project", "zone", "instance-1", &compute.AttachedDisk{})
	assert.NotNil(iAttach)

	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(iAttach, iAttach.Context(ctx))
	assert.Equal(iAttach, iAttach.ForceAttach(true))

	// cannot actually test success, just test failure
	op, err = iAttach.Do()
	assert.Nil(op)
	assert.Regexp("context deadline exceeded", err)

	iDetach := i.DetachDisk("project", "zone", "instance-1", "device-name")
	assert.NotNil(iDetach)

	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(iDetach, iDetach.Context(ctx))

	// cannot actually test success, just test failure
	op, err = iDetach.Do()
	assert.Nil(op)
	assert.Regexp("context deadline exceeded", err)

	o := cs.ZoneOperations()
	assert.NotNil(o)
	_, ok = o.(*zoneOperationsService)
	assert.True(ok)

	oGet := o.Get("project", "zone", "op")
	assert.NotNil(oGet)

	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(oGet, oGet.Context(ctx))

	// cannot actually test success, just test failure
	op, err = oGet.Do()
	assert.Nil(op)
	assert.Regexp("context deadline exceeded", err)

	z := cs.Zones()
	assert.NotNil(z)
	_, ok = z.(*zonesService)
	assert.True(ok)

	zGet := z.Get("project", "zone")
	assert.NotNil(zGet)

	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	assert.Equal(zGet, zGet.Context(ctx))

	// cannot actually test success, just test failure
	zone, err := zGet.Do()
	assert.Nil(zone)
	assert.Regexp("context deadline exceeded", err)

	ctx = context.Background()
	sc, err := api.NewStorageClient(ctx, option.WithCredentialsJSON([]byte("{")))
	assert.Nil(sc)
	assert.Regexp("unexpected end of JSON input", err)

	sc, err = api.NewStorageClient(ctx, option.WithCredentialsJSON([]byte(serviceAccount)))
	assert.NotNil(sc)
	assert.NoError(err)
	_, ok = sc.(*storageClient)
	assert.True(ok)

	b := sc.Bucket("my-bucket")
	assert.NotNil(b)
	_, ok = b.(*bucketHandle)
	assert.True(ok)

	// cannot actually test success, just test failure
	ctx, cancelFn = context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	err = b.Create(ctx, "project", &storage.BucketAttrs{})
	assert.Regexp("context deadline exceeded", err)
}
