package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WebhookRelayForwardSpec defines the desired state of WebhookRelayForward
type WebhookRelayForwardSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// WebhookRelayForwardStatus defines the observed state of WebhookRelayForward
type WebhookRelayForwardStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WebhookRelayForward is the Schema for the webhookrelayforwards API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=webhookrelayforwards,scope=Namespaced
type WebhookRelayForward struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookRelayForwardSpec   `json:"spec,omitempty"`
	Status WebhookRelayForwardStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WebhookRelayForwardList contains a list of WebhookRelayForward
type WebhookRelayForwardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebhookRelayForward `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebhookRelayForward{}, &WebhookRelayForwardList{})
}
