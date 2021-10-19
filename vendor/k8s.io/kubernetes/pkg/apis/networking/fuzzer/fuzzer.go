/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fuzzer

import (
	fuzz "github.com/google/gofuzz"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/kubernetes/pkg/apis/networking"
	utilpointer "k8s.io/utils/pointer"
)

// Funcs returns the fuzzer functions for the networking api group.
var Funcs = func(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(np *networking.NetworkPolicyPeer, c fuzz.Continue) {
			c.FuzzNoCustom(np) // fuzz self without calling this function again
			// TODO: Implement a fuzzer to generate valid keys, values and operators for
			// selector requirements.
			if np.IPBlock != nil {
				np.IPBlock = &networking.IPBlock{
					CIDR:   "192.168.1.0/24",
					Except: []string{"192.168.1.1/24", "192.168.1.2/24"},
				}
			}
		},
		func(np *networking.NetworkPolicy, c fuzz.Continue) {
			c.FuzzNoCustom(np) // fuzz self without calling this function again
			// TODO: Implement a fuzzer to generate valid keys, values and operators for
			// selector requirements.
			if len(np.Spec.PolicyTypes) == 0 {
				np.Spec.PolicyTypes = []networking.PolicyType{networking.PolicyTypeIngress}
			}
		},
		func(path *networking.HTTPIngressPath, c fuzz.Continue) {
			c.FuzzNoCustom(path) // fuzz self without calling this function again
			pathTypes := []networking.PathType{networking.PathTypeExact, networking.PathTypePrefix, networking.PathTypeImplementationSpecific}
			path.PathType = &pathTypes[c.Rand.Intn(len(pathTypes))]
		},
		func(p *networking.ServiceBackendPort, c fuzz.Continue) {
			c.FuzzNoCustom(p)
			// clear one of the fields
			if c.RandBool() {
				p.Name = ""
				if p.Number == 0 {
					p.Number = 1
				}
			} else {
				p.Number = 0
				if p.Name == "" {
					p.Name = "portname"
				}
			}
		},
		func(p *networking.IngressClass, c fuzz.Continue) {
			c.FuzzNoCustom(p) // fuzz self without calling this function again
			// default Parameters to Cluster
			if p.Spec.Parameters == nil || p.Spec.Parameters.Scope == nil {
				p.Spec.Parameters = &networking.IngressClassParametersReference{
					Scope: utilpointer.StringPtr(networking.IngressClassParametersReferenceScopeCluster),
				}
			}
		},
	}
}
