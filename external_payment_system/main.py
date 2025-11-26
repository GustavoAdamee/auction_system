import random
import threading
import time
import uuid

import requests
from flask import Flask, jsonify, request

app = Flask(__name__)

transactions = {}    # type: ignore


def simulate_payment(transaction_id):
    time.sleep(random.randint(3, 7))
    status = random.choice(["approved", "rejected"])
    tx = transactions.get(transaction_id)
    if not tx:
        return

    tx["status"] = status
    payload = {
        "transaction_id": transaction_id,
        "status": status,
        "amount": tx.get("amount", 0),
        "client_id": tx.get("client_id"),
        "auction_id": tx.get("auction_id"),
    }

    callback_url = tx.get("callback_url")
    if not callback_url:
        return

    try:
        requests.post(callback_url, json=payload, timeout=5)
    except requests.RequestException as exc:
        print(f"[External Payment] Failed to send webhook: {exc}")


@app.route("/payments", methods=["POST"])
def create_payment():
    data = request.get_json() or {}
    amount = data.get("amount")
    callback_url = data.get("callback_url")
    client_id = data.get("client_id")
    auction_id = data.get("auction_id")

    if not callback_url:
        return jsonify({"error": "callback_url is required"}), 400

    transaction_id = str(uuid.uuid4())
    payment_link = f"http://localhost:8092/pay/{transaction_id}"

    transactions[transaction_id] = {
        "status": "pending",
        "amount": amount,
        "callback_url": callback_url,
        "client_id": client_id,
        "auction_id": auction_id,
    }

    threading.Thread(target=simulate_payment, args=(transaction_id,), daemon=True).start()

    return jsonify(
        {
            "transaction_id": transaction_id,
            "payment_link": payment_link,
        }
    )


@app.route("/pay/<transaction_id>", methods=["GET"])
def view_payment(transaction_id):
    tx = transactions.get(transaction_id)
    status = tx.get("status") if tx else "desconhecido"
    return f"""
    <html>
        <head><title>Pagamento</title></head>
        <body>
            <h1>Pagamento {transaction_id}</h1>
            <p>Status: {status}</p>
            <p>Usuario: {tx.get("client_id")}</p>
            <p>Leilao: {tx.get("auction_id")}</p>
            <p>Valor: {tx.get("amount")}</p>
        </body>
    </html>
    """


if __name__ == "__main__":
    print("External Payment System on port 8092")
    app.run(host="0.0.0.0", port=8092, debug=False, use_reloader=False)


