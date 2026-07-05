package handler

// secureCookies controls whether cookies set by this package (login in
// openSession, logout in DeleteSession) carry the Secure flag. Set once at
// startup from main.go's PUBLIC_ORIGIN parsing, before routes are mounted —
// mirrors how UNEASY_DEV is read once at startup today. Left false (the zero
// value) in dev, where PUBLIC_ORIGIN is unset and the stack is plain http.
var secureCookies bool

// SetSecureCookies configures secureCookies. Call once from main.go before
// serving requests.
func SetSecureCookies(secure bool) {
	secureCookies = secure
}
