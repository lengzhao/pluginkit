package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/lengzhao/pluginkit"
)

type server struct {
	validateBuild func(ctx context.Context, doc Document) error
}

type fieldInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	List     bool   `json:"list"`
	Optional bool   `json:"optional"`
}

type kindInfo struct {
	Kind       string      `json:"kind"`
	ReturnType string      `json:"returnType"`
	Config     []fieldInfo `json:"config"`
	Extensions []fieldInfo `json:"extensions"`
}

type catalogResponse struct {
	Kinds []kindInfo `json:"kinds"`
}

type validateResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (s *server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/describe/{kind}", s.handleDescribe)
	mux.HandleFunc("GET /api/compatible", s.handleCompatible)
	mux.HandleFunc("POST /api/export", s.handleExport)
	mux.HandleFunc("POST /api/import", s.handleImport)
	mux.HandleFunc("POST /api/validate", s.handleValidate)
	mux.HandleFunc("POST /api/template/{kind}", s.handleTemplate)
}

func (s *server) handleCatalog(w http.ResponseWriter, _ *http.Request) {
	var kinds []kindInfo
	for _, kind := range pluginkit.ListKinds() {
		info, ok := describeKind(kind)
		if !ok {
			continue
		}
		kinds = append(kinds, info)
	}
	writeJSON(w, http.StatusOK, catalogResponse{Kinds: kinds})
}

func (s *server) handleDescribe(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	info, ok := describeKind(kind)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown kind %q", kind))
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *server) handleCompatible(w http.ResponseWriter, r *http.Request) {
	parent := r.URL.Query().Get("parent")
	extName := r.URL.Query().Get("ext")
	if parent == "" || extName == "" {
		writeError(w, http.StatusBadRequest, "parent and ext are required")
		return
	}
	desc, ok := pluginkit.Describe(parent)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown parent kind %q", parent))
		return
	}
	var wantExt *pluginkit.FieldDescription
	for i := range desc.Extensions {
		if desc.Extensions[i].Name == extName {
			wantExt = &desc.Extensions[i]
			break
		}
	}
	if wantExt == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("extension %q not found on %q", extName, parent))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kinds": pluginkit.CompatibleKinds(wantExt.Type),
	})
}

func (s *server) handleExport(w http.ResponseWriter, r *http.Request) {
	doc, err := readDocument(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := doc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	yamlBytes, err := doc.ToYAML()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"yaml": string(yamlBytes)})
}

func (s *server) handleImport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var payload struct {
		YAML string `json:"yaml"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := FromYAML([]byte(payload.YAML))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *server) handleValidate(w http.ResponseWriter, r *http.Request) {
	doc, err := readDocument(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateDocument(r.Context(), doc, s.validateBuild); err != nil {
		writeJSON(w, http.StatusOK, validateResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, validateResponse{OK: true})
}

func (s *server) handleTemplate(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	desc, ok := pluginkit.Describe(kind)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown kind %q", kind))
		return
	}
	writeJSON(w, http.StatusOK, desc.Template())
}

func readDocument(r *http.Request) (Document, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return Document{}, err
	}
	var doc Document
	if err := json.Unmarshal(body, &doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

func describeKind(kind string) (kindInfo, bool) {
	desc, ok := pluginkit.Describe(kind)
	if !ok {
		return kindInfo{}, false
	}
	return kindInfo{
		Kind:       desc.Kind,
		ReturnType: pluginkit.FormatType(desc.ReturnType),
		Config:     toFieldInfos(desc.Config),
		Extensions: toFieldInfos(desc.Extensions),
	}, true
}

func toFieldInfos(fields []pluginkit.FieldDescription) []fieldInfo {
	out := make([]fieldInfo, 0, len(fields))
	for _, field := range fields {
		out = append(out, fieldInfo{
			Name:     field.Name,
			Type:     pluginkit.FormatType(field.Type),
			List:     field.List,
			Optional: field.Optional,
		})
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
