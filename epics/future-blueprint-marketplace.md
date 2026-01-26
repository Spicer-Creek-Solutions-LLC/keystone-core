# Epic 34: Blueprint Marketplace & Ecosystem

## Overview

This epic establishes a complete blueprint ecosystem including a public registry, marketplace discovery, automatic updates, and comprehensive testing framework. While Epic 25 defined the blueprint format and Epic 28 created standard blueprints, this epic builds the infrastructure for community-driven blueprint sharing and lifecycle management.

**Epic Type**: Feature, Infrastructure, Community

**Scope**:
- Blueprint registry server (OCI-compatible)
- Marketplace web interface for discovery
- Blueprint search and categorization
- Automatic blueprint update mechanism
- Blueprint testing framework
- Publisher verification and signing
- Usage analytics and ratings
- Blueprint dependency resolution at scale

**Out of Scope**:
- Blueprint format changes (Epic 25)
- Standard blueprint content (Epic 28)
- Blueprint runtime execution (existing infrastructure)
- Monetization or paid blueprints (future consideration)

## Rationale

### Problem Statement

Epic 25 (Blueprints) defined the blueprint specification but explicitly deferred several items:

| TODO Item | Description |
|-----------|-------------|
| Finalize registry design and implementation | Registry server not implemented |
| Create blueprint marketplace and discovery mechanism | No discovery UI |
| Build comprehensive blueprint testing framework | Testing gaps |
| Implement automatic blueprint update mechanism | Manual updates only |
| Expand rollback mechanism with comprehensive testing | Limited rollback |

Without these components:
1. **Discovery Friction**: Users can't find community blueprints
2. **Trust Gap**: No verification of blueprint publishers
3. **Update Burden**: Manual tracking of blueprint versions
4. **Quality Unknown**: No standardized testing or ratings
5. **Fragmentation**: Blueprints scattered across repositories

### Benefits

1. **Discoverability**: Central marketplace for finding blueprints
2. **Trust**: Publisher verification and cryptographic signing
3. **Automation**: Automatic updates with policy controls
4. **Quality**: Testing framework ensures blueprint reliability
5. **Community**: Enable community contribution and sharing
6. **Ecosystem**: Foundation for third-party integrations

## Objectives

1. **O1**: Implement OCI-compatible blueprint registry server
2. **O2**: Create web-based marketplace interface for blueprint discovery
3. **O3**: Establish blueprint categorization and search system
4. **O4**: Implement automatic blueprint update mechanism with policies
5. **O5**: Build comprehensive blueprint testing framework
6. **O6**: Create publisher verification and signing infrastructure
7. **O7**: Implement usage analytics and community ratings
8. **O8**: Enable enterprise private registry deployments

## Architecture

### Blueprint Ecosystem Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      Blueprint Ecosystem                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    Marketplace Web UI                            │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │    │
│  │  │  Search  │  │ Category │  │ Publisher│  │  Detail  │        │    │
│  │  │  & Filter│  │  Browse  │  │  Profiles│  │  Views   │        │    │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │    │
│  └────────────────────────────┬────────────────────────────────────┘    │
│                               │                                          │
│  ┌────────────────────────────▼────────────────────────────────────┐    │
│  │                    Registry API Server                           │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │    │
│  │  │   OCI    │  │  Search  │  │ Analytics│  │  Webhook │        │    │
│  │  │ Registry │  │  Index   │  │  Engine  │  │  Events  │        │    │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │    │
│  └────────────────────────────┬────────────────────────────────────┘    │
│                               │                                          │
│         ┌─────────────────────┼─────────────────────┐                   │
│         │                     │                     │                    │
│         ▼                     ▼                     ▼                    │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐            │
│  │   Primary    │     │   Mirror     │     │  Enterprise  │            │
│  │   Storage    │     │   Storage    │     │   Private    │            │
│  │   (S3/GCS)   │     │   (Global)   │     │   Registry   │            │
│  └──────────────┘     └──────────────┘     └──────────────┘            │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    Client Components                             │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │    │
│  │  │kscore-      │  │  Auto-Update │  │   Testing    │           │    │
│  │  │blueprint CLI│  │   Daemon     │  │   Framework  │           │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘           │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Registry Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Blueprint Registry Server                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                      API Layer                            │   │
│  │                                                           │   │
│  │  /v2/                    OCI Distribution API             │   │
│  │  /api/v1/blueprints      Blueprint metadata API           │   │
│  │  /api/v1/search          Full-text search API             │   │
│  │  /api/v1/publishers      Publisher management API         │   │
│  │  /api/v1/analytics       Usage analytics API              │   │
│  │  /api/v1/webhooks        Event notification API           │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Service Layer                          │   │
│  │                                                           │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐         │   │
│  │  │  Registry  │  │   Search   │  │  Signing   │         │   │
│  │  │  Service   │  │   Service  │  │  Service   │         │   │
│  │  └────────────┘  └────────────┘  └────────────┘         │   │
│  │                                                           │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐         │   │
│  │  │ Publisher  │  │ Analytics  │  │  Webhook   │         │   │
│  │  │  Service   │  │  Service   │  │  Service   │         │   │
│  │  └────────────┘  └────────────┘  └────────────┘         │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Storage Layer                          │   │
│  │                                                           │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐         │   │
│  │  │   Blob     │  │  Metadata  │  │   Search   │         │   │
│  │  │  Storage   │  │  Database  │  │   Index    │         │   │
│  │  │ (S3/GCS)   │  │ (Postgres) │  │ (Elastic)  │         │   │
│  │  └────────────┘  └────────────┘  └────────────┘         │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Directory Structure

```
pkg/
├── registry/
│   ├── server/
│   │   ├── server.go            # Main registry server
│   │   ├── oci.go               # OCI distribution compliance
│   │   ├── api.go               # REST API handlers
│   │   └── middleware.go        # Auth, rate limiting
│   ├── storage/
│   │   ├── storage.go           # Storage interface
│   │   ├── s3.go                # S3 backend
│   │   ├── gcs.go               # GCS backend
│   │   ├── filesystem.go        # Local filesystem
│   │   └── mirror.go            # Mirror sync
│   ├── search/
│   │   ├── indexer.go           # Blueprint indexer
│   │   ├── search.go            # Search engine
│   │   └── ranking.go           # Result ranking
│   ├── publisher/
│   │   ├── publisher.go         # Publisher management
│   │   ├── verification.go      # Identity verification
│   │   └── signing.go           # Cosign integration
│   ├── analytics/
│   │   ├── collector.go         # Usage collection
│   │   ├── aggregator.go        # Metric aggregation
│   │   └── reporter.go          # Analytics reporting
│   └── webhook/
│       ├── events.go            # Event definitions
│       ├── dispatcher.go        # Event dispatch
│       └── subscribers.go       # Subscriber management
├── marketplace/
│   ├── web/
│   │   ├── static/              # Frontend assets
│   │   ├── templates/           # HTML templates
│   │   └── handlers.go          # Web handlers
│   └── api/
│       ├── categories.go        # Category management
│       ├── ratings.go           # Rating system
│       └── comments.go          # Comment system
└── blueprint/
    ├── updater/
    │   ├── updater.go           # Auto-update daemon
    │   ├── policy.go            # Update policies
    │   └── scheduler.go         # Update scheduling
    └── testing/
        ├── framework.go         # Test framework
        ├── runner.go            # Test runner
        ├── fixtures.go          # Test fixtures
        └── reporter.go          # Test reporting

cmd/
├── kscore-registry/             # Registry server binary
└── kscore-marketplace/          # Marketplace web server

web/
└── marketplace/
    ├── src/
    │   ├── components/          # React components
    │   ├── pages/               # Page components
    │   └── api/                 # API client
    ├── public/
    └── package.json
```

## Deliverables

### D1: Blueprint Registry Server

OCI-compatible registry server for blueprint distribution.

**Features**:
- OCI Distribution Spec v1.1 compliance
- Multi-backend storage (S3, GCS, Azure Blob, filesystem)
- Content-addressable storage with deduplication
- Pull-through caching for upstream registries
- Garbage collection for orphaned blobs
- High availability with stateless design

**API Endpoints**:
```
GET    /v2/                                    # API version check
GET    /v2/{name}/tags/list                    # List tags
GET    /v2/{name}/manifests/{reference}        # Get manifest
PUT    /v2/{name}/manifests/{reference}        # Push manifest
DELETE /v2/{name}/manifests/{reference}        # Delete manifest
GET    /v2/{name}/blobs/{digest}               # Get blob
POST   /v2/{name}/blobs/uploads/               # Start upload
```

### D2: Marketplace Web Interface

Web-based UI for blueprint discovery and exploration.

**Features**:
- Full-text search with filters
- Category browsing (infrastructure, security, observability, etc.)
- Publisher profiles with verification badges
- Blueprint detail pages with README rendering
- Version history and changelogs
- Dependency visualization
- Installation instructions
- Community ratings and reviews

**Technology**:
- React frontend with TypeScript
- Server-side rendering for SEO
- Responsive design (mobile-friendly)
- Dark mode support

### D3: Search and Categorization System

Comprehensive search with intelligent categorization.

**Search Features**:
- Full-text search across name, description, README
- Faceted search (category, tags, publisher, license)
- Fuzzy matching for typos
- Search suggestions and autocomplete
- Relevance ranking with popularity boost

**Categories**:
- Infrastructure (compute, storage, network)
- Security (compliance, hardening, identity)
- Observability (monitoring, logging, tracing)
- Database (PostgreSQL, MySQL, Redis)
- Web (nginx, Apache, load balancers)
- Container (Kubernetes, Docker)
- Development (CI/CD, testing)
- Custom (user-defined)

### D4: Automatic Update Mechanism

Automated blueprint updates with policy controls.

**Features**:
- Background update daemon
- Configurable update policies (auto, notify, manual)
- Semantic versioning constraints
- Pre-update validation
- Automatic rollback on failure
- Update scheduling (maintenance windows)
- Changelog notifications

**Update Policies**:
```yaml
blueprints:
  update_policy:
    default: notify           # auto | notify | manual

    rules:
      - match: "keystone/*"
        policy: auto
        constraints: "^1.0.0"  # Only minor/patch updates

      - match: "community/*"
        policy: notify

      - match: "internal/*"
        policy: manual
```

### D5: Blueprint Testing Framework

Comprehensive testing framework for blueprint validation.

**Test Types**:
- **Syntax Tests**: YAML validation, schema compliance
- **Parameter Tests**: Default values, constraints, required fields
- **Dependency Tests**: Resolution, compatibility, cycles
- **Dry-Run Tests**: Simulated apply without changes
- **Integration Tests**: Real infrastructure testing
- **Upgrade Tests**: Version migration validation
- **Rollback Tests**: Recovery verification

**Test Configuration**:
```yaml
# blueprint-test.yaml
tests:
  syntax:
    enabled: true

  parameters:
    scenarios:
      - name: minimal
        params: {}
      - name: production
        params:
          replicas: 3
          ha_enabled: true

  integration:
    platforms:
      - docker
      - kubernetes
    timeout: 30m

  upgrade:
    from_versions:
      - "1.0.0"
      - "1.1.0"
```

**CLI Integration**:
```bash
# Run all tests
kscore-blueprint test ./my-blueprint

# Run specific test types
kscore-blueprint test ./my-blueprint --type=syntax,parameters

# Run integration tests
kscore-blueprint test ./my-blueprint --type=integration --platform=kubernetes
```

### D6: Publisher Verification System

Trust and verification for blueprint publishers.

**Verification Levels**:
- **Unverified**: Anonymous publisher
- **Verified**: Email/identity verified
- **Organization**: Verified organization
- **Official**: Keystone project blueprints

**Signing Infrastructure**:
- Cosign integration for signature verification
- Transparency log for audit trail
- Key rotation support
- Revocation mechanism

**Publisher Profile**:
```yaml
publisher:
  name: acme-corp
  display_name: "ACME Corporation"
  verification: organization
  website: https://acme.example.com
  email: blueprints@acme.example.com
  public_key: |
    -----BEGIN PUBLIC KEY-----
    ...
    -----END PUBLIC KEY-----
  blueprints:
    - acme-corp/webserver
    - acme-corp/database
    - acme-corp/monitoring
```

### D7: Analytics and Ratings

Usage analytics and community feedback system.

**Analytics**:
- Download counts (total, weekly, daily)
- Version distribution
- Geographic distribution
- Referrer tracking
- Error rates

**Ratings**:
- 5-star rating system
- Written reviews with moderation
- Helpful/not helpful voting
- Publisher responses

**Metrics Dashboard**:
- Publisher analytics dashboard
- Download trends
- Version adoption curves
- Geographic heatmaps

### D8: Enterprise Private Registry

Self-hosted registry for enterprise deployments.

**Features**:
- Air-gapped deployment support
- LDAP/OIDC authentication
- Fine-grained access control
- Audit logging
- Upstream sync (pull-through)
- Custom branding

**Deployment Options**:
- Docker Compose
- Kubernetes Helm chart
- Binary installation

### D9: Documentation

Comprehensive documentation for the blueprint ecosystem.

**Contents**:
- Registry deployment guide
- Publisher onboarding guide
- Blueprint testing guide
- Search syntax reference
- API reference
- Enterprise deployment guide

## Acceptance Criteria

### AC1: Registry Server Operational
- [ ] OCI Distribution Spec compliance tests pass
- [ ] Multi-backend storage functional (S3, GCS, filesystem)
- [ ] Push/pull operations working
- [ ] Garbage collection operational

### AC2: Marketplace Functional
- [ ] Web UI deployed and accessible
- [ ] Search returns relevant results
- [ ] Category browsing works
- [ ] Blueprint detail pages render correctly

### AC3: Search Effective
- [ ] Full-text search functional
- [ ] Faceted filtering works
- [ ] Search results ranked by relevance
- [ ] Autocomplete suggestions working

### AC4: Auto-Update Working
- [ ] Update daemon detects new versions
- [ ] Policies enforced correctly
- [ ] Pre-update validation runs
- [ ] Rollback on failure works

### AC5: Testing Framework Complete
- [ ] All test types implemented
- [ ] CI integration working
- [ ] Test reports generated
- [ ] Platform matrix supported

### AC6: Publisher System Active
- [ ] Publisher registration working
- [ ] Verification flow functional
- [ ] Signing with Cosign working
- [ ] Verification badges displayed

### AC7: Analytics Collecting
- [ ] Download metrics captured
- [ ] Ratings system functional
- [ ] Publisher dashboard available
- [ ] Privacy-compliant collection

### AC8: Enterprise Ready
- [ ] Private registry deployable
- [ ] Authentication integration working
- [ ] Access control functional
- [ ] Upstream sync working

## Sub-Issues / Tasks

### Phase 1: Registry Server Core (Weeks 1-4)

#### T1.1: OCI Distribution Implementation
Implement OCI Distribution Spec compliant registry.

**Deliverables**:
- OCI API handlers
- Manifest management
- Blob storage abstraction
- Content-addressable storage

#### T1.2: Storage Backends
Implement storage backend drivers.

**Deliverables**:
- S3 backend
- GCS backend
- Azure Blob backend
- Filesystem backend

#### T1.3: Registry Authentication
Implement registry authentication.

**Deliverables**:
- Token-based auth
- OAuth2 integration
- API key support
- Rate limiting

#### T1.4: Registry CLI Integration
Update kscore-blueprint CLI for registry.

**Deliverables**:
- `kscore-blueprint push` command
- `kscore-blueprint pull` command
- Registry configuration
- Credential management

### Phase 2: Search and Metadata (Weeks 5-8)

#### T2.1: Metadata Database
Implement blueprint metadata storage.

**Deliverables**:
- PostgreSQL schema
- Metadata API
- Version management
- Publisher management

#### T2.2: Search Indexer
Implement search indexing.

**Deliverables**:
- Elasticsearch integration
- Index schema
- Incremental indexing
- Re-indexing support

#### T2.3: Search API
Implement search API.

**Deliverables**:
- Full-text search endpoint
- Faceted search
- Autocomplete
- Ranking algorithm

#### T2.4: Category System
Implement categorization.

**Deliverables**:
- Category taxonomy
- Auto-categorization
- Custom tags
- Category API

### Phase 3: Marketplace Web UI (Weeks 9-12)

#### T3.1: UI Foundation
Set up marketplace web application.

**Deliverables**:
- React application scaffold
- Component library
- API client
- Routing

#### T3.2: Search and Browse Pages
Implement discovery pages.

**Deliverables**:
- Search results page
- Category browse page
- Filter components
- Pagination

#### T3.3: Blueprint Detail Pages
Implement detail views.

**Deliverables**:
- Blueprint overview page
- Version history page
- Dependency graph
- Installation guide

#### T3.4: Publisher Pages
Implement publisher profiles.

**Deliverables**:
- Publisher profile page
- Blueprint listing
- Verification badges
- Contact information

### Phase 4: Testing Framework (Weeks 13-16)

#### T4.1: Test Framework Core
Implement core testing framework.

**Deliverables**:
- Test runner
- Test discovery
- Result reporting
- CI integration

#### T4.2: Syntax and Parameter Tests
Implement validation tests.

**Deliverables**:
- YAML validation
- Schema validation
- Parameter tests
- Constraint validation

#### T4.3: Integration Tests
Implement integration testing.

**Deliverables**:
- Docker platform tests
- Kubernetes platform tests
- Cleanup automation
- Timeout handling

#### T4.4: Upgrade and Rollback Tests
Implement migration tests.

**Deliverables**:
- Version upgrade tests
- Rollback tests
- State migration tests
- Compatibility checks

### Phase 5: Auto-Update System (Weeks 17-20)

#### T5.1: Update Daemon
Implement background updater.

**Deliverables**:
- Daemon service
- Version checking
- Scheduling
- Notification

#### T5.2: Update Policies
Implement policy engine.

**Deliverables**:
- Policy configuration
- Rule matching
- Constraint evaluation
- Override support

#### T5.3: Update Execution
Implement update application.

**Deliverables**:
- Pre-update validation
- Atomic update
- Rollback support
- State tracking

#### T5.4: Update CLI
Implement CLI commands.

**Deliverables**:
- `kscore-blueprint update` command
- `kscore-blueprint check-updates` command
- Policy management
- History viewing

### Phase 6: Publisher and Signing (Weeks 21-24)

#### T6.1: Publisher Registration
Implement publisher management.

**Deliverables**:
- Registration flow
- Profile management
- Namespace claiming
- Transfer support

#### T6.2: Verification System
Implement verification.

**Deliverables**:
- Email verification
- Organization verification
- Manual review process
- Badge assignment

#### T6.3: Signing Infrastructure
Implement Cosign signing.

**Deliverables**:
- Cosign integration
- Key management
- Signature verification
- Transparency log

#### T6.4: Trust Policies
Implement trust enforcement.

**Deliverables**:
- Trust policy config
- Signature requirements
- Verification levels
- Warning/blocking

### Phase 7: Analytics and Ratings (Weeks 25-28)

#### T7.1: Analytics Collection
Implement usage tracking.

**Deliverables**:
- Event collection
- Privacy controls
- Data aggregation
- Export API

#### T7.2: Publisher Dashboard
Implement analytics dashboard.

**Deliverables**:
- Download metrics
- Version charts
- Geographic data
- Trend analysis

#### T7.3: Rating System
Implement community ratings.

**Deliverables**:
- Rating submission
- Review moderation
- Helpful voting
- Publisher responses

### Phase 8: Enterprise and Polish (Weeks 29-32)

#### T8.1: Enterprise Registry
Implement private registry.

**Deliverables**:
- Standalone deployment
- Authentication backends
- Access control
- Upstream sync

#### T8.2: Helm Chart
Create Kubernetes deployment.

**Deliverables**:
- Helm chart
- High availability config
- Ingress configuration
- Monitoring integration

#### T8.3: Documentation
Write comprehensive docs.

**Deliverables**:
- Deployment guide
- Publisher guide
- API reference
- Enterprise guide

#### T8.4: Launch Preparation
Prepare for public launch.

**Deliverables**:
- Official blueprints migration
- Community outreach
- Launch announcement
- Support processes

## Dependencies

- **Epic 25** (Blueprints): Blueprint format specification
- **Epic 28** (Standard Blueprints): Initial blueprint content
- **Epic 17** (SPIFFE): Identity for publisher verification
- **External**: Cosign for signing, Elasticsearch for search

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Low adoption | Empty marketplace | Seed with official blueprints; community outreach |
| Malicious blueprints | Security risk | Publisher verification; code scanning; community reporting |
| Search relevance | Poor discovery | Iterative ranking tuning; user feedback |
| Infrastructure costs | Budget overrun | CDN caching; tiered storage; community sponsors |
| Spam/abuse | Quality degradation | Rate limiting; moderation; reputation system |

## Success Metrics

| Metric | Target (6 months) |
|--------|-------------------|
| Registered publishers | 50+ |
| Published blueprints | 200+ |
| Monthly downloads | 10,000+ |
| Search queries/day | 500+ |
| Average rating | >4.0 stars |
| Verified publishers | 20+ |
| Enterprise deployments | 5+ |

## Definition of Done

- [ ] All deliverables (D1-D9) implemented
- [ ] All acceptance criteria (AC1-AC8) met
- [ ] Registry server deployed and operational
- [ ] Marketplace web UI launched
- [ ] Testing framework documented and usable
- [ ] Publisher onboarding process active
- [ ] Enterprise deployment guide complete
- [ ] Official blueprints migrated to registry

## Future Considerations

- Blueprint monetization (paid blueprints, subscriptions)
- Blueprint composition (blueprints depending on blueprints)
- Visual blueprint editor
- AI-assisted blueprint generation
- Blueprint security scanning
- Compliance certification for blueprints
- Multi-language blueprint support
