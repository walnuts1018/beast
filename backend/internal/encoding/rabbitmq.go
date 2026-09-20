package encoding

import (
	"context"
	"encoding/json/v2"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Delivery struct {
	delivery amqp.Delivery
}

func (d Delivery) Body() []byte { return d.delivery.Body }

func (d Delivery) Ack() error { return d.delivery.Ack(false) }

func (d Delivery) Nack(requeue bool) error { return d.delivery.Nack(false, requeue) }

type RabbitMQ struct {
	connection    *amqp.Connection
	jobs          *amqp.Channel
	events        *amqp.Channel
	jobsQueueName string
	eventQueue    string
}

func NewRabbitMQ(url, jobQueue, eventQueue, consumerTag string) (*RabbitMQ, <-chan Delivery, error) {
	if url == "" || jobQueue == "" || eventQueue == "" || consumerTag == "" {
		return nil, nil, fmt.Errorf("RabbitMQ URL, queue names, and consumer tag are required")
	}
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	jobs, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("open RabbitMQ job channel: %w", err)
	}
	events, err := connection.Channel()
	if err != nil {
		_ = jobs.Close()
		_ = connection.Close()
		return nil, nil, fmt.Errorf("open RabbitMQ event channel: %w", err)
	}
	for channel, queueName := range map[*amqp.Channel]string{jobs: jobQueue, events: eventQueue} {
		if _, err := channel.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
			_ = events.Close()
			_ = jobs.Close()
			_ = connection.Close()
			return nil, nil, fmt.Errorf("declare RabbitMQ queue: %w", err)
		}
	}
	if err := events.Qos(1, 0, false); err != nil {
		_ = events.Close()
		_ = jobs.Close()
		_ = connection.Close()
		return nil, nil, fmt.Errorf("configure RabbitMQ QoS: %w", err)
	}
	deliveries, err := events.Consume(eventQueue, consumerTag, false, false, false, false, nil)
	if err != nil {
		_ = events.Close()
		_ = jobs.Close()
		_ = connection.Close()
		return nil, nil, fmt.Errorf("consume RabbitMQ events: %w", err)
	}
	result := make(chan Delivery, 1)
	go func() {
		defer close(result)
		for delivery := range deliveries {
			result <- Delivery{delivery: delivery}
		}
	}()
	return &RabbitMQ{connection: connection, jobs: jobs, events: events, jobsQueueName: jobQueue, eventQueue: eventQueue}, result, nil
}

func (r *RabbitMQ) PublishJob(ctx context.Context, job Job) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode encoder job: %w", err)
	}
	return r.jobs.PublishWithContext(ctx, "", r.jobsQueueName, false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body})
}

func (r *RabbitMQ) Close() error {
	var closeErr error
	if r.events != nil {
		closeErr = r.events.Close()
	}
	if r.jobs != nil {
		if err := r.jobs.Close(); closeErr == nil {
			closeErr = err
		}
	}
	if r.connection != nil {
		if err := r.connection.Close(); closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}
