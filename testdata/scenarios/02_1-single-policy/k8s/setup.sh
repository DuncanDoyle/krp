#!/bin/sh

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

pushd $SCRIPT_DIR

# Create namespaces if they do not yet exist
kubectl create namespace httpbin --dry-run=client -o yaml | kubectl apply -f -

# Deploy backend application (HTTPBin)
kubectl apply -f httpbin.yaml


# Create the self-signed certificates for our HTTPS gateway
./create-tls-cert-secret-api-example-com.sh
./create-tls-cert-secret-developer-example-com.sh

# Deploy Kubernetes Gateway API resources.
kubectl apply -f gateway.yaml
kubectl apply -f httproute-kgateway-system_service-httpbin-rg.yaml
kubectl apply -f api_developer-example-com-httproute.yaml
kubectl apply -f transformation-ektp.yaml

popd