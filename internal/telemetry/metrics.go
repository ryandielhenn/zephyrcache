
package telemetry

import "net/http"

// Expose a placeholder metrics handler (wire Prometheus later).
func MetricsHandler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("# HELP zephyrcache_placeholder 1\n# TYPE zephyrcache_placeholder counter\nzephyrcache_placeholder 1\n"))
    })
}
