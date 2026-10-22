package watch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadImportsLegacy(t *testing.T) {
	old := t.TempDir()
	t.Setenv("VOIPINFO_HOME", old)
	_ = os.WriteFile(filepath.Join(old, "watch.toml"), []byte("# x\nproducts = [\"asterisk\"]\ntopics = [\"Регулирование\"]\nvendors = []\nwgs = [\"stir\"]\n"), 0o644)
	dir := t.TempDir()
	w, err := Load(dir, func(s string) string { return map[string]string{"Регулирование": "Regulation"}[s] })
	if err != nil || !w.Has(Products, "asterisk") || !w.Has(Topics, "Regulation") || !w.Has(WGs, "stir") {
		t.Fatalf("%+v %v", w, err)
	}
	w.Set(Products, "asterisk", false)
	w.Set(Vendors, "sangoma", true)
	w.Set(Vendors, "sangoma", true)
	if err := Save(dir, w); err != nil {
		t.Fatal(err)
	}
	w, _ = Load(dir, nil)
	if w.Has(Products, "asterisk") || len(w.Vendors) != 1 {
		t.Errorf("%+v", w)
	}
}
