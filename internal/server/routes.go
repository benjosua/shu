package server

import "net/http"

func (a *App) routes(m *http.ServeMux) {
	a.coreRoutes(m)
	a.productRoutes(m)
	a.daemonRoutes(m)
}
