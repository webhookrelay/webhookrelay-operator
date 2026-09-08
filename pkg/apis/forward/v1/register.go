// NOTE: Boilerplate only.  Ignore this file.

// Package v1 contains API Schema definitions for the forward v1 API group
// +k8s:deepcopy-gen=package,register
// +groupName=forward.webhookrelay.com
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: "forward.webhookrelay.com", Version: "v1"}

	// SchemeBuilder is used to add Go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds this API group to a runtime scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &WebhookRelayForward{}, &WebhookRelayForwardList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
