package handlers

// This file shows the pattern you'll follow when adding new domains.
// Uncomment and flesh out when the skills package is ready.

// import (
// 	"log/slog"
// 	"net/http"
//
// 	"github.com/sriramsme/OnlyAgents/pkg/api"
// )
//
// SkillsHandler handles all /v1/skills endpoints.
// type SkillsHandler struct {
// 	deps   Deps
// 	logger *slog.Logger
// }
//
// func NewSkillsHandler(deps Deps, logger *slog.Logger) *SkillsHandler {
// 	return &SkillsHandler{deps: deps, logger: logger}
// }
//
// List handles GET /v1/skills
// func (h *SkillsHandler) List(w http.ResponseWriter, r *http.Request) {
// 	skills := h.deps.Skills.List()
// 	api.JSON(w, http.StatusOK, map[string]any{"skills": skills})
// }
//
// Run handles POST /v1/skills/{name}/run
// func (h *SkillsHandler) Run(w http.ResponseWriter, r *http.Request) {
// 	name := r.PathValue("name")    // Go 1.22 path params
//
// 	skill := h.deps.Skills.Get(name)
// 	if skill == nil {
// 		api.Error(w, http.StatusNotFound, "skill not found")
// 		return
// 	}
//
// 	var params map[string]any
// 	if !api.Decode(w, r, &params) {
// 		return
// 	}
//
// 	result, err := skill.Execute(r.Context(), params)
// 	if err != nil {
// 		api.Error(w, http.StatusInternalServerError, err.Error())
// 		return
// 	}
//
// 	api.JSON(w, http.StatusOK, result)
// }
