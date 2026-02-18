package internalversion

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/printers"
	kprinters "k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"

	_ "github.com/openshift/openshift-apiserver/pkg/api/install"
)

func int64p(i int64) *int64 {
	return &i
}

func TestImageTag(t *testing.T) {
	storage := printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(AddImageOpenShiftHandlers)}
	table, err := storage.ConvertToTable(context.Background(), &imageapi.ImageTagList{
		Items: []imageapi.ImageTag{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:0", Namespace: "test"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:1", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name: "1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:2", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name: "2",
					From: &kapi.ObjectReference{},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:3", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name:      "3",
					Reference: true,
					From:      &kapi.ObjectReference{Kind: "DockerImage", Name: "a/b:c"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:4", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name: "4",
					From: &kapi.ObjectReference{Kind: "ImageStreamTag", Name: "b:c"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:4a", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name: "4a",
					From: &kapi.ObjectReference{Kind: "ImageStreamImage", Name: "sha256:0000000000", Namespace: "a"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:5", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name:         "5",
					ImportPolicy: imageapi.TagImportPolicy{Scheduled: true},
					From:         &kapi.ObjectReference{Kind: "ImageStreamTag", Name: "b:c", Namespace: "a"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:6", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name: "6",
					From: &kapi.ObjectReference{Kind: "ImageStreamImage", Name: "a@sha256:0000000000", Namespace: "a"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:7", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name: "7",
					From: &kapi.ObjectReference{Kind: "ImageStreamImage", Name: "a@sha256:0000000000", Namespace: "a"},
				},
				Status: &imageapi.NamedTagEventList{
					Conditions: []imageapi.TagEventCondition{
						{
							Type:               imageapi.ImportSuccess,
							Status:             kapi.ConditionFalse,
							Reason:             "AbjectFailure",
							LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Hour * 2)),
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:8", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name: "8",
					From: &kapi.ObjectReference{Kind: "ImageStreamImage", Name: "a@sha256:0000000000", Namespace: "a"},
				},
				Status: &imageapi.NamedTagEventList{
					Conditions: []imageapi.TagEventCondition{
						{
							Type:   imageapi.ImportSuccess,
							Reason: "WithoutCondition",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:9", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name:       "9",
					From:       &kapi.ObjectReference{Kind: "ImageStreamImage", Name: "a@sha256:0000000000", Namespace: "a"},
					Generation: int64p(2),
				},
				Status: &imageapi.NamedTagEventList{
					Items: []imageapi.TagEvent{
						{Image: "sha256:0000000000", Generation: 1},
					},
					Conditions: []imageapi.TagEventCondition{
						{
							Type:   imageapi.ImportSuccess,
							Reason: "AbjectFailure",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:10", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name:       "10",
					From:       &kapi.ObjectReference{Kind: "ImageStreamImage", Name: "a@sha256:0000000000", Namespace: "a"},
					Generation: int64p(2),
				},
				Status: &imageapi.NamedTagEventList{
					Items: []imageapi.TagEvent{
						{Image: "sha256:0000000000", Generation: 2, Created: metav1.NewTime(time.Now().Add(-time.Hour * 24 * 2))},
						{DockerImageReference: "old/image:nowhere", Generation: 1, Created: metav1.NewTime(time.Now().Add(-time.Hour * 24 * 3))},
					},
					Conditions: []imageapi.TagEventCondition{
						{
							Type:   imageapi.ImportSuccess,
							Reason: "AbjectFailure",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:11", Namespace: "test"},
				Status: &imageapi.NamedTagEventList{
					Items: []imageapi.TagEvent{
						{Image: "sha256:0000000000", Generation: 2, Created: metav1.NewTime(time.Now().Add(-time.Hour * 24 * 2))},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:12", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name:       "12",
					From:       &kapi.ObjectReference{Kind: "ImageStreamImage", Name: "a@sha256:0000000001", Namespace: "a"},
					Generation: int64p(3),
				},
				Status: &imageapi.NamedTagEventList{
					Items: []imageapi.TagEvent{
						{Image: "sha256:0000000002", Generation: 4, Created: metav1.NewTime(time.Now().Add(-time.Hour * 24 * 2))},
						{Image: "sha256:0000000001", Generation: 3, Created: metav1.NewTime(time.Now().Add(-time.Hour * 24 * 3))},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:13", Namespace: "test"},
				Status: &imageapi.NamedTagEventList{
					Items: []imageapi.TagEvent{
						{DockerImageReference: "a/b:c", Generation: 2, Created: metav1.NewTime(time.Now().Add(-time.Hour * 24 * 2))},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo:14", Namespace: "test"},
				Spec: &imageapi.TagReference{
					Name:       "14",
					From:       &kapi.ObjectReference{Kind: "ImageStreamTag", Name: "other"},
					Generation: int64p(1),
				},
				Status: &imageapi.NamedTagEventList{
					Items: []imageapi.TagEvent{
						{Image: "sha256:0000000002", Generation: 2, Created: metav1.NewTime(time.Now().Add(-time.Hour * 24 * 2))},
					},
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for i := range table.Rows {
		table.Rows[i].Object.Object = nil
	}

	expected := []metav1.TableRow{
		{Cells: []interface{}{"foo:0", "", "", nil, ""}},
		{Cells: []interface{}{"foo:1", "", "", nil, ""}},
		{Cells: []interface{}{"foo:2", "Tag", "InvalidRefKind", nil, ""}},
		{Cells: []interface{}{"foo:3", "Ref", "a/b:c", nil, ""}},
		{Cells: []interface{}{"foo:4", "Tag", "istag/b:c", nil, ""}},
		{Cells: []interface{}{"foo:4a", "Tag", "InvalidRefName", nil, ""}},
		{Cells: []interface{}{"foo:5", "Scheduled", "istag a/b:c", nil, ""}},
		{Cells: []interface{}{"foo:6", "Tag", "image/sha256:0000000000", nil, ""}},
		{Cells: []interface{}{"foo:7", "Tag", "ImportFailed (AbjectFailure)", nil, "2 hours ago"}},
		{Cells: []interface{}{"foo:8", "Tag", "image/sha256:0000000000", nil, ""}},
		{Cells: []interface{}{"foo:9", "Tag", "Importing", int(1), ""}},
		{Cells: []interface{}{"foo:10", "Tag", "image/sha256:0000000000", int(2), "2 days ago"}},
		{Cells: []interface{}{"foo:11", "Push", "image/sha256:0000000000", int(1), "2 days ago"}},
		{Cells: []interface{}{"foo:12", "Push", "image/sha256:0000000002", int(2), "2 days ago"}},
		{Cells: []interface{}{"foo:13", "Push", "a/b:c", int(1), "2 days ago"}},
		{Cells: []interface{}{"foo:14", "Track", "image/sha256:0000000002", int(1), "2 days ago"}},
	}
	if !reflect.DeepEqual(expected, table.Rows) {
		t.Fatalf("%s", diff.Diff(expected, table.Rows))
	}
}

func TestPrintersWithDeepCopy(t *testing.T) {
	tests := []struct {
		name    string
		printer func() ([]metav1.TableRow, error)
	}{
		{
			name: "Image",
			printer: func() ([]metav1.TableRow, error) {
				return printImage(&imageapi.Image{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "ImageStream",
			printer: func() ([]metav1.TableRow, error) {
				return printImageStream(&imageapi.ImageStream{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "ImageStreamTag",
			printer: func() ([]metav1.TableRow, error) {
				return printImageStreamTag(&imageapi.ImageStreamTag{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "ImageTag",
			printer: func() ([]metav1.TableRow, error) {
				return printImageTag(&imageapi.ImageTag{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "ImageStreamImage",
			printer: func() ([]metav1.TableRow, error) {
				return printImageStreamImage(&imageapi.ImageStreamImage{}, kprinters.GenerateOptions{})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := test.printer()
			if err != nil {
				t.Fatalf("expected no error, but got: %#v", err)
			}
			if len(rows) <= 0 {
				t.Fatalf("expected to have at least one TableRow, but got: %d", len(rows))
			}

			func() {
				defer func() {
					if err := recover(); err != nil {
						// Same as stdlib http server code. Manually allocate stack
						// trace buffer size to prevent excessively large logs
						const size = 64 << 10
						buf := make([]byte, size)
						buf = buf[:runtime.Stack(buf, false)]
						err = fmt.Errorf("%q stack:\n%s", err, buf)

						t.Errorf("Expected no panic, but got: %v", err)
					}
				}()

				// should not panic
				rows[0].DeepCopy()
			}()

		})
	}
}
