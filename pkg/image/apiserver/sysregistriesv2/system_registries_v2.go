package sysregistriesv2

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/docker/distribution/reference"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// systemRegistriesConfPath is the path to the system-wide registry
// configuration file and is used to add/subtract potential registries for
// obtaining images.  You can override this at build time with
// -ldflags '-X github.com/containers/image/sysregistries.systemRegistriesConfPath=$your_path'
var systemRegistriesConfPath = builtinRegistriesConfPath

// builtinRegistriesConfPath is the path to the registry configuration file.
// DO NOT change this, instead see systemRegistriesConfPath above.
const builtinRegistriesConfPath = "/etc/containers/registries.conf"

// Endpoint describes a remote location of a registry.
type Endpoint struct {
	// The endpoint's remote location.
	Location string
	// If true, certs verification will be skipped and HTTP (non-TLS)
	// connections will be allowed.
	Insecure bool
}

// rewriteReference will substitute the provided reference `prefix` to the
// endpoints `location` from the `ref` and creates a new named reference from it.
// The function errors if the newly created reference is not parsable.
func (e *Endpoint) rewriteReference(ref reference.Named, prefix string) (reference.Named, error) {
	refString := ref.String()
	if !refMatchesPrefix(refString, prefix) {
		return nil, fmt.Errorf("invalid prefix '%v' for reference '%v'", prefix, refString)
	}

	newNamedRef := strings.Replace(refString, prefix, e.Location, 1)
	newParsedRef, err := reference.ParseNamed(newNamedRef)
	if err != nil {
		return nil, errors.Wrapf(err, "error rewriting reference")
	}
	logrus.Debugf("reference rewritten from '%v' to '%v'", refString, newParsedRef.String())
	return newParsedRef, nil
}

// Registry represents a registry.
type Registry struct {
	// Prefix is used for matching images, and to translate one namespace to
	// another.  If `Prefix="example.com/bar"`, `location="example.com/foo/bar"`
	// and we pull from "example.com/bar/myimage:latest", the image will
	// effectively be pulled from "example.com/foo/bar/myimage:latest".
	// If no Prefix is specified, it defaults to the specified location.
	Prefix string
	// A registry is an Endpoint too
	Endpoint
	// The registry's mirrors.
	Mirrors []Endpoint
	// If true, pulling from the registry will be blocked.
	Blocked bool
	// If true, mirrors will only be used for digest pulls. Pulling images by
	// tag can potentially yield different images, depending on which endpoint
	// we pull from.  Forcing digest-pulls for mirrors avoids that issue.
	MirrorByDigestOnly bool
}

// PullSource consists of an Endpoint and a Reference. Note that the reference is
// rewritten according to the registries prefix and the Endpoint's location.
type PullSource struct {
	Endpoint  Endpoint
	Reference reference.Named
}

// PullSourcesFromReference returns a slice of PullSource's based on the passed
// reference.
func (r *Registry) PullSourcesFromReference(ref reference.Named) ([]PullSource, error) {
	var endpoints []Endpoint

	if r.MirrorByDigestOnly {
		// Only use mirrors when the reference is a digest one.
		if _, isDigested := ref.(reference.Canonical); isDigested {
			endpoints = append(r.Mirrors, r.Endpoint)
		} else {
			endpoints = []Endpoint{r.Endpoint}
		}
	} else {
		endpoints = append(r.Mirrors, r.Endpoint)
	}

	sources := []PullSource{}
	for _, ep := range endpoints {
		rewritten, err := ep.rewriteReference(ref, r.Prefix)
		if err != nil {
			return nil, err
		}
		sources = append(sources, PullSource{Endpoint: ep, Reference: rewritten})
	}

	return sources, nil
}

// V2RegistriesConf is the sysregistries v2 configuration format.
type V2RegistriesConf struct {
	Registries []Registry
	// An array of host[:port] (not prefix!) entries to use for resolving unqualified image references
	UnqualifiedSearchRegistries []string
}

// Nonempty returns true if config contains at least one configuration entry.
func (config *V2RegistriesConf) Nonempty() bool {
	return (len(config.Registries) != 0 ||
		len(config.UnqualifiedSearchRegistries) != 0)
}

// InvalidRegistries represents an invalid registry configurations.  An example
// is when "registry.com" is defined multiple times in the configuration but
// with conflicting security settings.
type InvalidRegistries struct {
	s string
}

// Error returns the error string.
func (e *InvalidRegistries) Error() string {
	return e.s
}

// parseLocation parses the input string, performs some sanity checks and returns
// the sanitized input string.  An error is returned if the input string is
// empty or if contains an "http{s,}://" prefix.
func parseLocation(input string) (string, error) {
	trimmed := strings.TrimRight(input, "/")

	if trimmed == "" {
		return "", &InvalidRegistries{s: "invalid location: cannot be empty"}
	}

	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		msg := fmt.Sprintf("invalid location '%s': URI schemes are not supported", input)
		return "", &InvalidRegistries{s: msg}
	}

	return trimmed, nil
}

// anchoredDomainRegexp is an internal implementation detail of postProcess, defining the valid values of elements of UnqualifiedSearchRegistries.
var anchoredDomainRegexp = regexp.MustCompile("^" + reference.DomainRegexp.String() + "$")

// postProcess checks the consistency of all the configuration, looks for conflicts,
// and normalizes the configuration (e.g., sets the Prefix to Location if not set).
func (config *V2RegistriesConf) postProcess() error {
	regMap := make(map[string][]*Registry)

	for i := range config.Registries {
		reg := &config.Registries[i]
		// make sure Location and Prefix are valid
		var err error
		reg.Location, err = parseLocation(reg.Location)
		if err != nil {
			return err
		}

		if reg.Prefix == "" {
			reg.Prefix = reg.Location
		} else {
			reg.Prefix, err = parseLocation(reg.Prefix)
			if err != nil {
				return err
			}
		}

		// make sure mirrors are valid
		for _, mir := range reg.Mirrors {
			mir.Location, err = parseLocation(mir.Location)
			if err != nil {
				return err
			}
		}
		regMap[reg.Location] = append(regMap[reg.Location], reg)
	}

	// Given a registry can be mentioned multiple times (e.g., to have
	// multiple prefixes backed by different mirrors), we need to make sure
	// there are no conflicts among them.
	//
	// Note: we need to iterate over the registries array to ensure a
	// deterministic behavior which is not guaranteed by maps.
	for _, reg := range config.Registries {
		others, _ := regMap[reg.Location]
		for _, other := range others {
			if reg.Insecure != other.Insecure {
				msg := fmt.Sprintf("registry '%s' is defined multiple times with conflicting 'insecure' setting", reg.Location)
				return &InvalidRegistries{s: msg}
			}
			if reg.Blocked != other.Blocked {
				msg := fmt.Sprintf("registry '%s' is defined multiple times with conflicting 'blocked' setting", reg.Location)
				return &InvalidRegistries{s: msg}
			}
		}
	}

	for i := range config.UnqualifiedSearchRegistries {
		registry, err := parseLocation(config.UnqualifiedSearchRegistries[i])
		if err != nil {
			return err
		}
		if !anchoredDomainRegexp.MatchString(registry) {
			return &InvalidRegistries{fmt.Sprintf("Invalid unqualified-search-registries entry %#v", registry)}
		}
		config.UnqualifiedSearchRegistries[i] = registry
	}

	return nil
}

// configMutex is used to synchronize concurrent accesses to configCache.
var configMutex = sync.Mutex{}

// configCache caches already loaded configs with config paths as keys and is
// used to avoid redudantly parsing configs. Concurrent accesses to the cache
// are synchronized via configMutex.
var configCache = make(map[string]*V2RegistriesConf)

// InvalidateCache invalidates the registry cache.  This function is meant to be
// used for long-running processes that need to reload potential changes made to
// the cached registry config files.
func InvalidateCache() {
	configMutex.Lock()
	defer configMutex.Unlock()
	configCache = make(map[string]*V2RegistriesConf)
}

// refMatchesPrefix returns true iff ref,
// which is a registry, repository namespace, repository or image reference (as formatted by
// reference.Domain(), reference.Named.Name() or reference.Reference.String()
// — note that this requires the name to start with an explicit hostname!),
// matches a Registry.Prefix value.
// (This is split from the caller primarily to make testing easier.)
func refMatchesPrefix(ref, prefix string) bool {
	switch {
	case len(ref) < len(prefix):
		return false
	case len(ref) == len(prefix):
		return ref == prefix
	case len(ref) > len(prefix):
		if !strings.HasPrefix(ref, prefix) {
			return false
		}
		c := ref[len(prefix)]
		// This allows "example.com:5000" to match "example.com",
		// which is unintended; that will get fixed eventually, DON'T RELY
		// ON THE CURRENT BEHAVIOR.
		return c == ':' || c == '/' || c == '@'
	default:
		panic("Internal error: impossible comparison outcome")
	}
}
