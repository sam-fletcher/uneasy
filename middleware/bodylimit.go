package middleware

import "net/http"

// maxBodyBytes caps every API request body at 1 MiB. Nobody legitimate ever
// sends more than that; it exists to stop a stranger from posting a
// multi-gigabyte body at a JSON handler.
const maxBodyBytes = 1 << 20

// BodyLimit wraps the request body in http.MaxBytesReader. Handlers don't
// need to change: json.NewDecoder(r.Body).Decode already returns an error
// (a *http.MaxBytesError) once the reader hits the cap, and every handler's
// existing decode-error branch already maps that to a 400 — never a 500.
func BodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}
