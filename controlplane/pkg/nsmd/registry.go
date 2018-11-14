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

package nsmd

import (
	"fmt"

	"github.com/ligato/networkservicemesh/controlplane/pkg/apis/registry"
	"github.com/ligato/networkservicemesh/controlplane/pkg/model"
	"github.com/ligato/networkservicemesh/pkg/nsm/apis/common"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

type registryServer struct {
	model     model.Model
	workspace *Workspace
}

func NewRegistryServer(model model.Model, workspace *Workspace) registry.NetworkServiceRegistryServer {
	return &registryServer{
		model:     model,
		workspace: workspace,
	}
}

func (es *registryServer) RegisterNSE(ctx context.Context, request *registry.NetworkServiceEndpoint) (*registry.NetworkServiceEndpoint, error) {
	logrus.Infof("Received RegisterNSE request: %v", request)

	// Check if there is already Network Service Endpoint object with the same name, if there is
	// success will be returned to NSE, since it is a case of NSE pod coming back up.
	client, err := RegistryClient()
	if err != nil {
		err = fmt.Errorf("attempt to connect to upstream registry failed with: %v", err)
		logrus.Error(err)
		return nil, err
	}
	if client == nil {
		//TODO: manage this case with a specific error code returned by RegistryClient()
		logrus.Info("No registry URL defined, NSE will only be registered on this nsm server")
		return request, nil
	}
	// TODO fix url setting here
	if request.Labels == nil {
		request.Labels = make(map[string]string)
	}
	request.Labels[KEY_NSM_URL] = es.model.GetNsmUrl()

	endpoint, err := client.RegisterNSE(context.Background(), request)
	if err != nil {
		err = fmt.Errorf("attempt to pass through from nsm to upstream registry failed with: %v", err)
		logrus.Error(err)
		return nil, err
	}

	ep := es.model.GetEndpoint(endpoint.EndpointName)
	if ep == nil {
		es.model.AddEndpoint(endpoint)
		WorkSpaceRegistry().AddEndpointToWorkspace(es.workspace, endpoint)
	}
	WorkSpaceRegistry().AddEndpointToWorkspace(es.workspace, ep)

	return endpoint, nil
}

func (es *registryServer) RemoveNSE(ctx context.Context, request *registry.RemoveNSERequest) (*common.Empty, error) {
	// TODO make sure we track which registry server we got the RegisterNSE from so we can only allow a deletion
	// of what you advertised
	logrus.Infof("Received Endpoint Remove request: %+v", request)
	client, err := RegistryClient()
	if err != nil {
		err = fmt.Errorf("attempt to pass through from nsm to upstream registry failed with: %v", err)
		logrus.Error(err)
		return nil, err
	}
	_, err = client.RemoveNSE(context.Background(), request)
	if err != nil {
		err = fmt.Errorf("attempt to pass through from nsm to upstream registry failed with: %v", err)
		logrus.Error(err)
		return nil, err
	}
	WorkSpaceRegistry().DeleteEndpointToWorkspace(request.EndpointName)
	if err := es.model.DeleteEndpoint(request.EndpointName); err != nil {
		return &common.Empty{}, err
	}
	return &common.Empty{}, nil
}

func (es *registryServer) Close() {

}
