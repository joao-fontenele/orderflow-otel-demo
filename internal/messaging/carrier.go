package messaging

import "github.com/segmentio/kafka-go"

type MessageCarrier struct {
	msg *kafka.Message
}

func NewMessageCarrier(msg *kafka.Message) *MessageCarrier {
	return &MessageCarrier{msg: msg}
}

func (c *MessageCarrier) Get(key string) string {
	for _, h := range c.msg.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *MessageCarrier) Set(key, value string) {
	for i, h := range c.msg.Headers {
		if h.Key == key {
			c.msg.Headers[i].Value = []byte(value)
			return
		}
	}
	c.msg.Headers = append(c.msg.Headers, kafka.Header{
		Key:   key,
		Value: []byte(value),
	})
}

func (c *MessageCarrier) Keys() []string {
	keys := make([]string, len(c.msg.Headers))
	for i, h := range c.msg.Headers {
		keys[i] = h.Key
	}
	return keys
}
