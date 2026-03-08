#!/bin/sh

TLS_CERT_TMP_DIR=`mktemp -d`

pushd $TLS_CERT_TMP_DIR

printf "\nGenerating self-signed TLS cert for api.example.com.\n"
openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout tls.key -out tls.crt -subj "/CN=developer.example.com"

printf "\nCreating TLS cert secret in kgateway-system namespace.\n"
kubectl create secret tls developer-example-com --key tls.key --cert tls.crt --namespace kgateway-system

printf "\n\nTLS Cert Secret 'developer-example-com' for 'developer.example.com' domain created in 'kgateway-system' namespace. You can now create a Gateway with TLS configuration for the 'developer.example.com' domain.\n\n"

popd