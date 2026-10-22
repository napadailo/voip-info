package hub

import (
	"strings"

	"github.com/napadailo/voip-info/internal/ietf"
	"github.com/napadailo/voip-info/internal/radar"
	"github.com/napadailo/voip-info/internal/watch"
)

// legacyTopics are the topic names of the Python version, in radar.Topics order.
var legacyTopics = []string{"Voice AI / LLM-агенты", "STIR/SHAKEN, caller ID", "Мошенничество, toll fraud", "Media over QUIC",
	"WebRTC, WHIP/WHEP", "TLS / SRTP / шифрование", "Постквантовая криптография", "Кодеки (Opus, AV1, H.265)", "IMS / VoLTE / 5G",
	"RCS и мессенджинг", "Microsoft Teams, Direct Routing", "Kubernetes / cloud-native", "Отказ от меди / PSTN", "Регулирование",
	"Контакт-центры / CCaaS", "Экстренные вызовы", "STT / TTS / транскрибация", "Уязвимости и атаки", "Сделки и инвестиции"}

// LoadWatch reads the subscription from the data directory.
func (h *Hub) LoadWatch() error {
	w, err := watch.Load(h.DataDir, func(name string) string {
		for i, old := range legacyTopics {
			if old == name && i < len(radar.Topics) {
				return radar.Topics[i].Name
			}
		}
		return name
	})
	h.Watch = w
	return err
}

// Why a product is in my stack.
const (
	WhyWatch = "watch" // subscribed
	WhyMine  = "mine"  // in my versions
	WhyDep   = "dep"   // a dependency of a product of the stack
)

// Reason a product is watched; Of is the product it is a dependency of.
type Reason struct{ Why, Of string }

// WatchedProducts are the products of my stack: subscribed, in my versions
// and the dependencies of those.
func (h *Hub) WatchedProducts() map[string]Reason {
	out := map[string]Reason{}
	if h.Watch != nil {
		for _, id := range h.Watch.Products {
			if h.States[id] != nil {
				out[id] = Reason{Why: WhyWatch}
			}
		}
	}
	for _, id := range h.Inventory.IDs() {
		if _, ok := out[id]; !ok && h.States[id] != nil {
			out[id] = Reason{Why: WhyMine}
		}
	}
	roots := make([]string, 0, len(out))
	for id := range out {
		roots = append(roots, id)
	}
	for _, id := range roots {
		for _, hop := range h.Graph.Upstream(id) {
			if _, ok := out[hop.Edge.Dst]; !ok {
				out[hop.Edge.Dst] = Reason{Why: WhyDep, Of: id}
			}
		}
	}
	return out
}

// HasWatch reports whether my stack has anything in it.
func (h *Hub) HasWatch() bool {
	return len(h.WatchedProducts()) > 0 || h.Watch != nil && !h.Watch.Empty()
}

// Watching reports whether an item is subscribed explicitly.
func (h *Hub) Watching(kind, id string) bool { return h.Watch != nil && h.Watch.Has(kind, id) }

// SetWatch subscribes to an item or drops it and saves the subscription.
func (h *Hub) SetWatch(kind, id string, on bool) error {
	if h.Watch == nil {
		h.Watch = &watch.Watch{}
	}
	h.Watch.Set(kind, id, on)
	return watch.Save(h.DataDir, h.Watch)
}

// ResolveWatch turns free input into a subscription: a product or vendor id,
// an IETF working group, or a unique part of a topic name (English or as shown).
func (h *Hub) ResolveWatch(q string, shown func(string) string) (kind, id string, ok bool) {
	q = strings.TrimSpace(q)
	low := strings.ToLower(q)
	if h.States[low] != nil {
		return watch.Products, low, true
	}
	for _, v := range h.Vendors {
		if v.ID == low {
			return watch.Vendors, low, true
		}
	}
	if ietf.WGDesc(low) != "" {
		return watch.WGs, low, true
	}
	var hits []string
	for _, t := range radar.Topics {
		if strings.Contains(strings.ToLower(t.Name), low) || shown != nil && strings.Contains(strings.ToLower(shown(t.Name)), low) {
			hits = append(hits, t.Name)
		}
	}
	if len(hits) == 1 {
		return watch.Topics, hits[0], true
	}
	return "", "", false
}
