#!/bin/sh

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

pushd $SCRIPT_DIR

# Create namespaces if they do not yet exist
kubectl create namespace httpbin --dry-run=client -o yaml | kubectl apply -f -

# Deploy backend application (HTTPBin)
kubectl apply -f httpbin.yaml

# Deploy Kubernetes Gateway API resources
kubectl apply -f gateway.yaml
kubectl apply -f httproute-kgateway-system_service-httpbin-rg.yaml
kubectl apply -f api-example-com-httproute.yaml

# Deploy the rate limit policy
kubectl apply -f ratelimit-config.yaml
kubectl apply -f rate-limit-ektp.yaml

popd
