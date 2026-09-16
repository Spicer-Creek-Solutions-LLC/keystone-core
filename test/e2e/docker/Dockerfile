# The packaged binary, built the way the Makefile builds it -- ARCH-COMM-003
# requires production paths, and a container that compiled differently would be
# testing something else.
ARG BIN
FROM golang:1.27-alpine AS build
ARG BIN
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/${BIN}

FROM alpine:3
COPY --from=build /out/app /usr/local/bin/app
ENTRYPOINT ["/usr/local/bin/app"]
