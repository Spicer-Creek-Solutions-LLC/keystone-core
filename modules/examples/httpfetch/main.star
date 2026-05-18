# keystone/httpfetch — scoped outbound HTTP.
#
# The manifest's `domains` allowlist + `max_response_size` + timeout
# are enforced by the runtime. `shape()` is pure so it is unit
# testable without a network host; the live `http.get` path is shown
# in the Go example test with an injected HTTP host (record/replay
# test hosts are a post-v1.0 item — see the project ROADMAP).

def shape(resp):
    return {"status": resp["status"], "size": len(resp["body"])}

def fetch(url):
    return shape(http.get(url))

def main(input):
    return fetch(input["url"])
