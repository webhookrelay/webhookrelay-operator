FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY .test/fake-api/main.go ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /fake-relay-api ./main.go

FROM cgr.dev/chainguard/static:latest
COPY --from=build /fake-relay-api /fake-relay-api
USER 65532:65532
ENTRYPOINT ["/fake-relay-api"]
