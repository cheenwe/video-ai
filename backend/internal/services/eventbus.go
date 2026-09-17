package services

import (
	"encoding/json"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
	"baseplatform/internal/models"
)

type Event struct {
	ID        string                 `json:"event_id"`
	Type      string                 `json:"event_type"`
	Timestamp string                 `json:"timestamp"`
	UserID    uint                   `json:"user_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}
type EventBus struct{ DB *gorm.DB }

func NewEventBus(db *gorm.DB) *EventBus { return &EventBus{DB: db} }

// Emit writes an outbox event before acknowledging it; dispatch is restartable.
func (b *EventBus) Emit(typ string, user *models.User, data map[string]interface{}) error {
	evt := Event{ID: uuid.NewString(), Type: typ, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Data: maskSensitive(data)}
	if user != nil {
		evt.UserID = user.ID
	}
	raw, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return b.DB.Create(&models.PlatformEvent{ID: evt.ID, Payload: string(raw), CreatedAt: time.Now().UTC()}).Error
}
