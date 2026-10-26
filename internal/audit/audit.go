// Package audit publishes URL shortening events to configured observers.
package audit

import (
	"context"
	"errors"
	"fmt"
)

const (
	// ActionShorten identifies successful creation of a short URL.
	ActionShorten = "shorten"
	// ActionFollow identifies a successful redirect through a short URL.
	ActionFollow = "follow"
)

// Event describes one auditable action.
type Event struct {
	Timestamp int64  `json:"ts"`
	Action    string `json:"action"`
	UserID    string `json:"user_id,omitempty"`
	URL       string `json:"url"`
}

// Observer receives audit events.
type Observer interface {
	Notify(ctx context.Context, event Event) error
}

// Publisher sends each event to every registered observer.
type Publisher struct {
	observers []Observer
}

// NewPublisher creates a publisher for the supplied observers.
func NewPublisher(observers ...Observer) *Publisher {
	registered := make([]Observer, 0, len(observers))
	for _, observer := range observers {
		if observer != nil {
			registered = append(registered, observer)
		}
	}

	return &Publisher{observers: registered}
}

// Notify sends an event to all observers and combines delivery errors.
func (p *Publisher) Notify(ctx context.Context, event Event) error {
	if p == nil {
		return nil
	}

	var errs []error
	for i, observer := range p.observers {
		if err := observer.Notify(ctx, event); err != nil {
			errs = append(errs, fmt.Errorf("audit observer %d: %w", i+1, err))
		}
	}

	return errors.Join(errs...)
}
