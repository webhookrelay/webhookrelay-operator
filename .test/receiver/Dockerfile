FROM golang:1.27.1-alpine AS build

WORKDIR /src
COPY .test/receiver/main.go ./main.go
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /receiver ./main.go

FROM alpine:3.22
COPY --from=build /receiver /usr/local/bin/receiver
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/receiver"]
