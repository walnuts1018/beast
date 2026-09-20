package encoding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rabbitmq/amqp091-go"
	"github.com/walnuts1018/beast/backend/internal/domain"
)

type Job struct {
	SchemaVersion int                       `json:"schemaVersion"`
	VideoID       string                    `json:"videoId"`
	OwnerID       string                    `json:"ownerId"`
	ObjectKey     string                    `json:"objectKey"`
	Encryption    domain.EncryptionMetadata `json:"encryption"`
}

type Publisher interface {
	Publish(context.Context, Job) error
}

type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, Job) error { return nil }

type RabbitPublisher struct {
	connection *amqp091.Connection
	channel    *amqp091.Channel
	queue      string
}

func NewRabbitPublisher(url, queue string) (*RabbitPublisher, error) {
	if url == "" || queue == "" {
		return nil, fmt.Errorf("RabbitMQ URL and queue are required")
	}
	connection, err := amqp091.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if _, err := channel.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("declare encoding queue: %w", err)
	}
	return &RabbitPublisher{connection: connection, channel: channel, queue: queue}, nil
}

func (p *RabbitPublisher) Publish(ctx context.Context, job Job) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal encoding job: %w", err)
	}
	if err := p.channel.PublishWithContext(ctx, "", p.queue, false, false, amqp091.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		MessageId:    job.VideoID,
		Body:         body,
	}); err != nil {
		return fmt.Errorf("publish encoding job: %w", err)
	}
	return nil
}

func (p *RabbitPublisher) Close() error {
	channelErr := p.channel.Close()
	connectionErr := p.connection.Close()
	if channelErr != nil {
		return channelErr
	}
	return connectionErr
}

var _ Publisher = NoopPublisher{}
var _ Publisher = (*RabbitPublisher)(nil)
