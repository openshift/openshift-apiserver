# OpenShift API Server

The OpenShift API Server is a critical component of the OpenShift Container Platform that provides REST API endpoints for OpenShift-specific resource types. It extends the Kubernetes API server to support OpenShift's additional features and resources.

## Overview

This component serves as the API backend for OpenShift-specific resources, including:

- **Apps API**: DeploymentConfigs for application deployment and management
- **Build API**: BuildConfigs, Builds, and BuildLogs for container image creation
- **Image API**: ImageStreams and ImageStreamTags for image repository management
- **Project API**: Projects for multi-tenancy and namespace management with additional policies
- **Route API**: Routes for external access to services
- **Security API**: SecurityContextConstraints for pod security policies
- **Template API**: Templates for application scaffolding and deployment
- **User/OAuth API**: Users, Groups, and OAuth resources for authentication and authorization

The OpenShift API Server integrates with the Kubernetes API aggregation layer, allowing these OpenShift-specific resources to be accessed through the same unified API endpoint as native Kubernetes resources.

## Tests

This repository is compatible with the [OpenShift Tests Extension (OTE)](https://github.com/openshift-eng/openshift-tests-extension) framework.

### Building the test binary

```bash
make build
```

### Running test suites and tests

```bash
# Run a specific test suite or test
./openshift-apiserver-tests-ext run-suite openshift/openshift-apiserver/all
./openshift-apiserver-tests-ext run-test "test-name"

# Run with JUnit output
./openshift-apiserver-tests-ext run-suite openshift/openshift-apiserver/all --junit-path /tmp/junit.xml
```

### Listing available tests and suites

```bash
# List all test suites
./openshift-apiserver-tests-ext list suites

# List tests in a suite
./openshift-apiserver-tests-ext list tests --suite=openshift/openshift-apiserver/all
```

For more information about the OTE framework, see the [openshift-tests-extension documentation](https://github.com/openshift-eng/openshift-tests-extension).
