# Keystone Core OpenAPI Specifications

This directory contains OpenAPI 3.0 specifications for Keystone Core REST APIs.

## Files

- `openapi-spec.yaml` - Main OpenAPI specification covering all REST endpoints

## Viewing the Documentation

### Swagger UI

You can view the API documentation using Swagger UI:

```bash
# Using Docker
docker run -p 8080:8080 -e SWAGGER_JSON=/spec/openapi-spec.yaml \
  -v $(pwd):/spec swaggerapi/swagger-ui

# Then open http://localhost:8080
```

### Redoc

For a cleaner documentation experience:

```bash
# Using Docker
docker run -p 8080:80 -e SPEC_URL=/spec/openapi-spec.yaml \
  -v $(pwd):/usr/share/nginx/html/spec redocly/redoc

# Then open http://localhost:8080
```

### Local Development

Install the Swagger CLI:

```bash
npm install -g @apidevtools/swagger-cli

# Validate the spec
swagger-cli validate openapi-spec.yaml

# Bundle into a single file
swagger-cli bundle openapi-spec.yaml -o bundled.yaml
```

## Generating Client SDKs

Use OpenAPI Generator to create client libraries:

```bash
# Install OpenAPI Generator
npm install -g @openapitools/openapi-generator-cli

# Generate Go client
openapi-generator-cli generate -i openapi-spec.yaml -g go -o ../clients/go

# Generate Python client
openapi-generator-cli generate -i openapi-spec.yaml -g python -o ../clients/python

# Generate TypeScript client
openapi-generator-cli generate -i openapi-spec.yaml -g typescript-fetch -o ../clients/typescript
```

## API Categories

The specification covers these API categories:

| Tag | Description |
|-----|-------------|
| Health | Health check and status endpoints |
| Cluster | Cluster management operations |
| Agents | Agent management and visualization |
| Events | Event ingestion and processing |
| Mirrors | File mirror management |
| Discovery | Proxy device discovery |
| Gateway | Telemetry gateway operations |
| Webhooks | GitOps webhook receivers |

## Authentication

The API supports multiple authentication methods:

- **Bearer Token**: API key in `Authorization: Bearer <token>` header
- **mTLS**: Client certificate authentication
- **JWT**: JSON Web Token authentication

## Versioning

- REST APIs use `/api/v1/` prefix
- Breaking changes will increment the version number
- Deprecated endpoints will be marked in the spec

## Contributing

When adding or modifying API endpoints:

1. Update the OpenAPI specification in this directory
2. Run validation: `swagger-cli validate openapi-spec.yaml`
3. Update any affected documentation
4. Regenerate client SDKs if needed
