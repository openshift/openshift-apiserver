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
