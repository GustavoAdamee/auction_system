#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

PIDS=()

cleanup() {
    echo -e "\n${BLUE}Encerrando serviços...${NC}"
    for pid in "${PIDS[@]}"; do
        kill -0 "$pid" 2>/dev/null && kill "$pid" 2>/dev/null
    done
    wait
    echo -e "${GREEN}Serviços encerrados${NC}"
    exit 0
}

trap cleanup SIGINT SIGTERM EXIT

echo -e "${BLUE}Iniciando serviços...${NC}\n"

cd "$SCRIPT_DIR/auction_microservice"
go run . > /dev/null 2>&1 &
PIDS+=($!)
echo -e "${GREEN}✓ Auction MS (porta 8080)${NC}"
sleep 2

cd "$SCRIPT_DIR/bids_microservice"
go run . > /dev/null 2>&1 &
PIDS+=($!)
echo -e "${GREEN}✓ Bids MS (porta 8084)${NC}"
sleep 2

cd "$SCRIPT_DIR/payment_microservice"
go run . > /dev/null 2>&1 &
PIDS+=($!)
echo -e "${GREEN}✓ Payment MS (porta 8093)${NC}"
sleep 2

cd "$SCRIPT_DIR/external_payment_system"
[ ! -d "venv" ] && python3 -m venv venv
source venv/bin/activate
pip install -q -r requirements.txt 2>/dev/null
python main.py > /dev/null 2>&1 &
PIDS+=($!)
deactivate
echo -e "${GREEN}✓ External Payment System (porta 8092)${NC}"
sleep 2

cd "$SCRIPT_DIR/api_gateway"
[ ! -d "venv" ] && python3 -m venv venv
source venv/bin/activate
pip install -q -r requirements.txt 2>/dev/null
python main.py > /dev/null 2>&1 &
PIDS+=($!)
deactivate
echo -e "${GREEN}✓ API Gateway (porta 8081)${NC}"
sleep 2

echo -e "\n${GREEN}Todos os serviços iniciados!${NC}"
echo -e "${BLUE}Pressione Ctrl+C para encerrar${NC}\n"

while true; do sleep 1; done
