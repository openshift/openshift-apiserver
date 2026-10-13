// +k8s:conversion-gen=github.com/openshift/openshift-apiserver/pkg/route/apis/route
// +k8s:conversion-gen-external-types=github.com/openshift/api/route/v1
// +k8s:defaulter-gen=TypeMeta
// +k8s:defaulter-gen-input=../../../../../../../../github.com/openshift/api/route/v1

// +groupName=route.openshift.io
// Package v1 is the v1 version of the API.
package v1

import (
	_ "github.com/openshift/library-go/pkg/route/defaulting" // must be imported in order to be vendored in order to be read by defaulter-gen
)
