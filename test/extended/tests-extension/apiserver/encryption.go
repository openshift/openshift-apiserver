package apiserver

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	operatorclient "github.com/openshift/client-go/operator/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[Jira:openshift-apiserver][sig-api-machinery][openshift-apiserver][encryption]", func() {
	defer g.GinkgoRecover()

	g.It("Force encryption key rotation for etcd datastore should rotate keys and update encryption prefixes [Slow][Disruptive][Suite:openshift/openshift-apiserver/conformance/serial]", func(ctx context.Context) {
		e2e.Logf("=== Starting Encryption Key Rotation Test ===")

		// Get kubeconfig and create clients
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = clientcmd.RecommendedHomeFile
		}
		e2e.Logf("Using kubeconfig: %s", kubeconfig)

		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to load kubeconfig")
		e2e.Logf("Successfully loaded kubeconfig")

		kubeClient, err := kubernetes.NewForConfig(config)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to create kube client")
		e2e.Logf("Created Kubernetes client")

		configClient, err := configclient.NewForConfig(config)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to create config client")
		e2e.Logf("Created OpenShift config client")

		operatorClient, err := operatorclient.NewForConfig(config)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to create operator client")
		e2e.Logf("Created OpenShift operator client")

		g.By("1. Check if cluster is Etcd Encryption On")
		e2e.Logf("Checking APIServer encryption configuration...")
		apiserver, err := configClient.ConfigV1().APIServers().Get(ctx, "cluster", metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())

		encryptionType := string(apiserver.Spec.Encryption.Type)
		e2e.Logf("Cluster encryption type: %s", encryptionType)
		if encryptionType != "aescbc" && encryptionType != "aesgcm" {
			e2e.Logf("Skipping test - encryption is not enabled (type: %s)", encryptionType)
			g.Skip("The cluster is Etcd Encryption Off, this case intentionally runs nothing")
		}
		e2e.Logf("Etcd Encryption is ON with type: %s", encryptionType)

		g.By("2. Get encryption prefix before rotation")
		e2e.Logf("Retrieving current encryption prefixes from etcd...")
		oasEncValPrefix1, err := getEncryptionPrefix(ctx, config, kubeClient, "/openshift.io/routes")
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to get encryption prefix for key routes")
		e2e.Logf("OpenShift API Server encryption prefix (BEFORE): %s", oasEncValPrefix1)

		kasEncValPrefix1, err := getEncryptionPrefix(ctx, config, kubeClient, "/kubernetes.io/secrets")
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to get encryption prefix for key secrets")
		e2e.Logf("Kube API Server encryption prefix (BEFORE): %s", kasEncValPrefix1)

		e2e.Logf("Getting current encryption key numbers...")
		oasEncNumber, err := getEncryptionKeyNumber(ctx, kubeClient, `encryption-key-openshift-apiserver-\d+`)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current OpenShift API Server encryption key number: %d", oasEncNumber)

		kasEncNumber, err := getEncryptionKeyNumber(ctx, kubeClient, `encryption-key-openshift-kube-apiserver-\d+`)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current Kube API Server encryption key number: %d", kasEncNumber)

		t := time.Now().Format(time.RFC3339)
		restorePatch := []byte(`[{"op":"replace","path":"/spec/unsupportedConfigOverrides","value":null}]`)
		mergePatch := []byte(fmt.Sprintf(`{"spec":{"unsupportedConfigOverrides":{"encryption":{"reason":"force OAS rotation %s"}}}}`, t))

		g.By("3. Force encryption key rotation for both openshiftapiserver and kubeapiserver")
		e2e.Logf("Preparing to force encryption key rotation with timestamp: %s", t)

		// Patch OpenShift API Server
		defer func() {
			e2e.Logf("CLEANUP: Restoring openshiftapiserver/cluster's spec")
			_, patchErr := operatorClient.OperatorV1().OpenShiftAPIServers().Patch(ctx, "cluster", types.JSONPatchType, restorePatch, metav1.PatchOptions{})
			if patchErr != nil {
				e2e.Failf("Failed to restore openshiftapiserver: %v", patchErr)
			}
			e2e.Logf("Successfully restored openshiftapiserver/cluster")
		}()
		e2e.Logf("3.1) Patching openshiftapiserver to force encryption rotation...")
		_, err = operatorClient.OperatorV1().OpenShiftAPIServers().Patch(ctx, "cluster", types.MergePatchType, mergePatch, metav1.PatchOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Successfully patched openshiftapiserver/cluster")

		// Patch Kube API Server
		defer func() {
			e2e.Logf("CLEANUP: Restoring kubeapiserver/cluster's spec")
			_, patchErr := operatorClient.OperatorV1().KubeAPIServers().Patch(ctx, "cluster", types.JSONPatchType, restorePatch, metav1.PatchOptions{})
			if patchErr != nil {
				e2e.Failf("Failed to restore kubeapiserver: %v", patchErr)
			}
			e2e.Logf("Successfully restored kubeapiserver/cluster")

			// Wait for kube-apiserver operator to stabilize after restoration
			e2e.Logf("CLEANUP: Waiting for kube-apiserver operator to stabilize...")
			waitErr := waitForKubeAPIServerOperatorStable(ctx, kubeClient)
			if waitErr != nil {
				e2e.Failf("Kube-apiserver operator did not stabilize after cleanup: %v", waitErr)
			}
			e2e.Logf("CLEANUP: Kube-apiserver operator is stable")
		}()
		e2e.Logf("3.2) Patching kubeapiserver to force encryption rotation...")
		_, err = operatorClient.OperatorV1().KubeAPIServers().Patch(ctx, "cluster", types.MergePatchType, mergePatch, metav1.PatchOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Successfully patched kubeapiserver/cluster")

		newOASEncSecretName := "encryption-key-openshift-apiserver-" + strconv.Itoa(oasEncNumber+1)
		newKASEncSecretName := "encryption-key-openshift-kube-apiserver-" + strconv.Itoa(kasEncNumber+1)

		g.By("4. Check the new encryption key secrets appear")
		e2e.Logf("Waiting for new encryption key secrets to be created...")
		e2e.Logf("Expected new OAS secret: %s", newOASEncSecretName)
		e2e.Logf("Expected new KAS secret: %s", newKASEncSecretName)
		e2e.Logf("Polling every 5 seconds for up to 120 seconds...")

		errKey := wait.PollUntilContextTimeout(ctx, 5*time.Second, 120*time.Second, false, func(cxt context.Context) (bool, error) {
			_, err1 := kubeClient.CoreV1().Secrets("openshift-config-managed").Get(cxt, newOASEncSecretName, metav1.GetOptions{})
			_, err2 := kubeClient.CoreV1().Secrets("openshift-config-managed").Get(cxt, newKASEncSecretName, metav1.GetOptions{})
			if err1 != nil || err2 != nil {
				e2e.Logf("  Still waiting for secrets... (OAS: %v, KAS: %v)", err1 != nil, err2 != nil)
				return false, nil
			}
			e2e.Logf("Found both new encryption key secrets!")
			return true, nil
		})

		// Print openshift-apiserver and kube-apiserver secrets for debugging if time out
		if errKey != nil {
			e2e.Logf("ERROR: Timeout waiting for new encryption key secrets!")
			e2e.Logf("Listing all OpenShift API Server encryption secrets for debugging...")
			secrets, _ := kubeClient.CoreV1().Secrets("openshift-config-managed").List(ctx, metav1.ListOptions{
				LabelSelector: "encryption.apiserver.operator.openshift.io/component=openshift-apiserver",
			})
			if secrets != nil {
				e2e.Logf("  Total OpenShift API Server secrets: %d", len(secrets.Items))
				for i, secret := range secrets.Items {
					e2e.Logf("    [%d] %s (created: %v)", i+1, secret.Name, secret.CreationTimestamp)
				}
			}

			e2e.Logf("Listing all Kube API Server encryption secrets for debugging...")
			secrets, _ = kubeClient.CoreV1().Secrets("openshift-config-managed").List(ctx, metav1.ListOptions{
				LabelSelector: "encryption.apiserver.operator.openshift.io/component=openshift-kube-apiserver",
			})
			if secrets != nil {
				e2e.Logf("  Total Kube API Server secrets: %d", len(secrets.Items))
				for i, secret := range secrets.Items {
					e2e.Logf("    [%d] %s (created: %v)", i+1, secret.Name, secret.CreationTimestamp)
				}
			}
		}
		o.Expect(errKey).NotTo(o.HaveOccurred(), fmt.Sprintf("new encryption key secrets %s, %s not found", newOASEncSecretName, newKASEncSecretName))

		g.By("5. Waiting for the force encryption completion")
		e2e.Logf("Waiting for encryption migration to complete for secret: %s", newKASEncSecretName)
		e2e.Logf("This may take up to 35 minutes - checking every 30 seconds...")

		// Check the operator status before waiting
		kasOperator, err := operatorClient.OperatorV1().KubeAPIServers().Get(ctx, "cluster", metav1.GetOptions{})
		if err == nil {
			e2e.Logf("KubeAPIServer operator status conditions:")
			for i, cond := range kasOperator.Status.Conditions {
				if i < 5 { // Log first 5 conditions
					e2e.Logf("  - %s: %s (reason: %s)", cond.Type, cond.Status, cond.Reason)
				}
			}
		}

		// Only need to check kubeapiserver because kubeapiserver takes more time.
		completed, err := waitEncryptionKeyMigration(ctx, kubeClient, newKASEncSecretName)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("failed to complete encryption migration for %s", newKASEncSecretName))
		o.Expect(completed).Should(o.Equal(true))
		e2e.Logf("Encryption migration completed successfully!")

		g.By("6. Get encryption prefix after force encryption completed")
		e2e.Logf("Retrieving new encryption prefixes from etcd...")
		oasEncValPrefix2, err := getEncryptionPrefix(ctx, config, kubeClient, "/openshift.io/routes")
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to get encryption prefix for key routes")
		e2e.Logf("OpenShift API Server encryption prefix (AFTER): %s", oasEncValPrefix2)

		kasEncValPrefix2, err := getEncryptionPrefix(ctx, config, kubeClient, "/kubernetes.io/secrets")
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to get encryption prefix for key secrets")
		e2e.Logf("Kube API Server encryption prefix (AFTER): %s", kasEncValPrefix2)

		e2e.Logf("Verifying encryption prefixes changed after rotation...")
		e2e.Logf("Expected prefix format: k8s:enc:%s:v1", encryptionType)

		// Verify OAS prefix has correct format
		o.Expect(oasEncValPrefix2).Should(o.ContainSubstring(fmt.Sprintf("k8s:enc:%s:v1", encryptionType)),
			fmt.Sprintf("OAS encryption prefix should contain k8s:enc:%s:v1, but got: %s", encryptionType, oasEncValPrefix2))
		e2e.Logf("[OK] OAS prefix contains expected format: %s", oasEncValPrefix2)

		// Verify KAS prefix has correct format
		o.Expect(kasEncValPrefix2).Should(o.ContainSubstring(fmt.Sprintf("k8s:enc:%s:v1", encryptionType)),
			fmt.Sprintf("KAS encryption prefix should contain k8s:enc:%s:v1, but got: %s", encryptionType, kasEncValPrefix2))
		e2e.Logf("[OK] KAS prefix contains expected format: %s", kasEncValPrefix2)

		// Verify OAS prefix actually changed
		o.Expect(oasEncValPrefix2).NotTo(o.Equal(oasEncValPrefix1),
			fmt.Sprintf("OAS prefix should have changed after rotation, but remained: %s", oasEncValPrefix1))
		e2e.Logf("[OK] OAS prefix changed from %s to %s", oasEncValPrefix1, oasEncValPrefix2)

		// Verify KAS prefix actually changed
		o.Expect(kasEncValPrefix2).NotTo(o.Equal(kasEncValPrefix1),
			fmt.Sprintf("KAS prefix should have changed after rotation, but remained: %s", kasEncValPrefix1))
		e2e.Logf("[OK] KAS prefix changed from %s to %s", kasEncValPrefix1, kasEncValPrefix2)

		e2e.Logf("=== Test Completed Successfully ===")
		e2e.Logf("Summary:")
		e2e.Logf("  - Encryption Type: %s", encryptionType)
		e2e.Logf("  - OAS Key Rotation: %d → %d", oasEncNumber, oasEncNumber+1)
		e2e.Logf("  - KAS Key Rotation: %d → %d", kasEncNumber, kasEncNumber+1)
		e2e.Logf("  - All encryption prefixes successfully rotated")
	})
})
