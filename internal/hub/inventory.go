package hub

import (
	"sort"

	"github.com/napadailo/voip-info/internal/inventory"
	"github.com/napadailo/voip-info/internal/model"
)

// LoadInventory reads my versions from the data directory.
func (h *Hub) LoadInventory() error {
	inv, err := inventory.Load(h.DataDir)
	h.Inventory = inv
	return err
}

// SetVersion records the version of a product in use; "" or "-" removes it.
func (h *Hub) SetVersion(id, version, note string) error {
	if h.Inventory == nil {
		h.Inventory = inventory.Inventory{}
	}
	if version == "" || version == "-" {
		delete(h.Inventory, id)
	} else {
		h.Inventory[id] = inventory.Entry{Version: version, Note: note}
	}
	return inventory.Save(h.DataDir, h.Inventory)
}

// Assess my version of a product; nil when it is not in the inventory.
func (h *Hub) Assess(id string) *inventory.Assessment {
	e, ok := h.Inventory[id]
	st := h.States[id]
	if !ok || st == nil {
		return nil
	}
	return inventory.Assess(st, e.Version, e.Note, inventory.Context{
		Vulns:   h.Vulns(id),
		KEV:     func(v *model.Vuln) bool { return h.KEVOf(v) != nil },
		Support: h.Support(st),
	})
}

// Assessments of the inventory, the most urgent first.
func (h *Hub) Assessments() []*inventory.Assessment {
	var out []*inventory.Assessment
	for _, id := range h.Inventory.IDs() {
		if a := h.Assess(id); a != nil {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Urgency > out[j].Urgency })
	return out
}
