package main

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"math/rand"
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
	scoreOne     int
	scoreTwo     int
	playSound    string
}

type Ball struct {
	X, Y      int
	VelocityX int
	VelocityY int
}

func getRandomDirection(speed int) int {
	// Return either -1 or 1 randomly to set the direction.
	if rand.Intn(2) == 0 {
		return -speed
	}
	return speed
}

func getRandomSpeed(min, max int) int {
	speed := rand.Intn(max-min+1) + min
	return getRandomDirection(speed)
}

func getRandomheight(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min+1) + min
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
		time.Sleep(15 * time.Millisecond)

		gs.mutex.Lock()
		if len(gs.players) >= 2 {
			gs.ball.X += gs.ball.VelocityX
			gs.ball.Y += gs.ball.VelocityY
		}
		gs.mutex.Unlock()

		gs.checkPaddleCollision()

		gs.broadcast <- gs.serializeGameState()

	}
}

func (gs *GameServer) Replay(player int) {
	gs.ball.X = 400
	gs.ball.Y = getRandomheight(50, 550)
	if player == 0 {
		gs.scoreOne += 1
		gs.playSound = "score"
	} else {
		gs.scoreTwo += 1
		gs.playSound = "score"
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
			gs.playSound = "pong"
		}
	}

	if gs.ball.Y-10 <= 0 || gs.ball.Y+10 >= gs.canvasHeight {
		gs.ball.VelocityY = -gs.ball.VelocityY
		gs.playSound = "pong"
	}

	if gs.ball.X-10 <= 0 {
		gs.Replay(1)
	}

	if gs.ball.X+10 >= 800 {
		gs.Replay(0)
	}

}

func (gs *GameServer) resetGame() {
	gs.ball.X = 400
	gs.ball.Y = 300
	gs.scoreOne = 0
	gs.scoreTwo = 0

	for _, player := range gs.players {
		player.positionY = 250
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
		gs.resetGame()
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
			player.positionY -= 70
			if player.positionY < 0 {
				player.positionY = 0
			}
		case "moveDown":
			player.positionY += 70
			if player.positionY > 500 {
				player.positionY = 500
			}
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

	gameState["score1"] = gs.scoreOne
	gameState["score2"] = gs.scoreTwo

	if gs.playSound == "score" {
		gameState["audio"] = 2
		gs.playSound = "none"
	} else if gs.playSound == "pong" {
		gameState["audio"] = 1
		gs.playSound = "none"
	} else {
		gameState["audio"] = 0
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
			VelocityX: getRandomSpeed(3, 4),
			VelocityY: getRandomSpeed(3, 4),
		},
		scoreOne:  0,
		scoreTwo:  0,
		playSound: "none",
	}

	go gs.run()
	go gs.controlBall()

	http.HandleFunc("/ws", gs.handleConnection)

	log.Println("Server is running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
