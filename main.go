package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

type ChatMessage struct {
	Username string `json:"username"`
	Text     string `json:"text"`
}

var (
	rdb *redis.Client
)

var ctx = context.Background()

var clients = make(map[*websocket.Conn]bool)
var broadcaster = make(chan ChatMessage)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	redisURL := os.Getenv("REDIS_URL")
	rdb = redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: "",
		DB:       0,
	})

	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/websocket", handleConnections)
	go handleMessages()

	log.Printf("Start 4040 server")

	err := http.ListenAndServe(":4040", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}

	defer ws.Close()
	clients[ws] = true

	if rdb.Exists(ctx, "messages").Val() != 0 {
		sendPreviousMessages(ws)
	}

	for {
		var msg ChatMessage

		err := ws.ReadJSON(&msg)
		if err != nil {
			delete(clients, ws)
			break
		}

		if msg.Text == "" {
			return
		}
		saveMessage(msg)
		broadcaster <- msg
	}
}

func handleMessages() {
	for {
		msg := <-broadcaster

		for client := range clients {
			err := client.WriteJSON(msg)

			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}

func saveMessage(msg ChatMessage) {
	json, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	err = rdb.RPush(ctx, "messages", json).Err()
	if err != nil {
		panic(err)
	}
}

func sendPreviousMessages(client *websocket.Conn) {
	messages := getPreviousMessages()

	for _, message := range messages {
		var msg ChatMessage
		json.Unmarshal([]byte(message), &msg)
		err := client.WriteJSON(msg)
		if err != nil {
			log.Printf("error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
}

func getPreviousMessages() []string {
	chatMessages, err := rdb.LRange(ctx, "messages", 0, -1).Result()
	if err != nil {
		panic(err)
	}

	return chatMessages
}
