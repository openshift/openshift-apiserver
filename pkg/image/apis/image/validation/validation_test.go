package validation

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kapi "k8s.io/kubernetes/pkg/apis/core"

	imagev1 "github.com/openshift/api/image/v1"
	openshiftcontrolplanev1 "github.com/openshift/api/openshiftcontrolplane/v1"
	imageref "github.com/openshift/library-go/pkg/image/reference"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation/whitelist"
)

func TestValidateImageOK(t *testing.T) {
	errs := ValidateImage(&imageapi.Image{
		ObjectMeta:           metav1.ObjectMeta{Name: "foo"},
		DockerImageReference: "openshift/ruby-19-centos",
	})
	if len(errs) > 0 {
		t.Errorf("Unexpected non-empty error list: %#v", errs)
	}
}

func TestValidateImageMissingFields(t *testing.T) {
	errorCases := map[string]struct {
		I imageapi.Image
		T field.ErrorType
		F string
	}{
		"missing Name": {
			imageapi.Image{DockerImageReference: "ref"},
			field.ErrorTypeRequired,
			"metadata.name",
		},
		"no slash in Name": {
			imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "foo/bar"}},
			field.ErrorTypeInvalid,
			"metadata.name",
		},
		"no percent in Name": {
			imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "foo%%bar"}},
			field.ErrorTypeInvalid,
			"metadata.name",
		},
		"missing DockerImageReference": {
			imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
			field.ErrorTypeRequired,
			"dockerImageReference",
		},
	}

	for k, v := range errorCases {
		errs := ValidateImage(&v.I)
		if len(errs) == 0 {
			t.Errorf("Expected failure for %s", k)
			continue
		}
		match := false
		for i := range errs {
			if errs[i].Type == v.T && errs[i].Field == v.F {
				match = true
				break
			}
		}
		if !match {
			t.Errorf("%s: expected errors to have field %s and type %s: %v", k, v.F, v.T, errs)
		}
	}
}

func TestValidateImageSignature(t *testing.T) {
	for _, tc := range []struct {
		name      string
		signature imageapi.ImageSignature
		expected  field.ErrorList
	}{
		{
			name: "valid",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{
					Name: "imgname@valid",
				},
				Type:    "valid",
				Content: []byte("blob"),
			},
			expected: nil,
		},

		{
			name: "valid trusted",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{
					Name: "imgname@trusted",
				},
				Type:    "valid",
				Content: []byte("blob"),
				Conditions: []imageapi.SignatureCondition{
					{
						Type:   imageapi.SignatureTrusted,
						Status: kapi.ConditionTrue,
					},
					{
						Type:   imageapi.SignatureForImage,
						Status: kapi.ConditionTrue,
					},
				},
				ImageIdentity: "registry.company.ltd/app/core:v1.2",
			},
			expected: nil,
		},

		{
			name: "valid untrusted",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{
					Name: "imgname@untrusted",
				},
				Type:    "valid",
				Content: []byte("blob"),
				Conditions: []imageapi.SignatureCondition{
					{
						Type:   imageapi.SignatureTrusted,
						Status: kapi.ConditionTrue,
					},
					{
						Type:   imageapi.SignatureForImage,
						Status: kapi.ConditionFalse,
					},
					// compare the latest condition
					{
						Type:   imageapi.SignatureTrusted,
						Status: kapi.ConditionFalse,
					},
				},
				ImageIdentity: "registry.company.ltd/app/core:v1.2",
			},
			expected: nil,
		},

		{
			name: "invalid name and missing type",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{Name: "notype"},
				Content:    []byte("blob"),
			},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata").Child("name"), "notype", "name must be of format <imageName>@<signatureName>"),
				field.Required(field.NewPath("type"), ""),
			},
		},

		{
			name: "missing content",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{Name: "img@nocontent"},
				Type:       "invalid",
			},
			expected: field.ErrorList{
				field.Required(field.NewPath("content"), ""),
			},
		},

		{
			name: "missing ForImage condition",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{Name: "img@noforimage"},
				Type:       "invalid",
				Content:    []byte("blob"),
				Conditions: []imageapi.SignatureCondition{
					{
						Type:   imageapi.SignatureTrusted,
						Status: kapi.ConditionTrue,
					},
				},
				ImageIdentity: "registry.company.ltd/app/core:v1.2",
			},
			expected: field.ErrorList{field.Invalid(field.NewPath("conditions"),
				[]imageapi.SignatureCondition{
					{
						Type:   imageapi.SignatureTrusted,
						Status: kapi.ConditionTrue,
					},
				},
				fmt.Sprintf("missing %q condition type", imageapi.SignatureForImage))},
		},

		{
			name: "adding labels and anotations",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "img@annotated",
					Annotations: map[string]string{"key": "value"},
					Labels:      map[string]string{"label": "value"},
				},
				Type:    "valid",
				Content: []byte("blob"),
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("metadata").Child("labels"), "signature labels cannot be set"),
				field.Forbidden(field.NewPath("metadata").Child("annotations"), "signature annotations cannot be set"),
			},
		},

		{
			name: "filled metadata for unknown signature state",
			signature: imageapi.ImageSignature{
				ObjectMeta: metav1.ObjectMeta{Name: "img@metadatafilled"},
				Type:       "invalid",
				Content:    []byte("blob"),
				Conditions: []imageapi.SignatureCondition{
					{
						Type:   imageapi.SignatureTrusted,
						Status: kapi.ConditionUnknown,
					},
					{
						Type:   imageapi.SignatureForImage,
						Status: kapi.ConditionUnknown,
					},
				},
				ImageIdentity: "registry.company.ltd/app/core:v1.2",
				SignedClaims:  map[string]string{"claim": "value"},
				IssuedBy: &imageapi.SignatureIssuer{
					SignatureGenericEntity: imageapi.SignatureGenericEntity{Organization: "org"},
				},
				IssuedTo: &imageapi.SignatureSubject{PublicKeyID: "id"},
			},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("imageIdentity"), "registry.company.ltd/app/core:v1.2", "must be unset for unknown signature state"),
				field.Invalid(field.NewPath("signedClaims"), map[string]string{"claim": "value"}, "must be unset for unknown signature state"),
				field.Invalid(field.NewPath("issuedBy"), &imageapi.SignatureIssuer{
					SignatureGenericEntity: imageapi.SignatureGenericEntity{Organization: "org"},
				}, "must be unset for unknown signature state"),
				field.Invalid(field.NewPath("issuedTo"), &imageapi.SignatureSubject{PublicKeyID: "id"}, "must be unset for unknown signature state"),
			},
		},
	} {
		errs := validateImageSignature(&tc.signature, nil)
		if e, a := tc.expected, errs; !reflect.DeepEqual(a, e) {
			t.Errorf("[%s] unexpected errors: %s", tc.name, diff.ObjectDiff(e, a))
		}
	}

}

func TestValidateImageManifests(t *testing.T) {
	testCases := []struct {
		name           string
		imageManifests []imageapi.ImageManifest
		expected       field.ErrorList
	}{
		{
			name: "invalid digest",
			imageManifests: []imageapi.ImageManifest{
				{
					Digest:       "abc",
					MediaType:    "foo/bar",
					Architecture: "foo",
					OS:           "bar",
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("dockerImageManifests").Index(0).Child("digest"),
					"abc",
					"digest does not conform with OCI image specification",
				),
			},
		},
		{
			name: "invalid media type",
			imageManifests: []imageapi.ImageManifest{
				{
					Digest:       "sha256:82bc737c1fede1c1534446cd5fdd0737c4e0f3650454c9e1905e2d70af95778e",
					MediaType:    "foobar",
					Architecture: "foo",
					OS:           "bar",
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("dockerImageManifests").Index(0).Child("mediaType"),
					"foobar",
					"media type does not conform to RFC6838",
				),
			},
		},
		{
			name: "required architecture",
			imageManifests: []imageapi.ImageManifest{
				{
					Digest:    "sha256:82bc737c1fede1c1534446cd5fdd0737c4e0f3650454c9e1905e2d70af95778e",
					MediaType: "foo/bar",
					OS:        "bar",
				},
			},
			expected: field.ErrorList{
				field.Required(
					field.NewPath("dockerImageManifests").Index(0).Child("architecture"),
					"",
				),
			},
		},
		{
			name: "required OS",
			imageManifests: []imageapi.ImageManifest{
				{
					Digest:       "sha256:82bc737c1fede1c1534446cd5fdd0737c4e0f3650454c9e1905e2d70af95778e",
					MediaType:    "foo/bar",
					Architecture: "bar",
				},
			},
			expected: field.ErrorList{
				field.Required(
					field.NewPath("dockerImageManifests").Index(0).Child("os"),
					"",
				),
			},
		},
		{
			name: "negative size",
			imageManifests: []imageapi.ImageManifest{
				{
					Digest:       "sha256:82bc737c1fede1c1534446cd5fdd0737c4e0f3650454c9e1905e2d70af95778e",
					MediaType:    "foo/bar",
					Architecture: "bar",
					OS:           "linux",
					ManifestSize: -2,
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("dockerImageManifests").Index(0).Child("size"),
					int64(-2),
					"manifest size cannot be negative",
				),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			errs := validateImageManifests(testCase.imageManifests, field.NewPath("dockerImageManifests"))
			if !reflect.DeepEqual(errs, testCase.expected) {
				t.Errorf(
					"unexpected errors: %s",
					diff.ObjectDiff(testCase.expected, errs),
				)
			}
		})
	}
}

func TestValidateImageStreamMappingNotOK(t *testing.T) {
	errorCases := map[string]struct {
		I imageapi.ImageStreamMapping
		T field.ErrorType
		F string
	}{
		"missing DockerImageRepository": {
			imageapi.ImageStreamMapping{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Tag: imagev1.DefaultImageTag,
				Image: imageapi.Image{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
					},
					DockerImageReference: "openshift/ruby-19-centos",
				},
			},
			field.ErrorTypeRequired,
			"dockerImageRepository",
		},
		"missing Name": {
			imageapi.ImageStreamMapping{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Tag: imagev1.DefaultImageTag,
				Image: imageapi.Image{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
					},
					DockerImageReference: "openshift/ruby-19-centos",
				},
			},
			field.ErrorTypeRequired,
			"name",
		},
		"missing Tag": {
			imageapi.ImageStreamMapping{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				DockerImageRepository: "openshift/ruby-19-centos",
				Image: imageapi.Image{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
					},
					DockerImageReference: "openshift/ruby-19-centos",
				},
			},
			field.ErrorTypeRequired,
			"tag",
		},
		"missing image name": {
			imageapi.ImageStreamMapping{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				DockerImageRepository: "openshift/ruby-19-centos",
				Tag:                   imagev1.DefaultImageTag,
				Image: imageapi.Image{
					DockerImageReference: "openshift/ruby-19-centos",
				},
			},
			field.ErrorTypeRequired,
			"image.metadata.name",
		},
		"invalid repository pull spec": {
			imageapi.ImageStreamMapping{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				DockerImageRepository: "registry/extra/openshift//ruby-19-centos",
				Tag:                   imagev1.DefaultImageTag,
				Image: imageapi.Image{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
					},
					DockerImageReference: "openshift/ruby-19-centos",
				},
			},
			field.ErrorTypeInvalid,
			"dockerImageRepository",
		},
	}

	for k, v := range errorCases {
		errs := ValidateImageStreamMapping(&v.I)
		if len(errs) == 0 {
			t.Errorf("Expected failure for %s", k)
			continue
		}
		match := false
		for i := range errs {
			if errs[i].Type == v.T && errs[i].Field == v.F {
				match = true
				break
			}
		}
		if !match {
			t.Errorf("%s: expected errors to have field %s and type %s: %v", k, v.F, v.T, errs)
		}
	}
}

func TestValidateImageStream(t *testing.T) {

	namespace63Char := strings.Repeat("a", 63)
	name191Char := strings.Repeat("b", 191)
	name192Char := "x" + name191Char

	missingNameErr := field.Required(field.NewPath("metadata", "name"), "")
	missingNameErr.Detail = "name or generateName is required"

	for name, test := range map[string]struct {
		namespace             string
		name                  string
		dockerImageRepository string
		specTags              map[string]imageapi.TagReference
		statusTags            map[string]imageapi.TagEventList
		expected              field.ErrorList
	}{
		"missing name": {
			namespace: "foo",
			name:      "",
			expected:  field.ErrorList{missingNameErr},
		},
		"no slash in Name": {
			namespace: "foo",
			name:      "foo/bar",
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), "foo/bar", `may not contain '/'`),
			},
		},
		"no percent in Name": {
			namespace: "foo",
			name:      "foo%%bar",
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), "foo%%bar", `may not contain '%'`),
			},
		},
		"other invalid name": {
			namespace: "foo",
			name:      "foo bar",
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), "foo bar", `must match "[a-z0-9]+(?:[._-][a-z0-9]+)*"`),
			},
		},
		"missing namespace": {
			namespace: "",
			name:      "foo",
			expected: field.ErrorList{
				field.Required(field.NewPath("metadata", "namespace"), ""),
			},
		},
		"invalid namespace": {
			namespace: "!$",
			name:      "foo",
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "namespace"), "!$", `a lowercase RFC 1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')`),
			},
		},
		"invalid dockerImageRepository": {
			namespace:             "namespace",
			name:                  "foo",
			dockerImageRepository: "a-|///bbb",
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "dockerImageRepository"), "a-|///bbb", "invalid reference format"),
			},
		},
		"invalid dockerImageRepository with tag": {
			namespace:             "namespace",
			name:                  "foo",
			dockerImageRepository: "a/b:tag",
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "dockerImageRepository"), "a/b:tag", "the repository name may not contain a tag"),
			},
		},
		"invalid dockerImageRepository with ID": {
			namespace:             "namespace",
			name:                  "foo",
			dockerImageRepository: "a/b@sha256:something",
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "dockerImageRepository"), "a/b@sha256:something", "invalid reference format"),
			},
		},
		"status tag missing dockerImageReference": {
			namespace: "namespace",
			name:      "foo",
			statusTags: map[string]imageapi.TagEventList{
				"tag": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: ""},
						{DockerImageReference: "foo/bar:latest"},
						{DockerImageReference: ""},
					},
				},
			},
			expected: field.ErrorList{
				field.Required(field.NewPath("status", "tags").Key("tag").Child("items").Index(0).Child("dockerImageReference"), ""),
				field.Required(field.NewPath("status", "tags").Key("tag").Child("items").Index(2).Child("dockerImageReference"), ""),
			},
		},
		"referencePolicy.type must be valid": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"tag": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.TagReferencePolicyType("Other")},
				},
			},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "tags").Key("tag").Child("referencePolicy", "type"), imageapi.TagReferencePolicyType("Other"), "valid values are \"Source\", \"Local\""),
			},
		},
		"ImageStreamTags can't be scheduled": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"tag": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc",
					},
					ImportPolicy:    imageapi.TagImportPolicy{Scheduled: true},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				"other": {
					From: &kapi.ObjectReference{
						Kind: "ImageStreamTag",
						Name: "other:latest",
					},
					ImportPolicy:    imageapi.TagImportPolicy{Scheduled: true},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "tags").Key("other").Child("importPolicy", "scheduled"), true, "only tags pointing to Docker repositories may be scheduled for background import"),
			},
		},
		"image IDs can't be scheduled": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"badid": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc@badid",
					},
					ImportPolicy:    imageapi.TagImportPolicy{Scheduled: true},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "tags").Key("badid").Child("from", "name"), "abc@badid", "invalid reference format"),
			},
		},
		"ImageStreamImages can't be scheduled": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"otherimage": {
					From: &kapi.ObjectReference{
						Kind: "ImageStreamImage",
						Name: "other@latest",
					},
					ImportPolicy:    imageapi.TagImportPolicy{Scheduled: true},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "tags").Key("otherimage").Child("importPolicy", "scheduled"), true, "only tags pointing to Docker repositories may be scheduled for background import"),
			},
		},
		"valid": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"tag": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				"other": {
					From: &kapi.ObjectReference{
						Kind: "ImageStreamTag",
						Name: "other:latest",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			statusTags: map[string]imageapi.TagEventList{
				"tag": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "foo/bar:latest"},
					},
				},
			},
			expected: nil,
		},
		"shortest name components": {
			namespace: "f",
			name:      "g",
			expected:  nil,
		},
		"all possible characters used": {
			namespace: "abcdefghijklmnopqrstuvwxyz-1234567890",
			name:      "abcdefghijklmnopqrstuvwxyz-1234567890.dot_underscore-dash",
			expected:  nil,
		},
		"max name and namespace length met": {
			namespace: namespace63Char,
			name:      name191Char,
			expected:  nil,
		},
		"max name and namespace length exceeded": {
			namespace: namespace63Char,
			name:      name192Char,
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), name192Char, "'namespace/name' cannot be longer than 255 characters"),
			},
		},
		"valid importMode": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"tag": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
					ImportPolicy:    imageapi.TagImportPolicy{ImportMode: imageapi.ImportModePreserveOriginal},
				},
			},
			expected: nil,
		},
		"invalid importMode": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"tag": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
					ImportPolicy:    imageapi.TagImportPolicy{ImportMode: "Invalid"},
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("spec", "tags").Key("tag").Child("importPolicy", "importMode"),
					imageapi.ImportModeType("Invalid"),
					fmt.Sprintf(
						"invalid import mode, valid modes are '', '%s', '%s'",
						imageapi.ImportModeLegacy,
						imageapi.ImportModePreserveOriginal,
					),
				),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			stream := imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.namespace,
					Name:      test.name,
				},
				Spec: imageapi.ImageStreamSpec{
					DockerImageRepository: test.dockerImageRepository,
					Tags:                  test.specTags,
				},
				Status: imageapi.ImageStreamStatus{
					Tags: test.statusTags,
				},
			}

			errs := ValidateImageStream(&stream)
			if e, a := test.expected, errs; !reflect.DeepEqual(e, a) {
				t.Errorf("unexpected errors: %s", diff.ObjectDiff(e, a))
			}
		})
	}
}

func TestValidateImageStreamWithWhitelister(t *testing.T) {
	for name, test := range map[string]struct {
		namespace             string
		name                  string
		dockerImageRepository string
		specTags              map[string]imageapi.TagReference
		statusTags            map[string]imageapi.TagEventList
		whitelist             openshiftcontrolplanev1.AllowedRegistries
		expected              field.ErrorList
	}{
		"forbid spec references not on the whitelist": {
			namespace: "foo",
			name:      "bar",
			whitelist: mkAllowed(false, "example.com", "localhost:5000", "dev.null.io:80"),
			specTags: map[string]imageapi.TagReference{
				"fail": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "registry.ltd/a/b",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "tags").Key("fail").Child("from", "name"),
					`registry "registry.ltd" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
			},
		},

		"forbid status references not on the whitelist - secure": {
			namespace: "foo",
			name:      "bar",
			whitelist: mkAllowed(false, "example.com", "localhost:5000", "dev.null.io:80"),
			specTags: map[string]imageapi.TagReference{
				"secure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com:443/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: false,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				"insecure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: true,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			statusTags: map[string]imageapi.TagEventList{
				"secure": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "docker.io/foo/bar:latest"},
						{DockerImageReference: "example.com/bar:latest"},
						{DockerImageReference: "example.com:80/repo:latest"},
						{DockerImageReference: "dev.null.io/myapp"},
						{DockerImageReference: "dev.null.io:80/myapp"},
					},
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("status", "tags").Key("secure").Child("items").Index(0).Child("dockerImageReference"),
					`registry "docker.io" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
				field.Forbidden(field.NewPath("status", "tags").Key("secure").Child("items").Index(2).Child("dockerImageReference"),
					`registry "example.com:80" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
				field.Forbidden(field.NewPath("status", "tags").Key("secure").Child("items").Index(3).Child("dockerImageReference"),
					`registry "dev.null.io" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
			},
		},

		"forbid status references not on the whitelist - insecure": {
			namespace: "foo",
			name:      "bar",
			whitelist: mkAllowed(false, "example.com", "localhost:5000", "dev.null.io:80"),
			specTags: map[string]imageapi.TagReference{
				"secure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com:443/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: false,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				"insecure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: true,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			statusTags: map[string]imageapi.TagEventList{
				"insecure": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "localhost:5000/baz:latest"},
						{DockerImageReference: "example.com:80/bar:latest"},
						{DockerImageReference: "registry.ltd/repo:latest"},
						{DockerImageReference: "dev.null.io/myapp"},
						{DockerImageReference: "dev.null.io:80/myapp"},
					},
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("status", "tags").Key("insecure").Child("items").Index(1).Child("dockerImageReference"),
					`registry "example.com:80" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
				field.Forbidden(field.NewPath("status", "tags").Key("insecure").Child("items").Index(2).Child("dockerImageReference"),
					`registry "registry.ltd" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
			},
		},

		"forbid status references not on the whitelist": {
			namespace: "foo",
			name:      "bar",
			whitelist: mkAllowed(false, "example.com", "localhost:5000", "dev.null.io:80"),
			specTags: map[string]imageapi.TagReference{
				"secure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com:443/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: false,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				"insecure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: true,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			statusTags: map[string]imageapi.TagEventList{
				"securebydefault": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "localhost/repo:latest"},
						{DockerImageReference: "example.com:443/bar:latest"},
						{DockerImageReference: "example.com/repo:latest"},
					},
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("status", "tags").Key("securebydefault").Child("items").Index(0).Child("dockerImageReference"),
					`registry "localhost" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
			},
		},

		"local reference policy does not matter": {
			namespace: "foo",
			name:      "bar",
			whitelist: mkAllowed(false, "example.com", "localhost:5000", "dev.null.io:80"),
			specTags: map[string]imageapi.TagReference{
				"secure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com:443/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: false,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				},
				"insecure": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ImportPolicy: imageapi.TagImportPolicy{
						Insecure: true,
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				},
			},
			statusTags: map[string]imageapi.TagEventList{
				"securebydefault": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "localhost/repo:latest"},
						{DockerImageReference: "example.com:443/bar:latest"},
						{DockerImageReference: "example.com/repo:latest"},
					},
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("status", "tags").Key("securebydefault").Child("items").Index(0).Child("dockerImageReference"),
					`registry "localhost" not allowed by whitelist: "example.com:443", "localhost:5000", "dev.null.io:80"`),
			},
		},

		"whitelisted repository": {
			namespace:             "foo",
			name:                  "bar",
			whitelist:             mkAllowed(false, "example.com"),
			dockerImageRepository: "example.com/openshift/origin",
			expected:              nil,
		},

		"not whitelisted repository": {
			namespace:             "foo",
			name:                  "bar",
			whitelist:             mkAllowed(false, "*.example.com"),
			dockerImageRepository: "example.com/openshift/origin",
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "dockerImageRepository"),
					`registry "example.com" not allowed by whitelist: "*.example.com:443"`),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			stream := imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.namespace,
					Name:      test.name,
				},
				Spec: imageapi.ImageStreamSpec{
					DockerImageRepository: test.dockerImageRepository,
					Tags:                  test.specTags,
				},
				Status: imageapi.ImageStreamStatus{
					Tags: test.statusTags,
				},
			}

			errs := ValidateImageStreamWithWhitelister(context.TODO(), mkWhitelister(t, test.whitelist), &stream)
			if e, a := test.expected, errs; !reflect.DeepEqual(e, a) {
				t.Errorf("unexpected errors: %s", diff.ObjectDiff(e, a))
			}
		})
	}
}

func TestValidateImageStreamUpdateWithWhitelister(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		whitelist                openshiftcontrolplanev1.AllowedRegistries
		oldDockerImageRepository string
		newDockerImageRepository string
		oldSpecTags              map[string]imageapi.TagReference
		oldStatusTags            map[string]imageapi.TagEventList
		newSpecTags              map[string]imageapi.TagReference
		newStatusTags            map[string]imageapi.TagEventList
		expected                 field.ErrorList
	}{
		{
			name:      "no old referencess",
			whitelist: mkAllowed(false, "docker.io", "example.com"),
			newSpecTags: map[string]imageapi.TagReference{
				"latest": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			newStatusTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "example.com"},
					},
				},
			},
		},

		{
			name:      "report not whitelisted",
			whitelist: mkAllowed(false, "docker.io"),
			newSpecTags: map[string]imageapi.TagReference{
				"fail": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				"ok": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/busybox",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			newStatusTags: map[string]imageapi.TagEventList{
				"fail": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "example.com/repo@sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25"},
					},
				},
				"ok": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "docker.io/library/busybox:latest"},
					},
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "tags").Key("fail").Child("from", "name"),
					`registry "example.com" not allowed by whitelist: "docker.io:443"`),
				field.Forbidden(field.NewPath("status", "tags").Key("fail").Child("items").Index(0).Child("dockerImageReference"),
					`registry "example.com" not allowed by whitelist: "docker.io:443"`),
			},
		},

		{
			name:      "allow old not whitelisted references",
			whitelist: mkAllowed(false, "docker.io"),
			oldSpecTags: map[string]imageapi.TagReference{
				"fail": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			newSpecTags: map[string]imageapi.TagReference{
				"fail": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "example.com/repo",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			newStatusTags: map[string]imageapi.TagEventList{
				"fail": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "example.com/repo"},
					},
				},
			},
		},

		{
			name:      "allow old not whitelisted references from status",
			whitelist: mkAllowed(false, "docker.io"),
			oldStatusTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "abcd.com/repo/myapp:latest"},
					},
				},
			},
			newSpecTags: map[string]imageapi.TagReference{
				"whitelisted": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abcd.com/repo/myapp:latest",
					},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			newStatusTags: map[string]imageapi.TagEventList{
				"fail": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "abcd.com/repo/myapp@sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25"},
					},
				},
			},
		},

		{
			name:                     "allow whitelisted dockerImageRepository",
			whitelist:                mkAllowed(false, "docker.io"),
			oldDockerImageRepository: "example.com/my/app",
			newDockerImageRepository: "docker.io/my/newapp",
		},

		{
			name:                     "forbid not whitelisted dockerImageRepository",
			whitelist:                mkAllowed(false, "docker.io"),
			oldDockerImageRepository: "docker.io/my/app",
			newDockerImageRepository: "example.com/my/newapp",
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "dockerImageRepository"),
					`registry "example.com" not allowed by whitelist: "docker.io:443"`)},
		},

		{
			name:                     "permit no change to not whitelisted dockerImageRepository",
			whitelist:                mkAllowed(false, "docker.io"),
			oldDockerImageRepository: "example.com/my/newapp",
			newDockerImageRepository: "example.com/my/newapp",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			whitelister := mkWhitelister(t, tc.whitelist)
			objMeta := metav1.ObjectMeta{
				Namespace:       "nm",
				Name:            "testis",
				ResourceVersion: "1",
			}
			oldStream := imageapi.ImageStream{
				ObjectMeta: objMeta,
				Spec: imageapi.ImageStreamSpec{
					DockerImageRepository: tc.oldDockerImageRepository,
					Tags:                  tc.oldSpecTags,
				},
				Status: imageapi.ImageStreamStatus{
					Tags: tc.oldStatusTags,
				},
			}
			newStream := imageapi.ImageStream{
				ObjectMeta: objMeta,
				Spec: imageapi.ImageStreamSpec{
					DockerImageRepository: tc.newDockerImageRepository,
					Tags:                  tc.newSpecTags,
				},
				Status: imageapi.ImageStreamStatus{
					Tags: tc.newStatusTags,
				},
			}
			errs := ValidateImageStreamUpdateWithWhitelister(context.TODO(), whitelister, &newStream, &oldStream)
			if e, a := tc.expected, errs; !reflect.DeepEqual(e, a) {
				t.Errorf("unexpected errors: %s", diff.ObjectDiff(a, e))
			}
		})
	}
}

func TestValidateISTUpdate(t *testing.T) {
	old := &imageapi.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
		Tag: &imageapi.TagReference{
			From: &kapi.ObjectReference{Kind: "DockerImage", Name: "some/other:system"},
		},
	}

	errs := ValidateImageStreamTagUpdate(
		&imageapi.ImageStreamTag{
			ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two", "three": "four"}},
		},
		old,
	)
	if len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	errorCases := map[string]struct {
		A imageapi.ImageStreamTag
		T field.ErrorType
		F string
	}{
		"changedLabel": {
			A: imageapi.ImageStreamTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}, Labels: map[string]string{"a": "b"}},
			},
			T: field.ErrorTypeInvalid,
			F: "metadata",
		},
		"mismatchedAnnotations": {
			A: imageapi.ImageStreamTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
				Tag: &imageapi.TagReference{
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "some/other:system"},
					Annotations:     map[string]string{"one": "three"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			T: field.ErrorTypeInvalid,
			F: "tag.annotations",
		},
		"tagToNameRequired": {
			A: imageapi.ImageStreamTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
				Tag: &imageapi.TagReference{
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: ""},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			T: field.ErrorTypeRequired,
			F: "tag.from.name",
		},
		"tagToKindRequired": {
			A: imageapi.ImageStreamTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
				Tag: &imageapi.TagReference{
					From:            &kapi.ObjectReference{Kind: "", Name: "foo/bar:biz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			T: field.ErrorTypeRequired,
			F: "tag.from.kind",
		},
	}
	for k, v := range errorCases {
		errs := ValidateImageStreamTagUpdate(&v.A, old)
		if len(errs) == 0 {
			t.Errorf("expected failure %s for %v", k, v.A)
			continue
		}
		for i := range errs {
			if errs[i].Type != v.T {
				t.Errorf("%s: expected errors to have type %s: %v", k, v.T, errs[i])
			}
			if errs[i].Field != v.F {
				t.Errorf("%s: expected errors to have field %s: %v", k, v.F, errs[i])
			}
		}
	}
}

func TestValidateISTUpdateWithWhitelister(t *testing.T) {
	for _, tc := range []struct {
		name        string
		whitelist   openshiftcontrolplanev1.AllowedRegistries
		oldTagRef   *imageapi.TagReference
		newTagRef   *imageapi.TagReference
		registryURL string
		expected    field.ErrorList
	}{
		{
			name:      "allow whitelisted",
			whitelist: mkAllowed(false, "docker.io"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
		},

		{
			name:      "forbid not whitelisted",
			whitelist: mkAllowed(false, "example.com:*"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("tag", "from", "name"),
					`registry "docker.io:443" not allowed by whitelist: "example.com:*"`),
			},
		},

		{
			name:      "allow old not whitelisted",
			whitelist: mkAllowed(false, "example.com:*"),
			oldTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
		},

		{
			name:      "exact match not old references",
			whitelist: mkAllowed(false, "example.com:*"),
			oldTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
		},

		{
			name:      "do not match insecure registries if not flagged as insecure",
			whitelist: mkAllowed(true, "example.com"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "example.com/foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("tag", "from", "name"),
					`registry "example.com" not allowed by whitelist: "example.com:80"`),
			},
		},

		{
			name:      "match insecure registry if flagged",
			whitelist: mkAllowed(false, "example.com"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "example.com/foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				ImportPolicy: imageapi.TagImportPolicy{
					Insecure: true,
				},
			},
		},

		{
			name:      "match integrated registry URL",
			whitelist: mkAllowed(false, "example.com"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "172.30.30.30:5000/foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			registryURL: "172.30.30.30:5000",
		},

		{
			name:      "ignore old reference of unexpected kind",
			whitelist: mkAllowed(false, "example.com"),
			oldTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "ImageStreamTag", Name: "bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				ImportPolicy: imageapi.TagImportPolicy{
					Insecure: true,
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("tag", "from", "name"),
					`registry "docker.io" not allowed by whitelist: "example.com:443"`),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objMeta := metav1.ObjectMeta{
				Namespace:       metav1.NamespaceDefault,
				Name:            "foo:bar",
				ResourceVersion: "1",
			}
			istOld := imageapi.ImageStreamTag{
				ObjectMeta: objMeta,
				Tag:        tc.oldTagRef,
			}
			istNew := imageapi.ImageStreamTag{
				ObjectMeta: objMeta,
				Tag:        tc.newTagRef,
			}

			whitelister, err := whitelist.NewRegistryWhitelister(tc.whitelist, &simpleHostnameRetriever{registryURL: tc.registryURL})
			if err != nil {
				t.Fatal(err)
			}
			errs := ValidateImageStreamTagUpdateWithWhitelister(context.TODO(), whitelister, &istNew, &istOld)
			if e, a := tc.expected, errs; !reflect.DeepEqual(e, a) {
				t.Errorf("unexpected errors: %s", diff.ObjectDiff(a, e))
			}
		})
	}
}

func TestValidateITUpdate(t *testing.T) {
	old := &imageapi.ImageTag{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
		Spec: &imageapi.TagReference{
			From: &kapi.ObjectReference{Kind: "DockerImage", Name: "some/other:system"},
		},
	}

	errs := ValidateImageTagUpdate(
		&imageapi.ImageTag{
			ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
		},
		old,
	)
	if len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	errorCases := map[string]struct {
		A imageapi.ImageTag
		T field.ErrorType
		F string
	}{
		"changedAnnotations": {
			A: imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two", "three": "four"}},
			},
			T: field.ErrorTypeInvalid,
			F: "metadata",
		},
		"changedLabel": {
			A: imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}, Labels: map[string]string{"a": "b"}},
			},
			T: field.ErrorTypeInvalid,
			F: "metadata",
		},
		"mismatchedSpecName": {
			A: imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
				Spec: &imageapi.TagReference{
					Name:            "baz",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "some/valid:image"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			T: field.ErrorTypeInvalid,
			F: "spec.name",
		},
		"tagToNameRequired": {
			A: imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
				Spec: &imageapi.TagReference{
					Name:            "bar",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: ""},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			T: field.ErrorTypeRequired,
			F: "spec.from.name",
		},
		"tagToKindRequired": {
			A: imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "foo:bar", ResourceVersion: "1", Annotations: map[string]string{"one": "two"}},
				Spec: &imageapi.TagReference{
					Name:            "bar",
					From:            &kapi.ObjectReference{Kind: "", Name: "foo/bar:biz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			T: field.ErrorTypeRequired,
			F: "spec.from.kind",
		},
	}
	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			errs := ValidateImageTagUpdate(&v.A, old)
			if len(errs) == 0 {
				t.Fatalf("expected failure %s for %v", k, v.A)
			}
			for i := range errs {
				if errs[i].Type != v.T {
					t.Errorf("%s: expected errors to have type %s: %v", k, v.T, errs[i])
				}
				if errs[i].Field != v.F {
					t.Errorf("%s: expected errors to have field %s: %v", k, v.F, errs[i])
				}
			}
		})
	}
}

func TestValidateITUpdateWithWhitelister(t *testing.T) {
	for _, tc := range []struct {
		name        string
		whitelist   openshiftcontrolplanev1.AllowedRegistries
		oldTagRef   *imageapi.TagReference
		newTagRef   *imageapi.TagReference
		registryURL string
		expected    field.ErrorList
	}{
		{
			name:      "allow whitelisted",
			whitelist: mkAllowed(false, "docker.io"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
		},

		{
			name:      "forbid not whitelisted",
			whitelist: mkAllowed(false, "example.com:*"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "from", "name"),
					`registry "docker.io:443" not allowed by whitelist: "example.com:*"`),
			},
		},

		{
			name:      "allow old not whitelisted",
			whitelist: mkAllowed(false, "example.com:*"),
			oldTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
		},

		{
			name:      "exact match not old references",
			whitelist: mkAllowed(false, "example.com:*"),
			oldTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
		},

		{
			name:      "do not match insecure registries if not flagged as insecure",
			whitelist: mkAllowed(true, "example.com"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "example.com/foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "from", "name"),
					`registry "example.com" not allowed by whitelist: "example.com:80"`),
			},
		},

		{
			name:      "match insecure registry if flagged",
			whitelist: mkAllowed(false, "example.com"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "example.com/foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				ImportPolicy: imageapi.TagImportPolicy{
					Insecure: true,
				},
			},
		},

		{
			name:      "match integrated registry URL",
			whitelist: mkAllowed(false, "example.com"),
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "172.30.30.30:5000/foo/bar:baz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			registryURL: "172.30.30.30:5000",
		},

		{
			name:      "ignore old reference of unexpected kind",
			whitelist: mkAllowed(false, "example.com"),
			oldTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "ImageTag", Name: "bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
			},
			newTagRef: &imageapi.TagReference{
				From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "bar:biz"},
				ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				ImportPolicy: imageapi.TagImportPolicy{
					Insecure: true,
				},
			},
			expected: field.ErrorList{
				field.Forbidden(field.NewPath("spec", "from", "name"),
					`registry "docker.io" not allowed by whitelist: "example.com:443"`),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objMeta := metav1.ObjectMeta{
				Namespace:       metav1.NamespaceDefault,
				Name:            "foo:bar",
				ResourceVersion: "1",
			}
			if tc.oldTagRef != nil {
				tc.oldTagRef.Name = "bar"
			}
			tc.newTagRef.Name = "bar"
			istOld := imageapi.ImageTag{
				ObjectMeta: objMeta,
				Spec:       tc.oldTagRef,
			}
			istNew := imageapi.ImageTag{
				ObjectMeta: objMeta,
				Spec:       tc.newTagRef,
			}

			whitelister, err := whitelist.NewRegistryWhitelister(tc.whitelist, &simpleHostnameRetriever{registryURL: tc.registryURL})
			if err != nil {
				t.Fatal(err)
			}
			errs := ValidateImageTagUpdateWithWhitelister(context.TODO(), whitelister, &istNew, &istOld)
			if e, a := tc.expected, errs; !reflect.DeepEqual(e, a) {
				t.Errorf("unexpected errors: %s", diff.ObjectDiff(a, e))
			}
		})
	}
}

type simpleHostnameRetriever struct {
	registryURL string
}

func (r *simpleHostnameRetriever) InternalRegistryHostname(_ context.Context) (string, bool) {
	return r.registryURL, len(r.registryURL) > 0
}

func (r *simpleHostnameRetriever) ExternalRegistryHostname() (string, bool) {
	return "", false
}

func TestValidateRegistryAllowedForImport(t *testing.T) {
	const fieldName = "fieldName"

	for _, tc := range []struct {
		name      string
		hostname  string
		whitelist openshiftcontrolplanev1.AllowedRegistries
		expected  field.ErrorList
	}{
		{
			name:      "allow whitelisted",
			hostname:  "example.com:443",
			whitelist: mkAllowed(false, "example.com"),
			expected:  nil,
		},

		{
			name:      "fail when not on whitelist",
			hostname:  "example.com:443",
			whitelist: mkAllowed(false, "foo.bar"),
			expected: field.ErrorList{field.Forbidden(nil,
				`importing images from registry "example.com:443" is forbidden: registry "example.com:443" not allowed by whitelist: "foo.bar:443"`)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			whitelister := mkWhitelister(t, tc.whitelist)
			host, port, err := net.SplitHostPort(tc.hostname)
			if err != nil {
				t.Fatal(err)
			}
			errs := ValidateRegistryAllowedForImport(context.TODO(), whitelister, nil, fieldName, host, port)
			if e, a := tc.expected, errs; !reflect.DeepEqual(e, a) {
				t.Errorf("unexpected errors: %s", diff.ObjectDiff(e, a))
			}
		})
	}
}

func TestValidateImageStreamImport(t *testing.T) {
	namespace63Char := strings.Repeat("a", 63)
	name191Char := strings.Repeat("b", 191)
	name192Char := "x" + name191Char

	missingNameErr := field.Required(field.NewPath("metadata", "name"), "")
	missingNameErr.Detail = "name or generateName is required"

	validMeta := metav1.ObjectMeta{Namespace: "foo", Name: "foo"}
	validSpec := imageapi.ImageStreamImportSpec{Repository: &imageapi.RepositoryImportSpec{From: kapi.ObjectReference{Kind: "DockerImage", Name: "redis"}}}
	repoFn := func(spec string) imageapi.ImageStreamImportSpec {
		return imageapi.ImageStreamImportSpec{Repository: &imageapi.RepositoryImportSpec{From: kapi.ObjectReference{Kind: "DockerImage", Name: spec}}}
	}

	tests := map[string]struct {
		isi      *imageapi.ImageStreamImport
		expected field.ErrorList

		namespace             string
		name                  string
		dockerImageRepository string
		specTags              map[string]imageapi.TagReference
		statusTags            map[string]imageapi.TagEventList
	}{
		"missing name": {
			isi:      &imageapi.ImageStreamImport{ObjectMeta: metav1.ObjectMeta{Namespace: "foo"}, Spec: validSpec},
			expected: field.ErrorList{missingNameErr},
		},
		"no slash in Name": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: metav1.ObjectMeta{Namespace: "foo", Name: "foo/bar"}, Spec: validSpec},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), "foo/bar", `may not contain '/'`),
			},
		},
		"no percent in Name": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: metav1.ObjectMeta{Namespace: "foo", Name: "foo%%bar"}, Spec: validSpec},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), "foo%%bar", `may not contain '%'`),
			},
		},
		"other invalid name": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: metav1.ObjectMeta{Namespace: "foo", Name: "foo bar"}, Spec: validSpec},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), "foo bar", `must match "[a-z0-9]+(?:[._-][a-z0-9]+)*"`),
			},
		},
		"missing namespace": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: validSpec},
			expected: field.ErrorList{
				field.Required(field.NewPath("metadata", "namespace"), ""),
			},
		},
		"invalid namespace": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: metav1.ObjectMeta{Namespace: "!$", Name: "foo"}, Spec: validSpec},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "namespace"), "!$", `a lowercase RFC 1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')`),
			},
		},
		"invalid dockerImageRepository": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: validMeta, Spec: repoFn("a-|///bbb")},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "repository", "from", "name"), "a-|///bbb", "invalid reference format"),
			},
		},
		"invalid dockerImageRepository with tag": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: validMeta, Spec: repoFn("a/b:tag")},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "repository", "from", "name"), "a/b:tag", "you must specify an image repository, not a tag or ID"),
			},
		},
		"invalid dockerImageRepository with ID": {
			isi: &imageapi.ImageStreamImport{ObjectMeta: validMeta, Spec: repoFn("a/b@sha256:something")},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "repository", "from", "name"), "a/b@sha256:something", "invalid reference format"),
			},
		},
		"only DockerImage tags can be scheduled": {
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: validMeta, Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "abc",
							},
							ImportPolicy: imageapi.TagImportPolicy{Scheduled: true},
						},
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "abc@badid",
							},
							ImportPolicy: imageapi.TagImportPolicy{Scheduled: true},
						},
						{
							From: kapi.ObjectReference{
								Kind: "ImageStreamTag",
								Name: "other:latest",
							},
							ImportPolicy: imageapi.TagImportPolicy{Scheduled: true},
						},
						{
							From: kapi.ObjectReference{
								Kind: "ImageStreamImage",
								Name: "other@latest",
							},
							ImportPolicy: imageapi.TagImportPolicy{Scheduled: true},
						},
					},
				},
			},
			expected: field.ErrorList{
				field.Invalid(field.NewPath("spec", "images").Index(1).Child("from", "name"), "abc@badid", "invalid reference format"),
				field.Invalid(field.NewPath("spec", "images").Index(2).Child("from", "kind"), "ImageStreamTag", "only DockerImage is supported"),
				field.Invalid(field.NewPath("spec", "images").Index(3).Child("from", "kind"), "ImageStreamImage", "only DockerImage is supported"),
			},
		},
		"valid": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"tag": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc",
					},
				},
				"other": {
					From: &kapi.ObjectReference{
						Kind: "ImageStreamTag",
						Name: "other:latest",
					},
				},
			},
			statusTags: map[string]imageapi.TagEventList{
				"tag": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "foo/bar:latest"},
					},
				},
			},
			expected: field.ErrorList{},
		},
		"valid scheduled dockerimage sha/digest ref": {
			namespace: "namespace",
			name:      "foo",
			specTags: map[string]imageapi.TagReference{
				"tag": {
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "abc@sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
					},
					ImportPolicy: imageapi.TagImportPolicy{Scheduled: true},
				},
			},
			statusTags: map[string]imageapi.TagEventList{},
			expected:   field.ErrorList{},
		},
		"shortest name components": {
			namespace: "f",
			name:      "g",
			expected:  field.ErrorList{},
		},
		"all possible characters used": {
			namespace: "abcdefghijklmnopqrstuvwxyz-1234567890",
			name:      "abcdefghijklmnopqrstuvwxyz-1234567890.dot_underscore-dash",
			expected:  field.ErrorList{},
		},
		"max name and namespace length met": {
			namespace: namespace63Char,
			name:      name191Char,
			expected:  field.ErrorList{},
		},
		"max name and namespace length exceeded": {
			namespace: namespace63Char,
			name:      name192Char,
			expected: field.ErrorList{
				field.Invalid(field.NewPath("metadata", "name"), name192Char, "'namespace/name' cannot be longer than 255 characters"),
			},
		},
		"invalid image importMode": {
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: validMeta,
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "abc",
							},
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: "Invalid",
							},
						},
					},
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("spec", "images").Index(0).Child("importPolicy", "importMode"),
					imageapi.ImportModeType("Invalid"),
					fmt.Sprintf(
						"invalid import mode, valid modes are '', '%s', '%s'",
						imageapi.ImportModeLegacy,
						imageapi.ImportModePreserveOriginal,
					),
				),
			},
		},
		"invalid repository importMode": {
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: validMeta,
				Spec: imageapi.ImageStreamImportSpec{
					Repository: &imageapi.RepositoryImportSpec{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "abc",
						},
						ImportPolicy: imageapi.TagImportPolicy{
							ImportMode: "Invalid",
						},
					},
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("spec", "repository", "importPolicy", "importMode"),
					imageapi.ImportModeType("Invalid"),
					fmt.Sprintf(
						"invalid import mode, valid modes are '', '%s', '%s'",
						imageapi.ImportModeLegacy,
						imageapi.ImportModePreserveOriginal,
					),
				),
			},
		},
	}

	for name, test := range tests {
		if test.isi == nil {
			continue
		}
		errs := ValidateImageStreamImport(test.isi)
		if e, a := test.expected, errs; !reflect.DeepEqual(e, a) {
			t.Errorf("%s: unexpected errors: %s", name, diff.ObjectDiff(e, a))
		}
	}
}

func mkAllowed(insecure bool, regs ...string) openshiftcontrolplanev1.AllowedRegistries {
	ret := make(openshiftcontrolplanev1.AllowedRegistries, 0, len(regs))
	for _, reg := range regs {
		ret = append(ret, openshiftcontrolplanev1.RegistryLocation{DomainName: reg, Insecure: insecure})
	}
	return ret
}

func mkWhitelister(t *testing.T, wl openshiftcontrolplanev1.AllowedRegistries) whitelist.RegistryWhitelister {
	var whitelister whitelist.RegistryWhitelister
	if wl == nil {
		whitelister = whitelist.WhitelistAllRegistries(context.TODO())
	} else {
		rw, err := whitelist.NewRegistryWhitelister(wl, nil)
		if err != nil {
			t.Fatal(err)
		}
		whitelister = rw
	}
	return whitelister
}

func TestValidateImageStreamLayers(t *testing.T) {
	configDigest := "sha256:bc01a3326866eedd68525a4d2d91d2cf86f9893db054601d6be524d5c9d03981"
	testCases := []struct {
		name     string
		isl      *imageapi.ImageStreamLayers
		expected field.ErrorList
	}{
		{
			name: "empty",
			isl: &imageapi.ImageStreamLayers{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
			},
			expected: field.ErrorList{},
		},
		{
			name: "valid",
			isl: &imageapi.ImageStreamLayers{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
				Blobs:      map[string]imageapi.ImageLayerData{},
				Images: map[string]imageapi.ImageBlobReferences{
					"sha256:dacd1aa51e0b27c0e36c4981a7a8d9d8ec2c4a74bf125c0a44d0709497a522e9": {
						ImageMissing: false,
						Layers: []string{
							"sha256:22b70bddd3acadc892fca4c2af4260629bfda5dfd11ebc106a93ce24e752b5ed",
						},
						Config: &configDigest,
					},
				},
			},
			expected: field.ErrorList{},
		},
		{
			name: "invalid layer",
			isl: &imageapi.ImageStreamLayers{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
				Blobs:      map[string]imageapi.ImageLayerData{},
				Images: map[string]imageapi.ImageBlobReferences{
					"sha256:dacd1aa51e0b27c0e36c4981a7a8d9d8ec2c4a74bf125c0a44d0709497a522e9": {
						ImageMissing: false,
						Layers: []string{
							"sha256:22b70bddd3acadc892fca4c2af4260629bfda5dfd11ebc106a93ce24e752b5ed",
							"",
						},
					},
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("images").Key("sha256:dacd1aa51e0b27c0e36c4981a7a8d9d8ec2c4a74bf125c0a44d0709497a522e9").Child("layers").Index(1),
					"",
					"layer cannot be empty",
				),
			},
		},
		{
			name: "invalid manifest",
			isl: &imageapi.ImageStreamLayers{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
				Blobs:      map[string]imageapi.ImageLayerData{},
				Images: map[string]imageapi.ImageBlobReferences{
					"sha256:6bdd92bf5240be1b5f3bf71324f5e371fe59f0e153b27fa1f1620f78ba16963c": {
						ImageMissing: false,
						Manifests: []string{
							"",
						},
					},
				},
			},
			expected: field.ErrorList{
				field.Invalid(
					field.NewPath("images").Key("sha256:6bdd92bf5240be1b5f3bf71324f5e371fe59f0e153b27fa1f1620f78ba16963c").Child("manifests").Index(0),
					"",
					"manifest cannot be empty",
				),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateImageStreamLayers(tc.isl)
			if !reflect.DeepEqual(errs, tc.expected) {
				t.Errorf("unexpected errors: %s", diff.ObjectDiff(tc.expected, errs))
			}
		})
	}
}

func Test_shouldContactRegistry(t *testing.T) {
	ctx := context.Background()

	mockIPLookup := func(ctx context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "localmachine":
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		case "metadata":
			return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
		case "oneninetwo":
			return []net.IPAddr{{IP: net.ParseIP("192.168.0.4")}}, nil
		case "multipleips":
			return []net.IPAddr{
				{IP: net.ParseIP("2.3.4.5")},
				{IP: net.ParseIP("192.168.0.4")},
				{IP: net.ParseIP("1.1.1.1")},
			}, nil
		case "mixedallowedandblocked":
			// This hostname resolves to both an allowed IP and a loopback IP
			// Should be BLOCKED because one of the IPs is loopback
			return []net.IPAddr{
				{IP: net.ParseIP("10.0.0.1")},  // Could be in allowed list
				{IP: net.ParseIP("127.0.0.1")}, // Loopback - must block!
			}, nil
		default:
			return net.DefaultResolver.LookupIPAddr(ctx, host)
		}
	}

	testCases := []struct {
		name     string
		registry string
		blocked  []netip.Prefix
		allowed  []netip.Prefix
		errorstr string
	}{
		{
			name:     "empty registry - allowed",
			registry: "",
		},
		{
			name:     "loopback IPv4 - blocked",
			registry: "127.0.0.1:5000",
			errorstr: "loopback",
		},
		{
			name:     "loopback IPv4 without port - blocked",
			registry: "127.0.0.1",
			errorstr: "loopback",
		},
		{
			name:     "loopback IPv4 alternate - blocked",
			registry: "127.0.0.2:443",
			errorstr: "loopback",
		},
		{
			name:     "loopback IPv6 - blocked",
			registry: "[::1]:5000",
			errorstr: "loopback",
		},
		{
			name:     "IPv4-mapped IPv6 loopback - blocked",
			registry: "[::ffff:127.0.0.1]:5000",
			errorstr: "loopback",
		},
		{
			name:     "link-local metadata endpoint - blocked",
			registry: "169.254.169.254",
			errorstr: "link-local",
		},
		{
			name:     "link-local metadata endpoint with port - blocked",
			registry: "169.254.169.254:80",
			errorstr: "link-local",
		},
		{
			name:     "link-local IPv4 - blocked",
			registry: "169.254.1.1:5000",
			errorstr: "link-local",
		},
		{
			name:     "link-local IPv6 - blocked",
			registry: "[fe80::1]:5000",
			errorstr: "link-local",
		},
		{
			name:     "public IPv4 - allowed",
			registry: "1.2.3.4:5000",
		},
		{
			name:     "public IPv6 - allowed",
			registry: "[2001:db8::1]:5000",
		},
		{
			name:     "public IPv6 without port - allowed",
			registry: "[2001:db8::1]",
		},
		{
			name:     "IP in custom CIDR block - blocked",
			registry: "10.96.0.1:443",
			blocked:  []netip.Prefix{netip.MustParsePrefix("10.96.0.0/12")},
			errorstr: "not allowed",
		},
		{
			name:     "IP in custom CIDR block without port - blocked",
			registry: "10.96.5.10",
			blocked:  []netip.Prefix{netip.MustParsePrefix("10.96.0.0/12")},
			errorstr: "not allowed",
		},
		{
			name:     "IP NOT in custom CIDR block - allowed",
			registry: "10.95.0.1:5000",
			blocked:  []netip.Prefix{netip.MustParsePrefix("10.96.0.0/12")},
		},
		{
			name:     "IP in multiple custom CIDRs - blocked",
			registry: "192.168.1.100:5000",
			blocked: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.0/8"),
				netip.MustParsePrefix("192.168.0.0/16"),
			},
			errorstr: "not allowed",
		},
		{
			name:     "private IP not in custom CIDR - allowed",
			registry: "172.16.0.1:5000",
			blocked: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.0/8"),
				netip.MustParsePrefix("192.168.0.0/16"),
			},
		},
		{
			name:     "resolving to local machine",
			registry: "localmachine",
			errorstr: "loopback",
		},
		{
			name:     "metadata ip resolved via DNS",
			registry: "metadata",
			errorstr: "link-local",
		},
		{
			name:     "local network address - allowed",
			registry: "oneninetwo",
		},
		{
			name:     "local network address - blocked",
			registry: "oneninetwo",
			blocked:  []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24")},
			errorstr: "not allowed",
		},
		{
			name:     "multiple resolved ips - allowed",
			registry: "multipleips",
		},
		{
			name:     "multiple resolved ips - one blocked",
			registry: "multipleips",
			blocked:  []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24")},
			errorstr: "not allowed",
		},
		{
			name:     "allowed IP bypasses loopback",
			registry: "localmachine",
			allowed:  []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		},
		{
			name:     "allowed IP bypasses custom CIDR",
			registry: "oneninetwo",
			blocked:  []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24")},
			allowed:  []netip.Prefix{netip.MustParsePrefix("192.168.0.4/32")},
		},
		{
			name:     "allowed IP does not match - still blocked",
			registry: "localmachine",
			allowed:  []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")},
			errorstr: "loopback",
		},
		{
			name:     "hostname resolves to allowed IP and loopback - blocked",
			registry: "mixedallowedandblocked",
			allowed:  []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")},
			errorstr: "loopback",
		},
		{
			name:     "malformed registry - IPv6 without brackets",
			registry: "::1:5000",
			errorstr: "invalid registry url",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ref := imageref.DockerImageReference{Registry: tc.registry}

			err := shouldContactRegistry(ctx, ref, tc.blocked, tc.allowed, mockIPLookup)
			if err == nil {
				if tc.errorstr != "" {
					t.Errorf("expected error but got none")
					return
				}
				return
			}

			if tc.errorstr == "" {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !strings.Contains(err.Error(), tc.errorstr) {
				t.Errorf("expected error to contain %q but got: %v", tc.errorstr, err)
			}
		})
	}
}

func Test_validateImageStreamImportDisallowedHosts(t *testing.T) {
	mockIPLookup := func(ctx context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "evil.local":
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		case "metadata.local":
			return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
		case "safe.local":
			return []net.IPAddr{{IP: net.ParseIP("1.2.3.4")}}, nil
		case "mixed.local":
			// Hostname resolves to both an allowed IP and a link-local IP
			return []net.IPAddr{
				{IP: net.ParseIP("10.0.0.1")},        // Could be allowed
				{IP: net.ParseIP("169.254.169.254")}, // Metadata endpoint - must block!
			}, nil
		default:
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
	}

	testCases := []struct {
		name          string
		isi           *imageapi.ImageStreamImport
		blocked       []netip.Prefix
		allowed       []netip.Prefix
		errorContains []string
	}{
		{
			name: "repository from loopback hostname - blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Repository: &imageapi.RepositoryImportSpec{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "evil.local/image:latest",
						},
					},
				},
			},
			errorContains: []string{"loopback import not allowed"},
		},
		{
			name: "image from metadata endpoint - blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "metadata.local/image:v1",
							},
						},
					},
				},
			},
			errorContains: []string{"link-local import not allowed"},
		},
		{
			name: "safe registry - allowed",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "safe.local/image:v1",
							},
						},
					},
				},
			},
			errorContains: nil,
		},
		{
			name: "multiple images - one blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "safe.local/image:v1",
							},
						},
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "evil.local/image:v2",
							},
						},
					},
				},
			},
			errorContains: []string{"loopback import not allowed"},
		},
		{
			name: "registry in custom CIDR - blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "safe.local/image:v1",
							},
						},
					},
				},
			},
			blocked:       []netip.Prefix{netip.MustParsePrefix("1.2.3.0/24")},
			errorContains: []string{"import from 1.2.3.0/24 not allowed"},
		},
		{
			name: "non-docker image kind - ignored",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "ImageStreamTag",
								Name: "evil.local/image:v1",
							},
						},
					},
				},
			},
			errorContains: nil,
		},
		{
			name: "both repository and image blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Repository: &imageapi.RepositoryImportSpec{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "evil.local/repo:latest",
						},
					},
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "metadata.local/image:v1",
							},
						},
					},
				},
			},
			errorContains: []string{"loopback import not allowed", "link-local import not allowed"},
		},
		{
			name: "IP address without port - blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "127.0.0.1/image:latest",
							},
						},
					},
				},
			},
			errorContains: []string{"loopback import not allowed"},
		},
		{
			name: "IP address with port - blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "169.254.169.254:5000/image:v1",
							},
						},
					},
				},
			},
			errorContains: []string{"link-local import not allowed"},
		},
		{
			name: "allowed IP bypasses loopback check",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "evil.local/image:latest",
							},
						},
					},
				},
			},
			allowed:       []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			errorContains: nil,
		},
		{
			name: "allowed IP in custom CIDR bypasses block",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "safe.local/image:v1",
							},
						},
					},
				},
			},
			blocked:       []netip.Prefix{netip.MustParsePrefix("1.2.3.0/24")},
			allowed:       []netip.Prefix{netip.MustParsePrefix("1.2.3.4/32")},
			errorContains: nil,
		},
		{
			name: "multiple allowed IPs",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "evil.local/image:v1",
							},
						},
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "metadata.local/image:v2",
							},
						},
					},
				},
			},
			allowed: []netip.Prefix{
				netip.MustParsePrefix("127.0.0.1/32"),
				netip.MustParsePrefix("169.254.169.254/32"),
			},
			errorContains: nil,
		},
		{
			name: "allowed IP does not match - still blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "evil.local/image:latest",
							},
						},
					},
				},
			},
			allowed:       []netip.Prefix{netip.MustParsePrefix("1.2.3.4/32")},
			errorContains: []string{"loopback import not allowed"},
		},
		{
			name: "hostname resolves to allowed IP and metadata endpoint - blocked",
			isi: &imageapi.ImageStreamImport{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "mixed.local/image:latest",
							},
						},
					},
				},
			},
			allowed:       []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")},
			errorContains: []string{"link-local import not allowed"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateImageStreamImportDisallowedHosts(context.Background(), tc.isi, tc.blocked, tc.allowed, mockIPLookup)

			if len(tc.errorContains) == 0 {
				if len(errs) != 0 {
					t.Errorf("expected no errors but got: %v", errs)
				}
				return
			}

			if len(errs) != len(tc.errorContains) {
				t.Errorf("expected %d errors but got %d: %v", len(tc.errorContains), len(errs), errs)
				return
			}

			for i, expectedMsg := range tc.errorContains {
				if !strings.Contains(errs[i].Error(), expectedMsg) {
					t.Errorf("error[%d]: expected to contain %q but got %q", i, expectedMsg, errs[i].Error())
				}
			}
		})
	}
}
