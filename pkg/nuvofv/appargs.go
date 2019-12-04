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


package nuvofv

// AppArgs are the starting arguments for the application
type AppArgs struct {
	Verbose        []bool `short:"v" long:"verbose" description:"Show debug information."`
	SocketPath     string `long:"socket-path" description:"The management service unix domain socket" default:"/var/run/nuvoloso/nvagentd.sock"`
	NodeID         string `long:"node-id" description:"The Nuvoloso node ID of this node"`
	NodeIdentifier string `long:"node-identifier" description:"The CSP node identifier of this node"`
	ClusterID      string `long:"cluster-id" description:"The Nuvoloso cluster ID of which this node is a member"`
	SystemID       string `long:"system-id" description:"The Nuvoloso system ID of which this node is a member"`
}
