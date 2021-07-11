/*
Copyright 2021 The Kubernetes Authors.

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

package nfs

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestGetVersion(t *testing.T) {
	version := GetVersion(DefaultDriverName)

	expected := VersionInfo{
		DriverName:    DefaultDriverName,
		DriverVersion: "N/A",
		GitCommit:     "N/A",
		BuildDate:     "N/A",
		GoVersion:     runtime.Version(),
		Compiler:      runtime.Compiler,
		Platform:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	if !reflect.DeepEqual(version, expected) {
		t.Errorf("Unexpected error. \n Expected: %v \n Found: %v", expected, version)
	}

}

func TestGetVersionYAML(t *testing.T) {
	resp, err := GetVersionYAML("")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	versionInfo := GetVersion("")
	marshalled, _ := yaml.Marshal(&versionInfo)
	expected := strings.TrimSpace(string(marshalled))

	if resp != expected {
		t.Fatalf("Unexpected error. \n Expected:%v\nFound:%v", expected, resp)
	}
}
