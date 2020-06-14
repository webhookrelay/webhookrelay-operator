package webhookrelayforward

import (
	"reflect"
	"strings"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// checkDeployment - checks whether deployment is equal, otherwise patches it
func (r *ReconcileWebhookRelayForward) checkDeployment(cr *forwardv1.WebhookRelayForward, current *appsv1.Deployment) (patched *appsv1.Deployment, equal bool) {
	// Assume deployment matches the spec
	equal = true
	// Creating a deep copy of the existing deployment
	patched = current.DeepCopy()
	// Getting a desired deployment and validating:
	// 1. Image
	// 2. Environment configuration (secrets, buckets)
	// 3. TODO: check resource limits
	desiredDeployment := r.newDeploymentForCR(cr)

	if len(current.Spec.Template.Spec.Containers) != len(desiredDeployment.Spec.Template.Spec.Containers) {
		equal = false
		patched.Spec.Template.Spec.Containers = desiredDeployment.Spec.Template.Spec.Containers
	}

	for i := range desiredDeployment.Spec.Template.Spec.Containers {

		if !containersEqual(&current.Spec.Template.Spec.Containers[i], &desiredDeployment.Spec.Template.Spec.Containers[i]) {
			equal = false
		}

	}

	// patching containers
	if !equal {
		patched.Spec.Template.Spec = desiredDeployment.Spec.Template.Spec
	}

	return
}

func containersEqual(r, l *corev1.Container) bool {
	if r.Image != l.Image {
		return false
	}
	if len(r.Env) != len(l.Env) {
		return false
	}

	for i := range r.Env {
		if r.Env[i].Name != l.Env[i].Name {
			return false
		}
		if r.Env[i].Value != l.Env[i].Value {
			return false
		}

		// envVarSourceEqual checking secret ref if set
		if !envVarSourceEqual(r.Env[i].ValueFrom, l.Env[i].ValueFrom) {
			return false
		}
	}

	return true
}

func envVarSourceEqual(current, desired *corev1.EnvVarSource) bool {
	if current == nil && desired == nil {
		// if not set, nothing to do
		return true
	}

	if current == nil || desired == nil {
		return false
	}

	if !reflect.DeepEqual(current, desired) {
		return false
	}

	// var (
	// 	desiredSecretRefName string
	// 	desiredSecretRefKey  string
	// )

	// if desired.SecretKeyRef != nil {
	// 	desiredSecretRefName = desired.SecretKeyRef.Name
	// 	desiredSecretRefKey = desired.SecretKeyRef.Key
	// }

	// if current.SecretKeyRef == nil {
	// 	return false
	// }

	// if current.SecretKeyRef.Name != desiredSecretRefName {
	// 	return false
	// }

	// if current.SecretKeyRef.Key != desiredSecretRefKey {
	// 	return false
	// }

	return true
}

// envForDeployment generates env configuration for the deployment based on the spec and credentials
func (r *ReconcileWebhookRelayForward) envForDeployment(cr *forwardv1.WebhookRelayForward) []corev1.EnvVar {
	var buckets []string
	for idx := range cr.Spec.Buckets {
		buckets = append(buckets, cr.Spec.Buckets[idx].Name)
	}

	env := []corev1.EnvVar{
		{
			Name:  containerBucketsEnvName,
			Value: strings.Join(buckets, ","),
		},
	}

	// configuring authentication for the container
	if cr.Spec.SecretRefName != "" {

		keyRefSelect := &corev1.SecretKeySelector{}
		keyRefSelect.Name = cr.Spec.SecretRefName
		keyRefSelect.Key = forwardv1.AccessTokenKeyName

		secretRefSelect := &corev1.SecretKeySelector{}
		secretRefSelect.Name = cr.Spec.SecretRefName
		secretRefSelect.Key = forwardv1.AccessTokenSecretName

		env = append(env,
			corev1.EnvVar{
				Name: containerTokenKeyEnvName,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: keyRefSelect,
				},
			},
			corev1.EnvVar{
				Name: containerTokenSecretEnvName,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: secretRefSelect,
				},
			},
		)
	} else {
		// setting the ones from the client that have likely come
		// from the environment variables set directly on the operator
		env = append(env,
			corev1.EnvVar{
				Name:  containerTokenKeyEnvName,
				Value: r.apiClient.accessTokenKey,
			},
			corev1.EnvVar{
				Name:  containerTokenSecretEnvName,
				Value: r.apiClient.accessTokenSecret,
			},
		)
	}

	return env
}

// newDeploymentForCR returns a new Webhook Relay forwarder deployment with the same name/namespace as the cr
func (r *ReconcileWebhookRelayForward) newDeploymentForCR(cr *forwardv1.WebhookRelayForward) *appsv1.Deployment {
	labels := map[string]string{
		"app": cr.Name,
	}
	podLabels := map[string]string{
		"name": "webhookrelay-forwarder",
	}

	image := cr.Spec.Image
	if image == "" {
		image = r.config.Image
	}

	env := r.envForDeployment(cr)

	podTemplateSpec := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "webhookrelayd",
					ImagePullPolicy: corev1.PullAlways,
					Image:           image,
					Env:             env,
				},
			},
		},
	}
	podTemplateSpec.Labels = podLabels
	podTemplateSpec.Name = "webhookrelay"
	// TODO: set namespace
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-whr-deployment",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: toInt32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "webhookrelay-forwarder",
				},
			},
			Template: podTemplateSpec,
		},
	}
}

func toInt32(val int32) *int32 {
	return &val
}
