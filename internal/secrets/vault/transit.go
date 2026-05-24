// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"encoding/base64"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// Encrypt routes to `transit/encrypt/<key>` with single-vs-batch
// dispatch based on len(Items).
func (b *Backend) Encrypt(ctx context.Context, req secrets.EncryptRequest) (*secrets.EncryptResponse, error) {
	if err := validateTransitRequest("Encrypt", req.Key, len(req.Items)); err != nil {
		return nil, err
	}
	body := buildEncryptBody(req.Items)
	resp, err := b.client.Logical().WriteWithContext(ctx, "transit/encrypt/"+req.Key, body)
	if err != nil {
		return nil, translateError("transit encrypt", req.Key, err)
	}
	return parseEncryptResponse(resp, len(req.Items))
}

// Decrypt routes to `transit/decrypt/<key>`.
func (b *Backend) Decrypt(ctx context.Context, req secrets.DecryptRequest) (*secrets.DecryptResponse, error) {
	if err := validateTransitRequest("Decrypt", req.Key, len(req.Items)); err != nil {
		return nil, err
	}
	body := buildDecryptBody(req.Items)
	resp, err := b.client.Logical().WriteWithContext(ctx, "transit/decrypt/"+req.Key, body)
	if err != nil {
		return nil, translateError("transit decrypt", req.Key, err)
	}
	return parseDecryptResponse(resp, len(req.Items))
}

// Sign routes to `transit/sign/<key>` (with optional `/<hash-algo>`
// suffix).
func (b *Backend) Sign(ctx context.Context, req secrets.SignRequest) (*secrets.SignResponse, error) {
	if err := validateTransitRequest("Sign", req.Key, len(req.Items)); err != nil {
		return nil, err
	}
	path := "transit/sign/" + req.Key
	if req.HashAlgorithm != "" {
		path += "/" + req.HashAlgorithm
	}
	body := buildSignBody(req)
	resp, err := b.client.Logical().WriteWithContext(ctx, path, body)
	if err != nil {
		return nil, translateError("transit sign", req.Key, err)
	}
	return parseSignResponse(resp, len(req.Items))
}

// Verify routes to `transit/verify/<key>` (with optional `/<hash-algo>`).
func (b *Backend) Verify(ctx context.Context, req secrets.VerifyRequest) (*secrets.VerifyResponse, error) {
	if err := validateTransitRequest("Verify", req.Key, len(req.Items)); err != nil {
		return nil, err
	}
	path := "transit/verify/" + req.Key
	if req.HashAlgorithm != "" {
		path += "/" + req.HashAlgorithm
	}
	body := buildVerifyBody(req)
	resp, err := b.client.Logical().WriteWithContext(ctx, path, body)
	if err != nil {
		return nil, translateError("transit verify", req.Key, err)
	}
	return parseVerifyResponse(resp, len(req.Items))
}

// HMAC routes to `transit/hmac/<key>` (with optional `/<algorithm>`).
func (b *Backend) HMAC(ctx context.Context, req secrets.HMACRequest) (*secrets.HMACResponse, error) {
	if err := validateTransitRequest("HMAC", req.Key, len(req.Items)); err != nil {
		return nil, err
	}
	path := "transit/hmac/" + req.Key
	if req.Algorithm != "" {
		path += "/" + req.Algorithm
	}
	body := buildHMACBody(req)
	resp, err := b.client.Logical().WriteWithContext(ctx, path, body)
	if err != nil {
		return nil, translateError("transit hmac", req.Key, err)
	}
	return parseHMACResponse(resp, len(req.Items))
}

// VerifyHMAC reuses the verify endpoint with the `hmac` parameter
// per Vault's contract — the same endpoint validates either
// signatures or HMACs based on which input field is set.
func (b *Backend) VerifyHMAC(ctx context.Context, req secrets.VerifyHMACRequest) (*secrets.VerifyResponse, error) {
	if err := validateTransitRequest("VerifyHMAC", req.Key, len(req.Items)); err != nil {
		return nil, err
	}
	path := "transit/verify/" + req.Key
	if req.Algorithm != "" {
		path += "/" + req.Algorithm
	}
	body := buildVerifyHMACBody(req)
	resp, err := b.client.Logical().WriteWithContext(ctx, path, body)
	if err != nil {
		return nil, translateError("transit verify-hmac", req.Key, err)
	}
	return parseVerifyResponse(resp, len(req.Items))
}

// Rewrap routes to `transit/rewrap/<key>`.
func (b *Backend) Rewrap(ctx context.Context, req secrets.RewrapRequest) (*secrets.RewrapResponse, error) {
	if err := validateTransitRequest("Rewrap", req.Key, len(req.Items)); err != nil {
		return nil, err
	}
	body := buildRewrapBody(req.Items)
	resp, err := b.client.Logical().WriteWithContext(ctx, "transit/rewrap/"+req.Key, body)
	if err != nil {
		return nil, translateError("transit rewrap", req.Key, err)
	}
	return parseRewrapResponse(resp, len(req.Items))
}

// GenerateDataKey routes to `transit/datakey/<mode>/<key>` with the
// mode being one of "plaintext" or "wrapped" per [secrets.DataKeyMode].
func (b *Backend) GenerateDataKey(ctx context.Context, req secrets.GenerateDataKeyRequest) (*secrets.GenerateDataKeyResponse, error) {
	if req.Key == "" {
		return nil, errInvalid("GenerateDataKey: Key is required")
	}
	mode := req.Mode
	if mode == "" {
		mode = secrets.DataKeyModePlaintext
	}
	if mode != secrets.DataKeyModePlaintext && mode != secrets.DataKeyModeWrapped {
		return nil, errInvalid(fmt.Sprintf("GenerateDataKey: invalid Mode %q (expected plaintext or wrapped)", req.Mode))
	}
	body := map[string]any{}
	if len(req.Context) > 0 {
		body["context"] = base64Encode(req.Context)
	}
	if req.Bits > 0 {
		body["bits"] = req.Bits
	}
	resp, err := b.client.Logical().WriteWithContext(ctx, "transit/datakey/"+string(mode)+"/"+req.Key, body)
	if err != nil {
		return nil, translateError("transit datakey", req.Key, err)
	}
	return parseGenerateDataKeyResponse(resp, mode)
}

// ---- request body builders ---------------------------------------

func buildEncryptBody(items []secrets.EncryptInput) map[string]any {
	if len(items) == 1 {
		return encryptItemBody(items[0])
	}
	batch := make([]map[string]any, len(items))
	for i, it := range items {
		batch[i] = encryptItemBody(it)
	}
	return map[string]any{"batch_input": batch}
}

func encryptItemBody(it secrets.EncryptInput) map[string]any {
	body := map[string]any{
		"plaintext": base64Encode(it.Plaintext),
	}
	if len(it.Context) > 0 {
		body["context"] = base64Encode(it.Context)
	}
	if it.KeyVersion > 0 {
		body["key_version"] = it.KeyVersion
	}
	if len(it.Nonce) > 0 {
		body["nonce"] = base64Encode(it.Nonce)
	}
	return body
}

func buildDecryptBody(items []secrets.DecryptInput) map[string]any {
	if len(items) == 1 {
		return decryptItemBody(items[0])
	}
	batch := make([]map[string]any, len(items))
	for i, it := range items {
		batch[i] = decryptItemBody(it)
	}
	return map[string]any{"batch_input": batch}
}

func decryptItemBody(it secrets.DecryptInput) map[string]any {
	body := map[string]any{
		"ciphertext": it.Ciphertext,
	}
	if len(it.Context) > 0 {
		body["context"] = base64Encode(it.Context)
	}
	if len(it.Nonce) > 0 {
		body["nonce"] = base64Encode(it.Nonce)
	}
	return body
}

func buildSignBody(req secrets.SignRequest) map[string]any {
	if len(req.Items) == 1 {
		body := signItemBody(req.Items[0])
		if req.SignatureAlgorithm != "" {
			body["signature_algorithm"] = req.SignatureAlgorithm
		}
		return body
	}
	batch := make([]map[string]any, len(req.Items))
	for i, it := range req.Items {
		batch[i] = signItemBody(it)
	}
	out := map[string]any{"batch_input": batch}
	if req.SignatureAlgorithm != "" {
		out["signature_algorithm"] = req.SignatureAlgorithm
	}
	return out
}

func signItemBody(it secrets.SignInput) map[string]any {
	body := map[string]any{
		"input": base64Encode(it.Input),
	}
	if len(it.Context) > 0 {
		body["context"] = base64Encode(it.Context)
	}
	if it.KeyVersion > 0 {
		body["key_version"] = it.KeyVersion
	}
	if it.Prehashed {
		body["prehashed"] = true
	}
	return body
}

func buildVerifyBody(req secrets.VerifyRequest) map[string]any {
	if len(req.Items) == 1 {
		body := verifyItemBody(req.Items[0])
		if req.SignatureAlgorithm != "" {
			body["signature_algorithm"] = req.SignatureAlgorithm
		}
		return body
	}
	batch := make([]map[string]any, len(req.Items))
	for i, it := range req.Items {
		batch[i] = verifyItemBody(it)
	}
	out := map[string]any{"batch_input": batch}
	if req.SignatureAlgorithm != "" {
		out["signature_algorithm"] = req.SignatureAlgorithm
	}
	return out
}

func verifyItemBody(it secrets.VerifyInput) map[string]any {
	body := map[string]any{
		"input":     base64Encode(it.Input),
		"signature": it.Signature,
	}
	if len(it.Context) > 0 {
		body["context"] = base64Encode(it.Context)
	}
	if it.Prehashed {
		body["prehashed"] = true
	}
	return body
}

func buildHMACBody(req secrets.HMACRequest) map[string]any {
	if len(req.Items) == 1 {
		body := map[string]any{"input": base64Encode(req.Items[0].Input)}
		if req.KeyVersion > 0 {
			body["key_version"] = req.KeyVersion
		}
		return body
	}
	batch := make([]map[string]any, len(req.Items))
	for i, it := range req.Items {
		batch[i] = map[string]any{"input": base64Encode(it.Input)}
	}
	out := map[string]any{"batch_input": batch}
	if req.KeyVersion > 0 {
		out["key_version"] = req.KeyVersion
	}
	return out
}

func buildVerifyHMACBody(req secrets.VerifyHMACRequest) map[string]any {
	if len(req.Items) == 1 {
		return map[string]any{
			"input": base64Encode(req.Items[0].Input),
			"hmac":  req.Items[0].HMAC,
		}
	}
	batch := make([]map[string]any, len(req.Items))
	for i, it := range req.Items {
		batch[i] = map[string]any{
			"input": base64Encode(it.Input),
			"hmac":  it.HMAC,
		}
	}
	return map[string]any{"batch_input": batch}
}

func buildRewrapBody(items []secrets.RewrapInput) map[string]any {
	if len(items) == 1 {
		return rewrapItemBody(items[0])
	}
	batch := make([]map[string]any, len(items))
	for i, it := range items {
		batch[i] = rewrapItemBody(it)
	}
	return map[string]any{"batch_input": batch}
}

func rewrapItemBody(it secrets.RewrapInput) map[string]any {
	body := map[string]any{"ciphertext": it.Ciphertext}
	if len(it.Context) > 0 {
		body["context"] = base64Encode(it.Context)
	}
	if len(it.Nonce) > 0 {
		body["nonce"] = base64Encode(it.Nonce)
	}
	if it.KeyVersion > 0 {
		body["key_version"] = it.KeyVersion
	}
	return body
}

// ---- response parsers --------------------------------------------

func parseEncryptResponse(resp *vaultapi.Secret, want int) (*secrets.EncryptResponse, error) {
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("%w: vault: transit encrypt: empty response", secrets.ErrInvalidBackend)
	}
	out := &secrets.EncryptResponse{Results: make([]secrets.EncryptResult, 0, want)}
	if results, ok := resp.Data["batch_results"]; ok {
		for _, raw := range toMapSlice(results) {
			out.Results = append(out.Results, secrets.EncryptResult{
				Ciphertext: stringField(raw, "ciphertext"),
				KeyVersion: intField(raw, "key_version"),
				Err:        stringField(raw, "error"),
			})
		}
		return out, nil
	}
	out.Results = append(out.Results, secrets.EncryptResult{
		Ciphertext: stringField(resp.Data, "ciphertext"),
		KeyVersion: intField(resp.Data, "key_version"),
	})
	return out, nil
}

func parseDecryptResponse(resp *vaultapi.Secret, want int) (*secrets.DecryptResponse, error) {
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("%w: vault: transit decrypt: empty response", secrets.ErrInvalidBackend)
	}
	out := &secrets.DecryptResponse{Results: make([]secrets.DecryptResult, 0, want)}
	if results, ok := resp.Data["batch_results"]; ok {
		for _, raw := range toMapSlice(results) {
			plain, err := decodePlaintextField(raw, "plaintext")
			res := secrets.DecryptResult{
				Plaintext: plain,
				Err:       stringField(raw, "error"),
			}
			if err != nil && res.Err == "" {
				res.Err = err.Error()
			}
			out.Results = append(out.Results, res)
		}
		return out, nil
	}
	plain, err := decodePlaintextField(resp.Data, "plaintext")
	if err != nil {
		return nil, fmt.Errorf("%w: vault: transit decrypt: %v", secrets.ErrInvalidBackend, err)
	}
	out.Results = append(out.Results, secrets.DecryptResult{Plaintext: plain})
	return out, nil
}

func parseSignResponse(resp *vaultapi.Secret, want int) (*secrets.SignResponse, error) {
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("%w: vault: transit sign: empty response", secrets.ErrInvalidBackend)
	}
	out := &secrets.SignResponse{Results: make([]secrets.SignResult, 0, want)}
	if results, ok := resp.Data["batch_results"]; ok {
		for _, raw := range toMapSlice(results) {
			out.Results = append(out.Results, secrets.SignResult{
				Signature:  stringField(raw, "signature"),
				KeyVersion: intField(raw, "key_version"),
				Err:        stringField(raw, "error"),
			})
		}
		return out, nil
	}
	out.Results = append(out.Results, secrets.SignResult{
		Signature:  stringField(resp.Data, "signature"),
		KeyVersion: intField(resp.Data, "key_version"),
	})
	return out, nil
}

func parseVerifyResponse(resp *vaultapi.Secret, want int) (*secrets.VerifyResponse, error) {
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("%w: vault: transit verify: empty response", secrets.ErrInvalidBackend)
	}
	out := &secrets.VerifyResponse{Results: make([]secrets.VerifyResult, 0, want)}
	if results, ok := resp.Data["batch_results"]; ok {
		for _, raw := range toMapSlice(results) {
			out.Results = append(out.Results, secrets.VerifyResult{
				Valid: boolField(raw, "valid"),
				Err:   stringField(raw, "error"),
			})
		}
		return out, nil
	}
	out.Results = append(out.Results, secrets.VerifyResult{Valid: boolField(resp.Data, "valid")})
	return out, nil
}

func parseHMACResponse(resp *vaultapi.Secret, want int) (*secrets.HMACResponse, error) {
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("%w: vault: transit hmac: empty response", secrets.ErrInvalidBackend)
	}
	out := &secrets.HMACResponse{Results: make([]secrets.HMACResult, 0, want)}
	if results, ok := resp.Data["batch_results"]; ok {
		for _, raw := range toMapSlice(results) {
			out.Results = append(out.Results, secrets.HMACResult{
				HMAC:       stringField(raw, "hmac"),
				KeyVersion: intField(raw, "key_version"),
				Err:        stringField(raw, "error"),
			})
		}
		return out, nil
	}
	out.Results = append(out.Results, secrets.HMACResult{
		HMAC:       stringField(resp.Data, "hmac"),
		KeyVersion: intField(resp.Data, "key_version"),
	})
	return out, nil
}

func parseRewrapResponse(resp *vaultapi.Secret, want int) (*secrets.RewrapResponse, error) {
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("%w: vault: transit rewrap: empty response", secrets.ErrInvalidBackend)
	}
	out := &secrets.RewrapResponse{Results: make([]secrets.RewrapResult, 0, want)}
	if results, ok := resp.Data["batch_results"]; ok {
		for _, raw := range toMapSlice(results) {
			out.Results = append(out.Results, secrets.RewrapResult{
				Ciphertext: stringField(raw, "ciphertext"),
				KeyVersion: intField(raw, "key_version"),
				Err:        stringField(raw, "error"),
			})
		}
		return out, nil
	}
	out.Results = append(out.Results, secrets.RewrapResult{
		Ciphertext: stringField(resp.Data, "ciphertext"),
		KeyVersion: intField(resp.Data, "key_version"),
	})
	return out, nil
}

func parseGenerateDataKeyResponse(resp *vaultapi.Secret, mode secrets.DataKeyMode) (*secrets.GenerateDataKeyResponse, error) {
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("%w: vault: transit datakey: empty response", secrets.ErrInvalidBackend)
	}
	out := &secrets.GenerateDataKeyResponse{
		Ciphertext: stringField(resp.Data, "ciphertext"),
		KeyVersion: intField(resp.Data, "key_version"),
	}
	if mode == secrets.DataKeyModePlaintext {
		plain, err := decodePlaintextField(resp.Data, "plaintext")
		if err != nil {
			return nil, fmt.Errorf("%w: vault: transit datakey: %v", secrets.ErrInvalidBackend, err)
		}
		out.Plaintext = plain
	}
	return out, nil
}

// ---- shape helpers -----------------------------------------------

func validateTransitRequest(method, key string, items int) error {
	if key == "" {
		return errInvalid(method + ": Key is required")
	}
	if items == 0 {
		return errInvalid(method + ": Items must be non-empty")
	}
	return nil
}

func base64Encode(in []byte) string {
	return base64.StdEncoding.EncodeToString(in)
}

// decodePlaintextField pulls a base64 plaintext out of the Vault
// response data map. Returns nil + an error if the field is missing
// or malformed.
func decodePlaintextField(data map[string]any, key string) ([]byte, error) {
	raw := stringField(data, key)
	if raw == "" {
		return nil, nil
	}
	out, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("base64 decode %s: %v", key, err)
	}
	return out, nil
}

// toMapSlice coerces an `any` that Vault decoded as `[]any` of
// `map[string]any` into the concrete `[]map[string]any`. Items that
// aren't maps drop silently (defensive — never observed in practice).
func toMapSlice(raw any) []map[string]any {
	asSlice, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(asSlice))
	for _, item := range asSlice {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolField(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func intField(m map[string]any, key string) int {
	return numToInt(m[key])
}
