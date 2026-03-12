package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"rag/internal/service"
)

type Handler struct {
	svc       *service.RAGService
	apiMux    *http.ServeMux
	staticDir string
	logger    *zap.Logger
}

func NewHandler(svc *service.RAGService, staticDir string, logger *zap.Logger) http.Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	h := &Handler{
		svc:       svc,
		apiMux:    http.NewServeMux(),
		staticDir: staticDir,
		logger:    logger,
	}
	h.registerRoutes()
	return h
}

func (h *Handler) registerRoutes() {
	h.apiMux.HandleFunc("/api/health", h.withMethods(h.handleHealth, http.MethodGet, http.MethodOptions))
	h.apiMux.HandleFunc("/api/kbs", h.withMethods(h.handleKnowledgeBases, http.MethodGet, http.MethodPost, http.MethodOptions))
	h.apiMux.HandleFunc("/api/kbs/", h.withMethods(h.handleKnowledgeBaseByID, http.MethodDelete, http.MethodOptions))

	h.apiMux.HandleFunc("/api/documents/upload", h.withMethods(h.handleUploadDocument, http.MethodPost, http.MethodOptions))
	h.apiMux.HandleFunc("/api/documents/url", h.withMethods(h.handleIngestURL, http.MethodPost, http.MethodOptions))
	h.apiMux.HandleFunc("/api/documents", h.withMethods(h.handleDocuments, http.MethodGet, http.MethodOptions))
	h.apiMux.HandleFunc("/api/documents/", h.withMethods(h.handleDocumentByID, http.MethodDelete, http.MethodPost, http.MethodOptions))

	h.apiMux.HandleFunc("/api/chunks", h.withMethods(h.handleChunks, http.MethodGet, http.MethodOptions))
	h.apiMux.HandleFunc("/api/chunks/", h.withMethods(h.handleChunkByID, http.MethodPut, http.MethodOptions))

	h.apiMux.HandleFunc("/api/search", h.withMethods(h.handleSearch, http.MethodPost, http.MethodOptions))
	h.apiMux.HandleFunc("/api/retrieve/debug", h.withMethods(h.handleRetrieveDebug, http.MethodPost, http.MethodOptions))
	h.apiMux.HandleFunc("/api/chat", h.withMethods(h.handleChat, http.MethodPost, http.MethodOptions))

	h.apiMux.HandleFunc("/api/qa/generate", h.withMethods(h.handleQAGenerate, http.MethodPost, http.MethodOptions))
	h.apiMux.HandleFunc("/api/qa", h.withMethods(h.handleQAList, http.MethodGet, http.MethodOptions))
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			h.logger.Error(
				"panic serving request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Any("recover", rec),
				zap.Stack("stack"),
			)
			if strings.HasPrefix(r.URL.Path, "/api/") {
				h.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"error": "internal server error",
				})
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/api/") {
		h.apiMux.ServeHTTP(w, r)
		return
	}
	h.serveStatic(w, r)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	result, _ := h.svc.Health(r.Context())
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       result["mysql"] == "ok" && result["es"] == "ok",
		"services": result,
		"time":     time.Now().Format(time.RFC3339),
	})
}

func (h *Handler) handleKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.svc.ListKnowledgeBases(r.Context())
		if err != nil {
			h.writeErr(w, http.StatusInternalServerError, err)
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := h.readJSON(r, &req); err != nil {
			h.writeErr(w, http.StatusBadRequest, err)
			return
		}
		item, err := h.svc.CreateKnowledgeBase(r.Context(), req.Name, req.Description)
		if err != nil {
			h.writeErr(w, http.StatusBadRequest, err)
			return
		}
		h.writeJSON(w, http.StatusCreated, item)
	default:
		h.writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (h *Handler) handleKnowledgeBaseByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/kbs/")
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.DeleteKnowledgeBase(r.Context(), id); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": id})
}

func (h *Handler) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	kbID, err := strconv.ParseInt(r.FormValue("kb_id"), 10, 64)
	if err != nil || kbID <= 0 {
		h.writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid kb_id"))
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	doc, chunks, err := h.svc.IngestFile(r.Context(), kbID, fh.Filename, data)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"document":    doc,
		"chunk_count": chunks,
	})
}

func (h *Handler) handleIngestURL(w http.ResponseWriter, r *http.Request) {
	var req service.URLIngestRequest
	if err := h.readJSON(r, &req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	doc, chunks, err := h.svc.IngestURL(r.Context(), req)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"document":    doc,
		"chunk_count": chunks,
	})
}

func (h *Handler) handleDocuments(w http.ResponseWriter, r *http.Request) {
	kbID, err := strconv.ParseInt(r.URL.Query().Get("kb_id"), 10, 64)
	if err != nil || kbID <= 0 {
		h.writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid kb_id"))
		return
	}
	items, err := h.svc.ListDocuments(r.Context(), kbID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (h *Handler) handleDocumentByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/documents/"), "/")
	if path == "" {
		h.writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid document path"))
		return
	}

	if strings.HasSuffix(path, "/reindex") {
		if r.Method != http.MethodPost {
			h.writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		idStr := strings.TrimSuffix(path, "/reindex")
		idStr = strings.Trim(idStr, "/")
		docID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || docID <= 0 {
			h.writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid doc_id"))
			return
		}
		result, err := h.svc.ReindexDocument(r.Context(), docID)
		if err != nil {
			h.writeErr(w, http.StatusBadRequest, err)
			return
		}
		h.writeJSON(w, http.StatusOK, result)
		return
	}

	if r.Method != http.MethodDelete {
		h.writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	docID, err := strconv.ParseInt(path, 10, 64)
	if err != nil || docID <= 0 {
		h.writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid doc_id"))
		return
	}
	if err := h.svc.DeleteDocument(r.Context(), docID); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": docID})
}

func (h *Handler) handleChunks(w http.ResponseWriter, r *http.Request) {
	docID, err := strconv.ParseInt(r.URL.Query().Get("doc_id"), 10, 64)
	if err != nil || docID <= 0 {
		h.writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid doc_id"))
		return
	}
	items, err := h.svc.ListChunksByDoc(r.Context(), docID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (h *Handler) handleChunkByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/chunks/")
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := h.readJSON(r, &req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.UpdateChunk(r.Context(), id, req.Content); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"updated": id})
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req service.SearchRequest
	if err := h.readJSON(r, &req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := h.svc.Search(r.Context(), req, false)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleRetrieveDebug(w http.ResponseWriter, r *http.Request) {
	var req service.SearchRequest
	if err := h.readJSON(r, &req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := h.svc.Search(r.Context(), req, true)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleQAGenerate(w http.ResponseWriter, r *http.Request) {
	var req service.QAGenerateRequest
	if err := h.readJSON(r, &req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	count, err := h.svc.GenerateQAPairs(r.Context(), req)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"generated": count})
}

func (h *Handler) handleQAList(w http.ResponseWriter, r *http.Request) {
	docID, err := strconv.ParseInt(r.URL.Query().Get("doc_id"), 10, 64)
	if err != nil || docID <= 0 {
		h.writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid doc_id"))
		return
	}
	items, err := h.svc.ListQAPairs(r.Context(), docID)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	var req service.ChatRequest
	if err := h.readJSON(r, &req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		req.SessionID = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	out, err := h.svc.Chat(r.Context(), req)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err)
		return
	}
	h.writeJSON(w, http.StatusOK, out)
}

func (h *Handler) serveStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "" || path == "/" {
		http.ServeFile(w, r, filepath.Join(h.staticDir, "index.html"))
		return
	}

	safe := filepath.Clean(path)
	safe = strings.TrimPrefix(safe, "/")
	safe = strings.TrimPrefix(safe, `\`)
	if safe == "." || safe == "" {
		http.ServeFile(w, r, filepath.Join(h.staticDir, "index.html"))
		return
	}
	if strings.Contains(safe, "..") {
		http.NotFound(w, r)
		return
	}

	full := filepath.Join(h.staticDir, safe)
	if _, err := os.Stat(full); err == nil {
		http.ServeFile(w, r, full)
		return
	}
	http.ServeFile(w, r, filepath.Join(h.staticDir, "index.html"))
}

func (h *Handler) withMethods(next http.HandlerFunc, methods ...string) http.HandlerFunc {
	set := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		set[m] = struct{}{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := set[r.Method]; !ok {
			h.writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		next(w, r)
	}
}

func parseIDFromPath(path, prefix string) (int64, error) {
	raw := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, fmt.Errorf("invalid id path")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func (h *Handler) readJSON(r *http.Request, out interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func (h *Handler) writeErr(w http.ResponseWriter, code int, err error) {
	h.writeJSON(w, code, map[string]interface{}{
		"error": err.Error(),
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
