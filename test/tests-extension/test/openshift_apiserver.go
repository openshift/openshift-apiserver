package test

import (
	"context"
	"os"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	operatorNamespace = "openshift-apiserver"
)

// getKubeClient returns a Kubernetes client
func getKubeClient() (kubernetes.Interface, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// getKubeConfig returns Kubernetes configuration, preferring kubeconfig over in-cluster config
func getKubeConfig() (*rest.Config, error) {
	// First try to use kubeconfig file
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err == nil {
			return config, nil
		}
	}

	// Fall back to in-cluster config
	return rest.InClusterConfig()
}

var _ = g.Describe("[Jira:openshift-apiserver][sig-api-machinery] OpenShift API Server", func() {
	defer g.GinkgoRecover()

	g.It("should have a running openshift-apiserver deployment [Suite:openshift/openshift-apiserver/conformance/parallel]", func() {
		client, err := getKubeClient()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("checking for the openshift-apiserver deployment")
		var apiserverDeployment *appsv1.Deployment
		o.Eventually(func(gomega o.Gomega) {
			var err error
			apiserverDeployment, err = client.AppsV1().Deployments(operatorNamespace).Get(context.Background(), "openshift-apiserver", metav1.GetOptions{})
			gomega.Expect(err).NotTo(o.HaveOccurred())
			gomega.Expect(apiserverDeployment.Status.AvailableReplicas).To(o.BeNumerically(">", 0))
		}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(o.Succeed())

		g.By("verifying the deployment is ready")
		o.Expect(apiserverDeployment.Status.ReadyReplicas).To(o.BeNumerically(">", 0))
		o.Expect(apiserverDeployment.Status.UpdatedReplicas).To(o.Equal(*apiserverDeployment.Spec.Replicas))
	})

	g.It("should have openshift-apiserver pods running [Suite:openshift/openshift-apiserver/conformance/parallel]", func() {
		client, err := getKubeClient()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("checking for openshift-apiserver pods")
		var pods *corev1.PodList
		o.Eventually(func(gomega o.Gomega) {
			var err error
			pods, err = client.CoreV1().Pods(operatorNamespace).List(context.Background(), metav1.ListOptions{
				LabelSelector: "app=openshift-apiserver",
			})
			gomega.Expect(err).NotTo(o.HaveOccurred())
			gomega.Expect(len(pods.Items)).To(o.BeNumerically(">", 0))
		}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(o.Succeed())

		g.By("verifying all pods are running")
		for _, pod := range pods.Items {
			o.Expect(pod.Status.Phase).To(o.Equal(corev1.PodRunning))
		}
	})

	g.It("should have openshift-apiserver service running [Suite:openshift/openshift-apiserver/conformance/parallel]", func() {
		client, err := getKubeClient()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("checking for openshift-apiserver service")
		service, err := client.CoreV1().Services(operatorNamespace).Get(context.Background(), "openshift-apiserver", metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(service.Name).To(o.Equal("openshift-apiserver"))
		o.Expect(service.Spec.Ports).To(o.HaveLen(1))
		o.Expect(service.Spec.Ports[0].Port).To(o.Equal(int32(443)))
	})

	g.It("should have openshift-apiserver endpoints [Suite:openshift/openshift-apiserver/conformance/parallel]", func() {
		client, err := getKubeClient()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("checking for openshift-apiserver endpoints")
		o.Eventually(func(gomega o.Gomega) {
			endpoints, err := client.CoreV1().Endpoints(operatorNamespace).Get(context.Background(), "openshift-apiserver", metav1.GetOptions{})
			gomega.Expect(err).NotTo(o.HaveOccurred())
			gomega.Expect(len(endpoints.Subsets)).To(o.BeNumerically(">", 0))
			gomega.Expect(len(endpoints.Subsets[0].Addresses)).To(o.BeNumerically(">", 0))
		}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(o.Succeed())
	})

	g.It("should have openshift-apiserver configmap [Suite:openshift/openshift-apiserver/conformance/parallel]", func() {
		client, err := getKubeClient()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("checking for openshift-apiserver configmap")
		configMap, err := client.CoreV1().ConfigMaps(operatorNamespace).Get(context.Background(), "openshift-apiserver", metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configMap.Name).To(o.Equal("openshift-apiserver"))
		o.Expect(configMap.Data).To(o.HaveKey("config.yaml"))
	})

	g.It("should have openshift-apiserver secret [Suite:openshift/openshift-apiserver/conformance/parallel]", func() {
		client, err := getKubeClient()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("checking for openshift-apiserver secret")
		secret, err := client.CoreV1().Secrets(operatorNamespace).Get(context.Background(), "openshift-apiserver", metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secret.Name).To(o.Equal("openshift-apiserver"))
		o.Expect(secret.Type).To(o.Equal(corev1.SecretTypeOpaque))
	})
})
