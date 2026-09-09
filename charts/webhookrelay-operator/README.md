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

The CRD in the chart's `crds/` directory is installed automatically by Helm 3.
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
