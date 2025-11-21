package validation

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		expectValid   bool
		expectedError string
	}{
		{
			name:        "valid path with no special chars",
			path:        "/api/v1/users",
			expectValid: true,
		},
		{
			name:        "valid path with hyphens",
			path:        "/my-api/v1/user-data",
			expectValid: true,
		},
		{
			name:        "valid path with underscores",
			path:        "/api_v1/user_data",
			expectValid: true,
		},
		{
			name:        "valid empty path",
			path:        "",
			expectValid: true,
		},
		{
			name:        "valid root path",
			path:        "/",
			expectValid: true,
		},
		{
			name:          "invalid path with hash",
			path:          "/api#v1",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
		{
			name:          "invalid path with hash at end",
			path:          "/api/v1#",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
		{
			name:          "invalid path with hash at beginning",
			path:          "#/api/v1",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
		{
			name:          "invalid path with single space",
			path:          "/api /v1",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
		{
			name:          "invalid path with multiple spaces",
			path:          "/api  /v1",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
		{
			name:          "invalid path with space at beginning",
			path:          " /api/v1",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
		{
			name:          "invalid path with space at end",
			path:          "/api/v1 ",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
		{
			name:          "invalid path with hash and space",
			path:          "/api# /v1",
			expectValid:   false,
			expectedError: "cannot contain # or spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fldPath := field.NewPath("spec", "path")
			errs := validatePath(tt.path, fldPath)

			if tt.expectValid {
				if len(errs) != 0 {
					t.Errorf("expected path %q to be valid, but got errors: %v", tt.path, errs)
				}
			} else {
				if len(errs) == 0 {
					t.Errorf("expected path %q to be invalid, but got no errors", tt.path)
				} else {
					// Verify the error message contains the expected text
					found := false
					for _, err := range errs {
						if err.Field == "spec.path" && err.Type == field.ErrorTypeInvalid {
							if err.Detail == tt.expectedError {
								found = true
								break
							}
						}
					}
					if !found {
						t.Errorf("expected error message %q, but got: %v", tt.expectedError, errs)
					}
				}
			}
		})
	}
}
