package auth

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/storage"
	informersv1 "k8s.io/client-go/informers"
	fakev1 "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/controller"

	projectapi "github.com/openshift/openshift-apiserver/pkg/project/apis/project"
	projectcache "github.com/openshift/openshift-apiserver/pkg/project/cache"
	projectutil "github.com/openshift/openshift-apiserver/pkg/project/util"
)

func newTestWatcher(username string, groups []string, predicate storage.SelectionPredicate, includeAllExistingProjects bool, namespaces ...*corev1.Namespace) (*userProjectWatcher, *fakeAuthCache, chan struct{}) {
	objects := []runtime.Object{}
	for i := range namespaces {
		objects = append(objects, namespaces[i])
	}
	mockClient := fakev1.NewSimpleClientset(objects...)

	informers := informersv1.NewSharedInformerFactory(mockClient, controller.NoResyncPeriodFunc())
	projectCache := projectcache.NewProjectCache(
		informers.Core().V1().Namespaces().Informer(),
		mockClient.CoreV1().Namespaces(),
		"",
	)
	fakeAuthCache := &fakeAuthCache{}
	if includeAllExistingProjects {
		fakeAuthCache.namespaces = namespaces
	}

	stopCh := make(chan struct{})
	go projectCache.Run(stopCh)

	return NewUserProjectWatcher(&user.DefaultInfo{Name: username, Groups: groups}, sets.NewString("*"), projectCache, fakeAuthCache, includeAllExistingProjects, predicate, false), fakeAuthCache, stopCh
}

type fakeAuthCache struct {
	namespaces []*corev1.Namespace

	removed []CacheWatcher
}

func (w *fakeAuthCache) RemoveWatcher(watcher CacheWatcher) {
	w.removed = append(w.removed, watcher)
}

func (w *fakeAuthCache) List(userInfo user.Info, selector labels.Selector) (*corev1.NamespaceList, error) {
	ret := &corev1.NamespaceList{}
	if w.namespaces != nil {
		for i := range w.namespaces {
			ret.Items = append(ret.Items, *w.namespaces[i])
		}
	}

	return ret, nil
}

func TestFullIncoming(t *testing.T) {
	watcher, fakeAuthCache, stopCh := newTestWatcher("bob", nil, matchAllPredicate(), false, newNamespaces("ns-01")...)
	defer close(stopCh)
	watcher.cacheIncoming = make(chan watch.Event)

	go watcher.Watch()
	watcher.cacheIncoming <- watch.Event{Type: watch.Added}

	// this call should not block and we should see a failure
	watcher.GroupMembershipChanged("ns-01", sets.NewString("bob"), sets.String{})
	if len(fakeAuthCache.removed) != 1 {
		t.Errorf("should have removed self")
	}

	err := wait.PollImmediate(10*time.Millisecond, 5*time.Second, func() (done bool, err error) {
		if len(watcher.cacheError) > 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	for {
		repeat := false
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				t.Fatalf("channel closed")
			}
			// this happens when the cacheIncoming block wins the select race
			if event.Type == watch.Added {
				repeat = true
				break
			}
			// this should be an error
			if event.Type != watch.Error {
				t.Errorf("expected error, got %v", event)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout")
		}
		if !repeat {
			break
		}
	}
}

func TestAddModifyDeleteEventsByUser(t *testing.T) {
	watcher, _, stopCh := newTestWatcher("bob", nil, matchAllPredicate(), false, newNamespaces("ns-01")...)
	defer close(stopCh)
	go watcher.Watch()

	watcher.GroupMembershipChanged("ns-01", sets.NewString("bob"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		if event.Type != watch.Added {
			t.Errorf("expected added, got %v", event)
		}
		if event.Object.(*projectapi.Project).Name != "ns-01" {
			t.Errorf("expected %v, got %#v", "ns-01", event.Object)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout")
	}

	// the object didn't change, we shouldn't observe it
	watcher.GroupMembershipChanged("ns-01", sets.NewString("bob"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		t.Fatalf("unexpected event %v", event)
	case <-time.After(3 * time.Second):
	}

	watcher.GroupMembershipChanged("ns-01", sets.NewString("alice"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		if event.Type != watch.Deleted {
			t.Errorf("expected Deleted, got %v", event)
		}
		if event.Object.(*projectapi.Project).Name != "ns-01" {
			t.Errorf("expected %v, got %#v", "ns-01", event.Object)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout")
	}
}

func TestProjectSelectionPredicate(t *testing.T) {
	field := fields.ParseSelectorOrDie("metadata.name=ns-03")
	m := projectutil.MatchProject(labels.Everything(), field)

	watcher, _, stopCh := newTestWatcher("bob", nil, m, false, newNamespaces("ns-01", "ns-02", "ns-03")...)
	defer close(stopCh)

	if watcher.emit == nil {
		t.Fatalf("unset emit function")
	}

	go watcher.Watch()

	// a namespace we did not select changed, we shouldn't observe it
	watcher.GroupMembershipChanged("ns-01", sets.NewString("bob"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		t.Fatalf("unexpected event %v", event)
	case <-time.After(3 * time.Second):
	}

	watcher.GroupMembershipChanged("ns-03", sets.NewString("bob"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		if event.Type != watch.Added {
			t.Errorf("expected added, got %v", event)
		}
		if event.Object.(*projectapi.Project).Name != "ns-03" {
			t.Errorf("expected %v, got %#v", "ns-03", event.Object)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout")
	}

	// the object didn't change, we shouldn't observe it
	watcher.GroupMembershipChanged("ns-03", sets.NewString("bob"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		t.Fatalf("unexpected event %v", event)
	case <-time.After(3 * time.Second):
	}

	// deletion occurred in a separate namespace, we should not observe it
	watcher.GroupMembershipChanged("ns-01", sets.NewString("alice"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		t.Fatalf("unexpected event %v", event)
	case <-time.After(3 * time.Second):
	}

	// deletion occurred in selected namespace, we should observe it
	watcher.GroupMembershipChanged("ns-03", sets.NewString("alice"), sets.String{})
	select {
	case event := <-watcher.ResultChan():
		if event.Type != watch.Deleted {
			t.Errorf("expected Deleted, got %v", event)
		}
		if event.Object.(*projectapi.Project).Name != "ns-03" {
			t.Errorf("expected %v, got %#v", "ns-03", event.Object)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout")
	}
}

func TestAddModifyDeleteEventsByGroup(t *testing.T) {
	watcher, _, stopCh := newTestWatcher("bob", []string{"group-one"}, matchAllPredicate(), false, newNamespaces("ns-01")...)
	defer close(stopCh)
	go watcher.Watch()

	watcher.GroupMembershipChanged("ns-01", sets.String{}, sets.NewString("group-one"))
	select {
	case event := <-watcher.ResultChan():
		if event.Type != watch.Added {
			t.Errorf("expected added, got %v", event)
		}
		if event.Object.(*projectapi.Project).Name != "ns-01" {
			t.Errorf("expected %v, got %#v", "ns-01", event.Object)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout")
	}

	// the object didn't change, we shouldn't observe it
	watcher.GroupMembershipChanged("ns-01", sets.String{}, sets.NewString("group-one"))
	select {
	case event := <-watcher.ResultChan():
		t.Fatalf("unexpected event %v", event)
	case <-time.After(3 * time.Second):
	}

	watcher.GroupMembershipChanged("ns-01", sets.String{}, sets.NewString("group-two"))
	select {
	case event := <-watcher.ResultChan():
		if event.Type != watch.Deleted {
			t.Errorf("expected Deleted, got %v", event)
		}
		if event.Object.(*projectapi.Project).Name != "ns-01" {
			t.Errorf("expected %v, got %#v", "ns-01", event.Object)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout")
	}
}

func newNamespaces(names ...string) []*corev1.Namespace {
	ret := []*corev1.Namespace{}
	for _, name := range names {
		ret = append(ret, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}

	return ret
}

func matchAllPredicate() storage.SelectionPredicate {
	return projectutil.MatchProject(labels.Everything(), fields.Everything())
}

func newNamespacesWithRV(names ...string) []*corev1.Namespace {
	ret := []*corev1.Namespace{}
	for i, name := range names {
		ret = append(ret, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				ResourceVersion: fmt.Sprintf("%d", i+10),
			},
		})
	}
	return ret
}

func newBookmarkTestWatcher(username string, includeAllExistingProjects bool, namespaces ...*corev1.Namespace) (*userProjectWatcher, chan struct{}) {
	objects := []runtime.Object{}
	for i := range namespaces {
		objects = append(objects, namespaces[i])
	}
	mockClient := fakev1.NewSimpleClientset(objects...)
	informers := informersv1.NewSharedInformerFactory(mockClient, controller.NoResyncPeriodFunc())
	projectCache := projectcache.NewProjectCache(
		informers.Core().V1().Namespaces().Informer(),
		mockClient.CoreV1().Namespaces(),
		"",
	)
	fakeAuthCache := &fakeAuthCache{namespaces: namespaces}
	stopCh := make(chan struct{})
	go projectCache.Run(stopCh)
	w := NewUserProjectWatcher(
		&user.DefaultInfo{Name: username},
		sets.NewString("*"),
		projectCache,
		fakeAuthCache,
		includeAllExistingProjects,
		matchAllPredicate(),
		true,
	)
	return w, stopCh
}

func TestSendInitialEventsBookmark(t *testing.T) {
	t.Run("with rv=0", func(t *testing.T) {
		watcher, stopCh := newBookmarkTestWatcher("bob", true, newNamespacesWithRV("ns-01", "ns-02")...)
		defer close(stopCh)

		go watcher.Watch()

		// expect 2 initial Added events
		for i := 0; i < 2; i++ {
			select {
			case event := <-watcher.ResultChan():
				if event.Type != watch.Added {
					t.Errorf("expected Added, got %v", event.Type)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("timeout waiting for initial event %d", i)
			}
		}

		// expect bookmark with annotation and ResourceVersion
		select {
		case event := <-watcher.ResultChan():
			if event.Type != watch.Bookmark {
				t.Errorf("expected Bookmark, got %v", event.Type)
			}
			project := event.Object.(*projectapi.Project)
			if project.Annotations[metav1.InitialEventsAnnotationKey] != "true" {
				t.Errorf("expected initial-events-end annotation")
			}
			if project.ResourceVersion != "11" {
				t.Errorf("expected bookmark ResourceVersion %q, got %q", "11", project.ResourceVersion)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout waiting for bookmark")
		}
	})

	t.Run("bookmark bypasses field selector predicate", func(t *testing.T) {
		// Verify that bookmark events are delivered even when a field selector
		// (e.g. metadata.name=<project>) would reject the bookmark's empty Name.
		// This is critical for oc delete --wait which watches a specific project
		// and needs the bookmark to complete the initial events stream.
		field := fields.ParseSelectorOrDie("metadata.name=ns-01")
		m := projectutil.MatchProject(labels.Everything(), field)

		objects := []runtime.Object{}
		namespaces := newNamespacesWithRV("ns-01", "ns-02")
		for i := range namespaces {
			objects = append(objects, namespaces[i])
		}
		mockClient := fakev1.NewSimpleClientset(objects...)
		informers := informersv1.NewSharedInformerFactory(mockClient, controller.NoResyncPeriodFunc())
		projectCache := projectcache.NewProjectCache(
			informers.Core().V1().Namespaces().Informer(),
			mockClient.CoreV1().Namespaces(),
			"",
		)
		fakeAuthCache := &fakeAuthCache{namespaces: namespaces}
		stopCh := make(chan struct{})
		defer close(stopCh)
		go projectCache.Run(stopCh)

		w := NewUserProjectWatcher(
			&user.DefaultInfo{Name: "bob"},
			sets.NewString("*"),
			projectCache,
			fakeAuthCache,
			false,
			m,
			true,
		)
		go w.Watch()

		select {
		case event := <-w.ResultChan():
			if event.Type != watch.Bookmark {
				t.Errorf("expected Bookmark, got %v", event.Type)
			}
			project := event.Object.(*projectapi.Project)
			if project.Annotations[metav1.InitialEventsAnnotationKey] != "true" {
				t.Errorf("expected initial-events-end annotation")
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout waiting for bookmark — predicate likely filtered it out")
		}
	})

	t.Run("without rv=0", func(t *testing.T) {
		watcher, stopCh := newBookmarkTestWatcher("bob", false, newNamespacesWithRV("ns-01", "ns-02")...)
		defer close(stopCh)

		go watcher.Watch()

		// expect bookmark with annotation and ResourceVersion immediately
		select {
		case event := <-watcher.ResultChan():
			if event.Type != watch.Bookmark {
				t.Errorf("expected Bookmark, got %v", event.Type)
			}
			project := event.Object.(*projectapi.Project)
			if project.Annotations[metav1.InitialEventsAnnotationKey] != "true" {
				t.Errorf("expected initial-events-end annotation")
			}
			if project.ResourceVersion != "11" {
				t.Errorf("expected bookmark ResourceVersion %q, got %q", "11", project.ResourceVersion)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout waiting for bookmark")
		}

		// verify no additional events
		select {
		case event := <-watcher.ResultChan():
			t.Fatalf("unexpected event after bookmark: %v", event)
		case <-time.After(500 * time.Millisecond):
			// expected - no more events
		}
	})
}
