package webhookrelayforward

import (
	"sort"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

func (r *ReconcileWebhookRelayForward) shouldUpdatePublicEndpoints(instance *forwardv1.WebhookRelayForward) (*forwardv1.WebhookRelayForward, bool) {

	if len(instance.Spec.Buckets) == 0 && len(instance.Status.PublicEndpoints) == 0 {
		// nothing to do
		return nil, false
	}

	// if status is set but we don't have any buckets anymore, need to remove it
	if len(instance.Spec.Buckets) == 0 && len(instance.Status.PublicEndpoints) > 0 {
		patch := instance.DeepCopy()
		patch.Status.PublicEndpoints = []string{}
		return patch, true
	}

	desiredEndpoints := computePublicEndpoints(instance, r.apiClient.bucketsCache)

	sort.Strings(desiredEndpoints)
	sort.Strings(instance.Status.PublicEndpoints)

	if !sliceEquals(desiredEndpoints, instance.Status.PublicEndpoints) {

		patch := instance.DeepCopy()
		patch.Status.PublicEndpoints = desiredEndpoints
		return patch, true
	}

	return nil, false
}

func computePublicEndpoints(instance *forwardv1.WebhookRelayForward, bucketsCache *bucketsCache) []string {

	var (
		endpoints []string
		endpoint  string
		found     bool
	)

	for bIdx := range instance.Spec.Buckets {
		for idx := range instance.Spec.Buckets[bIdx].Inputs {
			endpoint, found = getInputPublicEndpoint(
				instance.Spec.Buckets[bIdx].Name,
				instance.Spec.Buckets[bIdx].Inputs[idx].Name,
				bucketsCache,
			)
			if found {
				endpoints = append(endpoints, endpoint)
			}
		}
	}

	return endpoints
}

func getInputPublicEndpoint(bucketName, inputName string, bucketsCache *bucketsCache) (string, bool) {
	b, ok := bucketsCache.Get(bucketName)
	if !ok {
		return "", false
	}
	for idx := range b.Inputs {
		if b.Inputs[idx].Name == inputName {
			return b.Inputs[idx].EndpointURL(), true
		}
	}

	return "", false
}

func sliceEquals(l, r []string) bool {
	// If one is nil, the other must also be nil.
	if (l == nil) != (r == nil) {
		return false
	}
	if len(l) != len(r) {
		return false
	}

	for i := range l {
		if l[i] != r[i] {
			return false
		}
	}
	return true
}
