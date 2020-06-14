FROM golang:1.14.2 as build
COPY . /go/src/github.com/webhookrelay/webhookrelay-operator
WORKDIR /go/src/github.com/webhookrelay/webhookrelay-operator
RUN make build

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

LABEL name="Webhook Relay Operator" \
    maintainer="info@webhookrelay.com" \
    vendor="Webhook Relay" \
    version="v1.0.0" \
    release="1" \
    distribution-scope="public" \
    architecture="x86_64" \
    url="https://webhookrelay.com/v1/guide/" \
    io.k8s.description="The Webhook Relay Operator installs and manages forwarding nodes within a cluster." \
    io.k8s.display-name="Webhook Relay Operator" \
    io.openshift.tags="webhookrelay,webhooks,operator,networking,tunnelling,cicd,ci,cd" \
    summary="Receive, transform webhooks and expose web services to the internet without having public IP or configuring NAT/firewall in a secure way." \
    description="Webhook Relay allows anyone to receive, transform webhooks and expose web services to the internet without having public IP or configuring NAT/firewall in a secure way. Perfect for edge cluster, asynchronous webhook processing."


ENV OPERATOR=/usr/local/bin/webhookrelay-operator \
    USER_UID=1001 \
    USER_NAME=webhookrelay-operator

RUN mkdir -p /licenses
COPY --from=build /go/src/github.com/webhookrelay/webhookrelay-operator/LICENSE /licenses/

# copy binaries and user setup scripts
COPY --from=build /go/src/github.com/webhookrelay/webhookrelay-operator/build/_output/bin/webhookrelay-operator ${OPERATOR}
COPY --from=build /go/src/github.com/webhookrelay/webhookrelay-operator/build/bin /usr/local/bin

RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
