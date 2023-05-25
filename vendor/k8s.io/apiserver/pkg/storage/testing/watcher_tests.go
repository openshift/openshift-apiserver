/*
Copyright 2016 The Kubernetes Authors.

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

package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/apis/example"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/value"
	utilflowcontrol "k8s.io/apiserver/pkg/util/flowcontrol"
)

func RunTestWatch(ctx context.Context, t *testing.T, store storage.Interface) {
	testWatch(ctx, t, store, false)
	testWatch(ctx, t, store, true)
}

// It tests that
// - first occurrence of objects should notify Add event
// - update should trigger Modified event
// - update that gets filtered should trigger Deleted event
func testWatch(ctx context.Context, t *testing.T, store storage.Interface, recursive bool) {
	basePod := &example.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		Spec:       example.PodSpec{NodeName: ""},
	}
	basePodAssigned := &example.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		Spec:       example.PodSpec{NodeName: "bar"},
	}

	tests := []struct {
		name       string
		namespace  string
		key        string
		pred       storage.SelectionPredicate
		watchTests []*testWatchStruct
	}{{
		name:       "create a key",
		namespace:  fmt.Sprintf("test-ns-1-%t", recursive),
		watchTests: []*testWatchStruct{{basePod, true, watch.Added}},
		pred:       storage.Everything,
	}, {
		name:       "key updated to match predicate",
		namespace:  fmt.Sprintf("test-ns-2-%t", recursive),
		watchTests: []*testWatchStruct{{basePod, false, ""}, {basePodAssigned, true, watch.Added}},
		pred: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.ParseSelectorOrDie("spec.nodeName=bar"),
			GetAttrs: func(obj runtime.Object) (labels.Set, fields.Set, error) {
				pod := obj.(*example.Pod)
				return nil, fields.Set{"spec.nodeName": pod.Spec.NodeName}, nil
			},
		},
	}, {
		name:       "update",
		namespace:  fmt.Sprintf("test-ns-3-%t", recursive),
		watchTests: []*testWatchStruct{{basePod, true, watch.Added}, {basePodAssigned, true, watch.Modified}},
		pred:       storage.Everything,
	}, {
		name:       "delete because of being filtered",
		namespace:  fmt.Sprintf("test-ns-4-%t", recursive),
		watchTests: []*testWatchStruct{{basePod, true, watch.Added}, {basePodAssigned, true, watch.Deleted}},
		pred: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.ParseSelectorOrDie("spec.nodeName!=bar"),
			GetAttrs: func(obj runtime.Object) (labels.Set, fields.Set, error) {
				pod := obj.(*example.Pod)
				return nil, fields.Set{"spec.nodeName": pod.Spec.NodeName}, nil
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watchKey := fmt.Sprintf("/pods/%s", tt.namespace)
			key := watchKey + "/foo"
			if !recursive {
				watchKey = key
			}

			// Get the current RV from which we can start watching.
			out := &example.PodList{}
			if err := store.GetList(ctx, watchKey, storage.ListOptions{ResourceVersion: "", Predicate: tt.pred, Recursive: recursive}, out); err != nil {
				t.Fatalf("List failed: %v", err)
			}

			w, err := store.Watch(ctx, watchKey, storage.ListOptions{ResourceVersion: out.ResourceVersion, Predicate: tt.pred, Recursive: recursive})
			if err != nil {
				t.Fatalf("Watch failed: %v", err)
			}
			var prevObj *example.Pod
			for _, watchTest := range tt.watchTests {
				out := &example.Pod{}
				err := store.GuaranteedUpdate(ctx, key, out, true, nil, storage.SimpleUpdate(
					func(runtime.Object) (runtime.Object, error) {
						obj := watchTest.obj.DeepCopy()
						obj.Namespace = tt.namespace
						return obj, nil
					}), nil)
				if err != nil {
					t.Fatalf("GuaranteedUpdate failed: %v", err)
				}
				if watchTest.expectEvent {
					expectObj := out
					if watchTest.watchType == watch.Deleted {
						expectObj = prevObj
						expectObj.ResourceVersion = out.ResourceVersion
					}
					testCheckResult(t, watchTest.watchType, w, expectObj)
				}
				prevObj = out
			}
			w.Stop()
			testCheckStop(t, w)
		})
	}
}

// RunTestWatchFromZero tests that
// - watch from 0 should sync up and grab the object added before
// - watch from 0 is able to return events for objects whose previous version has been compacted
func RunTestWatchFromZero(ctx context.Context, t *testing.T, store storage.Interface, compaction Compaction) {
	key, storedObj := testPropagateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns"}})

	w, err := store.Watch(ctx, key, storage.ListOptions{ResourceVersion: "0", Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	testCheckResult(t, watch.Added, w, storedObj)
	w.Stop()

	// Update
	out := &example.Pod{}
	err = store.GuaranteedUpdate(ctx, key, out, true, nil, storage.SimpleUpdate(
		func(runtime.Object) (runtime.Object, error) {
			return &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns", Annotations: map[string]string{"a": "1"}}}, nil
		}), nil)
	if err != nil {
		t.Fatalf("GuaranteedUpdate failed: %v", err)
	}

	// Make sure when we watch from 0 we receive an ADDED event
	w, err = store.Watch(ctx, key, storage.ListOptions{ResourceVersion: "0", Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	testCheckResult(t, watch.Added, w, out)
	w.Stop()

	if compaction == nil {
		t.Skip("compaction callback not provided")
	}

	// Update again
	out = &example.Pod{}
	err = store.GuaranteedUpdate(ctx, key, out, true, nil, storage.SimpleUpdate(
		func(runtime.Object) (runtime.Object, error) {
			return &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns"}}, nil
		}), nil)
	if err != nil {
		t.Fatalf("GuaranteedUpdate failed: %v", err)
	}

	// Compact previous versions
	compaction(ctx, t, out.ResourceVersion)

	// Make sure we can still watch from 0 and receive an ADDED event
	w, err = store.Watch(ctx, key, storage.ListOptions{ResourceVersion: "0", Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	testCheckResult(t, watch.Added, w, out)
}

func RunTestDeleteTriggerWatch(ctx context.Context, t *testing.T, store storage.Interface) {
	key, storedObj := testPropagateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns"}})
	w, err := store.Watch(ctx, key, storage.ListOptions{ResourceVersion: storedObj.ResourceVersion, Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	if err := store.Delete(ctx, key, &example.Pod{}, nil, storage.ValidateAllObjectFunc, nil); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	testCheckEventType(t, watch.Deleted, w)
}

func RunTestWatchFromNoneZero(ctx context.Context, t *testing.T, store storage.Interface) {
	key, storedObj := testPropagateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns"}})

	w, err := store.Watch(ctx, key, storage.ListOptions{ResourceVersion: storedObj.ResourceVersion, Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	out := &example.Pod{}
	store.GuaranteedUpdate(ctx, key, out, true, nil, storage.SimpleUpdate(
		func(runtime.Object) (runtime.Object, error) {
			newObj := storedObj.DeepCopy()
			newObj.Annotations = map[string]string{"version": "2"}
			return newObj, nil
		}), nil)
	testCheckResult(t, watch.Modified, w, out)
}

func RunTestWatchError(ctx context.Context, t *testing.T, store InterfaceWithPrefixTransformer) {
	obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns"}}
	key := computePodKey(obj)

	// Compute the initial resource version from which we can start watching later.
	list := &example.PodList{}
	storageOpts := storage.ListOptions{
		ResourceVersion: "0",
		Predicate:       storage.Everything,
		Recursive:       true,
	}
	if err := store.GetList(ctx, "/pods", storageOpts, list); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := store.GuaranteedUpdate(ctx, key, &example.Pod{}, true, nil, storage.SimpleUpdate(
		func(runtime.Object) (runtime.Object, error) {
			return obj, nil
		}), nil); err != nil {
		t.Fatalf("GuaranteedUpdate failed: %v", err)
	}

	// Now trigger watch error by injecting failing transformer.
	revertTransformer := store.UpdatePrefixTransformer(
		func(previousTransformer *PrefixTransformer) value.Transformer {
			return &failingTransformer{}
		})
	defer revertTransformer()

	w, err := store.Watch(ctx, key, storage.ListOptions{ResourceVersion: list.ResourceVersion, Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	testCheckEventType(t, watch.Error, w)
}

func RunTestWatchContextCancel(ctx context.Context, t *testing.T, store storage.Interface) {
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	// When we watch with a canceled context, we should detect that it's context canceled.
	// We won't take it as error and also close the watcher.
	w, err := store.Watch(canceledCtx, "/pods/not-existing", storage.ListOptions{
		ResourceVersion: "0",
		Predicate:       storage.Everything,
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case _, ok := <-w.ResultChan():
		if ok {
			t.Error("ResultChan() should be closed")
		}
	case <-time.After(wait.ForeverTestTimeout):
		t.Errorf("timeout after %v", wait.ForeverTestTimeout)
	}
}

func RunTestWatchDeleteEventObjectHaveLatestRV(ctx context.Context, t *testing.T, store storage.Interface) {
	key, storedObj := testPropagateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns"}})

	watchCtx, cancel := context.WithTimeout(ctx, wait.ForeverTestTimeout)
	t.Cleanup(cancel)
	w, err := store.Watch(watchCtx, key, storage.ListOptions{ResourceVersion: storedObj.ResourceVersion, Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	deletedObj := &example.Pod{}
	if err := store.Delete(ctx, key, deletedObj, &storage.Preconditions{}, storage.ValidateAllObjectFunc, nil); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify that ResourceVersion has changed on deletion.
	if storedObj.ResourceVersion == deletedObj.ResourceVersion {
		t.Fatalf("ResourceVersion didn't changed on deletion: %s", deletedObj.ResourceVersion)
	}

	select {
	case event := <-w.ResultChan():
		watchedDeleteObj := event.Object.(*example.Pod)
		if e, a := deletedObj.ResourceVersion, watchedDeleteObj.ResourceVersion; e != a {
			t.Errorf("Unexpected resource version: %v, expected %v", a, e)
		}
	}
}

func RunTestWatchInitializationSignal(ctx context.Context, t *testing.T, store storage.Interface) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	t.Cleanup(cancel)
	initSignal := utilflowcontrol.NewInitializationSignal()
	ctx = utilflowcontrol.WithInitializationSignal(ctx, initSignal)

	key, storedObj := testPropagateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "test-ns"}})
	_, err := store.Watch(ctx, key, storage.ListOptions{ResourceVersion: storedObj.ResourceVersion, Predicate: storage.Everything})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	initSignal.Wait()
}

// RunOptionalTestProgressNotify tests ProgressNotify feature of ListOptions.
// Given this feature is currently not explicitly used by higher layers of Kubernetes
// (it rather is used by wrappers of storage.Interface to implement its functionalities)
// this test is currently considered optional.
func RunOptionalTestProgressNotify(ctx context.Context, t *testing.T, store storage.Interface) {
	input := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "test-ns"}}
	key := computePodKey(input)
	out := &example.Pod{}
	if err := store.Create(ctx, key, input, out, 0); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	validateResourceVersion := resourceVersionNotOlderThan(out.ResourceVersion)

	opts := storage.ListOptions{
		ResourceVersion: out.ResourceVersion,
		Predicate:       storage.Everything,
		ProgressNotify:  true,
	}
	w, err := store.Watch(ctx, key, opts)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// when we send a bookmark event, the client expects the event to contain an
	// object of the correct type, but with no fields set other than the resourceVersion
	testCheckResultFunc(t, watch.Bookmark, w, func(object runtime.Object) error {
		// first, check that we have the correct resource version
		obj, ok := object.(metav1.Object)
		if !ok {
			return fmt.Errorf("got %T, not metav1.Object", object)
		}
		if err := validateResourceVersion(obj.GetResourceVersion()); err != nil {
			return err
		}

		// then, check that we have the right type and content
		pod, ok := object.(*example.Pod)
		if !ok {
			return fmt.Errorf("got %T, not *example.Pod", object)
		}
		pod.ResourceVersion = ""
		ExpectNoDiff(t, "bookmark event should contain an object with no fields set other than resourceVersion", &example.Pod{}, pod)
		return nil
	})
}

// It tests watches of cluster-scoped resources.
func TestClusterScopedWatch(ctx context.Context, t *testing.T, store storage.Interface) {
	tests := []struct {
		name string
		// For watch request, the name of object is specified with field selector
		// "metadata.name=objectName". So in this watch tests, we should set the
		// requestedName and field selector "metadata.name=requestedName" at the
		// same time or set neighter of them.
		requestedName string
		recursive     bool
		fieldSelector fields.Selector
		indexFields   []string
		watchTests    []*testWatchStruct
	}{
		{
			name:          "cluster-wide watch, request without name, without field selector",
			recursive:     true,
			fieldSelector: fields.Everything(),
			watchTests: []*testWatchStruct{
				{basePod("t1-foo1"), true, watch.Added},
				{basePodUpdated("t1-foo1"), true, watch.Modified},
				{basePodAssigned("t1-foo2", "t1-bar1"), true, watch.Added},
			},
		},
		{
			name:          "cluster-wide watch, request without name, field selector with spec.nodeName",
			recursive:     true,
			fieldSelector: fields.ParseSelectorOrDie("spec.nodeName=t2-bar1"),
			indexFields:   []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{basePod("t2-foo1"), false, ""},
				{basePodAssigned("t2-foo1", "t2-bar1"), true, watch.Added},
			},
		},
		{
			name:          "cluster-wide watch, request without name, field selector with spec.nodeName to filter out watch",
			recursive:     true,
			fieldSelector: fields.ParseSelectorOrDie("spec.nodeName!=t3-bar1"),
			indexFields:   []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{basePod("t3-foo1"), true, watch.Added},
				{basePod("t3-foo2"), true, watch.Added},
				{basePodUpdated("t3-foo1"), true, watch.Modified},
				{basePodAssigned("t3-foo1", "t3-bar1"), true, watch.Deleted},
			},
		},
		{
			name:          "cluster-wide watch, request with name, field selector with metadata.name",
			requestedName: "t4-foo1",
			fieldSelector: fields.ParseSelectorOrDie("metadata.name=t4-foo1"),
			watchTests: []*testWatchStruct{
				{basePod("t4-foo1"), true, watch.Added},
				{basePod("t4-foo2"), false, ""},
				{basePodUpdated("t4-foo1"), true, watch.Modified},
				{basePodUpdated("t4-foo2"), false, ""},
			},
		},
		{
			name:          "cluster-wide watch, request with name, field selector with metadata.name and spec.nodeName",
			requestedName: "t5-foo1",
			fieldSelector: fields.SelectorFromSet(fields.Set{
				"metadata.name": "t5-foo1",
				"spec.nodeName": "t5-bar1",
			}),
			indexFields: []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{basePod("t5-foo1"), false, ""},
				{basePod("t5-foo2"), false, ""},
				{basePodUpdated("t5-foo1"), false, ""},
				{basePodUpdated("t5-foo2"), false, ""},
				{basePodAssigned("t5-foo1", "t5-bar1"), true, watch.Added},
			},
		},
		{
			name:          "cluster-wide watch, request with name, field selector with metadata.name, and with spec.nodeName to filter out watch",
			requestedName: "t6-foo1",
			fieldSelector: fields.AndSelectors(
				fields.ParseSelectorOrDie("spec.nodeName!=t6-bar1"),
				fields.SelectorFromSet(fields.Set{"metadata.name": "t6-foo1"}),
			),
			indexFields: []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{basePod("t6-foo1"), true, watch.Added},
				{basePod("t6-foo2"), false, ""},
				{basePodUpdated("t6-foo1"), true, watch.Modified},
				{basePodAssigned("t6-foo1", "t6-bar1"), true, watch.Deleted},
				{basePodAssigned("t6-foo2", "t6-bar1"), false, ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestInfo := &genericapirequest.RequestInfo{}
			requestInfo.Name = tt.requestedName
			requestInfo.Namespace = ""
			ctx = genericapirequest.WithRequestInfo(ctx, requestInfo)
			ctx = genericapirequest.WithNamespace(ctx, "")

			watchKey := "/pods"
			if tt.requestedName != "" {
				watchKey += "/" + tt.requestedName
			}

			predicate := createPodPredicate(tt.fieldSelector, false, tt.indexFields)

			list := &example.PodList{}
			opts := storage.ListOptions{
				ResourceVersion: "",
				Predicate:       predicate,
				Recursive:       true,
			}
			if err := store.GetList(ctx, "/pods", opts, list); err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			opts.ResourceVersion = list.ResourceVersion
			opts.Recursive = tt.recursive

			w, err := store.Watch(ctx, watchKey, opts)
			if err != nil {
				t.Fatalf("Watch failed: %v", err)
			}

			currentObjs := map[string]*example.Pod{}
			for _, watchTest := range tt.watchTests {
				out := &example.Pod{}
				key := "pods/" + watchTest.obj.Name
				err := store.GuaranteedUpdate(ctx, key, out, true, nil, storage.SimpleUpdate(
					func(runtime.Object) (runtime.Object, error) {
						obj := watchTest.obj.DeepCopy()
						return obj, nil
					}), nil)
				if err != nil {
					t.Fatalf("GuaranteedUpdate failed: %v", err)
				}

				expectObj := out
				if watchTest.watchType == watch.Deleted {
					expectObj = currentObjs[watchTest.obj.Name]
					expectObj.ResourceVersion = out.ResourceVersion
					delete(currentObjs, watchTest.obj.Name)
				} else {
					currentObjs[watchTest.obj.Name] = out
				}
				if watchTest.expectEvent {
					testCheckResult(t, watchTest.watchType, w, expectObj)
				}
			}
			w.Stop()
			testCheckStop(t, w)
		})
	}
}

// It tests watch of namespace-scoped resources.
func TestNamespaceScopedWatch(ctx context.Context, t *testing.T, store storage.Interface) {
	tests := []struct {
		name string
		// For watch request, the name of object is specified with field selector
		// "metadata.name=objectName". So in this watch tests, we should set the
		// requestedName and field selector "metadata.name=requestedName" at the
		// same time or set neighter of them.
		requestedName      string
		requestedNamespace string
		recursive          bool
		fieldSelector      fields.Selector
		indexFields        []string
		watchTests         []*testWatchStruct
	}{
		{
			name:          "namespaced watch, request without name, request without namespace, without field selector",
			recursive:     true,
			fieldSelector: fields.Everything(),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t1-foo1", "t1-ns1"), true, watch.Added},
				{baseNamespacedPod("t1-foo2", "t1-ns2"), true, watch.Added},
				{baseNamespacedPodUpdated("t1-foo1", "t1-ns1"), true, watch.Modified},
				{baseNamespacedPodUpdated("t1-foo2", "t1-ns2"), true, watch.Modified},
			},
		},
		{
			name:          "namespaced watch, request without name, request without namespace, field selector with metadata.namespace",
			recursive:     true,
			fieldSelector: fields.ParseSelectorOrDie("metadata.namespace=t2-ns1"),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t2-foo1", "t2-ns1"), true, watch.Added},
				{baseNamespacedPod("t2-foo1", "t2-ns2"), false, ""},
				{baseNamespacedPodUpdated("t2-foo1", "t2-ns1"), true, watch.Modified},
				{baseNamespacedPodUpdated("t2-foo1", "t2-ns2"), false, ""},
			},
		},
		{
			name:          "namespaced watch, request without name, request without namespace, field selector with spec.nodename",
			recursive:     true,
			fieldSelector: fields.ParseSelectorOrDie("spec.nodeName=t3-bar1"),
			indexFields:   []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t3-foo1", "t3-ns1"), false, ""},
				{baseNamespacedPod("t3-foo2", "t3-ns2"), false, ""},
				{baseNamespacedPodAssigned("t3-foo1", "t3-ns1", "t3-bar1"), true, watch.Added},
				{baseNamespacedPodAssigned("t3-foo2", "t3-ns2", "t3-bar1"), true, watch.Added},
			},
		},
		{
			name:          "namespaced watch, request without name, request without namespace, field selector with spec.nodename to filter out watch",
			recursive:     true,
			fieldSelector: fields.ParseSelectorOrDie("spec.nodeName!=t4-bar1"),
			indexFields:   []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t4-foo1", "t4-ns1"), true, watch.Added},
				{baseNamespacedPod("t4-foo2", "t4-ns1"), true, watch.Added},
				{baseNamespacedPodUpdated("t4-foo1", "t4-ns1"), true, watch.Modified},
				{baseNamespacedPodAssigned("t4-foo1", "t4-ns1", "t4-bar1"), true, watch.Deleted},
			},
		},
		{
			name:               "namespaced watch, request without name, request with namespace, without field selector",
			requestedNamespace: "t5-ns1",
			recursive:          true,
			fieldSelector:      fields.Everything(),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t5-foo1", "t5-ns1"), true, watch.Added},
				{baseNamespacedPod("t5-foo1", "t5-ns2"), false, ""},
				{baseNamespacedPod("t5-foo2", "t5-ns1"), true, watch.Added},
				{baseNamespacedPodUpdated("t5-foo1", "t5-ns1"), true, watch.Modified},
				{baseNamespacedPodUpdated("t5-foo1", "t5-ns2"), false, ""},
			},
		},
		{
			name:               "namespaced watch, request without name, request with namespace, field selector with matched metadata.namespace",
			requestedNamespace: "t6-ns1",
			recursive:          true,
			fieldSelector:      fields.ParseSelectorOrDie("metadata.namespace=t6-ns1"),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t6-foo1", "t6-ns1"), true, watch.Added},
				{baseNamespacedPod("t6-foo1", "t6-ns2"), false, ""},
				{baseNamespacedPodUpdated("t6-foo1", "t6-ns1"), true, watch.Modified},
			},
		},
		{
			name:               "namespaced watch, request without name, request with namespace, field selector with non-matched metadata.namespace",
			requestedNamespace: "t7-ns1",
			recursive:          true,
			fieldSelector:      fields.ParseSelectorOrDie("metadata.namespace=t7-ns2"),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t7-foo1", "t7-ns1"), false, ""},
				{baseNamespacedPod("t7-foo1", "t7-ns2"), false, ""},
				{baseNamespacedPodUpdated("t7-foo1", "t7-ns1"), false, ""},
				{baseNamespacedPodUpdated("t7-foo1", "t7-ns2"), false, ""},
			},
		},
		{
			name:               "namespaced watch, request without name, request with namespace, field selector with spec.nodename",
			requestedNamespace: "t8-ns1",
			recursive:          true,
			fieldSelector:      fields.ParseSelectorOrDie("spec.nodeName=t8-bar2"),
			indexFields:        []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t8-foo1", "t8-ns1"), false, ""},
				{baseNamespacedPodAssigned("t8-foo1", "t8-ns1", "t8-bar1"), false, ""},
				{baseNamespacedPodAssigned("t8-foo1", "t8-ns2", "t8-bar2"), false, ""},
				{baseNamespacedPodAssigned("t8-foo1", "t8-ns1", "t8-bar2"), true, watch.Added},
			},
		},
		{
			name:               "namespaced watch, request without name, request with namespace, field selector with spec.nodename to filter out watch",
			requestedNamespace: "t9-ns2",
			recursive:          true,
			fieldSelector:      fields.ParseSelectorOrDie("spec.nodeName!=t9-bar1"),
			indexFields:        []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t9-foo1", "t9-ns1"), false, ""},
				{baseNamespacedPod("t9-foo1", "t9-ns2"), true, watch.Added},
				{baseNamespacedPodAssigned("t9-foo1", "t9-ns2", "t9-bar1"), true, watch.Deleted},
				{baseNamespacedPodAssigned("t9-foo1", "t9-ns2", "t9-bar2"), true, watch.Added},
			},
		},
		{
			name:          "namespaced watch, request with name, request without namespace, field selector with metadata.name",
			requestedName: "t10-foo1",
			recursive:     true,
			fieldSelector: fields.ParseSelectorOrDie("metadata.name=t10-foo1"),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t10-foo1", "t10-ns1"), true, watch.Added},
				{baseNamespacedPod("t10-foo1", "t10-ns2"), true, watch.Added},
				{baseNamespacedPod("t10-foo2", "t10-ns1"), false, ""},
				{baseNamespacedPodUpdated("t10-foo1", "t10-ns1"), true, watch.Modified},
				{baseNamespacedPodAssigned("t10-foo1", "t10-ns1", "t10-bar1"), true, watch.Modified},
			},
		},
		{
			name:          "namespaced watch, request with name, request without namespace, field selector with metadata.name and metadata.namespace",
			requestedName: "t11-foo1",
			recursive:     true,
			fieldSelector: fields.SelectorFromSet(fields.Set{
				"metadata.name":      "t11-foo1",
				"metadata.namespace": "t11-ns1",
			}),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t11-foo1", "t11-ns1"), true, watch.Added},
				{baseNamespacedPod("t11-foo2", "t11-ns1"), false, ""},
				{baseNamespacedPod("t11-foo1", "t11-ns2"), false, ""},
				{baseNamespacedPodUpdated("t11-foo1", "t11-ns1"), true, watch.Modified},
				{baseNamespacedPodAssigned("t11-foo1", "t11-ns1", "t11-bar1"), true, watch.Modified},
			},
		},
		{
			name:          "namespaced watch, request with name, request without namespace, field selector with metadata.name and spec.nodeName",
			requestedName: "t12-foo1",
			recursive:     true,
			fieldSelector: fields.SelectorFromSet(fields.Set{
				"metadata.name": "t12-foo1",
				"spec.nodeName": "t12-bar1",
			}),
			indexFields: []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t12-foo1", "t12-ns1"), false, ""},
				{baseNamespacedPodUpdated("t12-foo1", "t12-ns1"), false, ""},
				{baseNamespacedPodAssigned("t12-foo1", "t12-ns1", "t12-bar1"), true, watch.Added},
			},
		},
		{
			name:          "namespaced watch, request with name, request without namespace, field selector with metadata.name, and with spec.nodeName to filter out watch",
			requestedName: "t15-foo1",
			recursive:     true,
			fieldSelector: fields.AndSelectors(
				fields.ParseSelectorOrDie("spec.nodeName!=t15-bar1"),
				fields.SelectorFromSet(fields.Set{"metadata.name": "t15-foo1"}),
			),
			indexFields: []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t15-foo1", "t15-ns1"), true, watch.Added},
				{baseNamespacedPod("t15-foo2", "t15-ns1"), false, ""},
				{baseNamespacedPodUpdated("t15-foo1", "t15-ns1"), true, watch.Modified},
				{baseNamespacedPodAssigned("t15-foo1", "t15-ns1", "t15-bar1"), true, watch.Deleted},
				{baseNamespacedPodAssigned("t15-foo1", "t15-ns1", "t15-bar2"), true, watch.Added},
			},
		},
		{
			name:               "namespaced watch, request with name, request with namespace, with field selector metadata.name",
			requestedName:      "t16-foo1",
			requestedNamespace: "t16-ns1",
			fieldSelector:      fields.ParseSelectorOrDie("metadata.name=t16-foo1"),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t16-foo1", "t16-ns1"), true, watch.Added},
				{baseNamespacedPod("t16-foo2", "t16-ns1"), false, ""},
				{baseNamespacedPodUpdated("t16-foo1", "t16-ns1"), true, watch.Modified},
				{baseNamespacedPodAssigned("t16-foo1", "t16-ns1", "t16-bar1"), true, watch.Modified},
			},
		},
		{
			name:               "namespaced watch, request with name, request with namespace, with field selector metadata.name and metadata.namespace",
			requestedName:      "t17-foo2",
			requestedNamespace: "t17-ns1",
			fieldSelector: fields.SelectorFromSet(fields.Set{
				"metadata.name":      "t17-foo2",
				"metadata.namespace": "t17-ns1",
			}),
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t17-foo1", "t17-ns1"), false, ""},
				{baseNamespacedPod("t17-foo2", "t17-ns1"), true, watch.Added},
				{baseNamespacedPodUpdated("t17-foo1", "t17-ns1"), false, ""},
				{baseNamespacedPodAssigned("t17-foo2", "t17-ns1", "t17-bar1"), true, watch.Modified},
			},
		},
		{
			name:               "namespaced watch, request with name, request with namespace, with field selector metadata.name, metadata.namespace and spec.nodename",
			requestedName:      "t18-foo1",
			requestedNamespace: "t18-ns1",
			fieldSelector: fields.SelectorFromSet(fields.Set{
				"metadata.name":      "t18-foo1",
				"metadata.namespace": "t18-ns1",
				"spec.nodeName":      "t18-bar1",
			}),
			indexFields: []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t18-foo1", "t18-ns1"), false, ""},
				{baseNamespacedPod("t18-foo2", "t18-ns1"), false, ""},
				{baseNamespacedPod("t18-foo1", "t18-ns2"), false, ""},
				{baseNamespacedPodUpdated("t18-foo1", "t18-ns1"), false, ""},
				{baseNamespacedPodAssigned("t18-foo1", "t18-ns1", "t18-bar1"), true, watch.Added},
			},
		},
		{
			name:               "namespaced watch, request with name, request with namespace, with field selector metadata.name, metadata.namespace, and with spec.nodename to filter out watch",
			requestedName:      "t19-foo2",
			requestedNamespace: "t19-ns1",
			fieldSelector: fields.AndSelectors(
				fields.ParseSelectorOrDie("spec.nodeName!=t19-bar1"),
				fields.SelectorFromSet(fields.Set{"metadata.name": "t19-foo2", "metadata.namespace": "t19-ns1"}),
			),
			indexFields: []string{"spec.nodeName"},
			watchTests: []*testWatchStruct{
				{baseNamespacedPod("t19-foo1", "t19-ns1"), false, ""},
				{baseNamespacedPod("t19-foo2", "t19-ns2"), false, ""},
				{baseNamespacedPod("t19-foo2", "t19-ns1"), true, watch.Added},
				{baseNamespacedPodUpdated("t19-foo2", "t19-ns1"), true, watch.Modified},
				{baseNamespacedPodAssigned("t19-foo2", "t19-ns1", "t19-bar1"), true, watch.Deleted},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestInfo := &genericapirequest.RequestInfo{}
			requestInfo.Name = tt.requestedName
			requestInfo.Namespace = tt.requestedNamespace
			ctx = genericapirequest.WithRequestInfo(ctx, requestInfo)
			ctx = genericapirequest.WithNamespace(ctx, tt.requestedNamespace)

			watchKey := "/pods"
			if tt.requestedNamespace != "" {
				watchKey += "/" + tt.requestedNamespace
				if tt.requestedName != "" {
					watchKey += "/" + tt.requestedName
				}
			}

			predicate := createPodPredicate(tt.fieldSelector, true, tt.indexFields)

			list := &example.PodList{}
			opts := storage.ListOptions{
				ResourceVersion: "",
				Predicate:       predicate,
				Recursive:       true,
			}
			if err := store.GetList(ctx, "/pods", opts, list); err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			opts.ResourceVersion = list.ResourceVersion
			opts.Recursive = tt.recursive

			w, err := store.Watch(ctx, watchKey, opts)
			if err != nil {
				t.Fatalf("Watch failed: %v", err)
			}

			currentObjs := map[string]*example.Pod{}
			for _, watchTest := range tt.watchTests {
				out := &example.Pod{}
				key := "pods/" + watchTest.obj.Namespace + "/" + watchTest.obj.Name
				err := store.GuaranteedUpdate(ctx, key, out, true, nil, storage.SimpleUpdate(
					func(runtime.Object) (runtime.Object, error) {
						obj := watchTest.obj.DeepCopy()
						return obj, nil
					}), nil)
				if err != nil {
					t.Fatalf("GuaranteedUpdate failed: %v", err)
				}

				expectObj := out
				podIdentifier := watchTest.obj.Namespace + "/" + watchTest.obj.Name
				if watchTest.watchType == watch.Deleted {
					expectObj = currentObjs[podIdentifier]
					expectObj.ResourceVersion = out.ResourceVersion
					delete(currentObjs, podIdentifier)
				} else {
					currentObjs[podIdentifier] = out
				}
				if watchTest.expectEvent {
					testCheckResult(t, watchTest.watchType, w, expectObj)
				}
			}
			w.Stop()
			testCheckStop(t, w)
		})
	}
}

type testWatchStruct struct {
	obj         *example.Pod
	expectEvent bool
	watchType   watch.EventType
}

func createPodPredicate(field fields.Selector, namespaceScoped bool, indexField []string) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:       labels.Everything(),
		Field:       field,
		GetAttrs:    determinePodGetAttrFunc(namespaceScoped, indexField),
		IndexFields: indexField,
	}
}

func determinePodGetAttrFunc(namespaceScoped bool, indexField []string) storage.AttrFunc {
	if indexField != nil {
		if namespaceScoped {
			return namespacedScopedNodeNameAttrFunc
		}
		return clusterScopedNodeNameAttrFunc
	}
	if namespaceScoped {
		return storage.DefaultNamespaceScopedAttr
	}
	return storage.DefaultClusterScopedAttr
}

func namespacedScopedNodeNameAttrFunc(obj runtime.Object) (labels.Set, fields.Set, error) {
	pod := obj.(*example.Pod)
	return nil, fields.Set{
		"spec.nodeName":      pod.Spec.NodeName,
		"metadata.name":      pod.ObjectMeta.Name,
		"metadata.namespace": pod.ObjectMeta.Namespace,
	}, nil
}

func clusterScopedNodeNameAttrFunc(obj runtime.Object) (labels.Set, fields.Set, error) {
	pod := obj.(*example.Pod)
	return nil, fields.Set{
		"spec.nodeName": pod.Spec.NodeName,
		"metadata.name": pod.ObjectMeta.Name,
	}, nil
}

func basePod(podName string) *example.Pod {
	return baseNamespacedPod(podName, "")
}

func basePodUpdated(podName string) *example.Pod {
	return baseNamespacedPodUpdated(podName, "")
}

func basePodAssigned(podName, nodeName string) *example.Pod {
	return baseNamespacedPodAssigned(podName, "", nodeName)
}

func baseNamespacedPod(podName, namespace string) *example.Pod {
	return &example.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
	}
}

func baseNamespacedPodUpdated(podName, namespace string) *example.Pod {
	return &example.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
		Status:     example.PodStatus{Phase: "Running"},
	}
}

func baseNamespacedPodAssigned(podName, namespace, nodeName string) *example.Pod {
	return &example.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
		Spec:       example.PodSpec{NodeName: nodeName},
	}
}
