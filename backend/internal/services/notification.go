package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"baseplatform/internal/models"
	"baseplatform/internal/security"
)

type NotificationService struct {
	DB     *gorm.DB
	Bus    *EventBus
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewNotificationService(db *gorm.DB, bus *EventBus) *NotificationService {
	return &NotificationService{DB: db, Bus: bus}
}
func stableKey(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func (s *NotificationService) Notify(userID uint, typ, title, content, level, refType, refID, eventID string) error {
	if level == "" {
		level = "info"
	}
	n := models.Notification{UserID: userID, Type: typ, Title: title, Content: content, Level: level, RefType: refType, RefID: refID, EventID: eventID}
	if eventID != "" {
		key := stableKey(fmt.Sprintf("%s:%d", eventID, userID))
		n.DedupeKey = &key
	}
	return s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&n).Error
}
func (s *NotificationService) NotifyAdmins(typ, title, content, level string) error {
	var users []models.User
	if err := s.DB.Where("role IN ? AND disabled = 0", []string{"admin", "superadmin"}).Find(&users).Error; err != nil {
		return err
	}
	for _, u := range users {
		if err := s.Notify(u.ID, typ, title, content, level, "", "", ""); err != nil {
			return err
		}
	}
	return nil
}
func (s *NotificationService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			if err := s.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
				log.Printf("notification worker: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}
func (s *NotificationService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}
func (s *NotificationService) ProcessOnce(ctx context.Context) error {
	var events []models.PlatformEvent
	if err := s.DB.WithContext(ctx).Where("processed = 0").Order("created_at ASC").Limit(50).Find(&events).Error; err != nil {
		return err
	}
	for _, event := range events {
		if err := s.fanout(ctx, event); err != nil {
			return err
		}
	}
	var rows []models.WebhookDelivery
	now := time.Now().UTC()
	if err := s.DB.WithContext(ctx).Where(s.duePredicate(), "pending", now, "sending", now).Order("id ASC").Limit(20).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.attempt(ctx, row); err != nil {
			return err
		}
	}
	return nil
}
func (s *NotificationService) fanout(ctx context.Context, event models.PlatformEvent) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.PlatformEvent{}).Where("id = ? AND processed = 0", event.ID).Update("processed", 1)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		var evt Event
		if err := json.Unmarshal([]byte(event.Payload), &evt); err != nil {
			return err
		}
		var recipients []uint
		if evt.UserID != 0 {
			recipients = []uint{evt.UserID}
		} else {
			if err := tx.Model(&models.User{}).Where("role IN ? AND disabled = 0", []string{"superadmin", "admin"}).Pluck("id", &recipients).Error; err != nil {
				return err
			}
		}
		title, _ := evt.Data["title"].(string)
		if title == "" {
			title = evt.Type
		}
		content, _ := evt.Data["content"].(string)
		level, _ := evt.Data["level"].(string)
		for _, uid := range recipients {
			if err := NewNotificationService(tx, nil).Notify(uid, evt.Type, title, content, level, "", "", event.ID); err != nil {
				return err
			}
		}
		var endpoints []models.WebhookEndpoint
		if err := tx.Where("enabled = 1").Find(&endpoints).Error; err != nil {
			return err
		}
		for _, ep := range endpoints {
			if !s.matchEventType(ep, evt.Type) {
				continue
			}
			delivery := models.WebhookDelivery{EndpointID: ep.ID, DeliveryID: stableKey(fmt.Sprintf("%s:%d", event.ID, ep.ID)), EventType: evt.Type, Payload: event.Payload, Status: "pending"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&delivery).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *NotificationService) matchEventType(ep models.WebhookEndpoint, typ string) bool {
	if strings.TrimSpace(ep.EventTypes) == "" {
		return true
	}
	for _, v := range strings.Split(ep.EventTypes, ",") {
		if strings.TrimSpace(v) == typ {
			return true
		}
	}
	return false
}
func (s *NotificationService) attempt(ctx context.Context, row models.WebhookDelivery) error {
	now := time.Now().UTC()
	lease := now.Add(time.Minute)
	claimed := s.DB.WithContext(ctx).Model(&models.WebhookDelivery{}).Where("id = ? AND ("+s.duePredicate()+")", row.ID, "pending", now, "sending", now).Updates(map[string]any{"status": "sending", "next_retry_at": lease})
	if claimed.Error != nil {
		return claimed.Error
	}
	if claimed.RowsAffected == 0 {
		return nil
	}
	var ep models.WebhookEndpoint
	if err := s.DB.First(&ep, row.EndpointID).Error; err != nil || ep.Enabled == 0 {
		return s.DB.Model(&row).Updates(map[string]any{"status": "failed", "response_body": "端点不存在或已停用", "next_retry_at": nil}).Error
	}
	code, body, err := s.doPost(ctx, ep, row.EventType, row.DeliveryID, row.Payload)
	if ctx.Err() != nil {
		return s.DB.Model(&row).Updates(map[string]any{"status": "pending", "next_retry_at": time.Now().UTC()}).Error
	}
	attempts := row.RetryCount + 1
	updates := map[string]any{"status_code": code, "response_body": body, "retry_count": attempts, "next_retry_at": nil}
	if err == nil && code >= 200 && code < 300 {
		updates["status"] = "success"
	} else {
		updates["status"] = "failed"
		if err != nil {
			updates["response_body"] = "投递网络失败"
		}
		if attempts <= max(0, min(ep.MaxRetries, 5)) {
			delays := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}
			updates["status"] = "pending"
			updates["next_retry_at"] = time.Now().UTC().Add(delays[attempts-1])
		}
	}
	return s.DB.Model(&row).Updates(updates).Error
}
func (s *NotificationService) doPost(ctx context.Context, ep models.WebhookEndpoint, eventType, deliveryID, payload string) (int, string, error) {
	if err := security.ResolveAndValidatePublicHTTPURL(ep.URL); err != nil {
		return 0, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(max(1, min(ep.TimeoutSec, 30)))*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", ep.URL, bytes.NewBufferString(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Type", eventType)
	req.Header.Set("X-Delivery-ID", deliveryID)
	if ep.Secret != "" {
		mac := hmac.New(sha256.New, []byte(ep.Secret))
		mac.Write([]byte(payload))
		req.Header.Set("X-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return resp.StatusCode, string(body), err
}
func (s *NotificationService) Retry(id uint) error {
	result := s.DB.Model(&models.WebhookDelivery{}).Where("id = ? AND status = ?", id, "failed").Updates(map[string]any{"status": "pending", "retry_count": 0, "next_retry_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("仅失败的投递允许重试")
	}
	return nil
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// SQLite stores timestamps as text. Compare instants rather than lexical local/UTC representations.
func (s *NotificationService) duePredicate() string {
	before := "next_retry_at <= ?"
	if s.DB.Dialector.Name() == "sqlite" {
		before = "julianday(next_retry_at) <= julianday(?)"
	}
	return "(status = ? AND (next_retry_at IS NULL OR " + before + ")) OR (status = ? AND " + before + ")"
}
