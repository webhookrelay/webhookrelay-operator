package webhookrelayforward

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/webhookrelay/webhookrelay-go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

const (
	testBucketAuthName    = "bucket-auth"
	testBucketAuthKey     = "password"
	testBucketAuthValue   = "test-value"
	testBucketAuthUser    = "relay-user"
	testBucketDescription = "managed"
)

func TestBucketAuthFromSpecReadsBasicPasswordFromSameNamespace(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testBucketAuthName, Namespace: testForwardNamespace},
		Data:       map[string][]byte{testBucketAuthKey: []byte(testBucketAuthValue)},
	}
	reconciler := &ReconcileWebhookRelayForward{client: fake.NewFakeClient(secret)}

	auth, err := reconciler.bucketAuthFromSpec(context.Background(), testForwardNamespace, &forwardv1.BucketAuthSpec{
		Type: bucketAuthTypeBasic, Username: testBucketAuthUser,
		SecretKeyRef: &forwardv1.SecretKeyRef{
			Name: testBucketAuthName,
			Key:  testBucketAuthKey,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, webhookrelay.AuthTypeBasic, auth.Type)
	assert.Equal(t, testBucketAuthUser, auth.Username)
	assert.Equal(t, testBucketAuthValue, auth.Password)
	assert.Empty(t, auth.Token)
}

func TestBucketAuthFromSpecValidatesBeforeSecretRead(t *testing.T) {
	reconciler := &ReconcileWebhookRelayForward{}

	_, err := reconciler.bucketAuthFromSpec(context.Background(), testForwardNamespace, &forwardv1.BucketAuthSpec{Type: bucketAuthTypeBasic})

	require.ErrorContains(t, err, "requires username")
}

func TestPatchBucketFromSpecAppliesOnlyDeclaredControls(t *testing.T) {
	stream := true
	largeWebhooks := true
	current := &webhookrelay.Bucket{
		ID: "bucket-id", Description: "old", Ephemeral: true,
		Auth: webhookrelay.BucketAuth{Type: webhookrelay.AuthTypeNone},
	}
	desiredAuth := &webhookrelay.BucketAuth{Type: webhookrelay.AuthTypeToken, Token: testBucketAuthValue}

	patched := patchBucketFromSpec(current, &forwardv1.BucketSpec{
		Description: "new", Stream: &stream, LargeWebhooks: &largeWebhooks,
	}, desiredAuth)

	assert.Equal(t, "new", patched.Description)
	assert.True(t, patched.Stream)
	assert.True(t, patched.LargeWebhooks)
	assert.True(t, patched.Ephemeral, "omitted controls must preserve remote state")
	assert.Equal(t, webhookrelay.AuthTypeToken, patched.Auth.Type)
	assert.Equal(t, testBucketAuthValue, patched.Auth.Token)
}

func TestBucketEqualIgnoresAuthenticationMetadata(t *testing.T) {
	desiredAuth := &webhookrelay.BucketAuth{
		Type: webhookrelay.AuthTypeBasic, Username: testBucketAuthUser, Password: testBucketAuthValue,
	}
	current := &webhookrelay.Bucket{
		Description: testBucketDescription,
		Auth: webhookrelay.BucketAuth{
			ID: "auth-id", CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Type: webhookrelay.AuthTypeBasic, Username: testBucketAuthUser, Password: testBucketAuthValue,
		},
	}

	assert.True(t, bucketEqual(&forwardv1.BucketSpec{Description: testBucketDescription}, current, desiredAuth))
}
