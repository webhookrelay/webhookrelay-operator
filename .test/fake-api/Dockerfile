FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY .test/fake-api/main.go ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /fake-relay-api ./main.go

FROM cgr.dev/chainguard/static:latest@sha256:f51c2493951313c3ad4069080b2814ffb6ed6fe3909dabeb84a9482f42d5600b
COPY --from=build /fake-relay-api /fake-relay-api
USER 65532:65532
ENTRYPOINT ["/fake-relay-api"]
