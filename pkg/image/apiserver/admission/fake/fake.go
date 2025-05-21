package fake

import (
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

type ImageStreamLimitVerifier struct {
	ImageStreamEvaluator func(ns string, oldStream, newStream *imageapi.ImageStream) error
	Err                  error
}

func (f *ImageStreamLimitVerifier) VerifyLimits(ns string, oldStream, newStream *imageapi.ImageStream) error {
	if f.ImageStreamEvaluator != nil {
		return f.ImageStreamEvaluator(ns, oldStream, newStream)
	}
	return f.Err
}
