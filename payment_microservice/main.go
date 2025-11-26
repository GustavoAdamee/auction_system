package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type PaymentService struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	winnerQueue amqp.Queue
	payments    map[string]*PaymentRecord
	mutex       sync.RWMutex
	externalURL string
	webhookURL  string
}

type PaymentRecord struct {
	AuctionID     string  `json:"auction_id"`
	ClientID      string  `json:"client_id"`
	Amount        float64 `json:"amount"`
	TransactionID string  `json:"transaction_id"`
	PaymentLink   string  `json:"payment_link"`
}

type WinnerEvent struct {
	AuctionID string  `json:"auction_id"`
	ClientID  string  `json:"client_id"`
	Bid       float64 `json:"bid"`
}

type ExternalPaymentResponse struct {
	TransactionID string `json:"transaction_id"`
	PaymentLink   string `json:"payment_link"`
}

type WebhookPayload struct {
	TransactionID string  `json:"transaction_id"`
	Status        string  `json:"status"`
	Amount        float64 `json:"amount"`
	ClientID      string  `json:"client_id"`
	AuctionID     string  `json:"auction_id"`
}

func NewPaymentService() *PaymentService {
	externalURL := getEnv("EXTERNAL_PAYMENT_URL", "http://localhost:8092/payments")
	webhookURL := getEnv("PAYMENT_WEBHOOK_URL", "http://localhost:8093/payments/webhook")
	return &PaymentService{
		payments:    make(map[string]*PaymentRecord),
		externalURL: externalURL,
		webhookURL:  webhookURL,
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func (ps *PaymentService) Init() error {
	var err error
	ps.conn, err = amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ps.channel, err = ps.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	exchanges := []struct {
		name string
		kind string
	}{
		{"winner_auction_exchange", "direct"},
		{"payment_link_exchange", "direct"},
		{"payment_status_exchange", "direct"},
	}

	for _, ex := range exchanges {
		if err := ps.channel.ExchangeDeclare(
			ex.name,
			ex.kind,
			true,
			false,
			false,
			false,
			nil,
		); err != nil {
			return fmt.Errorf("failed to declare exchange %s: %w", ex.name, err)
		}
	}

	ps.winnerQueue, err = ps.channel.QueueDeclare(
		"payment_winner_queue",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare winner queue: %w", err)
	}

	if err := ps.channel.QueueBind(
		ps.winnerQueue.Name,
		"winner_auction_routing_key",
		"winner_auction_exchange",
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to bind winner queue: %w", err)
	}

	return nil
}

func (ps *PaymentService) Close() {
	if ps.channel != nil {
		ps.channel.Close()
	}
	if ps.conn != nil {
		ps.conn.Close()
	}
}

func (ps *PaymentService) listenForWinners() {
	msgs, err := ps.channel.Consume(
		ps.winnerQueue.Name,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to register consumer: %v", err)
	}

	log.Println("Payment Microservice listening for winners...")
	for msg := range msgs {
		var event WinnerEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			log.Printf("Invalid winner event: %v", err)
			continue
		}
		go ps.processWinner(event)
	}
}

func (ps *PaymentService) processWinner(event WinnerEvent) {
	if event.AuctionID == "" || event.ClientID == "" || event.Bid <= 0 {
		log.Printf("Ignoring incomplete winner event: %+v", event)
		return
	}

	resp, err := ps.requestPaymentLink(event)
	if err != nil {
		log.Printf("Error requesting payment link: %v", err)
		return
	}

	record := &PaymentRecord{
		AuctionID:     event.AuctionID,
		ClientID:      event.ClientID,
		Amount:        event.Bid,
		TransactionID: resp.TransactionID,
		PaymentLink:   resp.PaymentLink,
	}

	ps.mutex.Lock()
	ps.payments[resp.TransactionID] = record
	ps.mutex.Unlock()

	ps.publishPaymentLink(record)
}

func (ps *PaymentService) requestPaymentLink(event WinnerEvent) (*ExternalPaymentResponse, error) {
	payload := map[string]interface{}{
		"auction_id":   event.AuctionID,
		"client_id":    event.ClientID,
		"amount":       event.Bid,
		"callback_url": ps.webhookURL,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(ps.externalURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("external payment service returned %d", resp.StatusCode)
	}

	var extResp ExternalPaymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&extResp); err != nil {
		return nil, err
	}

	return &extResp, nil
}

func (ps *PaymentService) publishPaymentLink(record *PaymentRecord) {
	event := map[string]interface{}{
		"auction_id":     record.AuctionID,
		"client_id":      record.ClientID,
		"amount":         record.Amount,
		"payment_link":   record.PaymentLink,
		"transaction_id": record.TransactionID,
	}

	ps.publishEvent("payment_link_exchange", "payment_link_routing_key", event)
}

func (ps *PaymentService) publishPaymentStatus(record *PaymentRecord, status string) {
	event := map[string]interface{}{
		"auction_id":     record.AuctionID,
		"client_id":      record.ClientID,
		"amount":         record.Amount,
		"transaction_id": record.TransactionID,
		"status":         status,
	}

	ps.publishEvent("payment_status_exchange", "payment_status_routing_key", event)
}

func (ps *PaymentService) publishEvent(exchange, routingKey string, payload map[string]interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal event for %s: %v", exchange, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ps.channel.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	); err != nil {
		log.Printf("Failed to publish event to %s: %v", exchange, err)
	}
}

func (ps *PaymentService) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
		return
	}

	ps.mutex.RLock()
	record, exists := ps.payments[payload.TransactionID]
	ps.mutex.RUnlock()

	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "transaction not found"})
		return
	}

	status := payload.Status
	if status == "" {
		status = "unknown"
	}

	ps.publishPaymentStatus(record, status)
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

func (ps *PaymentService) startHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/payments/webhook", ps.handleWebhook)

	server := &http.Server{
		Addr:    ":8093",
		Handler: mux,
	}

	log.Println("Payment Microservice HTTP server started on port 8093")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Payment HTTP server error: %v", err)
	}
}

func main() {
	service := NewPaymentService()
	if err := service.Init(); err != nil {
		log.Fatalf("Failed to init payment service: %v", err)
	}
	defer service.Close()

	go service.startHTTPServer()
	service.listenForWinners()
}
