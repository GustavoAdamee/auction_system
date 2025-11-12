package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Client struct {
	id                           string
	name                         string
	keys                         AssimetricKeys
	conn                         *amqp.Connection
	channel                      *amqp.Channel
	client_context               context.Context
	client_cancel                context.CancelFunc
	auction_started_queue        amqp.Queue
	auction_started_messages     <-chan amqp.Delivery
	subscribed_auctions_queue    amqp.Queue
	subscribed_auctions_messages <-chan amqp.Delivery
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

func (c *Client) init() {
	var err error
	c.conn, err = amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitM!")

	c.channel, err = c.conn.Channel()
	failOnError(err, "Failed to open a channel")

	//auction started exchange
	err = c.channel.ExchangeDeclare(
		"auction_started_exchange", // name
		"fanout",                   // type
		true,                       // durable
		false,                      // auto-deleted
		false,                      // internal
		false,                      // no-wait
		nil,                        // arguments
	)
	failOnError(err, "Failed to create exchange")
	c.auction_started_queue, err = c.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")
	err = c.channel.QueueBind(
		c.auction_started_queue.Name, // name
		"",                           // routing key
		"auction_started_exchange",   // exchange
		false,                        // no-wait
		nil,                          // arguments
	)
	failOnError(err, "Failed to bind a queue")

	//public keys exchange
	err = c.channel.ExchangeDeclare(
		"public_keys_exchange", // name
		"direct",               // type
		true,                   // durable
		false,                  // auto-deleted
		false,                  // internal
		false,                  // no-wait
		nil,                    // arguments
	)
	failOnError(err, "Failed to create exchange")

	//bids exchange
	err = c.channel.ExchangeDeclare(
		"bids_exchange", // name
		"direct",        // type
		true,            // durable
		false,           // auto-deleted
		false,           // internal
		false,           // no-wait
		nil,             // arguments
	)
	failOnError(err, "Failed to create exchange")

	//notification auction exchange
	err = c.channel.ExchangeDeclare(
		"notification_auction_exchange", // name
		"topic",                         // type
		true,                            // durable
		false,                           // auto-deleted
		false,                           // internal
		false,                           // no-wait
		nil,                             // arguments
	)
	failOnError(err, "Failed to create exchange")

	//subscribed auctions queue
	c.subscribed_auctions_queue, err = c.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")

	c.client_context, c.client_cancel = context.WithTimeout(context.Background(), 10*time.Second)

}

func (c *Client) generate_keys() {

	c.keys.Init()

	err := c.keys.Generate_private_key_pem_file("../private_keys/" + c.name + "_private_key")
	if err != nil {
		log.Fatalf("%s", err)
	}

	err = c.keys.Generate_public_key_pem_file("../public_keys/" + c.name + "_public_key")
	if err != nil {
		log.Fatalf("%s", err)
	}

}

func (c *Client) send_public_key() {
	// Convert public key to PEM format
	publicKeyPEM, err := c.keys.Get_public_key_pem()
	if err != nil {
		log.Fatalf("Failed to get public key PEM: %s", err)
	}

	body := map[string]interface{}{
		"id":         c.id,
		"public_key": string(publicKeyPEM),
	}
	json_body, err := json.Marshal(body)
	if err != nil {
		log.Fatalf("%s", err)
	}
	err = c.channel.PublishWithContext(c.client_context,
		"public_keys_exchange", // exchange
		"public_key",           // routing key
		false,                  // mandatory
		false,                  // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        json_body,
		})
	failOnError(err, "Failed to publish public key")
	log.Printf("Public key sent for client: %s", c.id)
}

func (c *Client) create_auction_binding(auction_id string) {
	err := c.channel.QueueBind(
		c.subscribed_auctions_queue.Name, // name
		auction_id,                       // routing key
		"notification_auction_exchange",  // exchange
		false,                            // no-wait
		nil,                              // arguments
	)
	failOnError(err, "Failed to bind a queue")
}

func (c *Client) send_bid(bid string, auction_id string) {
	var body = map[string]interface{}{
		"bid":        bid,
		"client_id":  c.id,
		"auction_id": auction_id,
	}

	json_body, err := json.Marshal(body)
	if err != nil {
		log.Fatalf("Error marshaling body: %s", err)
	}

	signature, err := c.keys.Sign_data(string(json_body))
	if err != nil {
		log.Fatalf("Error signing data: %s", err)
	}

	body["signature"] = signature

	final_json_body, err := json.Marshal(body)
	if err != nil {
		log.Fatalf("Error marshaling final body: %s", err)
	}

	err = c.channel.PublishWithContext(c.client_context,
		"bids_exchange",   // exchange
		"bid_routing_key", // routing key
		false,             // mandatory
		false,             // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        final_json_body,
		})
	if err != nil {
		log.Fatalf("Error publishing bid: %s", err)
	} else {
		c.create_auction_binding(auction_id)
	}

}

func (c *Client) get_bid() {
	for {
		fmt.Printf("\n\nEnter the bid and aution id: {bid auction_id}\n\n")
		var bid, auction_id string
		fmt.Scanln(&bid, &auction_id)
		if bid != "" {
			c.send_bid(bid, auction_id)
		}
	}
}

func (c *Client) listen_to_subscribed_auctions() {
	var err error
	c.subscribed_auctions_messages, err = c.channel.Consume(
		c.subscribed_auctions_queue.Name, // queue name
		"",                               // consumer
		true,                             // auto-ack
		false,                            // exclusive
		false,                            // no-local
		false,                            // no-wait
		nil,                              // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range c.subscribed_auctions_messages {
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			failOnError(err, "Failed to unmarshal body")
			if body["event_type"] == "new_valid_bid" {
				log.Printf("NEW VALID BID, \nAuction ID: %s\nHighest Bid: %.2f", body["auction_id"], body["bid"])
			}
			if body["event_type"] == "winner_bid" {
				log.Printf("AUCTION FINISHED, \nAuction ID: %s\nHighest Bid: %.2f", body["auction_id"], body["bid"])
			}
		}
	}()
}

func (c *Client) listen_to_auction_started() {
	var err error
	c.auction_started_messages, err = c.channel.Consume(
		c.auction_started_queue.Name, // queue name
		"",                           // consumer
		true,                         // auto-ack
		false,                        // exclusive
		false,                        // no-local
		false,                        // no-wait
		nil,                          // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range c.auction_started_messages {
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			failOnError(err, "Failed to unmarshal body")
			log.Printf("AUCTION STARTED, \nAuction ID: %s\nDescription: %s\nStart Time: %s\nEnd Time: %s", body["id"], body["description"], body["Start_time"], body["End_time"])
		}
	}()
}

func (c *Client) close() {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	if c.client_cancel != nil {
		c.client_cancel()
	}
}

func main() {
	// Each call of this function is a client
	id := os.Args[1]
	name := os.Args[2]
	var client = Client{id: id, name: name}
	log.Printf("Client initialized: %v", client)
	client.init()
	defer client.close()
	client.generate_keys()
	client.send_public_key()
	client.listen_to_subscribed_auctions()
	client.listen_to_auction_started()
	client.get_bid()

	select {}
}
