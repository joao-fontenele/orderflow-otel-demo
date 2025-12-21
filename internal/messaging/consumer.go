package messaging

import (
	"context"

	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader *kafka.Reader
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
		reader: kafka.NewReader(cfg),
	}
}

func (c *Consumer) Consume(ctx context.Context, handler func(ctx context.Context, payload []byte) error) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			return err
		}

		if err := handler(ctx, msg.Value); err != nil {
			return err
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			return err
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
