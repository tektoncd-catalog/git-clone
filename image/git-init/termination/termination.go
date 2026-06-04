/*
Copyright 2019 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package termination writes Tekton results to the pod termination message file.
package termination

import (
	"encoding/json"
	"fmt"
	"os"
)

const maxLength = 4096

// Result is a key/value pair written to the termination message.
type Result struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// WriteMessage writes results as JSON to the termination message path.
func WriteMessage(path string, results []Result) error {
	// If the file already exists, merge with existing entries.
	if data, err := os.ReadFile(path); err == nil {
		var existing []Result
		if err := json.Unmarshal(data, &existing); err == nil {
			results = append(existing, results...)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	out, err := json.Marshal(results)
	if err != nil {
		return err
	}
	if len(out) > maxLength {
		return fmt.Errorf("termination message is above max allowed size %d", maxLength)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.Write(out); err != nil {
		return err
	}
	return f.Sync()
}
