package queue

import (
	"context"
	"encoding/json/v2"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/walnuts1018/beast/encoder/internal/worker"
)

type RabbitMQ struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	eventQueue string
}

func New(url, jobQueue, eventQueue, consumerTag string) (*RabbitMQ, <-chan amqp.Delivery, error) {
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if _, err := channel.QueueDeclare(jobQueue, true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, nil, fmt.Errorf("declare job queue: %w", err)
	}
	if _, err := channel.QueueDeclare(eventQueue, true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, nil, fmt.Errorf("declare event queue: %w", err)
	}
	if err := channel.Qos(1, 0, false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, nil, fmt.Errorf("configure RabbitMQ QoS: %w", err)
	}
	deliveries, err := channel.Consume(jobQueue, consumerTag, false, false, false, false, nil)
	if err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, nil, fmt.Errorf("consume job queue: %w", err)
	}
	return &RabbitMQ{connection: connection, channel: channel, eventQueue: eventQueue}, deliveries, nil
}

func (r *RabbitMQ) Publish(ctx context.Context, event worker.Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	return r.channel.PublishWithContext(ctx, "", r.eventQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

func (r *RabbitMQ) Close() error {
	var closeErr error
	if r.channel != nil {
		closeErr = r.channel.Close()
	}
	if r.connection != nil {
		if err := r.connection.Close(); closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}
