// Package module implements Keystone Core's secure, extensible module system.
// NOTE: API/ABI is not finalized and may change without notice.
//
// The module package provides cryptographic verification, capability-based security,
// dependency resolution, sandboxed execution (Starlark and WASM), registry integration,
// and auditing. Modules are versioned packages with fine-grained access controls that
// extend Keystone's functionality without compromising security.
//
// # Subpackages
//
// Core Loading and Verification:
//   - loader: Module loading pipeline (verify, validate policy, prepare execution)
//   - manifest: Module metadata (name, version, capabilities, limits, dependencies)
//   - verify: Cryptographic verification (hash, signature, transparency log)
//   - lockfile: Dependency pinning for reproducible builds
//
// Security and Capabilities:
//   - capabilities: Capability-based access control (fs, http, exec, secrets, kv)
//   - policy: Module policy enforcement and validation
//
// Execution Runtimes:
//   - runtime: Base runtime interfaces
//   - runtime/starlark: Starlark script execution with capability builtins
//   - runtime/wasm: WASM module execution via wazero (pure Go)
//   - wasm: WASM build tools and optimization
//
// Registry and Dependencies:
//   - registry: HTTP-based module registry client
//   - resolver: Dependency resolution with MVS conflict resolution
//   - versioning: Semantic versioning and version policy
//
// Supporting:
//   - audit: Audit logging for module operations
//   - sdk: Module development SDK
//   - testing: Module test framework
//
// # Module Loading
//
// The loader subpackage provides the complete module loading pipeline:
//
//	import "github.com/your-org/keystone-core/pkg/module/loader"
//
//	loader := loader.NewDefaultModuleLoader(verifier, policyEngine)
//	result, err := loader.Load(ctx, modulePath, options)
//
// Loading includes hash verification, signature verification, SumDB checks,
// trust policy enforcement, and capability validation.
//
// # Capability-Based Security
//
// Modules declare required capabilities in their manifest. The capabilities
// subpackage enforces fine-grained access control:
//
//	import "github.com/your-org/keystone-core/pkg/module/capabilities"
//
//	// Module can only read /etc/*, write to /var/app/*, make HTTP GET to api.example.com
//	caps := []capabilities.Capability{
//	    capabilities.NewFSReadCapability("/etc/*"),
//	    capabilities.NewFSWriteCapability("/var/app/*"),
//	    capabilities.NewHTTPGetCapability("api.example.com"),
//	}
//
// Available capabilities: fs.read, fs.write, http.get, http.post, exec,
// secrets.read, secrets.write, kv, log.
//
// # Verification
//
// The verify subpackage provides cryptographic verification:
//
//	import "github.com/your-org/keystone-core/pkg/module/verify"
//
//	verifier := verify.NewSignatureVerifier(trustPolicy)
//	result, err := verifier.Verify(ctx, module, signature)
//
// Supports hash verification, RSA/ECDSA/Ed25519 signatures, Cosign keyless
// signing, and SumDB transparency log integration.
//
// # Execution Runtimes
//
// Modules execute in sandboxed runtimes with capability enforcement:
//
//	// Starlark execution
//	import "github.com/your-org/keystone-core/pkg/module/runtime/starlark"
//
//	rt := starlark.NewRuntime(options)
//	result, err := rt.ExecuteFile(ctx, "module.star", globals)
//
//	// WASM execution
//	import "github.com/your-org/keystone-core/pkg/module/runtime/wasm"
//
//	rt := wasm.NewRuntime(options)
//	mod, err := rt.LoadModule(ctx, wasmBytes)
//	result, err := rt.ExecuteFunction(ctx, mod, "main", args)
//
// # Dependency Resolution
//
// The resolver subpackage handles dependency resolution with MVS:
//
//	import "github.com/your-org/keystone-core/pkg/module/resolver"
//
//	resolver := resolver.NewModuleResolver(registry)
//	result, err := resolver.Resolve(ctx, request)
//
// Supports version constraints (^, ~, ranges), lock files for reproducibility,
// and prerelease filtering.
//
// # Module Registry
//
// The registry subpackage provides HTTP-based registry operations:
//
//	import "github.com/your-org/keystone-core/pkg/module/registry"
//
//	client := registry.NewHTTPClient(config)
//	versions, err := client.ListVersions(ctx, "vendor/package")
//	err = client.Download(ctx, "vendor/package", "1.2.3", destPath)
package module
