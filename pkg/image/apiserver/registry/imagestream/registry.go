package imagestream

import (
	"context"

	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

// Registry is an interface for things that know how to store ImageStream objects.
type Registry interface {
	// ListImageStreams obtains a list of image streams that match a selector.
	ListImageStreams(ctx context.Context, options *metainternal.ListOptions) (*imageapi.ImageStreamList, error)
	// GetImageStream retrieves a specific image stream.
	GetImageStream(ctx context.Context, id string, options *metav1.GetOptions) (*imageapi.ImageStream, error)
	// GetImageStreamLayers retrieves layers of a specific image stream
	GetImageStreamLayers(ctx context.Context, imageStreamID string, options *metav1.GetOptions) (*imageapi.ImageStreamLayers, error)
	// CreateImageStream creates a new image stream.
	CreateImageStream(ctx context.Context, repo *imageapi.ImageStream, options *metav1.CreateOptions) (*imageapi.ImageStream, error)
	// UpdateImageStream updates an image stream.
	UpdateImageStream(ctx context.Context, repo *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error)
	// UpdateImageStreamSpec updates an image stream's spec.
	UpdateImageStreamSpec(ctx context.Context, repo *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error)
	// UpdateImageStreamStatus updates an image stream's status.
	UpdateImageStreamStatus(ctx context.Context, repo *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error)
	// DeleteImageStream deletes an image stream.
	DeleteImageStream(ctx context.Context, id string) (*metav1.Status, error)
	// WatchImageStreams watches for new/changed/deleted image streams.
	WatchImageStreams(ctx context.Context, options *metainternal.ListOptions) (watch.Interface, error)
}

// Storage is an interface for a standard REST Storage backend
type Storage interface {
	rest.GracefulDeleter
	rest.Lister
	rest.Getter
	rest.Watcher

	Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error)
	Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error)
}

// storage puts strong typing around storage calls
type storage struct {
	Storage
	status   rest.Updater
	internal rest.Updater
	layers   rest.Getter
}

// NewRegistry returns a new Registry interface for the given Storage. Any mismatched
// types will panic.
func NewRegistry(s Storage, status, internal rest.Updater, layers rest.Getter) Registry {
	return &storage{Storage: s, status: status, internal: internal, layers: layers}
}

func (s *storage) ListImageStreams(ctx context.Context, options *metainternal.ListOptions) (*imageapi.ImageStreamList, error) {
	obj, err := s.List(ctx, options)
	if err != nil {
		return nil, err
	}
	return obj.(*imageapi.ImageStreamList), nil
}

func (s *storage) GetImageStream(ctx context.Context, imageStreamID string, options *metav1.GetOptions) (*imageapi.ImageStream, error) {
	obj, err := s.Get(ctx, imageStreamID, options)
	if err != nil {
		return nil, err
	}
	return obj.(*imageapi.ImageStream), nil
}

func (s *storage) CreateImageStream(ctx context.Context, imageStream *imageapi.ImageStream, options *metav1.CreateOptions) (*imageapi.ImageStream, error) {
	obj, err := s.Create(ctx, imageStream, rest.ValidateAllObjectFunc, options)
	if err != nil {
		return nil, err
	}
	return obj.(*imageapi.ImageStream), nil
}

func (s *storage) UpdateImageStream(ctx context.Context, imageStream *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error) {
	obj, _, err := s.internal.Update(ctx, imageStream.Name, rest.DefaultUpdatedObjectInfo(imageStream), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, forceAllowCreate, options)
	if err != nil {
		return nil, err
	}
	return obj.(*imageapi.ImageStream), nil
}

func (s *storage) UpdateImageStreamSpec(ctx context.Context, imageStream *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error) {
	obj, _, err := s.Update(ctx, imageStream.Name, rest.DefaultUpdatedObjectInfo(imageStream), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, forceAllowCreate, options)
	if err != nil {
		return nil, err
	}
	return obj.(*imageapi.ImageStream), nil
}

func (s *storage) UpdateImageStreamStatus(ctx context.Context, imageStream *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error) {
	obj, _, err := s.status.Update(ctx, imageStream.Name, rest.DefaultUpdatedObjectInfo(imageStream), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, forceAllowCreate, options)
	if err != nil {
		return nil, err
	}
	return obj.(*imageapi.ImageStream), nil
}

func (s *storage) DeleteImageStream(ctx context.Context, imageStreamID string) (*metav1.Status, error) {
	obj, _, err := s.Delete(ctx, imageStreamID, rest.ValidateAllObjectFunc, nil)
	if err != nil {
		return nil, err
	}
	return obj.(*metav1.Status), nil
}

func (s *storage) WatchImageStreams(ctx context.Context, options *metainternal.ListOptions) (watch.Interface, error) {
	return s.Watch(ctx, options)
}

func (s *storage) GetImageStreamLayers(
	ctx context.Context,
	imageStreamID string,
	options *metav1.GetOptions,
) (*imageapi.ImageStreamLayers, error) {
	obj, err := s.layers.Get(ctx, imageStreamID, options)
	if err != nil {
		return nil, err
	}
	return obj.(*imageapi.ImageStreamLayers), nil
}
