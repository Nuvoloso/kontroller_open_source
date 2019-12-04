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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

//ConvertPercentageStringToFloat takes a string in the format of a percentage and returns the percentage as a float
// NOTE: We use -1.0 as the value returned with error
// NOTE2: We do not allow negative percentages. I haven't seen those since my college math grades
// NOTE3: We do not allow > 100%. This is not a bad sports movie.
func (app *AppCtx) ConvertPercentageStringToFloat(value string) (float64, error) {
	var f float64
	var err error

	if value[len(value)-1:] != "%" {
		return -1.0, fmt.Errorf("expected PERCENTAGE value but got %s that did not end with '%%' character", value)
	}
	if f, err = strconv.ParseFloat(value[0:len(value)-1], 64); err != nil {
		return -1.0, fmt.Errorf("expected PERCENTAGE value but got %s with a non-numerical value before the %%", value)
	}
	if f < 0 {
		return -1.0, fmt.Errorf("expected PERCENTAGE value but got %s with a negative percentage", value)
	}
	if f > 100 {
		return -1.0, fmt.Errorf("expected PERCENTAGE value but got %s which is over 100%%", value)
	}
	return f, err
}

// ConvertDurationStringToMicroSeconds takes a string in the format of a duration and returns the number of microseconds
// NOTE: Duration only covers up to hours (h, m, s, ms, us, ns), so we need a special check for days
// NOTE2: We use MAXINT (1<<64 - 1) as the return value paired with error because '0' could mean "Synchronous Mirror"
// NOTE3: We do not allow negative durations. Time travel is scary.
func (app *AppCtx) ConvertDurationStringToMicroSeconds(value string) (uint64, error) {
	//First, the days case which we have to handle specially
	if strings.HasSuffix(value, "d") {
		f, err := strconv.ParseFloat(value[0:len(value)-1], 64)
		if err != nil {
			return 1<<64 - 1, fmt.Errorf("expected DURATION value but got %s that has a non-numerical value before the unit", value)
		}
		if f < 0 {
			return 1<<64 - 1, fmt.Errorf("expected positive DURATION value but got %s that has a negative value", value)
		}
		// Convert days into microseconds (* hours/day, minutes/hour, seconds/minute, microseconds/second)
		// Adding 0.5 because the conversion to uint64 doesn't round, it truncates. 1 us doesn't matter much, but the UTs care
		return (uint64)(f*24*60*60*1000000 + 0.5), err
	}

	// All other cases
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("expected DURATION value but got %s that did not end with a duration unit", value)
	}
	if duration < 0 {
		return 1<<64 - 1, fmt.Errorf("expected positive DURATION value but got %s that has a negative value", value)
	}
	// Duration uses nanoseconds, so convert to microseconds
	return (uint64)(duration / 1000), err
}

// ValidateValueType verifies the Kind and, in the case of INT, DURATION and PERCENTAGE the value, of a ValueType
func (app *AppCtx) ValidateValueType(o models.ValueType) error {
	if o.Value == "N/A" {
		return nil
	}

	if o.Kind == "INT" {
		if _, err := strconv.Atoi(o.Value); err != nil {
			return fmt.Errorf("expected INT value but got %s", o.Value)
		}
	} else if o.Kind == "PERCENTAGE" {
		if _, err := app.ConvertPercentageStringToFloat(o.Value); err != nil {
			return err
		}
	} else if o.Kind == "DURATION" {
		if _, err := app.ConvertDurationStringToMicroSeconds(o.Value); err != nil {
			return err
		}
	} else if o.Kind != "STRING" && o.Kind != "SECRET" {
		return fmt.Errorf("expected INT, STRING or SECRET kind but got %s", o.Kind)
	}
	return nil
}
