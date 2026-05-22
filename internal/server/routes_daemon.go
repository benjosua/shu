package server

import "net/http"

func (a *App) daemonRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/daemon/ws", a.auth(a.daemonWS))
	m.HandleFunc("POST /api/daemon/executors/register", a.auth(a.registerExecutor))
	m.HandleFunc("POST /api/daemon/executors/deregister", a.auth(a.deregisterExecutor))
	m.HandleFunc("POST /api/daemon/executors/heartbeat", a.auth(a.executorHeartbeat))
	m.HandleFunc("POST /api/daemon/executors/{id}/work/claim", a.auth(a.claimWork))
	m.HandleFunc("POST /api/daemon/work/{id}/start", a.auth(a.startWork))
	m.HandleFunc("POST /api/daemon/work/{id}/artifacts", a.auth(a.addArtifact))
	m.HandleFunc("POST /api/daemon/work/{id}/complete", a.auth(a.finishWork))
	m.HandleFunc("POST /api/daemon/work/{id}/fail", a.auth(a.finishWork))
	m.HandleFunc("GET /api/daemon/work/{id}/status", a.auth(a.workStatus))
}
