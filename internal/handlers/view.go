package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
)

// BaseView holds fields the shared layout expects on every page. Embed it in
// per-page view structs so the layout can reference user/CSRF info uniformly.
type BaseView struct {
	Title            string
	UserName         string
	UserRole         string
	CSRFToken        string
	AvatarPath       string
	FlashNotice      string
	IsImpersonating  bool
	ImpersonatorName string
	ImpersonatorRole string
}

// NewBaseView builds a BaseView from the signed-in user. AvatarPath resolves to
// the thumbnail URL when the user has uploaded a picture, empty otherwise so
// the layout can fall back to a placeholder.
func NewBaseView(user repository.User, csrfToken, title string) BaseView {
	return newBaseView(user, csrfToken, title, repository.User{}, false)
}

// NewBaseViewForRequest builds a BaseView and includes impersonation metadata
// from the request context when present.
func NewBaseViewForRequest(r *http.Request, user repository.User, csrfToken, title string) BaseView {
	impersonator, ok := apmw.ImpersonatorFromContext(r.Context())
	return newBaseView(user, csrfToken, title, impersonator, ok)
}

func newBaseView(user repository.User, csrfToken, title string, impersonator repository.User, isImpersonating bool) BaseView {
	avatar := ""
	if user.AvatarPath.Valid && user.AvatarPath.String != "" {
		name := user.AvatarPath.String
		base := strings.TrimSuffix(name, filepath.Ext(name))
		avatar = "/static/avatars/thumb_" + base + ".jpg"
	}
	view := BaseView{
		Title:           title,
		UserName:        user.Name,
		UserRole:        string(user.Role),
		CSRFToken:       csrfToken,
		AvatarPath:      avatar,
		IsImpersonating: isImpersonating,
	}
	if isImpersonating {
		view.ImpersonatorName = impersonator.Name
		view.ImpersonatorRole = string(impersonator.Role)
	}
	return view
}
