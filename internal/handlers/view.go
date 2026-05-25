package handlers

// BaseView holds fields the shared layout expects on every page. Embed it in
// per-page view structs so the layout can reference user/CSRF info uniformly.
type BaseView struct {
	Title     string
	UserName  string
	UserRole  string
	CSRFToken string
}
