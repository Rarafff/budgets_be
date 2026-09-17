package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type Config struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

type Subscription struct {
	Endpoint string       `json:"endpoint"`
	Keys     webpush.Keys `json:"keys"`
}

type Service struct {
	DB     *sql.DB
	Config Config
}

func (s Service) Enabled() bool {
	return strings.TrimSpace(s.Config.PublicKey) != "" && strings.TrimSpace(s.Config.PrivateKey) != ""
}

func (s Service) SaveSubscription(ctx context.Context, userID string, subscription Subscription) error {
	if !s.Enabled() {
		return errors.New("push notifications are not configured")
	}
	if strings.TrimSpace(subscription.Endpoint) == "" || strings.TrimSpace(subscription.Keys.Auth) == "" || strings.TrimSpace(subscription.Keys.P256dh) == "" {
		return errors.New("push subscription is invalid")
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
VALUES ($1, $2, $3, $4)
ON CONFLICT (endpoint) DO UPDATE
SET user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, updated_at = NOW()
`, userID, subscription.Endpoint, subscription.Keys.P256dh, subscription.Keys.Auth)
	return err
}

func (s Service) RemoveSubscription(ctx context.Context, userID, endpoint string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, endpoint)
	return err
}

func (s Service) SendDue(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT ps.id::text, ps.endpoint, ps.p256dh, ps.auth
FROM push_subscriptions ps
WHERE (ps.last_sent_at IS NULL OR ps.last_sent_at < NOW() - INTERVAL '24 hours')
	AND (
		EXISTS (SELECT 1 FROM bills b WHERE b.user_id = ps.user_id AND b.status IN ('upcoming', 'overdue') AND b.due_date <= CURRENT_DATE + 3)
		OR EXISTS (SELECT 1 FROM bills b WHERE b.user_id = ps.user_id AND b.auto_payment_failed_at >= NOW() - INTERVAL '24 hours')
		OR EXISTS (SELECT 1 FROM wallets w WHERE w.user_id = ps.user_id AND w.type NOT IN ('Credit Card', 'Paylater') AND w.minimum_balance > 0 AND w.balance <= w.minimum_balance)
	)
`)
	if err != nil {
		return err
	}
	defer rows.Close()

	payload, _ := json.Marshal(map[string]string{
		"title": "Budgets. Money moment",
		"body":  "Ada tagihan atau saldo dompet yang perlu diperiksa.",
		"url":   "/",
	})
	for rows.Next() {
		var id string
		var subscription Subscription
		if err := rows.Scan(&id, &subscription.Endpoint, &subscription.Keys.P256dh, &subscription.Keys.Auth); err != nil {
			return err
		}
		response, sendErr := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
			Endpoint: subscription.Endpoint,
			Keys:     subscription.Keys,
		}, &webpush.Options{
			Subscriber:      s.Config.Subject,
			VAPIDPublicKey:  s.Config.PublicKey,
			VAPIDPrivateKey: s.Config.PrivateKey,
			TTL:             86400,
		})
		if response != nil {
			response.Body.Close()
		}
		if sendErr == nil && response != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			_, _ = s.DB.ExecContext(ctx, `UPDATE push_subscriptions SET last_sent_at = NOW() WHERE id = $1`, id)
			continue
		}
		if response != nil && (response.StatusCode == 404 || response.StatusCode == 410) {
			_, _ = s.DB.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id)
		}
	}
	return rows.Err()
}

func (s Service) Start(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	go func() {
		_ = s.SendDue(ctx)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.SendDue(ctx)
			}
		}
	}()
}
