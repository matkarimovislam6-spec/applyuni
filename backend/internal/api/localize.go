package api

import (
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/i18n"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/requirements"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/service"
)

// The service produces English; responses are translated here, just before
// they are sent, into the language the visitor chose (X-Lang header).
// Proper names (universities, programmes, cities) stay as they are.

func localizeProgram(lang string, p *service.ProgramInfo) {
	if p == nil || lang == i18n.EN {
		return
	}
	i18n.TP(lang, p.FeeNote)
	i18n.TP(lang, p.CallNote)
	for i := range p.Deadlines {
		p.Deadlines[i].Round = i18n.T(lang, p.Deadlines[i].Round)
		i18n.TP(lang, p.Deadlines[i].Note)
	}
	// Next/LastYear point into Deadlines, so they're already translated.
}

func localizeChecklist(lang string, cl *requirements.Checklist) {
	if cl == nil || lang == i18n.EN {
		return
	}
	for i := range cl.Items {
		cl.Items[i].Title = i18n.T(lang, cl.Items[i].Title)
		cl.Items[i].Description = i18n.T(lang, cl.Items[i].Description)
	}
	for i := range cl.Steps {
		cl.Steps[i].Title = i18n.T(lang, cl.Steps[i].Title)
	}
	for i := range cl.Warnings {
		cl.Warnings[i] = i18n.T(lang, cl.Warnings[i])
	}
}

func localizeMatch(lang string, m *service.Match) {
	if m == nil || lang == i18n.EN {
		return
	}
	localizeProgram(lang, m.Program)
	for i := range m.Checks {
		m.Checks[i].Label = i18n.T(lang, m.Checks[i].Label)
		m.Checks[i].Detail = i18n.T(lang, m.Checks[i].Detail)
		m.Checks[i].Fix = i18n.T(lang, m.Checks[i].Fix)
	}
}

func localizeApplication(lang string, a *service.Application) {
	if a == nil || lang == i18n.EN {
		return
	}
	localizeProgram(lang, a.Program)
	i18n.TP(lang, a.StatusNote)
}
