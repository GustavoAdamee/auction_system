package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AuctionStatus represents the status of an auction
type AuctionStatus string

// Define the possible auction statuses
const (
	StatusIdle     AuctionStatus = "idle"
	StatusActive   AuctionStatus = "active"
	StatusFinished AuctionStatus = "finished"
)

type Auction struct {
	id              string
	description     string
	Start_time      time.Time
	End_time        time.Time
	Status          AuctionStatus
	conn            *amqp.Connection
	channel         *amqp.Channel
	auction_context context.Context
	auction_cancel  context.CancelFunc
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

func (a *Auction) Init() {
	fmt.Printf("AUCTION INITIALIZED: \n ID: %v \n description: %v \n", a.id, a.description)
	var err error
	a.conn, err = amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitMQ!")

	a.channel, err = a.conn.Channel()
	failOnError(err, "Failed to open a channel")

	//auction started exchange
	err = a.channel.ExchangeDeclare(
		"auction_started_exchange", // name
		"fanout",                   // type
		true,                       // durable
		false,                      // auto-deleted
		false,                      // internal
		false,                      // no-wait
		nil,                        // arguments
	)
	failOnError(err, "Failed to create exchange")

	//auction finished exchange
	err = a.channel.ExchangeDeclare(
		"auction_finished_exchange", // name
		"direct",                    // type
		true,                        // durable
		false,                       // auto-deleted
		false,                       // internal
		false,                       // no-wait
		nil,                         // arguments
	)
	failOnError(err, "Failed to create exchange")

	// Create context that lasts for the auction duration plus buffer
	auctionDuration := a.End_time.Sub(a.Start_time)
	contextDuration := auctionDuration + 1*time.Minute // Add 1 minute buffer
	a.auction_context, a.auction_cancel = context.WithTimeout(context.Background(), contextDuration)
}

// Add cleanup method to be called when auction is done
func (a *Auction) Close() {
	if a.auction_cancel != nil {
		a.auction_cancel()
	}
	if a.channel != nil {
		a.channel.Close()
	}
	if a.conn != nil {
		a.conn.Close()
	}
}

// Send auction_started to rabbitMQ
func (a *Auction) StartAuction() {
	body := map[string]interface{}{
		"id":          a.id,
		"description": a.description,
		"Start_time":  a.Start_time,
		"End_time":    a.End_time,
		"Status":      StatusActive,
	}

	json_body, err := json.Marshal(body)
	failOnError(err, "Failed to marshal body")

	err = a.channel.PublishWithContext(a.auction_context,
		"auction_started_exchange", // exchange
		"",                         // routing key
		false,                      // mandatory
		false,                      // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        json_body,
		})
	failOnError(err, "Failed to publish auction started")
}

// Send auction_finished to rabbitMQ
func (a *Auction) FinishAuction() {
	body := map[string]interface{}{
		"id":          a.id,
		"description": a.description,
		"Start_time":  a.Start_time,
		"End_time":    a.End_time,
		"Status":      StatusFinished,
	}

	json_body, err := json.Marshal(body)
	failOnError(err, "Failed to marshal body")

	err = a.channel.PublishWithContext(a.auction_context,
		"auction_finished_exchange",    // exchange
		"auction_finished_routing_key", // routing key
		false,                          // mandatory
		false,                          // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        json_body,
		})
	failOnError(err, "Failed to publish auction finished")
}
