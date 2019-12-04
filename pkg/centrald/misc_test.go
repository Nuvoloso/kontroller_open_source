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


package centrald

import (
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/stretchr/testify/assert"
)

func TestValidateValueType(t *testing.T) {
	assert := assert.New(t)

	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds})

	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "STRING", Value: "5"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "INT", Value: "5"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "SECRET", Value: "credential"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "1d"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "1.4d"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "10us"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "1000.2ns"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "5.0us"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "0.3ms"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "4.4s"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "1.1m"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "STRING", Value: "N/A"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "7.7h"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "PERCENTAGE", Value: "99.999%"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "INT", Value: "N/A"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "SECRET", Value: "N/A"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "N/A"}))
	assert.NoError(app.ValidateValueType(models.ValueType{Kind: "PERCENTAGE", Value: "N/A"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "INT", Value: "notAnInt"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "-10us"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "10.0rocks"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "10"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "-10d"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "DURATION", Value: "10dwaynejohnsond"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "PERCENTAGE", Value: "110%"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "PERCENTAGE", Value: "110"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "PERCENTAGE", Value: "-99.999%"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "PERCENTAGE", Value: "-99.hobbs999%"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "PERCENTAGE", Value: "99.999"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "float", Value: "unsupported kind"}))
	assert.Error(app.ValidateValueType(models.ValueType{Kind: "", Value: "unsupported kind"}))
}

func TestConvertDurationStringToMicroSeconds(t *testing.T) {
	assert := assert.New(t)

	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds})

	// Validate the return values
	us, _ := app.ConvertDurationStringToMicroSeconds("1d")
	assert.True(us == 86400000000)
	us, _ = app.ConvertDurationStringToMicroSeconds("10us")
	assert.True(us == 10)
	us, _ = app.ConvertDurationStringToMicroSeconds("1000.2ns")
	assert.True(us == 1)
	us, _ = app.ConvertDurationStringToMicroSeconds("5.0us")
	assert.True(us == 5)
	us, _ = app.ConvertDurationStringToMicroSeconds("3.3ms")
	assert.True(us == 3300)
	us, _ = app.ConvertDurationStringToMicroSeconds("4.4s")
	assert.True(us == 4400000)
	us, _ = app.ConvertDurationStringToMicroSeconds("1.1m")
	assert.True(us == 66000000)
	us, _ = app.ConvertDurationStringToMicroSeconds("7.7h")
	assert.True(us == 27720000000)
	us, _ = app.ConvertDurationStringToMicroSeconds("1.4d")
	assert.True(us == 120960000000)

	// Errors are tested in TestValidateValueType
}

func TestConvertPercentageStringToFloat(t *testing.T) {
	assert := assert.New(t)

	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds})

	// Validate the return values
	f, _ := app.ConvertPercentageStringToFloat("99.999%")
	assert.True(f == 99.999)
	f, _ = app.ConvertPercentageStringToFloat("99.995%")
	assert.True(f == 99.995)
	f, _ = app.ConvertPercentageStringToFloat("99.9%")
	assert.True(f == 99.9)
	f, _ = app.ConvertPercentageStringToFloat("99%")
	assert.True(f == 99)

	// Errors are tested in TestValidateValueType
}
