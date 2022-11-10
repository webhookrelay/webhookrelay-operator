<p align="center">
  <a href="https://webhookrelay.com" target="_blank"><img width="900"src="https://github.com/webhookrelay/webhookrelay-operator/blob/master/static/operator.png?raw=true"></a>
</p>

# Webhook Relay Kubernetes Operator

[![Build Status](https://drone-kr.webrelay.io/api/badges/webhookrelay/webhookrelay-operator/status.svg)](https://drone-kr.webrelay.io/webhookrelay/webhookrelay-operator)

Webhook Relay Operator provides an easy way to receive webhooks to an internal Kubernetes cluster without configuring public IP or load balancer. Perfect for:
- On-premise deployments 
- Cloud deployments where public load balancer is not required (single endpoint receiving webhooks and no need to expose the whole server)
- Edge deployments
- IoT & Edge computing with https://k3s.io/

Operator can manage buckets, configure your public endpoints that accept webhooks/API requests and sets up forwarding destinations (where HTTP requests will be sent).

## Features

Current operator project scope:

- [x] Deploy webhook forwarding agents with configured buckets
- [x] Read credentials from secrets and mount secrets to webhookrelayd containers
- [x] Ensure buckets are created 
- [x] Ensure inputs are configured (public endpoints)
- [x] Ensure outputs are configured (forwarding destinations)
- [x] K8s events on taken actions
- [x] Updates CR status

### Roadmap

- [ ] Create & manage [Functions](https://webhookrelay.com/v1/guide/functions.html) that transform webhook requests and responses
- [ ] Manage Function configuration through Kubernetes secrets
- [ ] Provision separate access tokens for webhookrelayd containers with disabled API access (only subscribe capability). CR should have a finalizer that would ensure that the secret is removed together with the agent configuration.
- [ ] Deploy Webhook Relay ingress controller (separate CRD)
- [ ] Expose webhookrelayd agent forwarding metrics
- [ ] Configure [notification integrations](https://webhookrelay.com/v1/guide/integrations.html) via CRDs

## Installation

Prerequisites:

* [Helm](https://docs.helm.sh/using_helm/#installing-helm)
* [Webhook Relay account](https://my.webhookrelay.com)
* Kubernetes

You need to add this Chart repo to Helm:

```bash
helm repo add webhookrelay https://charts.webhookrelay.com
helm repo update
```

Get access token from [here](https://my.webhookrelay.com/tokens). Once you click on 'Create Token', it will generate it and show a helper to set environment variables:

```
export RELAY_KEY=*****-****-****-****-*********
export RELAY_SECRET=**********
```

Install through Helm:

```bash
helm upgrade --install webhookrelay-operator --namespace=default webhookrelay/webhookrelay-operator \
  --set credentials.key=$RELAY_KEY --set credentials.secret=$RELAY_SECRET
```

## Usage

Operator works as a manager to configure your public endpoints and forwarding destinations. To start receiving webhooks you will need to create a [Custom Resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) (usually called just 'CR'). It's a short yaml file that describes your public endpoint characteristics and specifies where to forward the webhooks:

```yaml
# cr.yaml
apiVersion: forward.webhookrelay.com/v1
kind: WebhookRelayForward
metadata:
  name: example-forward
spec:
  buckets:
  - name: k8s-operator
    inputs:
    - name: public-endpoint
      description: "Public endpoint, supply this to the webhook producer"
      responseBody: "OK"
      responseStatusCode: 200
    outputs:
    - name: webhook-receiver
      lockPath: true  # set to 'false' to reuse any extra path WHR received
      disabled: false # set to 'true' to disable output
      destination: http://destination:5050/webhooks
```

```shell
kubectl apply -f cr.yaml
```

Now, to view CR status which will display our public endpoints:

```shell
# get available CRs
$ kubectl get webhookrelayforwards.forward.webhookrelay.com
# get our example forward status
$ kubectl describe webhookrelayforwards.forward.webhookrelay.com example-forward
Name:         example-forward
Namespace:    default
Labels:       <none>
Annotations:  API Version:  forward.webhookrelay.com/v1
Kind:         WebhookRelayForward
Metadata:
  Creation Timestamp:  2020-06-18T23:05:33Z
  Generation:          1
  Resource Version:    118902
  Self Link:           /apis/forward.webhookrelay.com/v1/namespaces/default/webhookrelayforwards/example-forward
  UID:                 998b0fca-f975-40dd-b2b5-91abd1edaee0
Spec:
  Buckets:
    Inputs:
      Description:           Public endpoint, supply this to the webhook producer
      Name:                  public-endpoint
      Response Body:         OK
      Response Status Code:  200
    Name:                    k8s-operator
    Outputs:
      Destination:       http://destination:5050/webhooks
      Name:              webhook-receiver
  Secret Ref Name:       whr-credentials
  Secret Ref Namespace:  
Status:
  Agent Status:  Running
  Public Endpoints:
    https://my.webhookrelay.com/v1/webhooks/92582560-738a-4eae-94b1-23299ed20b3c
  Ready:           true
  Routing Status:  Configured
Events:            <none>
```

Here we can see our public endpoints.

## Advanced Usage (multi-tenant, credentials per CR)

If more than one user is using the operator, it's possible to skip credentials setting during Helm install and just specify the [access token key & secret](https://my.webhookrelay.com/tokens) in the CR itself:

```yaml
# access_token.yaml
apiVersion: v1
kind: Secret
metadata:
  name: whr-credentials
type: Opaque
stringData:
  key: XXX    # your access token key
  secret: YYY # your access token secret
```

Create it:

```shell
kubectl apply -f access_token.yaml
```

Specify the secret ref in the CR as `secretRefName` and `secretRefNamespace` (this one is optional):

```yaml
# cr.yaml
apiVersion: forward.webhookrelay.com/v1
kind: WebhookRelayForward
metadata:
  name: example-forward
spec:
  secretRefName: whr-credentials # Secret 
  secretRefNamespace: ""
  buckets:
  - name: k8s-operator
    inputs:
    - name: public-endpoint
      description: "Public endpoint, supply this to the webhook producer"
      responseBody: "OK"
      responseStatusCode: 200
    outputs:
    - name: webhook-receiver
      lockPath: true  # set to 'false' to reuse any extra path WHR received
      disabled: false # set to 'true' to disable output
      destination: http://destination:5050/webhooks
  # Use custom Docker image
  #image: "quay.io/your-custom/image:latest"
  # Add custom env variables to the agent container
  extraEnvVars:
  - name: WEBSOCKET_TRANSPORT
    value: "true"
```

Create the CR:

```
kubectl apply -f cr.yaml
```

## HTTP Proxy settings

If your outgoing connections are intercepted by an HTTP/HTTPS proxy - you will need to supply connection details with `--set httpProxy` or `--set httpsProxy` Helm values:

```bash
helm upgrade --install webhookrelay-operator --namespace=default webhookrelay/webhookrelay-operator \
  --set credentials.key=$RELAY_KEY --set credentials.secret=$RELAY_SECRET \
  --set httpsProxy="https://example-proxy.com"
```

This will set environment variables for the operator and operator will propagate them to the deployed agent.