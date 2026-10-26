package easytier

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// collectCoreEvents uses WebClientService method 4 in v2.6.4. It polls the
// bounded Core event ring, as the official GUI does, not a log parser.
func collectCoreEvents(ctx context.Context, address string) ([]NetworkEvent, error) {
	return collectCoreEventsFor(ctx, address, "")
}
func collectCoreEventsFor(ctx context.Context, address, instanceID string) ([]NetworkEvent, error) {
	data, err := managementRPC(ctx, address, "WebClientService", 4, nil)
	if err != nil {
		return nil, err
	}
	fields, err := wireFields(data)
	if err != nil {
		return nil, err
	}
	var events []NetworkEvent
	for _, outer := range fields {
		if outer.number != 1 {
			continue
		}
		maps, err := wireFields(outer.bytes)
		if err != nil {
			return nil, err
		}
		for _, entry := range maps {
			if entry.number != 1 {
				continue
			}
			pair, err := wireFields(entry.bytes)
			if err != nil {
				return nil, err
			}
			id := ""
			for _, key := range pair {
				if key.number == 1 {
					id = string(key.bytes)
				}
			}
			if instanceID != "" && id != instanceID {
				continue
			}
			for _, value := range pair {
				if value.number != 2 {
					continue
				}
				info, err := wireFields(value.bytes)
				if err != nil {
					return nil, err
				}
				for _, field := range info {
					if field.number == 3 {
						event, err := decodeCoreEvent(field.bytes)
						if err != nil {
							return nil, err
						}
						events = append(events, event)
					}
				}
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
	return events, nil
}
func decodeCoreEvent(data []byte) (NetworkEvent, error) {
	var event struct {
		Time  time.Time       `json:"time"`
		Event json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return NetworkEvent{}, err
	}
	// Serde encodes unit variants as a JSON string.
	var unit string
	if json.Unmarshal(event.Event, &unit) == nil && unit != "" {
		return NetworkEvent{event.Time, unit, "", locale.T("Core 事件")}, nil
	}
	var variants map[string]json.RawMessage
	if err := json.Unmarshal(event.Event, &variants); err != nil || len(variants) != 1 {
		return NetworkEvent{}, fmt.Errorf("unknown Core event encoding")
	}
	for kind, value := range variants {
		return NetworkEvent{event.Time, kind, redact(string(value)), locale.T("Core 事件")}, nil
	}
	return NetworkEvent{}, fmt.Errorf("empty Core event")
}
