# The packaged binaries, built the way the Makefile builds them -- ARCH-COMM-003
# requires production paths, and a container that compiled differently would be
# testing something else.
#
# All three are in every image and BIN chooses which one the container runs.
# The server's host needs the operator CLI beside it: ADR-0009's socket is
# local, so `keystone enroll create` runs where the server does.
ARG BIN
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/keystone-server ./cmd/keystone-server \
 && CGO_ENABLED=0 go build -o /out/keystone ./cmd/keystone \
 && CGO_ENABLED=0 go build -o /out/keystone-agent ./cmd/keystone-agent

FROM alpine:3
ARG BIN
COPY --from=build /out/ /usr/local/bin/
RUN ln -s /usr/local/bin/${BIN} /usr/local/bin/app
ENTRYPOINT ["/usr/local/bin/app"]
