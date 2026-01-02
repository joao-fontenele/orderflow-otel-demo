package messaging

import (
	"context"
	"strconv"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var consumerTracer = otel.Tracer("messaging/consumer")

type Consumer struct {
	reader  *kafka.Reader
	topic   string
	groupID string
}

type ConsumerOption func(*kafka.ReaderConfig)

func WithStartOffset(offset int64) ConsumerOption {
	return func(cfg *kafka.ReaderConfig) {
		cfg.StartOffset = offset
	}
}

func NewConsumer(brokers []string, topic, groupID string, opts ...ConsumerOption) *Consumer {
	cfg := kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &Consumer{
		reader:  kafka.NewReader(cfg),
		topic:   topic,
		groupID: groupID,
	}
}

func (c *Consumer) Consume(ctx context.Context, handler func(ctx context.Context, payload []byte) error) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			return err
		}

		if err := c.processMessage(ctx, msg, handler); err != nil {
			return err
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			return err
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message, handler func(ctx context.Context, payload []byte) error) error {
	parentCtx := otel.GetTextMapPropagator().Extract(ctx, NewMessageCarrier(&msg))

	spanCtx, span := consumerTracer.Start(parentCtx, "process "+c.topic,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			semconv.MessagingSystemKafka,
			semconv.MessagingOperationName("process"),
			semconv.MessagingOperationTypeDeliver,
			semconv.MessagingDestinationName(c.topic),
			semconv.MessagingKafkaConsumerGroup(c.groupID),
			semconv.MessagingKafkaMessageOffset(int(msg.Offset)),
			semconv.MessagingDestinationPartitionID(strconv.Itoa(msg.Partition)),
			semconv.MessagingKafkaMessageKey(string(msg.Key)),
		),
	)
	defer span.End()

	if err := handler(spanCtx, msg.Value); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
