package apiserver

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// getEncryptionPrefix retrieves the encryption prefix for a given key path from etcd
// Note: This still requires exec'ing into the etcd pod, which is the appropriate way to access etcd
func getEncryptionPrefix(ctx context.Context, config *rest.Config, kubeClient kubernetes.Interface, keyPath string) (string, error) {
	e2e.Logf("    Getting encryption prefix for etcd key path: %s", keyPath)

	// Get first etcd pod
	pods, err := kubeClient.CoreV1().Pods("openshift-etcd").List(ctx, metav1.ListOptions{
		LabelSelector: "app=etcd",
	})
	if err != nil {
		e2e.Logf("    ERROR: Failed to list etcd pods: %v", err)
		return "", fmt.Errorf("failed to list etcd pods: %v", err)
	}
	if len(pods.Items) == 0 {
		e2e.Logf("    ERROR: No etcd pods found in openshift-etcd namespace")
		return "", fmt.Errorf("no etcd pods found")
	}

	podName := pods.Items[0].Name
	e2e.Logf("    Using etcd pod: %s (total etcd pods: %d)", podName, len(pods.Items))

	// Execute etcdctl command to get the encryption prefix
	command := fmt.Sprintf("etcdctl get %s --prefix --keys-only --limit=1 | head -1 | xargs -I {} etcdctl get {} --print-value-only | head -c 32", keyPath)
	e2e.Logf("    Executing command in 'etcd' container to retrieve encrypted data...")

	// Use client-go's remotecommand to exec into the pod
	req := kubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace("openshift-etcd").
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "etcd",
			Command:   []string{"/bin/sh", "-c", command},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		e2e.Logf("    ERROR: Failed to create remote executor: %v", err)
		return "", fmt.Errorf("failed to create executor: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		e2e.Logf("    ERROR: Failed to execute command in pod %s: %v", podName, err)
		if stderr.Len() > 0 {
			e2e.Logf("    STDERR: %s", stderr.String())
		}
		return "", fmt.Errorf("failed to exec command: %v, stderr: %s", err, stderr.String())
	}

	result := strings.TrimSpace(stdout.String())
	e2e.Logf("    Successfully retrieved encryption prefix (length: %d bytes)", len(result))
	return result, nil
}

// getEncryptionKeyNumber returns the current highest encryption key number
func getEncryptionKeyNumber(ctx context.Context, kubeClient kubernetes.Interface, pattern string) (int, error) {
	e2e.Logf("    Finding highest encryption key number matching pattern: %s", pattern)

	secrets, err := kubeClient.CoreV1().Secrets("openshift-config-managed").List(ctx, metav1.ListOptions{})
	if err != nil {
		e2e.Logf("    ERROR: Failed to list secrets in openshift-config-managed: %v", err)
		return 0, fmt.Errorf("failed to list secrets: %v", err)
	}
	e2e.Logf("    Total secrets in openshift-config-managed: %d", len(secrets.Items))

	maxNumber := 0
	matchCount := 0
	re := regexp.MustCompile(pattern)
	numberRe := regexp.MustCompile(`-(\d+)$`)

	for _, secret := range secrets.Items {
		if re.MatchString(secret.Name) {
			matchCount++
			matches := numberRe.FindStringSubmatch(secret.Name)
			if len(matches) > 1 {
				num, err := strconv.Atoi(matches[1])
				if err == nil && num > maxNumber {
					e2e.Logf("    Found encryption key: %s (number: %d)", secret.Name, num)
					maxNumber = num
				}
			}
		}
	}

	e2e.Logf("    Found %d secrets matching pattern, highest number: %d", matchCount, maxNumber)
	return maxNumber, nil
}

// waitEncryptionKeyMigration waits for encryption migration to complete
func waitEncryptionKeyMigration(ctx context.Context, kubeClient kubernetes.Interface, secretName string) (bool, error) {
	e2e.Logf("    Starting migration wait loop for secret: %s", secretName)
	e2e.Logf("    Timeout: 35 minutes | Poll interval: 30 seconds")

	pollCount := 0
	// Wait up to 35 minutes for encryption migration to complete
	// Increased from 25 to 35 minutes to accommodate slower clusters
	err := wait.PollUntilContextTimeout(ctx, 30*time.Second, 35*time.Minute, false, func(cxt context.Context) (bool, error) {
		pollCount++
		e2e.Logf("    [Poll #%d] Checking migration status...", pollCount)

		// Check if secret exists and get its annotations
		secret, err := kubeClient.CoreV1().Secrets("openshift-config-managed").Get(cxt, secretName, metav1.GetOptions{})
		if err != nil {
			e2e.Logf("    [Poll #%d] Could not retrieve secret %s: %v. Retrying...", pollCount, secretName, err)
			return false, nil
		}

		// Log all annotations for debugging (especially on first few polls)
		if pollCount <= 3 || pollCount%10 == 0 {
			annotations := secret.GetAnnotations()
			e2e.Logf("    [Poll #%d] Secret has %d annotations:", pollCount, len(annotations))
			for key, value := range annotations {
				if strings.Contains(key, "encryption") || strings.Contains(key, "migrat") {
					e2e.Logf("      - %s: %s", key, value)
				}
			}
		}

		annotations := secret.GetAnnotations()
		if migratedResources, ok := annotations["encryption.apiserver.operator.openshift.io/migrated-resources"]; ok {
			e2e.Logf("    [Poll #%d] Migrated resources annotation found: %s", pollCount, migratedResources)
			// When all resources are migrated, check if the expected resources are present
			if strings.Contains(migratedResources, "secrets") {
				e2e.Logf("    [Poll #%d] MIGRATION COMPLETE! 'secrets' found in migrated resources", pollCount)
				return true, nil
			}
			e2e.Logf("    [Poll #%d] Migration in progress (waiting for 'secrets' in annotation)", pollCount)
		} else {
			if pollCount <= 3 || pollCount%10 == 0 {
				e2e.Logf("    [Poll #%d] Migration annotation 'encryption.apiserver.operator.openshift.io/migrated-resources' not yet present on secret", pollCount)
			}
		}

		return false, nil
	})

	if err != nil {
		e2e.Logf("    ERROR: Migration wait failed after %d polls: %v", pollCount, err)
		return false, err
	}
	e2e.Logf("    Migration completed successfully after %d polls", pollCount)
	return true, nil
}

// waitForKubeAPIServerOperatorStable waits for all cluster operators to become stable after cleanup
// This is critical because KAS restoration can take ~20 minutes to stabilize all cluster operators
func waitForKubeAPIServerOperatorStable(ctx context.Context, kubeClient kubernetes.Interface) error {
	e2e.Logf("    Waiting for all cluster operators to stabilize after cleanup...")
	e2e.Logf("    Timeout: 25 minutes | Poll interval: 15 seconds")
	e2e.Logf("    Checking: AVAILABLE=True, PROGRESSING=False, DEGRADED=False")

	pollCount := 0
	// KAS can take ~20 minutes to stabilize, so we use 25 minutes timeout
	err := wait.PollUntilContextTimeout(ctx, 15*time.Second, 25*time.Minute, false, func(cxt context.Context) (bool, error) {
		pollCount++

		// Get cluster operator status using dynamic client approach via raw REST
		result, err := kubeClient.CoreV1().RESTClient().Get().
			AbsPath("/apis/config.openshift.io/v1/clusteroperators").
			DoRaw(cxt)
		if err != nil {
			e2e.Logf("    [Poll #%d] Could not get cluster operators: %v. Retrying...", pollCount, err)
			return false, nil
		}

		// Parse the result to check cluster operator conditions
		// We're looking for any operator that is not: Available=True, Progressing=False, Degraded=False
		// Simple string-based parsing since we just need to verify stability
		resultStr := string(result)

		// First verify we have operator status data
		if !strings.Contains(resultStr, `"type":"Available"`) ||
			!strings.Contains(resultStr, `"type":"Progressing"`) ||
			!strings.Contains(resultStr, `"type":"Degraded"`) {
			e2e.Logf("    [Poll #%d] Waiting for cluster operator status...", pollCount)
			return false, nil
		}

		// Check for BAD patterns (these indicate instability)
		// Allow for different field ordering and spacing in JSON
		hasProgressingTrue := strings.Contains(resultStr, `"type":"Progressing","status":"True"`) ||
			strings.Contains(resultStr, `"status":"True","type":"Progressing"`)

		if hasProgressingTrue {
			// At least one operator is progressing
			if pollCount%4 == 1 { // Log every ~1 minute
				e2e.Logf("    [Poll #%d] Some operators still progressing...", pollCount)
			}
			return false, nil
		}

		hasDegradedTrue := strings.Contains(resultStr, `"type":"Degraded","status":"True"`) ||
			strings.Contains(resultStr, `"status":"True","type":"Degraded"`)

		if hasDegradedTrue {
			// At least one operator is degraded
			e2e.Logf("    [Poll #%d] WARNING: Some operators are degraded", pollCount)
			return false, nil
		}

		hasAvailableFalse := strings.Contains(resultStr, `"type":"Available","status":"False"`) ||
			strings.Contains(resultStr, `"status":"False","type":"Available"`)

		if hasAvailableFalse {
			// At least one operator is not available
			if pollCount%4 == 1 {
				e2e.Logf("    [Poll #%d] Some operators not yet available...", pollCount)
			}
			return false, nil
		}

		// Verify we have GOOD patterns (positive confirmation of stability)
		// Check for Available=True with flexible field ordering
		hasAvailableTrue := strings.Contains(resultStr, `"type":"Available","status":"True"`) ||
			strings.Contains(resultStr, `"status":"True","type":"Available"`)

		if !hasAvailableTrue {
			e2e.Logf("    [Poll #%d] No operators showing Available=True yet...", pollCount)
			return false, nil
		}

		// All operators appear stable
		e2e.Logf("    [Poll #%d] All cluster operators are stable (Available=True, Progressing=False, Degraded=False)", pollCount)
		return true, nil
	})

	if err != nil {
		e2e.Logf("    ERROR: Cluster operators did not stabilize after %d polls: %v", pollCount, err)
		// Log final status for debugging
		e2e.Logf("    Attempting to get final cluster operator status for debugging...")
		result, debugErr := kubeClient.CoreV1().RESTClient().Get().
			AbsPath("/apis/config.openshift.io/v1/clusteroperators").
			DoRaw(ctx)
		if debugErr == nil {
			e2e.Logf("    Final cluster operators status (truncated): %s", string(result)[:500])
		}
		return err
	}
	e2e.Logf("    All cluster operators are stable after %d polls", pollCount)
	return nil
}
