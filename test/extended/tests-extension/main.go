package extended

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[Jira:openshift-apiserver][sig-api-machinery] sanity test", func() {
	g.It("should always pass [Suite:openshift/openshift-apiserver/conformance/parallel]", func() {
		o.Expect(true).To(o.BeTrue())
	})
})
