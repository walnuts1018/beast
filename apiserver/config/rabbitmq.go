package config

type RabbitMQConfig struct {
	URL              string `env:"RABBITMQ_URL" envDefault:"amqp://guest:guest@rabbitmq.rabbitmq.svc.cluster.local:5672/" validate:"required"`
	EncodeJobQueue   string `env:"RABBITMQ_ENCODE_JOB_QUEUE" envDefault:"beast.encoder.jobs" validate:"required"`
	EncodeEventQueue string `env:"RABBITMQ_ENCODE_EVENT_QUEUE" envDefault:"beast.encoder.events" validate:"required"`
	ConsumerTag      string `env:"RABBITMQ_CONSUMER_TAG" envDefault:"beast-apiserver" validate:"required"`
	RequeueOnFail    bool   `env:"RABBITMQ_REQUEUE_ON_FAIL" envDefault:"true"`
}
