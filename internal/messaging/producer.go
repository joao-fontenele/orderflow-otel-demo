package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var producerTracer = otel.Tracer("messaging/producer")

type Producer struct {
	writer *kafka.Writer
	topic  string
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		topic: topic,
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: true,
			BatchTimeout:           100 * time.Millisecond,
		},
	}
}

func (p *Producer) Publish(ctx context.Context, key string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Key:   []byte(key),
		Value: data,
	}

	ctx, span := producerTracer.Start(ctx, "send "+p.topic,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			semconv.MessagingSystemKafka,
			semconv.MessagingOperationName("send"),
			semconv.MessagingOperationTypePublish,
			semconv.MessagingDestinationName(p.topic),
			semconv.MessagingKafkaMessageKey(key),
		),
	)
	defer span.End()

	otel.GetTextMapPropagator().Inject(ctx, NewMessageCarrier(&msg))

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
