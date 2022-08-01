package imagestreamtag

import (
	"context"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/imagestream"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type imageStreamRegistryTester struct {
	// the true implementation
	imagestream.Registry

	// which apis are we providing responses for
	registryApiTesters map[string]*ApiTester
}

func NewImageStreamRegistryTester(registry imagestream.Registry, apiTesters map[string]*ApiTester) *imageStreamRegistryTester {
	tester := &imageStreamRegistryTester{}

	tester.Registry = registry
	tester.registryApiTesters = apiTesters

	return tester
}

// Takes a response variable name that will be cast to the proper response type
type ApiResponse struct {
	response map[string]interface{}
}

func NewApiResponse() ApiResponse {
	response := ApiResponse{}
	response.response = make(map[string]interface{})
	return response
}

type ApiTester struct {
	callResponses map[int]ApiResponse
	callCount     int
}

func NewApiTester() *ApiTester {
	tester := &ApiTester{}
	tester.callCount = 0
	tester.callResponses = make(map[int]ApiResponse)

	return tester
}

func (a *ApiTester) callComplete() {
	a.callCount++
}

func (r *imageStreamRegistryTester) ListImageStreams(ctx context.Context, options *metainternal.ListOptions) (*imageapi.ImageStreamList, error) {
	return r.Registry.ListImageStreams(ctx, options)
}

func (r *imageStreamRegistryTester) GetImageStream(ctx context.Context, id string, options *metav1.GetOptions) (*imageapi.ImageStream, error) {
	return r.Registry.GetImageStream(ctx, id, options)
}

func (r *imageStreamRegistryTester) CreateImageStream(ctx context.Context, repo *imageapi.ImageStream, options *metav1.CreateOptions) (*imageapi.ImageStream, error) {

	hasApiResponse, iStream, err := r.extractImageStreamResponse("CreateImageStream")

	if hasApiResponse {
		return iStream, err
	}

	return r.Registry.CreateImageStream(ctx, repo, options)
}

func (r *imageStreamRegistryTester) UpdateImageStream(ctx context.Context, repo *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error) {

	hasApiResponse, iStream, err := r.extractImageStreamResponse("UpdateImageStream")

	if hasApiResponse {
		return iStream, err
	}

	return r.Registry.UpdateImageStream(ctx, repo, forceAllowCreate, options)
}

func (r *imageStreamRegistryTester) UpdateImageStreamSpec(ctx context.Context, repo *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error) {
	return r.Registry.UpdateImageStreamSpec(ctx, repo, forceAllowCreate, options)
}

func (r *imageStreamRegistryTester) UpdateImageStreamStatus(ctx context.Context, repo *imageapi.ImageStream, forceAllowCreate bool, options *metav1.UpdateOptions) (*imageapi.ImageStream, error) {
	return r.Registry.UpdateImageStreamStatus(ctx, repo, forceAllowCreate, options)
}

func (r *imageStreamRegistryTester) DeleteImageStream(ctx context.Context, id string) (*metav1.Status, error) {
	return r.Registry.DeleteImageStream(ctx, id)
}

func (r *imageStreamRegistryTester) WatchImageStreams(ctx context.Context, options *metainternal.ListOptions) (watch.Interface, error) {
	return r.Registry.WatchImageStreams(ctx, options)
}

func (r *imageStreamRegistryTester) extractImageStreamResponse(apiName string) (bool, *imageapi.ImageStream, error) {

	if apiTester, ok := r.registryApiTesters[apiName]; ok {

		// increment the call count when done
		defer apiTester.callComplete()

		if responses, okResponses := apiTester.callResponses[apiTester.callCount]; okResponses {

			var err error = nil

			if responseErr, okResponse := responses.response["error"]; okResponse {
				err = responseErr.(error)
			}

			if responseImageStream, okImageStream := responses.response["imageStream"]; okImageStream {
				imageStream := responseImageStream.(imageapi.ImageStream)
				return true, &imageStream, err
			} else {
				return true, nil, err
			}
		}

	}

	return false, nil, nil

}
