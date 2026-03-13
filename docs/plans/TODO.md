# TODO Document

This document contains a number of TO-DO items. Items that I think are either missing from the tool (e.g. enhancements, improvements), tech debt that needs to be solved, or kgateway scenarios that might not yet work with the current tool./


## Global Policy Namespace
Kgateway has a feature called "Global Policy Namespace". This feature allows the user to define Kgateway policies (e.g. TrafficPolicy, EnterpriseKgatewayTrafficPolicy, etc.) in a different namespace than its target. Normally, in K8S Gateway API, a `targetRef` is a LocalObjectReference, meaning that the policy needs to be deployed in the same namespace as its target. "Global Policy Namespace" allows you to select a single namespace on your K8S cluster from which policies can attach to any target.

Currently, I don't think `kfp` is able to detect global policy attachment from policies deployed in the "Global Policy Namespace" to targets in other namespaces.


## Config Mismatches
Instead of just presenting the filterchain that is present in Envoy, and linking the filter configuration to K8S Gateway API, we should also be able to detect if an expected filter is not in the filterchain. This should be a very uncommon scenario, but it could occur in the case of bug in the translator, or an error in xDS that therefore is not loaded in the Gateway.
