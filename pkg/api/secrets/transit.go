package secrets

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// handleTransit routes POST /api/v1/transit/{operation}/{key}.
func (h *Handler) handleTransit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.transit == nil {
		writeError(w, http.StatusServiceUnavailable, "transit backend not available")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/transit/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "transit operation and key name are required")
		return
	}

	operation := parts[0]
	keyName := parts[1]

	switch operation {
	case "encrypt":
		h.handleTransitEncrypt(w, r, keyName)
	case "decrypt":
		h.handleTransitDecrypt(w, r, keyName)
	case "batch-encrypt":
		h.handleTransitBatchEncrypt(w, r, keyName)
	case "batch-decrypt":
		h.handleTransitBatchDecrypt(w, r, keyName)
	case "sign":
		h.handleTransitSign(w, r, keyName)
	case "verify":
		h.handleTransitVerify(w, r, keyName)
	case "hmac":
		h.handleTransitHMAC(w, r, keyName)
	case "rewrap":
		h.handleTransitRewrap(w, r, keyName)
	case "datakey":
		writeError(w, http.StatusNotImplemented, "datakey operation is not supported")
	default:
		writeError(w, http.StatusBadRequest, "unknown transit operation: "+operation)
	}
}

func (h *Handler) handleTransitEncrypt(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitEncryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	transitReq := &secrets.TransitRequest{
		Operation:  secrets.TransitOperationEncrypt,
		KeyName:    keyName,
		Plaintext:  req.Plaintext,
		Context:    req.Context,
		KeyVersion: req.KeyVersion,
	}

	resp, err := h.transit.Encrypt(r.Context(), transitReq)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, TransitEncryptResponse{
		Ciphertext: resp.Ciphertext,
		KeyVersion: resp.KeyVersion,
	})
}

func (h *Handler) handleTransitDecrypt(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitDecryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	transitReq := &secrets.TransitRequest{
		Operation:  secrets.TransitOperationDecrypt,
		KeyName:    keyName,
		Ciphertext: req.Ciphertext,
		Context:    req.Context,
	}

	resp, err := h.transit.Decrypt(r.Context(), transitReq)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, TransitDecryptResponse{
		Plaintext:  resp.Plaintext,
		KeyVersion: resp.KeyVersion,
	})
}

func (h *Handler) handleTransitBatchEncrypt(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitBatchEncryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items array is required")
		return
	}

	transitReqs := make([]*secrets.TransitRequest, len(req.Items))
	for i, item := range req.Items {
		transitReqs[i] = &secrets.TransitRequest{
			Operation:  secrets.TransitOperationEncrypt,
			KeyName:    keyName,
			Plaintext:  item.Plaintext,
			Context:    item.Context,
			KeyVersion: item.KeyVersion,
		}
	}

	responses, err := h.transit.BatchEncrypt(r.Context(), transitReqs)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	results := make([]TransitEncryptResponse, len(responses))
	for i, resp := range responses {
		results[i] = TransitEncryptResponse{
			Ciphertext: resp.Ciphertext,
			KeyVersion: resp.KeyVersion,
		}
	}

	writeJSON(w, http.StatusOK, TransitBatchEncryptResponse{Results: results})
}

func (h *Handler) handleTransitBatchDecrypt(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitBatchDecryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items array is required")
		return
	}

	transitReqs := make([]*secrets.TransitRequest, len(req.Items))
	for i, item := range req.Items {
		transitReqs[i] = &secrets.TransitRequest{
			Operation:  secrets.TransitOperationDecrypt,
			KeyName:    keyName,
			Ciphertext: item.Ciphertext,
			Context:    item.Context,
		}
	}

	responses, err := h.transit.BatchDecrypt(r.Context(), transitReqs)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	results := make([]TransitDecryptResponse, len(responses))
	for i, resp := range responses {
		results[i] = TransitDecryptResponse{
			Plaintext:  resp.Plaintext,
			KeyVersion: resp.KeyVersion,
		}
	}

	writeJSON(w, http.StatusOK, TransitBatchDecryptResponse{Results: results})
}

func (h *Handler) handleTransitSign(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitSignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	transitReq := &secrets.TransitRequest{
		Operation:  secrets.TransitOperationSign,
		KeyName:    keyName,
		Input:      req.Input,
		Algorithm:  req.Algorithm,
		KeyVersion: req.KeyVersion,
	}

	resp, err := h.transit.Sign(r.Context(), transitReq)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, TransitSignResponse{
		Signature:  resp.Signature,
		KeyVersion: resp.KeyVersion,
	})
}

func (h *Handler) handleTransitVerify(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	transitReq := &secrets.TransitRequest{
		Operation: secrets.TransitOperationVerify,
		KeyName:   keyName,
		Input:     req.Input,
		Signature: req.Signature,
		Algorithm: req.Algorithm,
	}

	resp, err := h.transit.Verify(r.Context(), transitReq)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, TransitVerifyResponse{
		Valid: resp.Valid,
	})
}

func (h *Handler) handleTransitHMAC(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitHMACRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	transitReq := &secrets.TransitRequest{
		Operation: secrets.TransitOperationHMAC,
		KeyName:   keyName,
		Input:     req.Input,
		Algorithm: req.Algorithm,
	}

	resp, err := h.transit.HMAC(r.Context(), transitReq)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, TransitHMACResponse{
		HMAC: resp.HMAC,
	})
}

func (h *Handler) handleTransitRewrap(w http.ResponseWriter, r *http.Request, keyName string) {
	var req TransitRewrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	transitReq := &secrets.TransitRequest{
		Operation:  secrets.TransitOperationRewrap,
		KeyName:    keyName,
		Ciphertext: req.Ciphertext,
		Context:    req.Context,
	}

	resp, err := h.transit.Rewrap(r.Context(), transitReq)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, TransitRewrapResponse{
		Ciphertext: resp.Ciphertext,
		KeyVersion: resp.KeyVersion,
	})
}
