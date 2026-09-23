# C04-A's host: the production keystone-server, built the way the e2e image
# builds it, and the contract's probe beside it. The server runs as root, as
# ADR-0009 § 1 requires of the party that owns /run/keystone.
#
# A container's accounts are PR feedback, not acceptance (ADR-0010 § 1). The
# release gate for everything this image exercises is C13's VM harness.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY test/contract/operator/probe ./test/contract/operator/probe
COPY test/contract/operator/sockprobe ./test/contract/operator/sockprobe
RUN CGO_ENABLED=0 go build -o /out/keystone-server ./cmd/keystone-server \
 && CGO_ENABLED=0 go build -o /out/operator-probe ./test/contract/operator/probe

FROM alpine:3
COPY --from=build /out/ /usr/local/bin/
ENTRYPOINT ["tail", "-f", "/dev/null"]
