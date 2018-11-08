#!/bin/bash

# Copyright (c) 2016-2017 Bitnami
# Copyright (c) 2018 Cisco and/or its affiliates.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

. scripts/integration-test-helpers.sh

function run_tests() {
    kubectl get nodes
    kubectl version
    kubectl api-versions

    deploy_nsm

    # Let's check number of injected interfaces and if found,
    # check connectivity between nsm-client and nse
    #
    client_pod_name="$(kubectl get pods --all-namespaces | grep nsm-client | awk '{print $2}')"
    client_pod_namespace="$(kubectl get pods --all-namespaces | grep nsm-client | awk '{print $1}')"
    intf_number="$(kubectl exec "$client_pod_name" -n "$client_pod_namespace" -- ifconfig -a | grep -c nse)"
    if [ "$intf_number" -eq 0 ] ; then
        return 1
    fi
    kubectl exec "$client_pod_name" -n "$client_pod_namespace" -- ping 1.1.1.2 -c 5
    #
    # Final log collection
    #
    kubectl get nodes
    kubectl get pods
    kubectl get crd
    kubectl logs "$(kubectl get pods -o name | grep nse)"
    kubectl logs "$(kubectl get pods -o name | grep nsm-client)" -c nsm-init
    DATAPLANES="$(kubectl get pods -o name | grep test-dataplane | cut -d "/" -f 2)"
    for TESTDP in ${DATAPLANES} ; do
        kubectl logs "${TESTDP}"
    done
    kubectl get NetworkService,NetworkServiceEndpoint --all-namespaces

    # Need to get kubeconfig full path
    # NOTE: Disable this for now until we fix the timing issue
    if [ ! -z "${KUBECONFIG}" ] ; then
        K8SCONFIG=${KUBECONFIG}
    else
        K8SCONFIG="$HOME"/.kube/config
    fi
    export GODEBUG=netdns=2
    go test ./plugins/crd/... -v --kube-config="$K8SCONFIG"

    # We're all good now
    return 0
}

# vim: sw=4 ts=4 et si
