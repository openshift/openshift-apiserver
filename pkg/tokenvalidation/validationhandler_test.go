package tokenvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emicklei/go-restful"

	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	corefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	oauthv1 "github.com/openshift/api/oauth/v1"
	userv1 "github.com/openshift/api/user/v1"
	oauthfake "github.com/openshift/client-go/oauth/clientset/versioned/fake"
	userfake "github.com/openshift/client-go/user/clientset/versioned/fake"
	userinformer "github.com/openshift/client-go/user/informers/externalversions"
	bootstrap "github.com/openshift/library-go/pkg/authentication/bootstrapauthenticator"

	"github.com/openshift/openshift-apiserver/pkg/tokenvalidation/usercache"
	tokenvalidators "github.com/openshift/openshift-apiserver/pkg/tokenvalidation/validators"
)

func TestAuthenticateTokenInvalidUID(t *testing.T) {
	fakeOAuthClient := oauthfake.NewSimpleClientset(
		&oauthv1.OAuthAccessToken{
			ObjectMeta: metav1.ObjectMeta{Name: "token", CreationTimestamp: metav1.Time{Time: time.Now()}},
			ExpiresIn:  600, // 10 minutes
			UserName:   "foo",
			UserUID:    string("bar1"),
		},
	)
	fakeKubeClient := corefake.NewSimpleClientset()
	fakeUserClient := userfake.NewSimpleClientset(&userv1.User{ObjectMeta: metav1.ObjectMeta{Name: "foo", UID: "bar2"}})

	bootstrapDataGetter := bootstrap.NewBootstrapUserDataGetter(fakeKubeClient.CoreV1(), fakeKubeClient.CoreV1())

	tokenAuthenticator := NewTokenValidationHandler(fakeOAuthClient.OauthV1().OAuthAccessTokens(), bootstrapDataGetter, fakeUserClient.UserV1().Users(), NoopGroupMapper{}, tokenvalidators.NewUIDValidator())

	userInfo, err := tokenAuthenticator.gatherUserInfo(context.TODO(), "token")
	if err.Error() != "user.UID (bar2) does not match token.userUID (bar1)" {
		t.Errorf("Unexpected error: %v", err)
	}
	if userInfo != nil {
		t.Errorf("Unexpected user: %v", userInfo)
	}
}

func TestAuthenticateTokenNotFoundSuppressed(t *testing.T) {
	fakeOAuthClient := oauthfake.NewSimpleClientset()
	fakeUserClient := userfake.NewSimpleClientset()
	fakeKubeClient := corefake.NewSimpleClientset()

	bootstrapDataGetter := bootstrap.NewBootstrapUserDataGetter(fakeKubeClient.CoreV1(), fakeKubeClient.CoreV1())

	tokenAuthenticator := NewTokenValidationHandler(fakeOAuthClient.OauthV1().OAuthAccessTokens(), bootstrapDataGetter, fakeUserClient.UserV1().Users(), NoopGroupMapper{})

	userInfo, err := tokenAuthenticator.gatherUserInfo(context.TODO(), "token")
	if err != errLookup {
		t.Error("Expected not found error to be suppressed with lookup error")
	}
	if userInfo != nil {
		t.Errorf("Unexpected user: %v", userInfo)
	}
}

func TestAuthenticateTokenOtherGetErrorSuppressed(t *testing.T) {
	fakeOAuthClient := oauthfake.NewSimpleClientset()
	fakeKubeClient := corefake.NewSimpleClientset()
	bootstrapDataGetter := bootstrap.NewBootstrapUserDataGetter(fakeKubeClient.CoreV1(), fakeKubeClient.CoreV1())

	fakeOAuthClient.PrependReactor("get", "oauthaccesstokens", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("get error")
	})
	fakeUserClient := userfake.NewSimpleClientset()
	tokenAuthenticator := NewTokenValidationHandler(fakeOAuthClient.OauthV1().OAuthAccessTokens(), bootstrapDataGetter, fakeUserClient.UserV1().Users(), NoopGroupMapper{})

	userInfo, err := tokenAuthenticator.gatherUserInfo(context.TODO(), "token")
	if err != errLookup {
		t.Error("Expected custom get error to be suppressed with lookup error")
	}
	if userInfo != nil {
		t.Errorf("Unexpected user: %v", userInfo)
	}
}

func TestTokenValidationHandler(t *testing.T) {
	tests := []struct {
		name             string
		accesstoken      *oauthv1.OAuthAccessToken
		user             *userv1.User
		reviewedToken    *string
		expectedResponse authenticationv1.TokenReviewStatus
		expectedStatus   int
	}{
		{
			name: "no input",
			expectedResponse: authenticationv1.TokenReviewStatus{
				Error: "the input data is not a token review request",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:          "empty token",
			reviewedToken: sptr(""),
			expectedResponse: authenticationv1.TokenReviewStatus{
				Error: "review request is missing a token",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "non-existent user",
			accesstoken: &oauthv1.OAuthAccessToken{
				ObjectMeta: metav1.ObjectMeta{Name: "atokencreatedmanuallybymetyping"},
				UserName:   "pepa",
				UserUID:    "some-uid",
			},
			user: &userv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tonda",
					UID:  "tonda-uid",
				},
			},
			reviewedToken: sptr("atokencreatedmanuallybymetyping"),
			expectedResponse: authenticationv1.TokenReviewStatus{
				Error: `users.user.openshift.io "pepa" not found`,
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "non-existent token",
			accesstoken: &oauthv1.OAuthAccessToken{
				ObjectMeta: metav1.ObjectMeta{Name: "adifferenttokenfromtheoneintherequest"},
				UserName:   "jenda",
				UserUID:    "some-uid",
				Scopes:     []string{"user:check-access"},
			},
			user: &userv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "jenda",
					UID:  "some-uid",
				},
			},
			reviewedToken: sptr("sometokenthatiscertainlynotpresent"),
			expectedResponse: authenticationv1.TokenReviewStatus{
				Error: "token lookup failed",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "invalid user UID in the token",
			accesstoken: &oauthv1.OAuthAccessToken{
				ObjectMeta: metav1.ObjectMeta{Name: "atokencreatedmanuallybymetyping"},
				UserName:   "usertypek",
				UserUID:    "weird-uid",
				Scopes:     []string{"user:full"},
			},
			user: &userv1.User{
				ObjectMeta: metav1.ObjectMeta{Name: "usertypek"},
			},
			reviewedToken: sptr("atokencreatedmanuallybymetyping"),
			expectedResponse: authenticationv1.TokenReviewStatus{
				Error: "user.UID () does not match token.userUID (weird-uid)",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "valid token review",
			accesstoken: &oauthv1.OAuthAccessToken{
				ObjectMeta: metav1.ObjectMeta{Name: "atokencreatedmanuallybymetyping"},
				UserName:   "usertypek",
				UserUID:    "some-uid",
				Scopes:     []string{"user:full"},
			},
			user: &userv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usertypek",
					UID:  "some-uid",
				},
			},
			reviewedToken: sptr("atokencreatedmanuallybymetyping"),
			expectedResponse: authenticationv1.TokenReviewStatus{
				Authenticated: true,
				User: authenticationv1.UserInfo{
					Username: "usertypek",
					UID:      "some-uid",
					Groups:   []string{"system:authenticated:oauth"},
					Extra: map[string]authenticationv1.ExtraValue{
						"scopes.authorization.openshift.io": []string{"user:full"},
					},
				},
			},
			expectedStatus: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var reviewBytes []byte
			var err error

			if tt.reviewedToken != nil {
				reviewBytes, err = runtime.Encode(
					encoder,
					&authenticationv1.TokenReview{
						Spec: authenticationv1.TokenReviewSpec{
							Token: *tt.reviewedToken,
						},
					})
				if err != nil {
					t.Fatalf("failed to encode review data: %v", err)
				}
			}
			req := httptest.NewRequest("POST", "https://localhost", bytes.NewBuffer(reviewBytes))
			responseRecorder := httptest.NewRecorder()

			users := []runtime.Object{}
			if tt.user != nil {
				users = append(users, tt.user)
			}
			tokens := []runtime.Object{}
			if tt.accesstoken != nil {
				tokens = append(tokens, tt.accesstoken)
			}

			userClient := userfake.NewSimpleClientset(users...)
			userInformer := userinformer.NewSharedInformerFactory(userClient, time.Second*60)
			if err := userInformer.User().V1().Groups().Informer().AddIndexers(cache.Indexers{
				usercache.ByUserIndexName: usercache.ByUserIndexKeys,
			}); err != nil {
				t.Fatalf("failed to create user index: %v", err)
			}

			h := TokenValidationHandler{
				accessTokenClient: oauthfake.NewSimpleClientset(tokens...).OauthV1().OAuthAccessTokens(),
				userClient:        userClient.UserV1().Users(),
				groupMapper:       usercache.NewGroupCache(userInformer.User().V1().Groups()),
				// testing just a single validator to prove errors propagate
				validators: tokenvalidators.OAuthTokenValidators{tokenvalidators.NewUIDValidator()},
			}
			h.ServeHTTP(restful.NewRequest(req), restful.NewResponse(responseRecorder))

			gotResponse := authenticationv1.TokenReview{}
			if err := json.NewDecoder(responseRecorder.Body).Decode(&gotResponse); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			expectedResponse := authenticationv1.TokenReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "TokenReview",
					APIVersion: "authentication.k8s.io/v1",
				},
				Status: tt.expectedResponse,
			}

			if gotStatus := responseRecorder.Result().StatusCode; tt.expectedStatus != gotStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, gotStatus)
			}

			if !equality.Semantic.DeepEqual(expectedResponse, gotResponse) {
				t.Errorf("expected != got: %s", diff.ObjectDiff(expectedResponse, gotResponse))
			}
		})
	}
}

func sptr(s string) *string {
	return &s
}
