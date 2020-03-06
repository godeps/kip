/*
Copyright 2020 Elotl Inc.

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

package main

import (
	"os"

	"contrib.go.opencensus.io/exporter/ocagent"
	opencensuscli "github.com/virtual-kubelet/node-cli/opencensus"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"go.opencensus.io/trace"
)

func initOCAgent(c *opencensuscli.Config) (trace.Exporter, error) {
	agentOpts := append([]ocagent.ExporterOption{}, ocagent.WithServiceName(c.ServiceName))

	if endpoint := os.Getenv("OCAGENT_ENDPOINT"); endpoint != "" {
		agentOpts = append(agentOpts, ocagent.WithAddress(endpoint))
	} else {
		return nil, errdefs.InvalidInput("must set endpoint address in OCAGENT_ENDPOINT")
	}

	switch os.Getenv("OCAGENT_INSECURE") {
	case "0", "no", "n", "off", "":
	case "1", "yes", "y", "on":
		agentOpts = append(agentOpts, ocagent.WithInsecure())
	default:
		return nil, errdefs.InvalidInput("invalid value for OCAGENT_INSECURE")
	}

	return ocagent.NewExporter(agentOpts...)
}
