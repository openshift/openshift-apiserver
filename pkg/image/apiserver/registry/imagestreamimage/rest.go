package imagestreamimage

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	imagev1 "github.com/openshift/api/image/v1"
	"github.com/openshift/library-go/pkg/image/imageutil"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/internalimageutil"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/imagestream"
	imageprinters "github.com/openshift/openshift-apiserver/pkg/image/printers/internalversion"
)

// REST implements the RESTStorage interface in terms of an image registry and
// image stream registry. It only supports the Get method and is used
// to retrieve an image by id, scoped to an ImageStream. REST ensures
// that the requested image belongs to the specified ImageStream.
type REST struct {
	imageRegistry       image.Registry
	imageStreamRegistry imagestream.Registry
	rest.TableConvertor
}

var (
	_ rest.Getter               = &REST{}
	_ rest.ShortNamesProvider   = &REST{}
	_ rest.Scoper               = &REST{}
	_ rest.Storage              = &REST{}
	_ rest.SingularNameProvider = &REST{}
)

// ShortNames implements the ShortNamesProvider interface. Returns a list of short names for a resource.
func (r *REST) ShortNames() []string {
	return []string{"isimage"}
}

// NewREST returns a new REST.
func NewREST(imageRegistry image.Registry, imageStreamRegistry imagestream.Registry) *REST {
	return &REST{
		imageRegistry,
		imageStreamRegistry,
		printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(imageprinters.AddImageOpenShiftHandlers)},
	}
}

// New is only implemented to make REST implement RESTStorage
func (r *REST) New() runtime.Object {
	return &imageapi.ImageStreamImage{}
}

func (r *REST) Destroy() {}

func (r *REST) NamespaceScoped() bool {
	return true
}

func (r *REST) GetSingularName() string {
	return "imagestreamimage"
}

// parseNameAndID splits a string into its name component and ID component, and returns an error
// if the string is not in the right form.
func parseNameAndID(input string) (name string, id string, err error) {
	name, id, err = imageutil.ParseImageStreamImageName(input)
	if err != nil {
		err = errors.NewBadRequest("ImageStreamImages must be retrieved with <name>@<id>")
	}
	return
}

// Get retrieves an image by ID that has previously been tagged into an image stream.
// `id` is of the form <repo name>@<image id>.
func (r *REST) Get(ctx context.Context, id string, options *metav1.GetOptions) (runtime.Object, error) {
	name, imageID, err := parseNameAndID(id)
	if err != nil {
		return nil, err
	}

	imageName := ""

	// using the layers api to get the image name instead of the image stream
	// tag history enable us to also list images belonging to a manifest list,
	// which are not listed in an image stream tag history.
	layers, err := r.imageStreamRegistry.GetImageStreamLayers(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	for id := range layers.Images {
		if imageutil.DigestOrImageMatch(id, imageID) {
			imageName = id
		}
	}
	if imageName == "" {
		return nil, errors.NewNotFound(imagev1.Resource("imagestreamimage"), id)
	}

	image, err := r.imageRegistry.GetImage(ctx, imageName, &metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if err := internalimageutil.InternalImageWithMetadata(image); err != nil {
		return nil, err
	}
	image.DockerImageManifest = ""
	image.DockerImageConfig = ""

	isi := imageapi.ImageStreamImage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         apirequest.NamespaceValue(ctx),
			Name:              imageutil.JoinImageStreamImage(name, imageID),
			CreationTimestamp: image.ObjectMeta.CreationTimestamp,
			Annotations:       layers.ObjectMeta.Annotations,
		},
		Image: *image,
	}

	return &isi, nil
}
