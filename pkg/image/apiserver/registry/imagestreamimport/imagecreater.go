package imagestreamimport

import (
	"context"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	internalimageutil "github.com/openshift/openshift-apiserver/pkg/image/apiserver/internal/imageutil"
)

type cachedImageCreater struct {
	strategy *strategy
	images   rest.Creater
	cache    map[string]*imageapi.Image
}

func newCachedImageCreater(strategy *strategy, images rest.Creater) *cachedImageCreater {
	return &cachedImageCreater{
		strategy: strategy,
		images:   images,
		cache:    make(map[string]*imageapi.Image),
	}
}

func (ic *cachedImageCreater) Create(ctx context.Context, image *imageapi.Image) (*imageapi.Image, error) {
	ic.strategy.PrepareImageForCreate(image)

	if cachedImage, ok := ic.cache[image.Name]; ok {
		return cachedImage, nil
	}

	createdImage, err := ic.images.Create(ctx, image, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	switch {
	case kapierrors.IsAlreadyExists(err):
		if err := internalimageutil.InternalImageWithMetadata(image); err != nil {
			klog.V(4).Infof("Unable to update image metadata during image import when image already exists %q: %v", image.Name, err)
		}
	case err == nil:
		image = createdImage.(*imageapi.Image)
	default:
		return nil, err
	}

	ic.cache[image.Name] = image

	return image, nil
}
