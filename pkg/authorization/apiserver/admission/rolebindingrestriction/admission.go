package rolebindingrestriction

import (
	"context"
	"errors"
	"fmt"
	"io"

	authzv1 "github.com/openshift/api/authorization/v1"
	configv1 "github.com/openshift/api/config/v1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configv1listers "github.com/openshift/client-go/config/listers/config/v1"
	"k8s.io/apiserver/pkg/admission"
)

const PluginName = "authorization.openshift.io/RoleBindingRestrictionOIDC"

var (
	_ admission.ValidationInterface     = (*roleBindingRestrictionOIDC)(nil)
	_ admission.InitializationValidator = (*roleBindingRestrictionOIDC)(nil)
)

type roleBindingRestrictionOIDC struct {
	*admission.Handler
	authnLister configv1listers.AuthenticationLister
}

func (rbr *roleBindingRestrictionOIDC) SetOpenShiftConfigInformers(informers configinformers.SharedInformerFactory) {
	rbr.authnLister = informers.Config().V1().Authentications().Lister()
}

func (rbr *roleBindingRestrictionOIDC) ValidateInitialization() error {
	if rbr.authnLister == nil {
		return fmt.Errorf("%s requires an AuthenticationLister", PluginName)
	}

	return nil
}

func (rbr *roleBindingRestrictionOIDC) Validate(ctx context.Context, attr admission.Attributes, _ admission.ObjectInterfaces) error {
	// if it is not a RoleBindingRestriction, ignore it
	if attr.GetKind() != authzv1.GroupVersion.WithKind("RoleBindingRestriction") {
		return nil
	}

	roleBindingRestriction, ok := attr.GetObject().(*authzv1.RoleBindingRestriction)
	if !ok {
		return errors.New("object is supposed to be of type RoleBindingRestriction, but could not be type casted as one")
	}

	authn, err := rbr.authnLister.Get("cluster")
	if err != nil {
		return fmt.Errorf("getting Authentication configuration for the cluster: %w", err)
	}

	// If the cluster authentication type is not OIDC, we do not
	// currently need to validate anything
	if authn.Spec.Type != configv1.AuthenticationTypeOIDC {
		return nil
	}

	// If the cluster authentication type is configured as OIDC,
	// we need to reject create/update requests that set user/group
	// restrictions. The User and Group APIs are no longer present when
	// the authentication type for the cluster is OIDC as the oauth-apiserver
	// is removed from the cluster. Blocking create/update of RoleBindingRestriction
	// resources with user/group restrictions to help prevent unexpected behavior of
	// the kube-apiserver authorization.openshift.io/RestrictSubjectBindings
	// admission plugin. When a RoleBindingRestriction that sets user/group
	// restrictions exists in the same namespace as a RoleBinding is being
	// created in, the authorization.openshift.io/RestrictSubjectBindings admission
	// plugin will reject it when authentication type of the cluster is OIDC.
	// This is because the admission plugin can not properly evaluate those
	// restrictions.
    if roleBindingRestriction.Spec.UserRestriction != nil {
        return errors.New("user restrictions can not be set on rolebindingrestrictions when authentication type is set to OIDC")
    }

    if roleBindingRestriction.Spec.GroupRestriction != nil {
        return errors.New("group restrictions can not be set on rolebindingrestrictions when authentication type is set to OIDC")
    }

	return nil
}

func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName,
		func(config io.Reader) (admission.Interface, error) {
			plugin, err := NewRoleBindingRestrictionOIDCPlugin(config)
			if err != nil {
				return nil, err
			}
			return plugin, nil
		})
}

func NewRoleBindingRestrictionOIDCPlugin(config io.Reader) (admission.Interface, error) {
	return &roleBindingRestrictionOIDC{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}, nil
}
