package ui

import (
	"github.com/napadailo/voip-tui/i18n"
	"github.com/napadailo/voip-tui/tui"
)

// T translates interface text (see package i18n of voip-tui).
func T(s string) string { return i18n.T(s) }

// Tf translates a format string and fills it in.
func Tf(format string, args ...any) string { return i18n.Tf(format, args...) }

// N marks text stored now and translated when drawn (titles, headers, hints).
func N(s string) string { return s }

// The views are built from the voip-tui framework; these aliases keep them short.
type (
	View      = tui.View
	Hint      = tui.Hint
	Row       = tui.Row
	TableView = tui.TableView
)
