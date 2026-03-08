#!/bin/sh

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

pushd $SCRIPT_DIR

# Delete backend application (HTTPBin)
kubectl delete -f httpbin.yaml

# Delete Kubernetes Gateway API resources
kubectl delete -f gateway.yaml
kubectl delete -f api-example-com-httproute.yaml
kubectl delete -f httproute-kgateway-system_service-httpbin-rg.yaml

# Delete the httpbin namespace
kubectl delete namespace httpbin

popd
