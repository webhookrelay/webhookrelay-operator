package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebhookRelayForwardSpec defines the desired state of WebhookRelayForward
type WebhookRelayForwardSpec struct {
	// SecretRefName is the name of the secret object that contains
	// generated token from https://my.webhookrelay.com/tokens
	// secret should have two fields:
	// key    - your token key (a long UUID)
	// secret - token secret, encrypted once generated and cannot be recovered from Webhook Relay.
	// If secret is lost, just create a new token
	SecretRefName string `json:"secretRefName,omitempty"`

	// SecretRefNamespace is the namespace of the secret reference.
	SecretRefNamespace string `json:"secretRefNamespace,omitempty"`

	// Image is webhookrelayd container, defaults to webhookrelay/webhookrelayd:latest
	Image string `json:"image,omitempty"`

	// Buckets to manage and subscribe to. Each CR can control one or more buckets. Buckets can be inspected
	// and manually created via Web UI here https://my.webhookrelay.com/buckets
	Buckets []BucketSpec `json:"buckets"`

	// Resources is to set the resource requirements of the Webhook Relay agent container`.
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// BucketSpec defines a bucket that groups one or more inputs (public endpoints) and
// one ore more outputs (where the webhooks should be routed)
type BucketSpec struct {
	// Name is the name of a bucket that can be reused
	// (if it already exists) or that will be created by the operator. Buckets
	// act as a grouping mechanism for Inputs and Outputs
	Name string `json:"name,omitempty"`

	Description string `json:"description,omitempty"`

	// Inputs are your public endpoints. Inputs can either be https://my.webhookrelay.com/v1/webhooks/[unique ID]
	// format or custom subdomains under https://[subdomain].hooks.webhookrelay.com or
	// completely custom domains such as https://hooks.example.com.
	// Important! Note that if you specify inputs, operator will automatically synchronize inputs of the specified
	// bucket with the provided CR spec.
	Inputs []InputSpec `json:"inputs,omitempty"`

	// Outputs are destinations where webhooks/API requests should be forwarded.
	Outputs []OutputSpec `json:"outputs,omitempty"`
}

// InputSpec defines an input that belong to a bucket
type InputSpec struct {
	Name string `json:"name,omitempty"`

	// FunctionID attaches function to this input. Functions on inputs can modify
	// responses to the caller and modify requests that are then passed to each
	// output.
	FunctionID string `json:"functionId,omitempty"`

	// Static response configuration
	ResponseHeaders    map[string][]string `json:"responseHeaders,omitempty"`
	ResponseStatusCode int                 `json:"responseStatusCode,omitempty"`
	ResponseBody       string              `json:"responseBody,omitempty"`

	// Dynamic response configuration
	// either output name, ID or "anyOutput" to indicate that the first response
	// from any output is good enough. Defaults to empty string
	ResponseFromOutput string `json:"responseFromOutput,omitempty"`

	// CustomDomain can be used to assign a permanent domain name for your input
	// such as example.hooks.webhookrelay.com
	CustomDomain string `json:"customDomain,omitempty"`
	// PathPrefix can be combined together with CustomDomain to create 'API like'
	// functionality where calls from:
	// petshop.com/dogs -> are forwarded to [dogs store]
	// petshop.com/cats -> are forwarded to [cats store]
	PathPrefix string `json:"pathPrefix,omitempty"`

	// Description can be any string
	Description string `json:"description,omitempty"`
}

// OutputSpec defines and output that belong to a bucket. Outputs are destinations
// where webhooks/API requests are forwarded.
type OutputSpec struct {
	Name string `json:"name,omitempty"`

	// FunctionID attaches function to this output. Functions on output can modify
	// requests that are then passed to destinations.
	FunctionID string `json:"function_id,omitempty"`

	// OverrideHeaders
	OverrideHeaders map[string]string `json:"overrideHeaders,omitempty"`

	// Destination is a URL that specifies where to send the webhooks. For example it can be
	// http://local-jenkins/ghpr for Jenkins webhooks or any other URL.
	Destination string `json:"destination"`

	// Internal specifies whether webhook should be sent to an internal destination. Since
	// operator is working with internal agents, this option defaults to True
	Internal *bool `json:"internal,omitempty"`

	// Timeout specifies how long agent should wait for the response
	Timeout int `json:"timeout,omitempty"`

	// Description can be any string
	Description string `json:"description,omitempty"`
}

// AgentStatus is the phase of the Webhook Relay forwarder node at a given point in time.
type AgentStatus string

// Constants for operator defaults values and different phases.
const (
	AgentStatusInitial     AgentStatus = ""
	AgentStatusRunning     AgentStatus = "Running"
	AgentStatusCreating    AgentStatus = "Creating"
	AgentStatusTerminating AgentStatus = "Terminating" // TODO: needs finalizer
)

// RoutingStatus is configuration status
type RoutingStatus string

// Constants for operator routing configuration status
const (
	RoutingStatusConfigured RoutingStatus = "Configured"
	RoutingStatusFailed     RoutingStatus = "Failed"
)

// WebhookRelayForwardStatus defines the observed state of WebhookRelayForward
// +k8s:openapi-gen=true
type WebhookRelayForwardStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// AgentStatus indicates agent deployment status
	AgentStatus AgentStatus `json:"agentStatus,omitempty"`
	// Ready indicates whether agent is deployed
	Ready bool `json:"ready,omitempty"`

	RoutingStatus RoutingStatus `json:"routingStatus,omitempty"`
	Message       string        `json:"message,omitempty"`
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
