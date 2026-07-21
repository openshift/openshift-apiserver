package deploylog

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	watchtools "k8s.io/client-go/tools/watch"

	appsv1 "github.com/openshift/api/apps/v1"
	"github.com/openshift/library-go/pkg/apps/appsutil"
)

var (
	// ErrUnknownDeploymentPhase is returned for WaitForRunningDeployment if an unknown phase is returned.
	ErrUnknownDeploymentPhase = errors.New("unknown deployment phase")
)

// WaitForRunningDeployment waits until the specified deployment is no longer New or Pending. Returns true if
// the deployment became running, complete, or failed within timeout, false if it did not, and an error if any
// other error state occurred. The last observed deployment state is returned.
func WaitForRunningDeployment(ctx context.Context, rn corev1client.ReplicationControllersGetter, observed *corev1.ReplicationController, timeout time.Duration) (*corev1.ReplicationController, error) {
	ctx, cancel := watchtools.ContextWithOptionalTimeout(ctx, timeout)
	defer cancel()

	// Check current state first.
	rc, err := rn.ReplicationControllers(observed.Namespace).Get(ctx, observed.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if observed.UID != rc.UID {
		return nil, fmt.Errorf("%s '%s/%s' no longer exists, expected UID %q, got UID %q", corev1.Resource("replicationcontrollers"), observed.Namespace, observed.Name, observed.UID, rc.UID)
	}
	if done, err := checkDeploymentStatus(rc); done {
		if err != nil {
			return nil, err
		}
		return rc, nil
	}

	// Watch for status changes.
	fieldSelector := fields.OneTermEqualSelector("metadata.name", observed.Name).String()
	w, err := rn.ReplicationControllers(observed.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector:   fieldSelector,
		ResourceVersion: rc.ResourceVersion,
	})
	if err != nil {
		return nil, err
	}
	defer w.Stop()

	event, err := watchtools.UntilWithoutRetry(ctx, w, func(e watch.Event) (bool, error) {
		switch e.Type {
		case watch.Added, watch.Modified:
			newRc, ok := e.Object.(*corev1.ReplicationController)
			if !ok {
				return true, fmt.Errorf("unknown event object %#v", e.Object)
			}
			return checkDeploymentStatus(newRc)

		case watch.Deleted:
			return true, fmt.Errorf("replicationController got deleted %#v", e.Object)

		case watch.Error:
			return true, fmt.Errorf("unexpected error %#v", e.Object)

		default:
			return true, fmt.Errorf("unexpected event type: %T", e.Type)
		}
	})
	if err != nil {
		return nil, err
	}

	return event.Object.(*corev1.ReplicationController), nil
}

func checkDeploymentStatus(rc *corev1.ReplicationController) (bool, error) {
	switch appsutil.DeploymentStatusFor(rc) {
	case appsv1.DeploymentStatusRunning, appsv1.DeploymentStatusFailed, appsv1.DeploymentStatusComplete:
		return true, nil
	case appsv1.DeploymentStatusNew, appsv1.DeploymentStatusPending:
		return false, nil
	default:
		return true, ErrUnknownDeploymentPhase
	}
}
