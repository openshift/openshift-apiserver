package importer

import (
	"net/url"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/credentialprovider"
	"k8s.io/kubernetes/pkg/credentialprovider/secrets"
)

var (
	emptyKeyring = &credentialprovider.BasicDockerKeyring{}
)

// secretsRetriever is a function that returns a list of kubernetes secrets.
type secretsRetriever func() ([]corev1.Secret, error)

// NewCredentialsForSecrets returns a credential store populated with a list
// of kubernetes secrets. Secrets are filtered as SecretCredentialStore uses
// only the ones containing docker credentials.
func NewCredentialsForSecrets(secrets []corev1.Secret) *SecretCredentialStore {
	return &SecretCredentialStore{
		secrets: secrets,
	}
}

// NewLazyCredentialsForSecrets returns a credential store populated with the
// return of fn(). The return of fn() is filtered as SecretCredentialStore uses
// only secrets that contain docker credentials.
func NewLazyCredentialsForSecrets(fn secretsRetriever) *SecretCredentialStore {
	return &SecretCredentialStore{
		secretsFn: fn,
	}
}

// SecretCredentialStore holds docker credentials. It uses a list of secrets
// from where it extracts docker credentials, allowing callers to retrieve
// BasicAuth information by URL.
type SecretCredentialStore struct {
	lock      sync.Mutex
	secrets   []corev1.Secret
	secretsFn secretsRetriever
	err       error
	keyring   credentialprovider.DockerKeyring
}

// Basic returns BasicAuth information for the given url (user and password).
// If url does not exist on SecretCredentialStore's internal keyring empty
// strings are returned.
func (s *SecretCredentialStore) Basic(url *url.URL) (string, string) {
	s.init()
	return basicCredentialsFromKeyring(s.keyring, url)
}

// Err returns SecretCredentialStore's internal error.
func (s *SecretCredentialStore) Err() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.err
}

// init runs only once and is reponsible for loading the internal keyring with
// Secrets data (if a secretsRetriever function was specified). This function
// initializes the internal keyring. In case of errors, internal err is set.
func (s *SecretCredentialStore) init() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.keyring != nil {
		return
	}

	// lazily load the secrets
	if s.secrets == nil && s.secretsFn != nil {
		s.secrets, s.err = s.secretsFn()
	}

	// TODO: need a version of this that is best effort secret - otherwise
	// one error blocks all secrets
	keyring, err := secrets.MakeDockerKeyring(s.secrets, emptyKeyring)
	if err != nil {
		klog.V(5).Infof("Loading keyring failed for credential store: %v", err)
		s.err = err
		keyring = emptyKeyring
	}
	s.keyring = keyring
}

// basicCredentialsFromKeyring extract basicAuth information from provided
// keyring. If keyring does not contain information for the provided URL, empty
// strings are returned instead.
func basicCredentialsFromKeyring(keyring credentialprovider.DockerKeyring, target *url.URL) (string, string) {
	regURL := getURLForLookup(target)
	if configs, found := keyring.Lookup(regURL); found {
		klog.V(5).Infof(
			"Found secret to match %s (%s): %s",
			target, regURL, configs[0].ServerAddress,
		)
		return configs[0].Username, configs[0].Password
	}

	// do a special case check for docker.io to match historical lookups
	// when we respond to a challenge
	if regURL == "auth.docker.io/token" {
		klog.V(5).Infof(
			"Being asked for %s (%s), trying %s, legacy behavior",
			target, regURL, "index.docker.io/v1",
		)
		return basicCredentialsFromKeyring(
			keyring, &url.URL{Host: "index.docker.io", Path: "/v1"},
		)
	}

	// docker 1.9 saves 'docker.io' in config in f23, see
	// https://bugzilla.redhat.com/show_bug.cgi?id=1309739
	if regURL == "index.docker.io" {
		klog.V(5).Infof(
			"Being asked for %s (%s), trying %s, legacy behavior",
			target, regURL, "docker.io",
		)
		return basicCredentialsFromKeyring(
			keyring, &url.URL{Host: "docker.io"},
		)
	}

	// try removing the canonical ports.
	if hasCanonicalPort(target) {
		host := strings.SplitN(target.Host, ":", 2)[0]
		klog.V(5).Infof(
			"Being asked for %s (%s), trying %s without port",
			target, regURL, host,
		)
		return basicCredentialsFromKeyring(
			keyring,
			&url.URL{
				Scheme: target.Scheme,
				Host:   host,
				Path:   target.Path,
			},
		)
	}

	klog.V(5).Infof("Unable to find a secret to match %s (%s)",
		target, regURL,
	)
	return "", ""
}

// getURLForLookup returns the URL we should use when looking for credentials
// on a keyring.
func getURLForLookup(target *url.URL) string {
	var res string
	if target == nil {
		return res
	}

	if len(target.Scheme) == 0 || target.Scheme == "https" {
		res = target.Host + target.Path
	} else {
		// always require an explicit port to look up HTTP credentials
		if strings.Contains(target.Host, ":") {
			res = target.Host + target.Path
		} else {
			res = target.Host + ":80" + target.Path
		}
	}

	// Lookup(...) expects an image (not a URL path). The keyring strips
	// /v1/ and /v2/ version prefixes so we should do the same when
	// selecting a valid auth for a URL.
	pathWithSlash := target.Path + "/"
	if strings.HasPrefix(pathWithSlash, "/v1/") || strings.HasPrefix(pathWithSlash, "/v2/") {
		res = target.Host + target.Path[3:]
	}

	return res
}

// hasCanonicalPort returns if port is specified on the url and is the default
// port for the protocol.
func hasCanonicalPort(target *url.URL) bool {
	switch {
	case target == nil:
		return false
	case strings.HasSuffix(target.Host, ":443") && target.Scheme == "https":
		return true
	case strings.HasSuffix(target.Host, ":80") && target.Scheme == "http":
		return true
	default:
		return false
	}
}
