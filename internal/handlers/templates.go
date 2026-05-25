package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// Templates owns parsed page and partial templates.
//
// A "page" is a file rendered inside layout.html via the "content" block.
// A "partial" is any file whose basename starts with "_": it is parsed
// standalone (no layout) so handlers can return HTMX fragments, and it is
// also injected into every page template so pages can compose with it via
// {{template "<basename>" .}}.
type Templates struct {
	pages    map[string]*template.Template
	partials map[string]*template.Template
}

// LoadTemplates walks the templates directory and builds a Template for each
// page and partial. Partials are discovered first so they can be inlined into
// every page parse.
func LoadTemplates(root string) (*Templates, error) {
	layoutPath := filepath.Join(root, "layout.html")

	var partialFiles []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), "_") {
			partialFiles = append(partialFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	pages := map[string]*template.Template{}
	partials := map[string]*template.Template{}

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		if p == layoutPath {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		base := filepath.Base(p)

		if strings.HasPrefix(base, "_") {
			// Parse every partial alongside this one so partials can compose
			// via {{template "<basename>" .}} even when rendered standalone.
			tmpl, err := template.New(base).ParseFiles(partialFiles...)
			if err != nil {
				return fmt.Errorf("parse partial %s: %w", rel, err)
			}
			partials[rel] = tmpl
			return nil
		}

		files := append([]string{layoutPath, p}, partialFiles...)
		tmpl, err := template.New("layout.html").ParseFiles(files...)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		pages[rel] = tmpl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Templates{pages: pages, partials: partials}, nil
}

// Render executes the named page (e.g. "auth/login.html") into w with data.
func (t *Templates) Render(w http.ResponseWriter, name string, data any) error {
	tmpl, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("unknown template %q", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, "layout.html", data)
}

// RenderPartial executes a standalone partial (no layout). The partial file's
// basename is also the template name produced by ParseFiles.
func (t *Templates) RenderPartial(w http.ResponseWriter, name string, data any) error {
	tmpl, ok := t.partials[name]
	if !ok {
		return fmt.Errorf("unknown partial %q", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, path.Base(name), data)
}
