package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Client struct {
	conn           *amqp.Connection
	ch             *amqp.Channel
	encodeJobQueue string
	encodeEvtQueue string
	consumerTag    string
}

type EncodeJobMessage struct {
	VideoID         string `json:"video_id"`
	OwnerUserID     string `json:"owner_user_id"`
	SourceObjectKey string `json:"source_object_key"`
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

func New(url string, encodeJobQueue string, encodeEvtQueue string, consumerTag string) (*Client, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	client := &Client{
		conn:           conn,
		ch:             ch,
		encodeJobQueue: encodeJobQueue,
		encodeEvtQueue: encodeEvtQueue,
		consumerTag:    consumerTag,
	}

	if err := client.declare(); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return client, nil
}

func (c *Client) declare() error {
	if _, err := c.ch.QueueDeclare(c.encodeJobQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare encode job queue: %w", err)
	}
	if _, err := c.ch.QueueDeclare(c.encodeEvtQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare encode event queue: %w", err)
	}
	return nil
}

func (c *Client) Close() {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Client) ConsumeJobs(ctx context.Context) (<-chan amqp.Delivery, error) {
	msgs, err := c.ch.Consume(c.encodeJobQueue, c.consumerTag, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("consume encode jobs: %w", err)
	}
	return msgs, nil
}

func (c *Client) DecodeJob(body []byte) (EncodeJobMessage, error) {
	var msg EncodeJobMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return EncodeJobMessage{}, fmt.Errorf("decode encode job: %w", err)
	}
	return msg, nil
}

func (c *Client) PublishEvent(ctx context.Context, event EncodeEventMessage) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal encode event: %w", err)
	}

	if err := c.ch.PublishWithContext(ctx, "", c.encodeEvtQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}); err != nil {
		return fmt.Errorf("publish encode event: %w", err)
	}

	return nil
}
