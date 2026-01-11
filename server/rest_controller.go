package server

import "net/http"

// RestController is an interface for HTTP REST controllers.
// Controllers implementing this interface can be registered with the Builder
// and will have their dependencies automatically injected.
type RestController interface {
	// Routes returns the HTTP routes handled by this controller.
	Routes() []Route
}

// Route defines a single HTTP route with its pattern and handler.
type Route struct {
	// Pattern is the URL pattern for the route (e.g., "/api/users", "/api/users/{id}").
	Pattern string

	// Method is the HTTP method for the route (GET, POST, PUT, DELETE, etc.).
	// If empty, the route will handle all HTTP methods.
	Method string

	// Handler is the HTTP handler function for the route.
	Handler http.HandlerFunc
}

// methodFilterHandler wraps a handler to only respond to a specific HTTP method.
func methodFilterHandler(method string, handler http.HandlerFunc) http.HandlerFunc {
	if method == "" {
		return handler
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		handler(w, r)
	}
}
