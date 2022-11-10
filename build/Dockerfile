FROM golang:1.19 as build
COPY . /go/src/github.com/webhookrelay/webhookrelay-operator
WORKDIR /go/src/github.com/webhookrelay/webhookrelay-operator
RUN make build

FROM alpine:latest

LABEL name="Webhook Relay Operator" \
    maintainer="info@webhookrelay.com" \
    vendor="Webhook Relay" \    
    io.k8s.description="The Webhook Relay Operator installs and manages forwarding nodes within a cluster." \
    io.k8s.display-name="Webhook Relay Operator" \
    io.openshift.tags="webhookrelay,webhooks,operator,networking,tunnelling,cicd,ci,cd" \
    summary="Receive, transform webhooks and expose web services to the internet without having public IP or configuring NAT/firewall in a secure way." \
    description="Webhook Relay allows anyone to receive, transform webhooks and expose web services to the internet without having public IP or configuring NAT/firewall in a secure way. Perfect for edge cluster, asynchronous webhook processing."


ENV OPERATOR=/usr/local/bin/webhookrelay-operator 

# copy binaries and user setup scripts
COPY --from=build /go/src/github.com/webhookrelay/webhookrelay-operator/build/_output/bin/webhookrelay-operator ${OPERATOR}

ENTRYPOINT ["usr/local/bin/webhookrelay-operator"]

