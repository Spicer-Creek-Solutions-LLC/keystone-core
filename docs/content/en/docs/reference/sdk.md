---
title: "SDK Reference"
weight: 13
description: >
  Language SDKs for building Keystone Core modules
---

## Overview

Keystone Core provides language SDKs for writing modules that compile to WASM and run in the module runtime.

Available SDKs:
- Go (TinyGo)
- Rust
- C++

All SDKs ship with a `hello-world` example under `modules/sdk/*/examples/hello-world`.

## Go SDK

Path: `modules/sdk/go`

Key notes:
- Build with TinyGo to target `wasm32-wasi`.
- Implement the `Run` entry point and use the SDK host APIs.

Example:

```bash
cd modules/sdk/go/examples/hello-world
tinygo build -o hello-world-go.wasm -target wasm32-wasi -opt=z .
```

## Rust SDK

Path: `modules/sdk/rust`

Key notes:
- Build with `wasm32-wasi`.
- Use the SDK crate to access host functions and types.

Example:

```bash
cd modules/sdk/rust/examples/hello-world
cargo build --target wasm32-wasi --release
```

## C++ SDK

Path: `modules/sdk/cpp`

Key notes:
- Build with the WASI SDK toolchain.
- Include `kscore` headers from the SDK.

Example:

```bash
cd modules/sdk/cpp/examples/hello-world
cmake -B build -DCMAKE_TOOLCHAIN_FILE=$WASI_SDK_PATH/share/cmake/wasi-sdk.cmake .
cmake --build build
```

## Module Packaging

Each SDK example includes a `module.yaml` manifest. Use `kscorectl module build` and
`kscorectl module sign` for packaging and signing.

## Benchmarks

The Go SDK includes micro-benchmarks for core helper functions. Run them from
`modules/sdk/go`:

```bash
go test -bench .
```

Sample results (Linux x86_64, Go 1.25):

```
BenchmarkLogLevelString-8    1000000000    0.1946 ns/op
BenchmarkErrorString-8       16122675     75.79 ns/op
BenchmarkLogInfoStub-8       1000000000    0.1983 ns/op
```

## Developing Client SDKs

This section provides guidelines for developing Keystone Core API client SDKs in any programming language.

### Overview

Keystone Core exposes two APIs for programmatic access:

| Protocol | Port | Use Case |
|----------|------|----------|
| REST (HTTP/1.1) | 8080 | Simple integrations, curl, web apps |
| gRPC (HTTP/2) | 9090 | High-performance, streaming, type-safe |

Both APIs offer equivalent functionality. Choose based on your language ecosystem and requirements.

### SDK Architecture

A well-designed SDK should include these components:

```
sdk/
├── client.{ext}           # Main client class/module
├── auth/
│   ├── api_key.{ext}      # API key authentication
│   └── mtls.{ext}         # mTLS authentication
├── resources/
│   ├── agents.{ext}       # Agent operations
│   ├── jobs.{ext}         # Job operations
│   ├── state.{ext}        # State management
│   ├── events.{ext}       # Event operations
│   ├── policies.{ext}     # Policy operations
│   └── cluster.{ext}      # Cluster operations (HA)
├── models/
│   ├── agent.{ext}        # Agent data model
│   ├── job.{ext}          # Job data model
│   └── ...                # Other models
├── errors.{ext}           # Custom exceptions/errors
├── pagination.{ext}       # Pagination utilities
└── streaming.{ext}        # Event streaming support
```

### Authentication

Support both authentication methods:

#### API Key Authentication

```python
# Python example
class ApiKeyAuth:
    def __init__(self, api_key: str):
        self.api_key = api_key

    def apply(self, request):
        request.headers["Authorization"] = f"Bearer {self.api_key}"
        return request
```

```typescript
// TypeScript example
class ApiKeyAuth implements Auth {
  constructor(private apiKey: string) {}

  apply(headers: Headers): Headers {
    headers.set("Authorization", `Bearer ${this.apiKey}`);
    return headers;
  }
}
```

```java
// Java example
public class ApiKeyAuth implements Authenticator {
    private final String apiKey;

    public ApiKeyAuth(String apiKey) {
        this.apiKey = apiKey;
    }

    @Override
    public Request authenticate(Request request) {
        return request.newBuilder()
            .header("Authorization", "Bearer " + apiKey)
            .build();
    }
}
```

#### mTLS Authentication

```python
# Python example
import ssl

class MtlsAuth:
    def __init__(self, cert_file: str, key_file: str, ca_file: str):
        self.ssl_context = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
        self.ssl_context.load_cert_chain(cert_file, key_file)
        self.ssl_context.load_verify_locations(ca_file)

    def get_ssl_context(self):
        return self.ssl_context
```

```typescript
// TypeScript/Node.js example
import * as fs from "fs";
import * as https from "https";

class MtlsAuth implements Auth {
  private agent: https.Agent;

  constructor(certFile: string, keyFile: string, caFile: string) {
    this.agent = new https.Agent({
      cert: fs.readFileSync(certFile),
      key: fs.readFileSync(keyFile),
      ca: fs.readFileSync(caFile),
    });
  }

  getAgent(): https.Agent {
    return this.agent;
  }
}
```

### Error Handling

Map Keystone API errors to language-appropriate exceptions:

| HTTP Status | Error Code | SDK Exception |
|-------------|------------|---------------|
| 400 | `invalid_request` | `InvalidRequestError` |
| 401 | `unauthorized` | `AuthenticationError` |
| 403 | `forbidden` | `PermissionError` |
| 404 | `not_found` | `NotFoundError` |
| 409 | `conflict` | `ConflictError` |
| 429 | `rate_limit_exceeded` | `RateLimitError` |
| 500 | `internal_error` | `ServerError` |
| 503 | `service_unavailable` | `ServiceUnavailableError` |

**Python Implementation**:
```python
class KeystoneError(Exception):
    """Base exception for Keystone SDK"""
    def __init__(self, message: str, code: str = None, details: dict = None):
        super().__init__(message)
        self.code = code
        self.details = details or {}

class InvalidRequestError(KeystoneError):
    """Request validation failed"""
    pass

class AuthenticationError(KeystoneError):
    """Authentication failed"""
    pass

class NotFoundError(KeystoneError):
    """Resource not found"""
    pass

class RateLimitError(KeystoneError):
    """Rate limit exceeded"""
    def __init__(self, message: str, retry_after: int = None, **kwargs):
        super().__init__(message, **kwargs)
        self.retry_after = retry_after

def handle_response(response):
    if response.status_code >= 400:
        data = response.json()
        error_class = {
            400: InvalidRequestError,
            401: AuthenticationError,
            403: PermissionError,
            404: NotFoundError,
            409: ConflictError,
            429: RateLimitError,
            500: ServerError,
            503: ServiceUnavailableError,
        }.get(response.status_code, KeystoneError)

        raise error_class(
            message=data.get("message", "Unknown error"),
            code=data.get("error"),
            details=data.get("details", {})
        )
    return response
```

**TypeScript Implementation**:
```typescript
export class KeystoneError extends Error {
  constructor(
    message: string,
    public code?: string,
    public details?: Record<string, unknown>
  ) {
    super(message);
    this.name = "KeystoneError";
  }
}

export class RateLimitError extends KeystoneError {
  constructor(
    message: string,
    public retryAfter?: number,
    code?: string,
    details?: Record<string, unknown>
  ) {
    super(message, code, details);
    this.name = "RateLimitError";
  }
}

// Error factory
function createError(status: number, data: ErrorResponse): KeystoneError {
  const ErrorClass = {
    400: InvalidRequestError,
    401: AuthenticationError,
    403: PermissionError,
    404: NotFoundError,
    409: ConflictError,
    429: RateLimitError,
    500: ServerError,
    503: ServiceUnavailableError,
  }[status] || KeystoneError;

  return new ErrorClass(
    data.message || "Unknown error",
    data.error,
    data.details
  );
}
```

### Pagination

Implement automatic pagination handling:

```python
# Python - Iterator-based pagination
class PaginatedIterator:
    def __init__(self, client, endpoint: str, params: dict = None, limit: int = 100):
        self.client = client
        self.endpoint = endpoint
        self.params = params or {}
        self.limit = limit
        self.offset = 0
        self.total = None
        self._buffer = []

    def __iter__(self):
        return self

    def __next__(self):
        if not self._buffer:
            if self.total is not None and self.offset >= self.total:
                raise StopIteration

            response = self.client.get(
                self.endpoint,
                params={**self.params, "limit": self.limit, "offset": self.offset}
            )
            data = response.json()

            self.total = data.get("total", 0)
            self._buffer = data.get("items", [])
            self.offset += self.limit

            if not self._buffer:
                raise StopIteration

        return self._buffer.pop(0)

# Usage
for agent in client.agents.list(environment="production"):
    print(agent.id)
```

```typescript
// TypeScript - Async iterator pagination
async function* paginate<T>(
  client: Client,
  endpoint: string,
  params: Record<string, string> = {},
  limit: number = 100
): AsyncGenerator<T> {
  let offset = 0;
  let total: number | null = null;

  while (total === null || offset < total) {
    const response = await client.get<PaginatedResponse<T>>(endpoint, {
      ...params,
      limit: limit.toString(),
      offset: offset.toString(),
    });

    total = response.total;

    for (const item of response.items) {
      yield item;
    }

    offset += limit;
  }
}

// Usage
for await (const agent of client.agents.list({ environment: "production" })) {
  console.log(agent.id);
}
```

### Rate Limiting

Implement automatic retry with exponential backoff:

```python
import time
import random

class RateLimitHandler:
    def __init__(self, max_retries: int = 3, base_delay: float = 1.0):
        self.max_retries = max_retries
        self.base_delay = base_delay

    def execute(self, func, *args, **kwargs):
        last_exception = None

        for attempt in range(self.max_retries + 1):
            try:
                return func(*args, **kwargs)
            except RateLimitError as e:
                last_exception = e

                if attempt == self.max_retries:
                    raise

                # Use retry_after header if available
                delay = e.retry_after or self._calculate_delay(attempt)
                time.sleep(delay)

        raise last_exception

    def _calculate_delay(self, attempt: int) -> float:
        # Exponential backoff with jitter
        delay = self.base_delay * (2 ** attempt)
        jitter = random.uniform(0, 0.1 * delay)
        return min(delay + jitter, 60)  # Cap at 60 seconds
```

```typescript
async function withRetry<T>(
  fn: () => Promise<T>,
  maxRetries: number = 3,
  baseDelay: number = 1000
): Promise<T> {
  let lastError: Error | null = null;

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (error) {
      if (!(error instanceof RateLimitError)) {
        throw error;
      }

      lastError = error;

      if (attempt === maxRetries) {
        throw error;
      }

      const delay =
        error.retryAfter != null
          ? error.retryAfter * 1000
          : Math.min(baseDelay * Math.pow(2, attempt), 60000);

      await new Promise((resolve) => setTimeout(resolve, delay));
    }
  }

  throw lastError;
}
```

### Event Streaming

For real-time events, implement Server-Sent Events (SSE) or gRPC streaming:

**SSE Client (REST)**:
```python
import json
import requests

class EventStream:
    def __init__(self, client, filter: str = None):
        self.client = client
        self.filter = filter

    def __iter__(self):
        url = f"{self.client.base_url}/api/v1/events/stream"
        params = {"filter": self.filter} if self.filter else {}

        with requests.get(
            url,
            params=params,
            headers=self.client.auth.headers(),
            stream=True
        ) as response:
            for line in response.iter_lines():
                if line:
                    line = line.decode("utf-8")
                    if line.startswith("data: "):
                        data = json.loads(line[6:])
                        yield Event(**data)

# Usage
for event in client.events.stream(filter="type =~ 'agent.*'"):
    print(f"Event: {event.type} from {event.source}")
```

**gRPC Streaming**:
```python
import grpc

class EventStreamGrpc:
    def __init__(self, channel, filter: str = None):
        self.stub = event_service_pb2_grpc.EventServiceStub(channel)
        self.filter = filter

    def __iter__(self):
        request = event_service_pb2.SubscribeEventsRequest(
            filter=self.filter or ""
        )

        for event in self.stub.SubscribeEvents(request):
            yield Event.from_proto(event)
```

### gRPC Client Generation

Generate gRPC clients from Keystone proto files:

**Protocol Buffer Location**:
```
proto/
├── agent_service.proto
├── event_service.proto
├── execution_service.proto
├── state_service.proto
├── policy_service.proto
└── cluster_service.proto
```

**Generation Commands**:

```bash
# Python
python -m grpc_tools.protoc \
  -I./proto \
  --python_out=./sdk/python/kscore/proto \
  --grpc_python_out=./sdk/python/kscore/proto \
  proto/*.proto

# TypeScript/JavaScript
npx grpc_tools_node_protoc \
  --proto_path=./proto \
  --js_out=import_style=commonjs,binary:./sdk/js/src/proto \
  --grpc_out=grpc_js:./sdk/js/src/proto \
  --ts_out=grpc_js:./sdk/js/src/proto \
  proto/*.proto

# Java
protoc \
  -I./proto \
  --java_out=./sdk/java/src/main/java \
  --grpc-java_out=./sdk/java/src/main/java \
  proto/*.proto

# Ruby
grpc_tools_ruby_protoc \
  -I./proto \
  --ruby_out=./sdk/ruby/lib/kscore/proto \
  --grpc_out=./sdk/ruby/lib/kscore/proto \
  proto/*.proto

# C#
protoc \
  -I./proto \
  --csharp_out=./sdk/csharp/Kscore/Proto \
  --grpc_out=./sdk/csharp/Kscore/Proto \
  --plugin=protoc-gen-grpc=grpc_csharp_plugin \
  proto/*.proto
```

### Request/Response Patterns

**Standard Request Pattern**:
```python
class BaseResource:
    def __init__(self, client):
        self.client = client

    def _request(
        self,
        method: str,
        path: str,
        params: dict = None,
        data: dict = None,
        timeout: float = 30.0
    ):
        url = f"{self.client.base_url}{path}"

        response = self.client.session.request(
            method=method,
            url=url,
            params=params,
            json=data,
            timeout=timeout
        )

        handle_response(response)
        return response.json()

    def get(self, path: str, **kwargs):
        return self._request("GET", path, **kwargs)

    def post(self, path: str, **kwargs):
        return self._request("POST", path, **kwargs)

    def patch(self, path: str, **kwargs):
        return self._request("PATCH", path, **kwargs)

    def delete(self, path: str, **kwargs):
        return self._request("DELETE", path, **kwargs)
```

**Resource Implementation**:
```python
class AgentResource(BaseResource):
    def list(
        self,
        datacenter: str = None,
        environment: str = None,
        role: str = None,
        status: str = None
    ) -> Iterator[Agent]:
        params = {
            k: v for k, v in {
                "datacenter": datacenter,
                "environment": environment,
                "role": role,
                "status": status
            }.items() if v is not None
        }

        return PaginatedIterator(
            self.client,
            "/api/v1/agents",
            params=params,
            item_class=Agent
        )

    def get(self, agent_id: str) -> Agent:
        data = super().get(f"/api/v1/agents/{agent_id}")
        return Agent(**data)

    def update_tags(
        self,
        agent_id: str,
        add_tags: list = None,
        remove_tags: list = None
    ) -> Agent:
        data = self.patch(
            f"/api/v1/agents/{agent_id}/tags",
            data={
                "add_tags": add_tags or [],
                "remove_tags": remove_tags or []
            }
        )
        return Agent(**data)
```

### Best Practices

#### 1. Connection Pooling

Reuse HTTP connections for better performance:

```python
import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

class Client:
    def __init__(self, base_url: str, auth: Auth):
        self.base_url = base_url
        self.auth = auth

        # Configure connection pooling
        self.session = requests.Session()
        adapter = HTTPAdapter(
            pool_connections=10,
            pool_maxsize=100,
            max_retries=Retry(total=3, backoff_factor=0.5)
        )
        self.session.mount("http://", adapter)
        self.session.mount("https://", adapter)

        # Apply authentication
        self.session.headers.update(auth.headers())
```

#### 2. Timeout Configuration

Always set appropriate timeouts:

```python
class Client:
    DEFAULT_TIMEOUT = 30.0
    LONG_RUNNING_TIMEOUT = 300.0

    def execute_command(self, command: str, target: str, timeout: float = None):
        # Long-running operations need longer timeouts
        request_timeout = timeout or self.LONG_RUNNING_TIMEOUT

        return self.post(
            "/api/v1/exec",
            data={"command": command, "target": target},
            timeout=request_timeout
        )
```

#### 3. Idempotency

For write operations, support idempotency keys:

```python
def create_webhook(self, url: str, events: list, idempotency_key: str = None):
    headers = {}
    if idempotency_key:
        headers["Idempotency-Key"] = idempotency_key

    return self.post(
        "/api/v1/webhooks",
        data={"url": url, "events": events},
        headers=headers
    )
```

#### 4. Logging and Debugging

Include request/response logging:

```python
import logging

class LoggingClient(Client):
    def __init__(self, *args, debug: bool = False, **kwargs):
        super().__init__(*args, **kwargs)
        self.debug = debug
        self.logger = logging.getLogger("kscore")

    def _request(self, method: str, path: str, **kwargs):
        if self.debug:
            self.logger.debug(f"Request: {method} {path}")
            if kwargs.get("data"):
                self.logger.debug(f"Body: {kwargs['data']}")

        response = super()._request(method, path, **kwargs)

        if self.debug:
            self.logger.debug(f"Response: {response}")

        return response
```

#### 5. Async Support

For I/O-bound operations, provide async variants:

```python
import aiohttp

class AsyncClient:
    def __init__(self, base_url: str, auth: Auth):
        self.base_url = base_url
        self.auth = auth
        self._session = None

    async def __aenter__(self):
        self._session = aiohttp.ClientSession(
            headers=self.auth.headers()
        )
        return self

    async def __aexit__(self, *args):
        await self._session.close()

    async def get(self, path: str, **kwargs):
        async with self._session.get(
            f"{self.base_url}{path}",
            **kwargs
        ) as response:
            await handle_async_response(response)
            return await response.json()

# Usage
async with AsyncClient(base_url, auth) as client:
    agents = await client.agents.list()
```

```typescript
// TypeScript with native fetch
class AsyncClient {
  constructor(
    private baseUrl: string,
    private auth: Auth
  ) {}

  async get<T>(path: string, params?: Record<string, string>): Promise<T> {
    const url = new URL(path, this.baseUrl);
    if (params) {
      Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
    }

    const response = await fetch(url.toString(), {
      headers: this.auth.headers(),
    });

    await handleResponse(response);
    return response.json();
  }
}
```

### Testing Your SDK

#### Unit Tests

Mock HTTP responses for unit testing:

```python
import pytest
from unittest.mock import patch, MagicMock

class TestAgentResource:
    @patch("requests.Session.request")
    def test_list_agents(self, mock_request):
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "agents": [{"id": "web-01", "status": "connected"}],
            "total": 1
        }
        mock_request.return_value = mock_response

        client = Client("http://localhost:8080", ApiKeyAuth("test"))
        agents = list(client.agents.list())

        assert len(agents) == 1
        assert agents[0].id == "web-01"
```

#### Integration Tests

Test against a real (or containerized) Keystone instance:

```python
import pytest
import os

@pytest.fixture
def live_client():
    """Requires KSCORE_API_KEY and KSCORE_URL environment variables"""
    return Client(
        os.environ["KSCORE_URL"],
        ApiKeyAuth(os.environ["KSCORE_API_KEY"])
    )

@pytest.mark.integration
def test_list_agents_live(live_client):
    agents = list(live_client.agents.list(limit=10))
    # Verify we got a response (may be empty in test environment)
    assert isinstance(agents, list)
```

### SDK Distribution

#### Python (PyPI)

```toml
# pyproject.toml
[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"

[project]
name = "kscore"
version = "1.0.0"
description = "Keystone Core Python SDK"
readme = "README.md"
requires-python = ">=3.8"
dependencies = [
    "requests>=2.28.0",
    "grpcio>=1.50.0",
    "protobuf>=4.21.0"
]

[project.optional-dependencies]
async = ["aiohttp>=3.8.0"]
```

#### JavaScript/TypeScript (npm)

```json
{
  "name": "@kscore/client",
  "version": "1.0.0",
  "description": "Keystone Core JavaScript/TypeScript SDK",
  "main": "dist/index.js",
  "types": "dist/index.d.ts",
  "files": ["dist"],
  "scripts": {
    "build": "tsc",
    "test": "jest"
  },
  "dependencies": {
    "@grpc/grpc-js": "^1.8.0",
    "google-protobuf": "^3.21.0"
  }
}
```

#### Java (Maven Central)

```xml
<!-- pom.xml -->
<project>
  <groupId>com.anthropic</groupId>
  <artifactId>kscore-sdk</artifactId>
  <version>1.0.0</version>

  <dependencies>
    <dependency>
      <groupId>io.grpc</groupId>
      <artifactId>grpc-netty</artifactId>
      <version>1.54.0</version>
    </dependency>
    <dependency>
      <groupId>com.google.protobuf</groupId>
      <artifactId>protobuf-java</artifactId>
      <version>3.22.0</version>
    </dependency>
  </dependencies>
</project>
```

### Reference Implementations

The official Go SDK serves as the reference implementation. Study these packages:

| Package | Purpose |
|---------|---------|
| `pkg/client` | HTTP client with authentication |
| `pkg/api/v1` | API resource implementations |
| `pkg/models` | Data model definitions |
| `pkg/errors` | Error types and handling |

Proto files are available at `proto/` in the Keystone Core repository for generating typed clients in any language.
