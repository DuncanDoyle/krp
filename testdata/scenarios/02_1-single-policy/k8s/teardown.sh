#!/bin/sh

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

pushd $SCRIPT_DIR

# Delete backend application (HTTPBin)
kubectl delete -f httpbin.yaml

# Delete Kubernetes Gateway API resources.
kubectl delete -f gateway.yaml
kubectl delete -f api_developer-example-com-httproute.yaml
kubectl delete -f httproute-kgateway-system_service-httpbin-rg.yaml
kubectl delete -f transformation-ektp.yaml

# Delete the Secret containing the self-signed TLS cert
kubectl -n kgateway-system delete secret api-example-com
kubectl -n kgateway-system delete secret developer-example-com

# Delete the httpbin namespace
kubectl delete namespace httpbin

popd