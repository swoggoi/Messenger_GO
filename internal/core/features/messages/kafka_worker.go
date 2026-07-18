package messages

import (
	"context"
	"encoding/json"
	"log"

	"messenger/internal/core/domain"

	"github.com/segmentio/kafka-go"
)

type KafkaWorker struct {
	reader *kafka.Reader
	repo   *Repository
}

func NewKafkaWorker(reader *kafka.Reader, repo *Repository) *KafkaWorker {
	return &KafkaWorker{
		reader: reader,
		repo:   repo,
	}
}

func (w *KafkaWorker) Start(ctx context.Context) {
	go func() {
		for {
			m, err := w.reader.ReadMessage(ctx)
			if err != nil {
				log.Printf("Kafka read error: %v", err)
				continue
			}

			var msg domain.Message
			if err := json.Unmarshal(m.Value, &msg); err != nil {
				log.Printf("Kafka unmarshal error: %v", err)
				continue
			}

			if err := w.repo.CreateMessage(ctx, &msg); err != nil {
				log.Printf("Failed to save message to DB from Kafka: %v", err)
			}
		}
	}()
}
