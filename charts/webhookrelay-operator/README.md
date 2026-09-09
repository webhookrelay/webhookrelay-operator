# Webhook Relay Operator Helm chart

This chart installs the namespaced Webhook Relay Kubernetes operator. The
operator reconciles `WebhookRelayForward` resources in its installation
namespace. See the [repository README](../../README.md) for secure credential
setup, complete CRD examples, delivery controls, migration, and troubleshooting.

## Install

```bash
helm repo add webhookrelay https://charts.webhookrelay.com
helm repo update
helm upgrade --install webhookrelay-operator webhookrelay/webhookrelay-operator \
  --namespace webhookrelay --create-namespace
```

The CRD in the chart's `crds/` directory is installed automatically on the
first Helm install. Helm deliberately does not upgrade or delete CRDs. Before
upgrading the operator, apply the CRD from the target chart or release checkout:

```bash
kubectl apply -f charts/webhookrelay-operator/crds/crd.yaml
helm upgrade webhookrelay-operator webhookrelay/webhookrelay-operator \
  --namespace webhookrelay
```

Uninstalling the chart leaves the CRD and all `WebhookRelayForward` objects in
place. Delete those objects before uninstalling if their owned relay-agent
Deployments should be garbage-collected. Delete the CRD separately only when
you intend to delete every `WebhookRelayForward` object cluster-wide.

A Helm rollback rolls back the operator resources, not the CRD. Before rolling
back across operator generations, ensure the existing custom resources use
fields understood by the older controller, scale the current operator to zero,
wait for its pods to terminate, and then run `helm rollback`. The newer CRD
remains installed.

Create the Relay access-token Secret and each `WebhookRelayForward` in the same
namespace. Per-resource `secretRefNamespace` is deprecated and cannot grant
cross-namespace access.

For compatibility, the chart can create one operator-wide credential Secret:

```bash
helm upgrade --install webhookrelay-operator webhookrelay/webhookrelay-operator \
  --namespace webhookrelay --create-namespace \
  --set-string credentials.key="$RELAY_KEY" \
  --set-string credentials.secret="$RELAY_SECRET"
```

Prefer the namespaced Secret workflow in the repository README because command
line values can be retained in shell history and Helm release data.

## Values

| Parameter | Description | Default |
| --- | --- | --- |
| `replicaCount` | Operator replicas | `1` |
| `image.repository` | Operator image repository | `webhookrelay/webhookrelay-operator` |
| `image.tag` | Operator image tag | `0.7.0` |
| `image.pullPolicy` | Operator image pull policy | `Always` |
| `credentials.key` | Optional operator-wide Relay token key | empty |
| `credentials.secret` | Optional operator-wide Relay token secret | empty |
| `httpsProxy` | HTTPS proxy used by the operator and propagated to agents | empty |
| `apiEndpointURL` | Trusted operator-wide Relay API base URL | `https://my.webhookrelay.com/v1` |
| `imagePullSecrets` | Image pull Secret references | `[]` |
| `serviceAccount.create` | Create the ServiceAccount | `true` |
| `serviceAccount.name` | ServiceAccount name | `webhookrelay-operator` |
| `rbac.create` | Create namespaced Role and RoleBinding | `true` |
| `resources` | Operator container requests and limits | see `values.yaml` |
| `podAnnotations` | Operator pod annotations | metrics annotations |
| `podSecurityContext` | Operator pod security context | `{}` |
| `securityContext` | Operator container security context | `{}` |
| `nodeSelector` | Operator pod node selector | `{}` |
| `tolerations` | Operator pod tolerations | `[]` |
| `affinity` | Operator pod affinity | `{}` |

`spec.image`, `spec.resources`, and `spec.websocketTransport` belong to the
`WebhookRelayForward` CR and configure the generated relay-agent Deployment;
they are not Helm values for the operator itself.
