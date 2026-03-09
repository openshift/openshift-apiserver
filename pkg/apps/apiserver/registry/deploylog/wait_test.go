package deploylog

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	appsv1 "github.com/openshift/api/apps/v1"
)

func TestWaitForRunningDeploymentSuccess(t *testing.T) {
	fakeController := &corev1.ReplicationController{}
	fakeController.Name = "test-1"
	fakeController.Namespace = "test"
	fakeController.Annotations = map[string]string{appsv1.DeploymentStatusAnnotation: string(appsv1.DeploymentStatusRunning)}

	kubeclient := fake.NewSimpleClientset([]runtime.Object{fakeController}...)
	// The fake client doesn't properly support bookmark events, so we reject SendInitialEvents to force fallback
	kubeclient.PrependWatchReactor("replicationcontrollers", func(action clientgotesting.Action) (handled bool, ret watch.Interface, err error) {
		watchAction := action.(clientgotesting.WatchActionImpl)
		if watchAction.ListOptions.SendInitialEvents != nil && *watchAction.ListOptions.SendInitialEvents {
			return true, nil, errors.New("sendInitialEvents is not supported in fake client")
		}
		// Fall back to default reactor for legacy watch
		fakeWatch := watch.NewFake()
		go fakeWatch.Modify(fakeController)
		return clientgotesting.DefaultWatchReactor(fakeWatch, nil)(action)
	})

	rc, err := WaitForRunningDeployment(context.TODO(), kubeclient.CoreV1(), fakeController, 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc == nil {
		t.Errorf("expected returned replication controller to not be nil")
	}
}
