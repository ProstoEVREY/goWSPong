package main

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
	"sync"
)

// Player represents a connected player.
type Player struct {
	conn      *websocket.Conn
	positionY int
	id        int
}

// GameServer represents the game server.
type GameServer struct {
	players      map[*Player]bool
	broadcast    chan []byte
	mutex        sync.Mutex
	upgrader     websocket.Upgrader
	canvasHeight int
}

func (gs *GameServer) run() {
	for {
		select {
		case message := <-gs.broadcast:
			gs.mutex.Lock()
			for player := range gs.players {
				err := player.conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Println("Error writing to player:", err)
					err := player.conn.Close()
					if err != nil {
						return
					}
					delete(gs.players, player)
				}
			}
			gs.mutex.Unlock()
		}
	}
}

func (gs *GameServer) handleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := gs.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading connection:", err)
		return
	}
	if len(gs.players) > 1 {
		log.Println("All player slots are filled.")
		return
	}

	player := &Player{conn: conn, positionY: 250, id: len(gs.players) + 1}

	if len(gs.players) == 0 {
		log.Println("Player 1 connected.")
	} else {
		log.Println("Player 2 connected.")
	}

	gs.mutex.Lock()
	gs.players[player] = true
	gs.mutex.Unlock()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Player disconnected:", err)
			gs.mutex.Lock()
			delete(gs.players, player)
			gs.mutex.Unlock()
			break
		}

		var actionMessage map[string]string
		if err := json.Unmarshal(message, &actionMessage); err != nil {
			log.Println("Error decoding JSON:", err)
			continue
		}

		// New: Update player position based on client action
		switch actionMessage["action"] {
		case "moveUp":
			player.positionY -= 20
			if player.positionY < 0 {
				player.positionY = 0
			}
		case "moveDown":
			player.positionY += 20
			if player.positionY > 500 {
				player.positionY = 500
			}
		case "stopMove":
			// Handle stopping player movement
		}

		// Broadcast updated player positions to all clients
		gs.broadcast <- gs.serializeGameState()
	}
}

// New: serializeGameState function to convert game state to JSON
func (gs *GameServer) serializeGameState() []byte {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	gameState := make(map[string]int)
	for player := range gs.players {
		insert := "player" + strconv.Itoa(player.id) + "Y"
		gameState[insert] = player.positionY
	}

	jsonData, err := json.Marshal(gameState)
	if err != nil {
		log.Println("Error encoding JSON:", err)
		return nil
	}

	return jsonData
}

func main() {

	gs := &GameServer{
		players:      make(map[*Player]bool),
		canvasHeight: 600,
		broadcast:    make(chan []byte),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	go gs.run()

	http.HandleFunc("/ws", gs.handleConnection)

	log.Println("Server is running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
