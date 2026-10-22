package ui

import (
	"testing"

	"github.com/napadailo/voip-tui/i18n/i18ntest"
	_ "github.com/napadailo/voip-tui/tui" // the framework's catalogs
)

var dirs = []string{".", "../health", "../eol", "../inventory", "../radar", "../ietf", "../dockerhub", "../digest", "../../cmd/voip-info"}

func TestCatalogs(t *testing.T) {
	i18ntest.Check(t, i18ntest.Messages(t, dirs...), map[string]map[string]string{"ru": catalogRU})
}
