package models

import "ride-hail-system/internal/domain/types"

type StatusUpdateWebSocketMessage struct {
	EventType types.RideEvent `json:"event_type"`
	Data      any             `json:"data"`
}
