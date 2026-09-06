# syntax=docker/dockerfile:1.7
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY webui ./webui
ARG VERSION=dev
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/fleetd ./cmd/fleetd && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/zyvorai/zyvor-fleet/internal/agent.Version=${VERSION}" -o /out/fleet-agent ./cmd/fleet-agent && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/fleetctl ./cmd/fleetctl

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/fleetd /usr/local/bin/fleetd
COPY --from=build /out/fleet-agent /usr/local/bin/fleet-agent
COPY --from=build /out/fleetctl /usr/local/bin/fleetctl
USER 65532:65532
EXPOSE 8080
VOLUME ["/var/lib/zyvor-fleet"]
ENV ZYVOR_FLEET_DATA=/var/lib/zyvor-fleet/state.json
ENTRYPOINT ["/usr/local/bin/fleetd"]
