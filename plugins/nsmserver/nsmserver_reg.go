// Copyright 2018 Red Hat, Inc.
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

// Package core manages the lifecycle of all plugins (start, graceful
// shutdown) and defines the core lifecycle SPI. The core lifecycle SPI
// must be implemented by each plugin.

package nsmserver

import (
	"fmt"
	"net"
	"path"
	"strconv"
	"time"

	"github.com/ligato/networkservicemesh/pkg/tools"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

const (
	// SocketBaseDir defines the location of NSM client socket
	SocketBaseDir = "/var/lib/networkservicemesh/"
	resourceName  = "nsm.ligato.io/socket"
	// ServerSock defines the name of NSM client socket
	ServerSock      = "nsm.ligato.io.sock"
	initDeviceCount = 10
	socketMask      = 0077
	folderMask      = 0777
)

// Register registers
func Register(kubeletEndpoint string) error {
	conn, err := grpc.Dial(kubeletEndpoint, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	defer conn.Close()
	if err != nil {
		return fmt.Errorf("device-plugin: cannot connect to kubelet service: %v", err)
	}
	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     ServerSock,
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return fmt.Errorf("device-plugin: cannot register to kubelet service: %v", err)
	}
	return nil
}

func startDeviceServer(nsm *nsmClientEndpoints) error {
	listenEndpoint := path.Join(pluginapi.DevicePluginPath, ServerSock)
	if err := tools.SocketCleanup(listenEndpoint); err != nil {
		return err
	}
	sock, err := net.Listen("unix", listenEndpoint)
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(grpcServer, nsm)

	nsm.logger.Infof("Starting Device Plugin's gRPC server listening on socket: %s", ServerSock)
	go func() {
		if err := grpcServer.Serve(sock); err != nil {
			nsm.logger.Error("failed to start device plugin grpc server", listenEndpoint, err)
		}
	}()
	// Check if the socket of device plugin server is operation
	conn, err := tools.SocketOperationCheck(listenEndpoint)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}

// NewNSMDeviceServer registers and starts Kubelet's device plugin
func NewNSMDeviceServer(p *Plugin) error {
	nsm := &nsmClientEndpoints{
		nsmSockets:        map[string]nsmSocket{},
		logger:            p.Deps.Log,
		objectStore:       p.Deps.ObjectStore,
		clientConnections: make(map[string]map[string]*clientNetworkService, 0),
		k8sClient:         p.Deps.Client.GetClientset(),
		nsmClient:         p.Deps.Client.GetNSMClientset(),
		namespace:         p.namespace,
		nsmPodIPAddress:   p.nsmPodIPAddress,
	}
	for i := 0; i < initDeviceCount; i++ {
		nsm.nsmSockets[strconv.Itoa(i)] = nsmSocket{device: &pluginapi.Device{ID: strconv.Itoa(i), Health: pluginapi.Healthy}}
	}
	if err := startDeviceServer(nsm); err != nil {
		return err
	}
	// Registers with Kubelet.
	err := Register(pluginapi.KubeletSocket)

	return err
}
