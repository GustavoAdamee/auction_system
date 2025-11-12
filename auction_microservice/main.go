package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	auctionPool []Auction
	poolMutex   sync.RWMutex
)

func main() {
	// Start HTTP server
	go startHTTPServer()

	// Start auction pooling
	auctionPooling()
}

func startHTTPServer() {
	http.HandleFunc("/auctions", handleAuctions)
	log.Println("Auction Microservice started on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleAuctions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case "POST":
		createAuction(w, r)
	case "GET":
		getAuctions(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
	}
}

func createAuction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string    `json:"id"`
		Description string    `json:"description"`
		StartTime   time.Time `json:"start_time"`
		EndTime     time.Time `json:"end_time"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	auction := Auction{
		id:          req.ID,
		description: req.Description,
		Start_time:  req.StartTime,
		End_time:    req.EndTime,
		Status:      StatusIdle,
	}

	auction.Init()

	poolMutex.Lock()
	auctionPool = append(auctionPool, auction)
	poolMutex.Unlock()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          auction.id,
		"description": auction.description,
		"start_time":  auction.Start_time,
		"end_time":    auction.End_time,
		"status":      auction.Status,
	})
}

func getAuctions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	poolMutex.RLock()
	var result []map[string]interface{}
	for _, auction := range auctionPool {
		if status == "" || string(auction.Status) == status {
			result = append(result, map[string]interface{}{
				"id":          auction.id,
				"description": auction.description,
				"start_time":  auction.Start_time,
				"end_time":    auction.End_time,
				"status":      auction.Status,
			})
		}
	}
	poolMutex.RUnlock()

	json.NewEncoder(w).Encode(result)
}

func auctionPooling() {
	for {
		poolMutex.Lock()
		for i := 0; i < len(auctionPool); i++ {
			if auctionPool[i].Status == StatusIdle || auctionPool[i].Status == StatusActive {
				checkAuctionStarted(&auctionPool[i])
				checkAuctionEnded(&auctionPool[i])
			} else {
				auctionPool[i].Close()
				auctionPool = append(auctionPool[:i], auctionPool[i+1:]...)
				i--
			}
		}
		poolMutex.Unlock()

		time.Sleep(1 * time.Second)
	}
}

func checkAuctionStarted(auction *Auction) {
	if (time.Now().After(auction.Start_time) || time.Now().Equal(auction.Start_time)) && auction.Status == StatusIdle {
		auction.Status = StatusActive
		auction.StartAuction()
		fmt.Printf("Auction %v started --- Auction status set to: %v\n", auction.id, auction.Status)
	}
}

func checkAuctionEnded(auction *Auction) {
	if time.Now().After(auction.End_time) && auction.Status == StatusActive {
		auction.Status = StatusFinished
		auction.FinishAuction()
		fmt.Printf("Auction %v ended --- Auction status set to: %v\n", auction.id, auction.Status)
		auction.Close()
	}
}
