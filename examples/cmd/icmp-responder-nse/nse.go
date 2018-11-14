// Copyright (c) 2018 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/ligato/networkservicemesh/controlplane/pkg/nsmd"

	"github.com/ligato/networkservicemesh/controlplane/pkg/apis/networkservice"
	"github.com/ligato/networkservicemesh/controlplane/pkg/apis/registry"

	"github.com/ligato/networkservicemesh/pkg/tools"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	// networkServiceName defines Network Service Name the NSE is serving for
	networkServiceName = "icmp-responder"
	// SocketBaseDir defines the location of NSM Endpoints listen socket
)

func main() {
	var wg sync.WaitGroup
	var nsmServerSocket, nsmClientSocket string

	if e, _ := os.LookupEnv(nsmd.NsmDevicePluginEnv); e == "false" {
		logrus.Info("Creating Workspace")
		reply, err := nsmd.RequestWorkspace()
		if err != nil {
			logrus.Error("Unable to create nsmd workspace")
		}
		logrus.Infof("RequestWorkspace reply: %s", reply)
		nsmServerSocket = reply.HostBasedir + reply.Workspace + "/" + reply.NsmServerSocket
		nsmClientSocket = reply.HostBasedir + reply.Workspace + "/" + reply.NsmClientSocket
	} else {
		logrus.Infof("Device Plugin context, workspace created")
		nsmServerSocket, _ := os.LookupEnv(nsmd.NsmServerSocketEnv)
		logrus.Infof("nsmServerSocket: %s", nsmServerSocket)
		// TODO handle missing env

		nsmClientSocket, _ := os.LookupEnv(nsmd.NsmClientSocketEnv)
		logrus.Infof("nsmClientSocket: %s", nsmClientSocket)
		// TODO handle missing env
	}

	// For NSE to program container's dataplane, container's linux namespace must be sent to NSM
	linuxNS, err := tools.GetCurrentNS()
	if err != nil {
		logrus.Fatalf("nse: failed to get a linux namespace with error: %v, exiting...", err)
		os.Exit(1)
	}
	logrus.Infof("Starting NSE, linux namespace: %s", linuxNS)

	// NSM socket path will be used to drop NSE socket for NSM's Connection request
	connectionServerSocket := nsmClientSocket
	if err := tools.SocketCleanup(nsmClientSocket); err != nil {
		logrus.Fatalf("nse: failure to cleanup stale socket %s with error: %v", nsmClientSocket, err)
	}

	logrus.Infof("nse: listening socket %s", nsmClientSocket)
	connectionServer, err := net.Listen("unix", nsmClientSocket)
	if err != nil {
		logrus.Fatalf("nse: failure to listen on a socket %s with error: %v", nsmClientSocket, err)
	}
	grpcServer := grpc.NewServer()

	// Registering NSE API, it will listen for Connection requests from NSM and return information
	// needed for NSE's dataplane programming.
	nseConn := New()

	networkservice.RegisterNetworkServiceServer(grpcServer, nseConn)

	go func() {
		wg.Add(1)
		if err := grpcServer.Serve(connectionServer); err != nil {
			logrus.Fatalf("nse: failed to start grpc server on socket %s with error: %v ", nsmClientSocket, err)
		}
	}()
	// Check if the socket of Endpoint Connection Server is operation
	testSocket, err := tools.SocketOperationCheck(nsmServerSocket)
	if err != nil {
		logrus.Fatalf("nse: failure to communicate with the nsm on socket %s with error: %v", nsmServerSocket, err)
	}
	testSocket.Close()

	// NSE connection server is ready and now endpoints can be advertised to NSM

	if _, err := os.Stat(nsmServerSocket); err != nil {
		logrus.Errorf("nse: failure to access nsm socket at %s with error: %+v, exiting...", nsmServerSocket, err)
		os.Exit(1)
	}

	conn, err := tools.SocketOperationCheck(nsmServerSocket)
	if err != nil {
		logrus.Fatalf("nse: failure to communicate with the registrySocket %s with error: %+v", nsmServerSocket, err)
	}
	defer conn.Close()
	logrus.Infof("nsm: connection to nsm server on socket: %s succeeded.", nsmServerSocket)

	registryConnection := registry.NewNetworkServiceRegistryClient(conn)

	nse := &registry.NetworkServiceEndpoint{
		NetworkServiceName: networkServiceName,
		Payload:            "IP",
		Labels:             make(map[string]string),
		SocketLocation:     connectionServerSocket,
	}

	registeredNSE, err := registryConnection.RegisterNSE(context.Background(), nse)
	if err != nil {
		logrus.Fatalln("unable to register endpoint", err)
	}
	logrus.Infoln("NSE registered: " + registeredNSE.EndpointName)

	logrus.Infof("nse: channel has been successfully advertised, waiting for connection from NSM...")
	// Now block on WaitGroup
	wg.Wait()
}
