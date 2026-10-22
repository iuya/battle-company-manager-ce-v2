package web

import "net/http"

type palette struct{ Key, Name string }

type styleGuideView struct {
	pageBase
	Palettes []palette
	Modes    []string
}

// handleStyleGuide shows every design token and component in the active theme. Registered only when Config.StyleGuide
// is on, so it can be switched off in production.
func (s *Server) handleStyleGuide(w http.ResponseWriter, r *http.Request) {
	s.render.page(w, r, http.StatusOK, "styleguide", styleGuideView{
		pageBase: s.base(r),
		Palettes: []palette{{"forest", "Forest & Gold"}, {"ember", "Parchment & Ember"}, {"mithril", "Mithril & Night"}},
		Modes:    []string{"system", "light", "dark"},
	})
}
