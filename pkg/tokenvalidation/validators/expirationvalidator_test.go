package validators

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oauthv1 "github.com/openshift/api/oauth/v1"
	userv1 "github.com/openshift/api/user/v1"
)

func TestTokenExpired(t *testing.T) {
	tokens := []*oauthv1.OAuthAccessToken{
		// expired token that had a lifetime of 10 minutes
		{
			ObjectMeta: metav1.ObjectMeta{Name: "token1", CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)}},
			ExpiresIn:  600,
			UserName:   "foo",
		},
		// non-expired token that has a lifetime of 10 minutes, but has a non-nil deletion timestamp
		{
			ObjectMeta: metav1.ObjectMeta{Name: "token2", CreationTimestamp: metav1.Time{Time: time.Now()}, DeletionTimestamp: &metav1.Time{}},
			ExpiresIn:  600,
			UserName:   "foo",
		},
	}

	expirationValidator := NewExpirationValidator()

	for _, token := range tokens {
		err := expirationValidator.Validate(token, &userv1.User{})
		if err != errExpired {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func TestTokenValidated(t *testing.T) {
	err := NewExpirationValidator().Validate(&oauthv1.OAuthAccessToken{
		ObjectMeta: metav1.ObjectMeta{Name: "token", CreationTimestamp: metav1.Time{Time: time.Now()}},
		ExpiresIn:  600, // 10 minutes
		UserName:   "foo",
		UserUID:    string("bar"),
	}, &userv1.User{})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
