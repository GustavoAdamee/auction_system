from flask import Flask, request, jsonify, Response, stream_with_context
from flask_cors import CORS
import pika
import json
import requests
import threading
import queue
from collections import defaultdict

app = Flask(__name__)
CORS(app)

class APIGateway:
    def __init__(self):
        self.sse_clients = defaultdict(list)  # map[auction_id] -> list of (client_id, queue)
        self.client_interests = defaultdict(set)  # map[auction_id] -> set of client_ids
        self.auction_url = "http://localhost:8080/auctions"  # Auction MS (assuming port 8080)
        self.bid_url = "http://localhost:8084/bids"  # Bids MS (assuming port 8084)
        self.connection = None
        self.channel = None
        self.consumer_started = False
        self.event_id_counter = 0  # Counter for SSE event IDs
        self.setup_rabbitmq()
    
    def setup_rabbitmq(self):
        """Setup RabbitMQ connection"""
        try:
            self.connection = pika.BlockingConnection(
                pika.ConnectionParameters('localhost')
            )
            self.channel = self.connection.channel()
            
            # Declare all exchanges
            exchanges = [
                ('valid_bids_exchange', 'direct'),
                ('invalid_bids_exchange', 'direct'),
                ('winner_auction_exchange', 'direct')
            ]
            
            for exchange_name, exchange_type in exchanges:
                self.channel.exchange_declare(
                    exchange=exchange_name,
                    exchange_type=exchange_type,
                    durable=True
                )
            
            # Setup event consumers
            self.setup_event_consumers()
            
            # Start consuming in single background thread
            if not self.consumer_started:
                thread = threading.Thread(target=self.start_consuming)
                thread.daemon = True
                thread.start()
                self.consumer_started = True
            
        except Exception as e:
            print(f"Error setting up RabbitMQ: {e}")
    
    def setup_event_consumers(self):
        """Setup event consumers"""
        # Valid bids events
        self.consume_events(
            'valid_bids_exchange',
            'valid_bid_routing_key',
            'api_gateway_valid_bids_queue',
            self.handle_valid_bid
        )
        
        # Invalid bids events
        self.consume_events(
            'invalid_bids_exchange',
            'invalid_bid_routing_key',
            'api_gateway_invalid_bids_queue',
            self.handle_invalid_bid
        )
        
        # Winner auction events
        self.consume_events(
            'winner_auction_exchange',
            'winner_auction_routing_key',
            'api_gateway_winner_auction_queue',
            self.handle_winner_auction
        )
    
    def consume_events(self, exchange, routing_key, queue_name, callback):
        """Setup event consumption"""
        try:
            self.channel.queue_declare(queue=queue_name, durable=True)
            
            if routing_key:
                self.channel.queue_bind(
                    exchange=exchange,
                    queue=queue_name,
                    routing_key=routing_key
                )
            
            def on_message(ch, method, properties, body):
                try:
                    # Decode body if it's bytes (pika sometimes returns bytes, sometimes str)
                    if isinstance(body, bytes):
                        body_str = body.decode('utf-8')
                    else:
                        body_str = str(body)
                    print(f"[RABBITMQ] Received message on queue {queue_name}: {body_str[:200]}")
                    callback(body_str)
                except Exception as e:
                    print(f"[ERROR] Error in on_message callback for {queue_name}: {e}")
                    import traceback
                    traceback.print_exc()
            
            self.channel.basic_consume(
                queue=queue_name,
                on_message_callback=on_message,
                auto_ack=True
            )
            
        except Exception as e:
            print(f"Error setting up consumer for {queue_name}: {e}")
    
    def start_consuming(self):
        """Start consuming messages"""
        try:
            print("Starting RabbitMQ message consumption...")
            self.channel.start_consuming()
        except Exception as e:
            print(f"Error consuming messages: {e}")
    
    def handle_valid_bid(self, body):
        """Process valid bid event"""
        try:
            event = json.loads(body)
            event['event_type'] = 'new_bid'
            auction_id = event.get('auction_id')
            if auction_id:
                # Only send to registered clients
                if auction_id in self.client_interests and self.client_interests[auction_id]:
                    self.broadcast_sse_to_auction(auction_id, 'new_bid', event)
        except Exception as e:
            print(f"Error processing valid_bid: {e}")
    
    def handle_invalid_bid(self, body):
        """Process invalid bid event"""
        try:
            event = json.loads(body)
            event['event_type'] = 'invalid_bid'
            auction_id = event.get('auction_id')
            client_id = event.get('client_id')
            if auction_id and client_id:
                self.broadcast_sse_to_client(client_id, 'invalid_bid', event)
        except Exception as e:
            print(f"Error processing invalid_bid: {e}")
    
    def handle_winner_auction(self, body):
        """Process auction winner event"""
        try:
            event = json.loads(body)
            event['event_type'] = 'auction_winner'
            auction_id = event.get('auction_id')
            if auction_id:
                if auction_id in self.client_interests and self.client_interests[auction_id]:
                    self.broadcast_sse_to_auction(auction_id, 'auction_winner', event)
        except Exception as e:
            print(f"Error processing winner_auction: {e}")
    
    def broadcast_sse_to_client(self, client_id, event_type, data):
        """Broadcast SSE event to a specific client"""
        message = {
            "event": event_type,
            "data": data
        }
        json_data = json.dumps(message)
        
        # Format SSE message with event type and ID
        self.event_id_counter += 1
        sse_message = {
            "event_type": event_type,
            "event_id": self.event_id_counter,
            "payload": json_data
        }
        
        for auction_id, clients in list(self.sse_clients.items()):
            for client_info in list(clients):
                if isinstance(client_info, tuple):
                    cid, client_queue = client_info
                    if cid == client_id:
                        try:
                            client_queue.put_nowait(sse_message)
                        except:
                            pass
    
    def broadcast_sse_to_auction(self, auction_id, event_type, data):
        """Broadcast SSE event to clients registered for a specific auction"""
        if auction_id not in self.client_interests or not self.client_interests[auction_id]:
            return
        
        message = {
            "event": event_type,
            "data": data
        }
        json_data = json.dumps(message)
        
        self.event_id_counter += 1
        sse_message = {
            "event_type": event_type,
            "event_id": self.event_id_counter,
            "payload": json_data
        }
        
        clients = list(self.sse_clients.get(auction_id, []))
        registered_client_ids = self.client_interests[auction_id]
        
        for client_info in clients:
            if isinstance(client_info, tuple):
                cid, client_queue = client_info
                if cid in registered_client_ids:
                    try:
                        client_queue.put_nowait(sse_message)
                    except:
                        pass
    
    def add_sse_client(self, auction_id, client_id=None):
        """Add SSE client"""
        client_queue = queue.Queue()
        if client_id:
            self.sse_clients[auction_id].append((client_id, client_queue))
            self.client_interests[auction_id].add(client_id)
        else:
            self.sse_clients[auction_id].append(client_queue)
        return client_queue
    
    def remove_sse_client(self, auction_id, client_queue, client_id=None):
        """Remove SSE client"""
        if auction_id in self.sse_clients:
            try:
                if client_id:
                    self.sse_clients[auction_id] = [
                        (cid, q) for cid, q in self.sse_clients[auction_id]
                        if not (cid == client_id and q == client_queue)
                    ]
                    self.client_interests[auction_id].discard(client_id)
                else:
                    self.sse_clients[auction_id] = [
                        q for q in self.sse_clients[auction_id] if q != client_queue
                    ]
            except ValueError:
                pass

# Global API Gateway instance
gateway = APIGateway()

@app.route('/api/auctions', methods=['POST'])
def create_auction():
    """Create a new auction"""
    data = request.get_json()
    
    if not data:
        return jsonify({"error": "Request body is required"}), 400
    
    # Validate required fields
    required_fields = ['id', 'description', 'start_time', 'end_time']
    missing_fields = [field for field in required_fields if field not in data]
    if missing_fields:
        return jsonify({"error": f"Missing required fields: {', '.join(missing_fields)}"}), 400
    
    # Forward to Auction MS via REST
    try:
        response = requests.post(
            gateway.auction_url,
            json=data,
            timeout=10
        )
        return jsonify(response.json()), response.status_code
    except Exception as e:
        return jsonify({"error": "Failed to create auction"}), 500

@app.route('/api/auctions', methods=['GET'])
def get_active_auctions():
    """Get active auctions"""
    try:
        # Forward to Auction MS via REST
        response = requests.get(
            f"{gateway.auction_url}",
            timeout=10
        )
        return jsonify(response.json()), response.status_code
    except Exception as e:
        return jsonify({"error": "Failed to get auctions"}), 500

@app.route('/api/auctions/<auction_id>/bids', methods=['POST'])
def handle_bid(auction_id):
    """Endpoint to submit a bid"""
    if not auction_id:
        return jsonify({"error": "Auction ID is required"}), 400
    
    data = request.get_json()
    if not data:
        return jsonify({"error": "Request body is required"}), 400
    
    bid = data.get('bid')
    client_id = data.get('client_id')
    
    if not all([bid, client_id]):
        return jsonify({"error": "Missing required fields: bid and client_id"}), 400
    
    # Validate bid is a number
    try:
        float(bid)
    except (ValueError, TypeError):
        return jsonify({"error": "Bid must be a valid number"}), 400
    
    # Send to Bids MS via REST
    try:
        bid_data = {
            "auction_id": auction_id,
            "client_id": client_id,
            "bid": bid
        }
        response = requests.post(
            gateway.bid_url,
            json=bid_data,
            timeout=10
        )
        return jsonify(response.json()), response.status_code
    except Exception as e:
        return jsonify({"error": "Failed to submit bid"}), 500

@app.route('/api/auctions/<auction_id>/events', methods=['GET'])
def handle_sse(auction_id):
    """SSE endpoint for real-time events"""
    if not auction_id:
        return jsonify({"error": "Auction ID is required"}), 400
    
    client_id = request.args.get('client_id')
    client_queue = gateway.add_sse_client(auction_id, client_id)
    
    def generate():
        # Send initial keepalive immediately to establish connection
        # This ensures HTTP headers are sent right away
        yield ": keepalive\n\n"
        
        try:
            while True:
                try:
                    sse_message = client_queue.get(timeout=60)
                    # Format SSE according to specification:
                    # event: <event_type>
                    # id: <event_id>
                    # data: <json_data>
                    event_type = sse_message.get('event_type', 'message')
                    event_id = sse_message.get('event_id', '')
                    payload = sse_message.get('payload', '{}')
                    
                    # Build SSE formatted message
                    sse_output = f"event: {event_type}\n"
                    if event_id:
                        sse_output += f"id: {event_id}\n"
                    sse_output += f"data: {payload}\n\n"
                    
                    yield sse_output
                except queue.Empty:
                    # Send keepalive comment (prevents connection timeout)
                    yield ": keepalive\n\n"
                except Exception as e:
                    print(f"SSE error: {e}")
                    break
        finally:
            gateway.remove_sse_client(auction_id, client_queue, client_id)
    
    return Response(
        stream_with_context(generate()),
        mimetype='text/event-stream',
        headers={
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive',
            'Access-Control-Allow-Origin': '*'
        }
    )

@app.route('/api/auctions/<auction_id>/register-interest', methods=['POST'])
def register_interest(auction_id):
    """Register interest in receiving notifications about an auction"""
    if not auction_id:
        return jsonify({"error": "Auction ID is required"}), 400
    
    data = request.get_json()
    if not data:
        return jsonify({"error": "Request body is required"}), 400
    
    client_id = data.get('client_id')
    
    if not client_id:
        return jsonify({"error": "Missing client_id"}), 400
    
    gateway.client_interests[auction_id].add(client_id)
    return jsonify({"message": "Interest registered successfully"}), 200

@app.route('/api/auctions/<auction_id>/cancel-interest', methods=['POST'])
def cancel_interest(auction_id):
    """Cancel interest in receiving notifications about an auction"""
    if not auction_id:
        return jsonify({"error": "Auction ID is required"}), 400
    
    data = request.get_json()
    if not data:
        return jsonify({"error": "Request body is required"}), 400
    
    client_id = data.get('client_id')
    
    if not client_id:
        return jsonify({"error": "Missing client_id"}), 400
    
    gateway.client_interests[auction_id].discard(client_id)
    return jsonify({"message": "Interest cancelled successfully"}), 200

if __name__ == '__main__':
    print("API Gateway started on port 8081")
    app.run(host='0.0.0.0', port=8081, debug=False, use_reloader=False)
