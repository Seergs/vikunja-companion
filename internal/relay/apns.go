package relay

import (
	"context"
	"fmt"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

// APNSConfig is the Apple credential set for the relay's .p8 key.
type APNSConfig struct {
	KeyPath string
	KeyID   string
	TeamID  string
	Topic   string // the app's bundle id
	Sandbox bool   // use the APNs sandbox gateway instead of production
}

// apnsClient is the subset of *apns2.Client this package uses (seam for tests).
type apnsClient interface {
	PushWithContext(ctx apns2.Context, n *apns2.Notification) (*apns2.Response, error)
}

// APNS is an APNSSender backed by a real APNs HTTP/2 connection.
type APNS struct {
	client apnsClient
	topic  string
}

// NewAPNS builds an APNS sender from a .p8 key on disk.
func NewAPNS(cfg APNSConfig) (*APNS, error) {
	authKey, err := token.AuthKeyFromFile(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("relay: loading APNs key %s: %w", cfg.KeyPath, err)
	}
	c := apns2.NewTokenClient(&token.Token{
		AuthKey: authKey,
		KeyID:   cfg.KeyID,
		TeamID:  cfg.TeamID,
	})
	if cfg.Sandbox {
		c = c.Development()
	} else {
		c = c.Production()
	}
	return &APNS{client: c, topic: cfg.Topic}, nil
}

// Send delivers payload to deviceToken. It returns ErrBadDeviceToken when APNs
// reports the token is invalid or no longer registered, so the relay can tell
// the companion to drop the device.
func (a *APNS) Send(ctx context.Context, deviceToken string, payload []byte, collapseID string) error {
	n := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       a.topic,
		Payload:     payload,
		CollapseID:  collapseID,
		PushType:    apns2.PushTypeAlert,
		Priority:    apns2.PriorityHigh,
	}

	res, err := a.client.PushWithContext(ctx, n)
	if err != nil {
		return fmt.Errorf("relay: apns push: %w", err)
	}
	if res.Sent() {
		return nil
	}
	switch res.Reason {
	case apns2.ReasonBadDeviceToken, apns2.ReasonUnregistered, apns2.ReasonDeviceTokenNotForTopic:
		return fmt.Errorf("%w: %s", ErrBadDeviceToken, res.Reason)
	default:
		return fmt.Errorf("relay: apns rejected (%d): %s", res.StatusCode, res.Reason)
	}
}
