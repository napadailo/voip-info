package catalog

import "gopkg.in/yaml.v3"

// Text is a text of the catalog in several languages:
//
//	desc:
//	  en: Classic open-source PBX…
//	  ru: Классическая open-source PBX…
//
// A plain string is English.
type Text map[string]string

// UnmarshalYAML takes a mapping of languages or a plain (English) string.
func (t *Text) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*t = Text{"en": n.Value}
		return nil
	}
	m := map[string]string{}
	if err := n.Decode(&m); err != nil {
		return err
	}
	*t = m
	return nil
}

// In is the text in lang, else in English, else in any language there is.
func (t Text) In(lang string) string {
	if s, ok := t[lang]; ok && s != "" {
		return s
	}
	if s, ok := t["en"]; ok && s != "" {
		return s
	}
	for _, s := range t {
		if s != "" {
			return s
		}
	}
	return ""
}
