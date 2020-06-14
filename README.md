# Webhook Relay Kubernetes Operator

Webhook Relay Operator provides an easy way to receive webhooks to an internal Kubernetes cluster without configuring public IP or load balancer. Perfect for:
- On-premise deployments 
- Cloud deployments where public load balancer is not required (single endpoint receiving webhooks and no need to expose the whole server)
- Edge deployments
- IoT with https://k3s.io/

## Features

Current operator project scope:

- [x] Deploy webhook forwarding agents with configured buckets
- [x] Read credentials from secrets and mount secrets to webhookrelayd containers
- [ ] Provision separate access tokens for webhookrelayd containers with disabled API access (only subscribe capability)
- [ ] Ensure buckets are created 
- [ ] Ensure inputs are configured (public endpoints)
- [ ] Ensure outputs are configured (forwarding destinations)
- [ ] K8s events on taken actions