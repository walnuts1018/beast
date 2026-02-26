package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/walnuts1018/beast/apiserver/config"
	"github.com/walnuts1018/beast/apiserver/domain"
)

type Client struct {
	conn *amqp.Connection
	ch   *amqp.Channel
	cfg  config.RabbitMQConfig
}

type EncodeEventMessage struct {
	Type              string   `json:"type"`
	VideoID           string   `json:"video_id"`
	OwnerUserID       string   `json:"owner_user_id"`
	Percent           *float64 `json:"percent,omitempty"`
	Message           *string  `json:"message,omitempty"`
	ManifestObjectKey *string  `json:"manifest_object_key,omitempty"`
	EncodedObjectKey  *string  `json:"encoded_object_key,omitempty"`
	DurationMillis    *int     `json:"duration_millis,omitempty"`
	Width             *int     `json:"width,omitempty"`
	Height            *int     `json:"height,omitempty"`
}

func New(cfg config.RabbitMQConfig) (*Client, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	client := &Client{conn: conn, ch: ch, cfg: cfg}
	if err := client.declare(); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return client, nil
}

func (c *Client) Close() {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Client) declare() error {
	if _, err := c.ch.QueueDeclare(c.cfg.EncodeJobQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare encode job queue: %w", err)
	}
	if _, err := c.ch.QueueDeclare(c.cfg.EncodeEventQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare encode event queue: %w", err)
	}
	return nil
}

func (c *Client) PublishEncodeVideo(ctx context.Context, job domain.EncodeVideoJob) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal encode job: %w", err)
	}

	if err := c.ch.PublishWithContext(ctx, "", c.cfg.EncodeJobQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}); err != nil {
		return fmt.Errorf("publish encode job: %w", err)
	}

	return nil
}

func (c *Client) StartEventConsumer(ctx context.Context, handler func(context.Context, domain.EncodingEvent) error) error {
	deliveries, err := c.ch.Consume(c.cfg.EncodeEventQueue, c.cfg.ConsumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume encode event queue: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case delivery, ok := <-deliveries:
				if !ok {
					return
				}

				var msg EncodeEventMessage
				if err := json.Unmarshal(delivery.Body, &msg); err != nil {
					slog.ErrorContext(ctx, "failed to decode encoder event", slog.Any("error", err))
					_ = delivery.Nack(false, false)
					continue
				}

				event := domain.EncodingEvent{
					Type:              domain.EncodingEventType(msg.Type),
					VideoID:           msg.VideoID,
					OwnerUserID:       msg.OwnerUserID,
					Percent:           msg.Percent,
					Message:           msg.Message,
					ManifestObjectKey: msg.ManifestObjectKey,
					EncodedObjectKey:  msg.EncodedObjectKey,
					DurationMillis:    msg.DurationMillis,
					Width:             msg.Width,
					Height:            msg.Height,
				}

				if err := handler(ctx, event); err != nil {
					slog.ErrorContext(ctx, "failed to handle encoder event", slog.Any("error", err), slog.String("videoID", msg.VideoID))
					_ = delivery.Nack(false, c.cfg.RequeueOnFail)
					continue
				}

				if err := delivery.Ack(false); err != nil {
					slog.ErrorContext(ctx, "failed to ack encoder event", slog.Any("error", err))
				}
			}
		}
	}()

	return nil
}
