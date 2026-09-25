# C05's hosts: the production keystone-server, keystone CLI and keystone-agent,
# built the way the e2e image builds them, and the contract's probe beside them.
# One image serves both roles; which binary runs is the container's.
#
# A container's accounts, modes and fsync are PR feedback, not acceptance
# (ADR-0010 § 1). C13's VM harness is the gate for those.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY test/contract/operator/sockprobe ./test/contract/operator/sockprobe
COPY test/contract/enrollment/probe ./test/contract/enrollment/probe
RUN CGO_ENABLED=0 go build -o /out/keystone-server ./cmd/keystone-server \
 && CGO_ENABLED=0 go build -o /out/keystone ./cmd/keystone \
 && CGO_ENABLED=0 go build -o /out/keystone-agent ./cmd/keystone-agent \
 && CGO_ENABLED=0 go build -o /out/enrollment-probe ./test/contract/enrollment/probe

FROM alpine:3
COPY --from=build /out/ /usr/local/bin/
ENTRYPOINT ["tail", "-f", "/dev/null"]
