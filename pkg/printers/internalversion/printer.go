package internalversion

import (
	kprinters "k8s.io/kubernetes/pkg/printers"
	kprintersinternal "k8s.io/kubernetes/pkg/printers/internalversion"

	appsinternalprinters "github.com/openshift/openshift-apiserver/pkg/apps/printers/internalversion"
	authinternalprinters "github.com/openshift/openshift-apiserver/pkg/authorization/printers/internalversion"
	buildinternalprinters "github.com/openshift/openshift-apiserver/pkg/build/printers/internalversion"
	imageinternalprinters "github.com/openshift/openshift-apiserver/pkg/image/printers/internalversion"
	oauthinternalprinters "github.com/openshift/openshift-apiserver/pkg/oauth/printers/internalversion"
	projectinternalprinters "github.com/openshift/openshift-apiserver/pkg/project/printers/internalversion"
	quotainternalprinters "github.com/openshift/openshift-apiserver/pkg/quota/printers/internalversion"
	routeinternalprinters "github.com/openshift/openshift-apiserver/pkg/route/printers/internalversion"
	securityinternalprinters "github.com/openshift/openshift-apiserver/pkg/security/printers/internalversion"
	templateinternalprinters "github.com/openshift/openshift-apiserver/pkg/template/printers/internalversion"
	userinternalprinters "github.com/openshift/openshift-apiserver/pkg/user/printers/internalversion"
)

func init() {
	// TODO this should be eliminated
	kprintersinternal.AddHandlers = func(p kprinters.PrintHandler) {
		// kubernetes handlers
		kprintersinternal.AddKubeHandlers(p)

		appsinternalprinters.AddAppsOpenShiftHandlers(p)
		buildinternalprinters.AddBuildOpenShiftHandlers(p)
		imageinternalprinters.AddImageOpenShiftHandlers(p)
		projectinternalprinters.AddProjectOpenShiftHandlers(p)
		routeinternalprinters.AddRouteOpenShiftHandlers(p)

		// template.openshift.io handlers
		templateinternalprinters.AddTemplateOpenShiftHandlers(p)

		// security.openshift.io handlers
		securityinternalprinters.AddSecurityOpenShiftHandler(p)

		// authorization.openshift.io handlers
		authinternalprinters.AddAuthorizationOpenShiftHandler(p)

		// quota.openshift.io handlers
		quotainternalprinters.AddQuotaOpenShiftHandler(p)

		// oauth.openshift.io handlers
		oauthinternalprinters.AddOAuthOpenShiftHandler(p)

		// user.openshift.io handlers
		userinternalprinters.AddUserOpenShiftHandler(p)
	}
}
