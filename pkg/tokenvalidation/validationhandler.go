package tokenvalidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/emicklei/go-restful"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	authorizationv1 "github.com/openshift/api/authorization/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	userv1 "github.com/openshift/api/user/v1"
	oauthv1client "github.com/openshift/client-go/oauth/clientset/versioned/typed/oauth/v1"
	userv1client "github.com/openshift/client-go/user/clientset/versioned/typed/user/v1"
	bootstrap "github.com/openshift/library-go/pkg/authentication/bootstrapauthenticator"

	tokenvalidators "github.com/openshift/openshift-apiserver/pkg/tokenvalidation/validators"
)

const (
	clusterAdminGroup       = "system:cluster-admins"
	authenticatedOAuthGroup = "system:authenticated:oauth"
)

var errLookup = errors.New("token lookup failed")

type TokenValidationHandler struct {
	accessTokenClient   oauthv1client.OAuthAccessTokenInterface
	userClient          userv1client.UserInterface
	bootstrapUserGetter bootstrap.BootstrapUserDataGetter
	groupMapper         UserToGroupMapper

	validators tokenvalidators.OAuthTokenValidator
}

func NewTokenValidationHandler(
	accessTokenClient oauthv1client.OAuthAccessTokenInterface,
	bootstrapUserGetter bootstrap.BootstrapUserDataGetter,
	userClient userv1client.UserInterface,
	groupMapper UserToGroupMapper,
	validators ...tokenvalidators.OAuthTokenValidator,
) *TokenValidationHandler {
	return &TokenValidationHandler{
		accessTokenClient:   accessTokenClient,
		userClient:          userClient,
		bootstrapUserGetter: bootstrapUserGetter,
		groupMapper:         groupMapper,

		validators: tokenvalidators.OAuthTokenValidators(validators),
	}
}

func (h *TokenValidationHandler) ServeHTTP(r *restful.Request, w *restful.Response) {
	tokenReview := authenticationv1.TokenReview{}

	if err := json.NewDecoder(r.Request.Body).Decode(&tokenReview); err != nil {
		handleFailure(w, http.StatusBadRequest, fmt.Sprintf("the input data is not a token review request"))
		return
	}

	tokenString := tokenReview.Spec.Token
	if len(tokenString) == 0 {
		handleFailure(w, http.StatusUnauthorized, "review request is missing a token")
		return
	}

	userInfo, err := h.gatherUserInfo(context.TODO(), tokenString)
	if err != nil {
		handleFailure(w, http.StatusUnauthorized, err.Error())
		return
	}

	handleSucces(w, userInfo)
}

func (h *TokenValidationHandler) gatherUserInfo(ctx context.Context, tokenName string) (*authenticationv1.UserInfo, error) {
	token, err := h.accessTokenClient.Get(ctx, tokenName, metav1.GetOptions{})
	if err != nil {
		return nil, errLookup // mask the error so we do not leak token data in logs
	}

	var user *userv1.User
	var groupNames []string
	// the bootstrap user is special-cased in authentication
	if token.UserName == bootstrap.BootstrapUser {
		user, groupNames, err = h.getBootstrapUser(token)
	} else {
		user, groupNames, err = h.getOpenShiftUser(ctx, token)
	}

	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("no such user")
	}

	return &authenticationv1.UserInfo{
		Username: user.Name,
		UID:      string(user.UID),
		Groups:   groupNames,
		Extra: map[string]authenticationv1.ExtraValue{
			authorizationv1.ScopesKey: token.Scopes,
		},
	}, nil
}

func (h *TokenValidationHandler) getBootstrapUser(token *oauthv1.OAuthAccessToken) (*userv1.User, []string, error) {
	data, ok, err := h.bootstrapUserGetter.Get()
	if err != nil || !ok {
		return nil, nil, err
	}

	// this allows us to reuse existing validators
	// since the uid is based on the secret, if the secret changes, all
	// tokens issued for the bootstrap user before that change stop working
	fakeUser := &userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			UID:  types.UID(data.UID),
			Name: "kube:admin",
		},
	}

	if err := h.validators.Validate(token, fakeUser); err != nil {
		return nil, nil, err
	}

	// we cannot use SystemPrivilegedGroup because it cannot be properly scoped.
	// see openshift/origin#18922 and how loopback connections are handled upstream via AuthorizeClientBearerToken.
	// api aggregation with delegated authorization makes this impossible to control, see WithAlwaysAllowGroups.
	// an openshift specific cluster role binding binds ClusterAdminGroup to the cluster role cluster-admin.
	// thus this group is authorized to do everything via RBAC.
	// this does make the bootstrap user susceptible to anything that causes the RBAC authorizer to fail.
	// this is a safe trade-off because scopes must always be evaluated before RBAC for them to work at all.
	// a failure in that logic means scopes are broken instead of a specific failure related to the bootstrap user.
	// if this becomes a problem in the future, we could generate a custom extra value based on the secret content
	// and store it in BootstrapUserData, similar to how UID is calculated.  this extra value would then be wired
	// to a custom authorizer that allows all actions.  the problem with such an approach is that since we do not
	// allow remote authorizers in OpenShift, the BootstrapUserDataGetter logic would have to be shared between the
	// the kube api server and osin instead of being an implementation detail hidden inside of osin.  currently the
	// only shared code is the value of the BootstrapUser constant (since it is special cased in validation)
	return fakeUser, []string{clusterAdminGroup}, nil
}

func (h *TokenValidationHandler) getOpenShiftUser(ctx context.Context, token *oauthv1.OAuthAccessToken) (*userv1.User, []string, error) {
	user, err := h.userClient.Get(ctx, token.UserName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	if err := h.validators.Validate(token, user); err != nil {
		return nil, nil, err
	}

	groups, err := h.groupMapper.GroupsFor(user.Name)
	if err != nil {
		return nil, nil, err
	}
	groupNames := make([]string, 0, len(groups))
	for _, group := range groups {
		groupNames = append(groupNames, group.Name)
	}

	// append system:authenticated:oauth group because if you have an OAuth
	// bearer token, you're a human (usually)
	return user, append(groupNames, authenticatedOAuthGroup), nil
}

func handleFailure(w *restful.Response, status int, errMsg string) {
	failedReview := authenticationv1.TokenReview{
		Status: authenticationv1.TokenReviewStatus{
			Authenticated: false,
			Error:         errMsg,
		},
	}
	respBytes, err := runtime.Encode(encoder, &failedReview)
	if err != nil {
		w.WriteError(http.StatusInternalServerError, fmt.Errorf("failed to encode authentication failure: %v", err))
		return
	}

	w.WriteHeader(status)
	w.Write(respBytes)
}

func handleSucces(w *restful.Response, userInfo *authenticationv1.UserInfo) {
	successReview := authenticationv1.TokenReview{
		Status: authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User:          *userInfo,
		},
	}

	bytes, err := runtime.Encode(encoder, &successReview)
	if err != nil {
		handleFailure(w, http.StatusInternalServerError, "failed to encode authentication success")
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}
