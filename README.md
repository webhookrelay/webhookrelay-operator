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
- [ ] Provision separate access tokens for webhookrelayd containers with disabled API access (only subscribe capability)
- [x] Ensure buckets are created 
- [x] Ensure inputs are configured (public endpoints)
- [x] Ensure outputs are configured (forwarding destinations)
- [x] K8s events on taken actions
- [x] Updates CR status 