package server

import "net/http"

func (a *App) coreRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/executors", a.auth(a.listExecutors))
	m.HandleFunc("POST /api/resources", a.auth(a.createResource))
	m.HandleFunc("GET /api/resources", a.auth(a.listResources))
	m.HandleFunc("POST /api/work", a.auth(a.createWork))
	m.HandleFunc("GET /api/work", a.auth(a.listWork))
	m.HandleFunc("GET /api/work/{id}", a.auth(a.getWork))
	m.HandleFunc("GET /api/work/{id}/artifacts", a.auth(a.listArtifacts))
	m.HandleFunc("GET /api/runs", a.auth(a.listRuns))
	m.HandleFunc("GET /api/runs/{id}", a.auth(a.getRun))
	m.HandleFunc("GET /api/activity", a.auth(a.listActivity))
}
