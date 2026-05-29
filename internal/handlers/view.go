package handlers

import (
	"path/filepath"
	"strings"

	"github.com/remorac/mutabaah/internal/repository"
)

// BaseView holds fields the shared layout expects on every page. Embed it in
// per-page view structs so the layout can reference user/CSRF info uniformly.
type BaseView struct {
	Title       string
	UserName    string
	UserRole    string
	CSRFToken   string
	AvatarPath  string
	FlashNotice string
}

// NewBaseView builds a BaseView from the signed-in user. AvatarPath resolves to
// the thumbnail URL when the user has uploaded a picture, empty otherwise so
// the layout can fall back to a placeholder.
func NewBaseView(user repository.User, csrfToken, title string) BaseView {
	avatar := ""
	if user.AvatarPath.Valid && user.AvatarPath.String != "" {
		name := user.AvatarPath.String
		base := strings.TrimSuffix(name, filepath.Ext(name))
		avatar = "/static/avatars/thumb_" + base + ".jpg"
	}
	return BaseView{
		Title:      title,
		UserName:   user.Name,
		UserRole:   string(user.Role),
		CSRFToken:  csrfToken,
		AvatarPath: avatar,
	}
}
