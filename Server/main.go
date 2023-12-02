package main

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Player represents a connected player.
type Player struct {
	conn      *websocket.Conn
	positionY int
	id        int
}

// GameServer represents the game server.

type GameServer struct {
	players      map[int]*Player
	broadcast    chan []byte
	mutex        sync.Mutex
	upgrader     websocket.Upgrader
	canvasHeight int
	canvasWidth  int
	ball         Ball
}

type Ball struct {
	X, Y      int
	VelocityX int
	VelocityY int
}

func (gs *GameServer) run() {
	for {
		select {
		case message := <-gs.broadcast:
			gs.mutex.Lock()
			for index, player := range gs.players {
				err := player.conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Println("Error writing to player:", err)
					err := player.conn.Close()
					if err != nil {
						return
					}
					delete(gs.players, index)
				}
			}
			gs.mutex.Unlock()
		}
	}
}

func (gs *GameServer) controlBall() {
	for {
		time.Sleep(10 * time.Millisecond)

		gs.ball.X += gs.ball.VelocityX
		gs.ball.Y += gs.ball.VelocityY

		gs.checkPaddleCollision()

		gs.broadcast <- gs.serializeGameState()

	}
}

func (gs *GameServer) checkPaddleCollision() {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	// Create a copy of the players map to avoid concurrent modification
	playersCopy := make(map[int]Player, len(gs.players))
	for key, value := range gs.players {
		playersCopy[key] = *value
	}

	for _, player := range playersCopy {

		if (gs.ball.X-10 <= 35 || gs.ball.X+10 >= gs.canvasWidth-35) && gs.ball.Y >= player.positionY && gs.ball.Y <= player.positionY+100 {
			gs.ball.VelocityX = -gs.ball.VelocityX
		}
	}

	if gs.ball.Y-10 <= 0 || gs.ball.Y+10 >= gs.canvasHeight {
		gs.ball.VelocityY = -gs.ball.VelocityY
	}

	if gs.ball.X-10 <= 0 || gs.ball.X+10 >= gs.canvasWidth {
		gs.ball.VelocityX = -gs.ball.VelocityX
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
	index := 0
	if len(gs.players) == 0 {
		log.Println("Player 1 connected.")
	} else {
		index = 1
		log.Println("Player 2 connected.")
	}

	gs.mutex.Lock()
	gs.players[index] = player
	gs.mutex.Unlock()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Player disconnected:", err)
			gs.mutex.Lock()
			delete(gs.players, index)
			gs.mutex.Unlock()
			break
		}

		var actionMessage map[string]string
		if err := json.Unmarshal(message, &actionMessage); err != nil {
			log.Println("Error decoding JSON:", err)
			continue
		}

		switch actionMessage["action"] {
		case "moveUp":
			player.positionY -= 30
			if player.positionY < 0 {
				player.positionY = 0
			}
		case "moveDown":
			player.positionY += 30
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

func (gs *GameServer) serializeGameState() []byte {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	gameState := make(map[string]int)

	for _, player := range gs.players {
		insert := "player" + strconv.Itoa(player.id) + "Y"
		gameState[insert] = player.positionY
	}

	gameState["ballX"] = gs.ball.X
	gameState["ballY"] = gs.ball.Y

	jsonData, err := json.Marshal(gameState)
	if err != nil {
		log.Println("Error encoding JSON:", err)
		return nil
	}

	return jsonData
}

func main() {

	gs := &GameServer{
		players:      make(map[int]*Player),
		canvasHeight: 600,
		canvasWidth:  800,
		broadcast:    make(chan []byte),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		ball: Ball{
			X:         400,
			Y:         300,
			VelocityX: 5,
			VelocityY: 5,
		},
	}

	go gs.run()
	go gs.controlBall()

	http.HandleFunc("/ws", gs.handleConnection)

	log.Println("Server is running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
