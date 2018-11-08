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


#
# Wait for network service object to become available in a $1 namespace
# Default timeout of 60 seconds can be overwritten in $2 parameter.
#
function wait_for_networkservice() {
    end=$(date +%s)
    if [ "x$2" != "x" ]; then
     end=$((end + "$2"))
    else
     end=$((end + 60))
    fi
    while true; do
        network_service=$(kubectl get networkservices --namespace="$1" -o json | jq -r '.items[].metadata.name')
        if [ "x$network_service" != "x" ]; then
            break
        fi
        sleep 1
        now=$(date +%s)
        if [ "$now" -gt "$end" ] ; then
            echo "NetworkService has not been created within 60 seconds, failing..."
            return 1
        fi
    done

    return 0
}

#
# Wait for all pods to become running in a $1 namespace
# Default timeout of 180 seconds can be overwritten in $2 parameter.
#
function wait_for_pods() {
    end=$(date +%s)
    if [ "x$2" != "x" ]; then
     end=$((end + "$2"))
    else
     end=$((end + 180))
    fi
    while true; do
        kubectl get pods --namespace="$1" -o json | jq -r \
            '.items[].status.phase' | grep Pending > /dev/null && \
            PENDING=True || PENDING=False
        query='.items[]|select(.status.phase=="Running")'
        query="$query|.status.containerStatuses[].ready"
        kubectl get pods --namespace="$1" -o json | jq -r "$query" | \
            grep false > /dev/null && READY="False" || READY="True"
        kubectl get jobs -o json --namespace="$1" | jq -r \
            '.items[] | .spec.completions == .status.succeeded' | \
            grep false > /dev/null && JOBR="False" || JOBR="True"
        if [ "$PENDING" == "False" ] && [ "$READY" == "True" ] && [ "$JOBR" == "True" ]
        then
            break
        fi
        sleep 1
        now=$(date +%s)
        if [ "$now" -gt "$end" ] ; then
            echo "Containers failed to start."
            return 1
        fi
    done

    return 0
}

#
# Dump logs for evidence for futher debugging
#
function dump_logs() {
    kubectl describe node || true
    kubectl get pods --all-namespaces || true
    nsm=$(kubectl get pods --all-namespaces | grep networkservice | awk '{print $2}')
    namespace=$(kubectl get pods --all-namespaces | grep networkservice | awk '{print $1}')
    if [[ "x$nsm" != "x" ]]; then
        kubectl describe pod "$nsm" -n "$namespace" || true
        kubectl logs "$nsm" -n "$namespace" || true
        kubectl logs "$nsm" -n "$namespace" -p || true
    fi
    nsm_client=$(kubectl get pods --all-namespaces | grep nsm-client | awk '{print $2}')
    if [[ "x$nsm_client" != "x" ]]; then
        kubectl describe pod "$nsm_client" -n "$namespace" || true
        kubectl logs "$nsm_client" -n "$namespace" nsm-init || true
        kubectl logs "$nsm_client" -n "$namespace" nsm-client || true
        kubectl logs "$nsm_client" -n "$namespace" nsm-init -p || true
        kubectl logs "$nsm_client" -n "$namespace" nsm-client -p || true
    fi
    nse=$(kubectl get pods --all-namespaces | grep nse | awk '{print $2}')
    if [[ "x$nse" != "x" ]]; then
        kubectl describe pod "$nse" -n "$namespace" || true
        kubectl logs "$nse" -n "$namespace"  || true
        kubectl logs "$nse" -n "$namespace"  -p || true
    fi
    dataplane=$(kubectl get pods --all-namespaces | grep test-dataplane | awk '{print $2}')
    if [[ "x$dataplane" != "x" ]]; then
        kubectl describe pod "$dataplane" -n "$namespace" || true
        kubectl logs "$dataplane" -n "$namespace"  || true
        kubectl logs "$dataplane" -n "$namespace"  -p || true
    fi
    sidecar=$(kubectl get pods --all-namespaces | grep sidecar-injector-webhook | awk '{print $2}')
    if [[ "x$sidecar" != "x" ]]; then
        kubectl describe pod "$sidecar" -n "$namespace" || true
        kubectl logs "$sidecar" -n "$namespace"  || true
        kubectl logs "$sidecar" -n "$namespace"  -p || true
    fi
    vppdataplane=$(kubectl get pods --all-namespaces | grep vpp-dataplane | awk '{print $2}')
    if [[ "x$vppdataplane" != "x" ]]; then
        kubectl describe pod "$vppdataplane" -n "$namespace" || true
        kubectl logs "$vppdataplane" -n "$namespace" vpp-daemon || true
        kubectl logs "$vppdataplane" -n "$namespace" vpp || true
        kubectl logs "$vppdataplane" -n "$namespace" vpp-daemon -p || true
        kubectl logs "$vppdataplane" -n "$namespace" vpp -p || true
    fi
    kubectl get nodes

    # The docker images can be super useful locally, but never works in CI
    # So this dance causes us to not break CI simply because its setup
    # Differently than local
    if sudo docker images; then
        exit 0
    fi
}

function deploy_nsm() {
    kubectl label --overwrite --all=true nodes app=networkservice-node
    #kubectl label --overwrite nodes kube-node-1 app=networkservice-node
    kubectl create -f conf/sample/networkservice-daemonset.yaml
    #
    # Now let's wait for all pods to get into running state
    #
    wait_for_pods default
    exit_code=$?
    [[ ${exit_code} != 0 ]] && return ${exit_code}


    # Wait til settles
    echo "INFO: Waiting for Network Service Mesh daemonset to be up and CRDs to be available ..."
    typeset -i cnt=240
    until kubectl get crd | grep networkserviceendpoints.networkservicemesh.io ; do
        ((cnt=cnt-1)) || return 1
        sleep 2
    done
    typeset -i cnt=240
    until kubectl get crd | grep networkservices.networkservicemesh.io ; do
        ((cnt=cnt-1)) || return 1
        sleep 2
    done

    #
    # Since daemonset is up and running, create CRD resources
    #
    kubectl create -f conf/sample/networkservice.yaml
    wait_for_networkservice default

    #
    # Starting nse pod which will advertise an endpoint for gold-network
    # network service
    kubectl create -f conf/sample/nse.yaml
    kubectl create -f conf/sample/test-dataplane.yaml
    wait_for_pods default

    #
    # Starting nsm client pod, nsm-client pod should discover gold-network
    # network service along with its endpoint and interface
    kubectl create -f conf/sample/nsm-client.yaml

    #
    # Now let's wait for nsm-cient pod to get into running state
    #
    wait_for_pods default

    #
    # Starting vpp-daemonset pod
    kubectl create -f dataplanes/vpp/yaml/vpp-daemonset.yaml

    #
    # Now let's wait for vpp-daemonset pod to get into running state
    #
    wait_for_pods default
    exit_ret=$?
    if [ "${exit_ret}" != "0" ] ; then
        return "${exit_ret}"
    fi

    #
    # tests are failing on minikube for adding sidecar containers,  will enable
    # tests once we move the testing to actual Kubernetes cluster.
    ## Refer https://github.com/kubernetes/website/issues/3956#issuecomment-407895766

    # # Side car tests
    # kubectl create -f conf/sidecar-injector/sample-deployment.yaml
    # wait_for_pods default
    # exit_ret=$?
    # if [ "${exit_ret}" != "0" ] ; then
    #     return "${exit_ret}"
    # fi

    # ## Sample test scripts for adding sidecar components in a Kubernetes cluster
    # SIDECAR_CONFIG=conf/sidecar-injector

    # ## Create SSL certificates
    # $SIDECAR_CONFIG/webhook-create-signed-cert.sh --service sidecar-injector-webhook-svc --secret sidecar-injector-webhook-certs --namespace default

    # ## Copy the cert to the webhook configuration YAML file
    # < $SIDECAR_CONFIG/mutatingWebhookConfiguration.yaml $SIDECAR_CONFIG/webhook-patch-ca-bundle.sh >  $SIDECAR_CONFIG/mutatingwebhook-ca-bundle.yaml

    # kubectl label namespace default sidecar-injector=enabled
    # ## Create all the required components
    # kubectl create -f $SIDECAR_CONFIG/configMap.yaml -f $SIDECAR_CONFIG/ServiceAccount.yaml -f $SIDECAR_CONFIG/server-deployment.yaml -f $SIDECAR_CONFIG/mutatingwebhook-ca-bundle.yaml -f $SIDECAR_CONFIG/sidecarInjectorService.yaml
    # wait_for_pods default
    # exit_ret=$?
    # if [ "${exit_ret}" != "0" ] ; then
    #     return "${exit_ret}"
    # fi

    # kubectl delete "$(kubectl get pods -o name | grep sleep)"
    # wait_for_pods default
    # exit_ret=$?
    # if [ "${exit_ret}" != "0" ] ; then
    #     error_collection
    #     return "${exit_ret}"
    # fi

    # pod_count=$(kubectl get pods | grep sleep | grep Running | awk '{print $2}')
    # if [ "${pod_count}" != "2/2" ]; then
    #     error_collection
    #     return 1
    # fi

    # kubectl describe pod "$(kubectl get pods | grep sleep | grep Running | awk '{print $1}')" | grep status=injected
    # exit_ret=$?
    # if [ "${exit_ret}" != "0" ] ; then
    #     error_collection
    #     return "${exit_ret}"
    # fi

}

function undeploy_nsm() {
    kubectl delete -f dataplanes/vpp/yaml/vpp-daemonset.yaml

    # remove endpoint and client finalizers to ensure pod's deletion can terminate
    NSMC=$(k get pods | grep nsm-client | awk '{print $1}')
    NSE=$(k get pods | grep nse | awk '{print $1}')

    kubectl patch pod $NSMC -p '{"metadata":{"finalizers":null}}'
    kubectl delete -f conf/sample/nsm-client.yaml

    kubectl patch pod $NSE -p '{"metadata":{"finalizers":null}}'
    kubectl delete -f conf/sample/nse.yaml

    kubectl delete -f conf/sample/test-dataplane.yaml
    kubectl delete -f conf/sample/networkservice.yaml
    kubectl delete -f conf/sample/networkservice-daemonset.yaml
}
# vim: sw=4 ts=4 et si
