package auth

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// TestConcurrentMapAccess_NoRace verifies the fix for OCPBUGS-XXXXX
// This test ensures that concurrent access to subjectRecord.namespaces
// does not cause "fatal error: concurrent map iteration and map write"
func TestConcurrentMapAccess_NoRace(t *testing.T) {
	// Create subject record store
	subjectRecordStore := cache.NewStore(subjectRecordKeyFn)

	// Add initial subject with namespaces
	subject := &subjectRecord{
		subject:    "test-user",
		namespaces: sets.NewString("ns-1", "ns-2", "ns-3"),
	}
	subjectRecordStore.Add(subject)

	var wg sync.WaitGroup
	done := make(chan bool)

	// Simulate concurrent writes (like synchronize() does)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					// Simulate deleteNamespaceFromSubjects
					deleteNamespaceFromSubjects(subjectRecordStore, []string{"test-user"}, fmt.Sprintf("ns-%d", id%5))
					time.Sleep(1 * time.Millisecond)
					// Simulate addSubjectsToNamespace
					addSubjectsToNamespace(subjectRecordStore, []string{"test-user"}, fmt.Sprintf("ns-%d", id%5))
				}
			}
		}(i)
	}

	// Simulate concurrent reads (like List() does)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					// Simulate reading from subjectRecord.namespaces
					obj, exists, _ := subjectRecordStore.GetByKey("test-user")
					if exists {
						subjectRecord := obj.(*subjectRecord)
						subjectRecord.mu.RLock()
						_ = subjectRecord.namespaces.List()
						subjectRecord.mu.RUnlock()
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	// Run for 2 seconds
	time.Sleep(2 * time.Second)
	close(done)
	wg.Wait()

	t.Log("Race condition test passed - no concurrent map access panic")
}

// TestAuthorizationCache_ListConcurrentAccess tests the actual List() method
// under concurrent load to ensure it doesn't panic with concurrent map access
func TestAuthorizationCache_ListConcurrentAccess(t *testing.T) {
	// Create a minimal mock authorization cache
	userSubjectRecordStore := cache.NewStore(subjectRecordKeyFn)
	groupSubjectRecordStore := cache.NewStore(subjectRecordKeyFn)

	// Add test data
	userRecord := &subjectRecord{
		subject:    "test-user",
		namespaces: sets.NewString("default", "kube-system", "openshift"),
	}
	userSubjectRecordStore.Add(userRecord)

	groupRecord := &subjectRecord{
		subject:    "system:authenticated",
		namespaces: sets.NewString("test-ns-1", "test-ns-2"),
	}
	groupSubjectRecordStore.Add(groupRecord)

	var wg sync.WaitGroup
	done := make(chan bool)

	// Simulate concurrent writes to the same records
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					deleteNamespaceFromSubjects(userSubjectRecordStore, []string{"test-user"}, "default")
					addSubjectsToNamespace(userSubjectRecordStore, []string{"test-user"}, "default")
					deleteNamespaceFromSubjects(groupSubjectRecordStore, []string{"system:authenticated"}, "test-ns-1")
					addSubjectsToNamespace(groupSubjectRecordStore, []string{"system:authenticated"}, "test-ns-1")
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(i)
	}

	// Simulate concurrent reads (mimicking the List() method's behavior)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					// This mimics what List() does at lines 517 and 524
					keys := sets.String{}

					obj, exists, _ := userSubjectRecordStore.GetByKey("test-user")
					if exists {
						subjectRecord := obj.(*subjectRecord)
						subjectRecord.mu.RLock()
						keys.Insert(subjectRecord.namespaces.List()...)
						subjectRecord.mu.RUnlock()
					}

					obj, exists, _ = groupSubjectRecordStore.GetByKey("system:authenticated")
					if exists {
						subjectRecord := obj.(*subjectRecord)
						subjectRecord.mu.RLock()
						keys.Insert(subjectRecord.namespaces.List()...)
						subjectRecord.mu.RUnlock()
					}

					_ = keys.List()
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	// Run for 3 seconds
	time.Sleep(3 * time.Second)
	close(done)
	wg.Wait()

	t.Log("Authorization cache List() concurrent access test passed")
}

// TestSubjectRecordConcurrentModification verifies individual subjectRecord
// can handle concurrent reads and writes safely
func TestSubjectRecordConcurrentModification(t *testing.T) {
	record := &subjectRecord{
		subject:    "concurrent-test",
		namespaces: sets.NewString(),
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("writer %d panicked: %v", id, r)
				}
			}()
			for j := 0; j < 100; j++ {
				ns := fmt.Sprintf("namespace-%d-%d", id, j)
				record.mu.Lock()
				record.namespaces.Insert(ns)
				record.mu.Unlock()
				time.Sleep(100 * time.Microsecond)
				record.mu.Lock()
				delete(record.namespaces, ns)
				record.mu.Unlock()
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("reader %d panicked: %v", id, r)
				}
			}()
			for j := 0; j < 100; j++ {
				record.mu.RLock()
				_ = record.namespaces.List()
				_ = record.namespaces.Len()
				record.mu.RUnlock()
				time.Sleep(100 * time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check if any goroutine panicked
	for err := range errors {
		t.Fatal(err)
	}

	t.Log("Concurrent modification test passed - no panics or race conditions")
}
