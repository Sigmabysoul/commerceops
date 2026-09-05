// This file implements the REST boundary: decoding input, calling services, and encoding consistent responses in the Product Master package.
package product

import (
	"errors"
	"net/http"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
)

type HTTPHandler struct{ service *Service }

func NewHTTPHandler(service *Service) *HTTPHandler { return &HTTPHandler{service: service} }

func (h *HTTPHandler) Marketplaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.service.ListMarketplaces(r.Context(), principal(r))
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"marketplaces": items})
}

func (h *HTTPHandler) Products(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.ListProducts(r.Context(), principal(r), r.URL.Query().Get("q"), r.URL.Query().Get("status"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"products": items})
	case http.MethodPost:
		var input ProductInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, err := h.service.CreateProduct(r.Context(), principal(r), input)
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"product": item})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) Product(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		item, err := h.service.GetProduct(r.Context(), principal(r), r.PathValue("product_id"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"product": item})
	case http.MethodPatch:
		var input ProductInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, err := h.service.UpdateProduct(r.Context(), principal(r), r.PathValue("product_id"), input)
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"product": item})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) Mappings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.ListMappings(r.Context(), principal(r), r.URL.Query().Get("product_id"), r.URL.Query().Get("marketplace"), r.URL.Query().Get("status"))
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"sku_mappings": items})
	case http.MethodPost:
		var input MappingInput
		if !httpserver.DecodeJSON(w, r, &input) {
			return
		}
		item, err := h.service.CreateMapping(r.Context(), principal(r), input)
		if writeError(w, err) {
			return
		}
		httpserver.WriteJSON(w, http.StatusCreated, map[string]any{"sku_mapping": item})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) Mapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	var input MappingInput
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	item, err := h.service.UpdateMapping(r.Context(), principal(r), r.PathValue("mapping_id"), input)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"sku_mapping": item})
}

func (h *HTTPHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input struct {
		MarketplaceKey string `json:"marketplace_key"`
		SKU            string `json:"sku"`
	}
	if !httpserver.DecodeJSON(w, r, &input) {
		return
	}
	result, err := h.service.Resolve(r.Context(), principal(r), input.MarketplaceKey, input.SKU)
	if writeError(w, err) {
		return
	}
	httpserver.WriteJSON(w, http.StatusOK, result)
}

func principal(r *http.Request) auth.Principal {
	value, _ := auth.PrincipalFromContext(r.Context())
	return value
}
func methodNotAllowed(w http.ResponseWriter) {
	httpserver.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
}
func writeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authorization.ErrPermissionDenied):
		httpserver.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Permission denied")
	case errors.Is(err, ErrNotFound):
		httpserver.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
	case errors.Is(err, ErrConflict):
		httpserver.WriteError(w, http.StatusConflict, "CONFLICT", "Resource conflicts with existing data")
	case errors.Is(err, ErrInvalidInput):
		httpserver.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request is invalid")
	default:
		httpserver.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
	return true
}
