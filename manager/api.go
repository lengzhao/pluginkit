package manager

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
)

type server struct {
	validateBuild func(ctx context.Context, doc Document) error
	initialYAML   string
	onChange      func(ctx context.Context, evt DocumentEvent) error
	onBuild       func(ctx context.Context, evt DocumentEvent) error
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

type bootstrapResponse struct {
	Kinds       []kindInfo   `json:"kinds"`
	Document    *Document    `json:"document,omitempty"`
	View        *View        `json:"view,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	YAML        string       `json:"yaml,omitempty"`
}

type loadRequest struct {
	YAML string `json:"yaml"`
}

type editRequest struct {
	Document Document  `json:"document"`
	Op       Operation `json:"op"`
}

type editResponse struct {
	Document    Document     `json:"document"`
	View        View         `json:"view"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	YAML        string       `json:"yaml"`
}

func (s *server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("POST /api/load", s.handleLoad)
	mux.HandleFunc("POST /api/edit", s.handleEdit)
	mux.HandleFunc("POST /api/build", s.handleBuild)
}

func (s *server) listKinds() []kindInfo {
	var kinds []kindInfo
	for _, kind := range pluginkit.ListKinds() {
		info, ok := describeKind(kind)
		if !ok {
			continue
		}
		kinds = append(kinds, info)
	}
	return kinds
}

func (s *server) handleCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, catalogResponse{Kinds: s.listKinds()})
}

func (s *server) handleBootstrap(w http.ResponseWriter, _ *http.Request) {
	resp := bootstrapResponse{Kinds: s.listKinds()}
	if s.initialYAML == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	doc, err := FromYAML([]byte(s.initialYAML))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	loaded, err := buildEditResponse(doc, collectDiagnostics(doc))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Document = &loaded.Document
	resp.View = &loaded.View
	resp.Diagnostics = loaded.Diagnostics
	resp.YAML = loaded.YAML
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleLoad(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req loadRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := apply(Document{}, Operation{Type: "importYAML", YAML: req.YAML})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeEditResponse(w, r, http.StatusOK, doc, ChangeLoad, "importYAML")
}

func (s *server) handleEdit(w http.ResponseWriter, r *http.Request) {
	req, err := readEditRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := apply(req.Document, req.Op)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeEditResponse(w, r, http.StatusOK, doc, ChangeEdit, req.Op.Type)
}

func (s *server) handleBuild(w http.ResponseWriter, r *http.Request) {
	req, err := readEditRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	doc := req.Document
	diags := collectDiagnostics(doc)
	if !hasErrorDiag(diags) && s.validateBuild != nil {
		if err := s.validateBuild(r.Context(), doc); err != nil {
			diags = append(diags, buildDiagnostic(doc, err))
		}
	}
	resp, err := buildEditResponse(doc, diags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
	s.notifyChange(r.Context(), ChangeBuild, "", resp)
}

func readEditRequest(r *http.Request) (editRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return editRequest{}, err
	}
	var req editRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return editRequest{}, err
	}
	return req, nil
}

func (s *server) writeEditResponse(w http.ResponseWriter, r *http.Request, status int, doc Document, reason ChangeReason, op string) {
	diags := collectDiagnostics(doc)
	resp, err := buildEditResponse(doc, diags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, status, resp)
	if r != nil {
		s.notifyChange(r.Context(), reason, op, resp)
	}
}

func buildEditResponse(doc Document, diags []Diagnostic) (editResponse, error) {
	if diags == nil {
		diags = []Diagnostic{}
	}
	yamlBytes, err := doc.ToYAML()
	if err != nil {
		return editResponse{}, err
	}
	return editResponse{
		Document:    doc,
		View:        projectView(doc),
		Diagnostics: diags,
		YAML:        string(yamlBytes),
	}, nil
}

func hasErrorDiag(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func buildDiagnostic(doc Document, err error) Diagnostic {
	path := "root"
	var be *build.Error
	if errors.As(err, &be) {
		if mapped, ok := planErrorPath(doc, be.ID); ok {
			path = mapped
		}
	}
	return Diagnostic{
		Path:     path,
		Severity: "error",
		Stage:    "build",
		Code:     "build",
		Message:  err.Error(),
	}
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
