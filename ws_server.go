package arithmospora

/**
 * Websocket server based on Gorilla Websocket chat example, see
 * https://github.com/gorilla/websocket/tree/master/examples/chat
 */

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type WebsocketConfig struct {
	WriteWait  int
	PongWait   int
	PingPeriod int
}

var websocketConfig = WebsocketConfig{WriteWait: 10, PongWait: 60, PingPeriod: 54}

func SetWebSocketConfig(config WebsocketConfig) {
	if config != (WebsocketConfig{}) {
		websocketConfig = config
	}
}

// Websocket upgrader: output only application, allow connections from any origin
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub: as per Gorilla chat example
type Hub struct {
	Broadcast  chan []byte
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	source     *Source
}

func NewHub(source *Source) *Hub {
	return &Hub{
		Broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		source:     source,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true

			// Source sends client initial data on connect
			h.source.SendInitialDataTo(client)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.Broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) ClientCount() int {
	return len(h.clients)
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Readpump handles client Pong messages and loops to disregard client messages
// TODO: Implement capability for clients to send messages to opt in or out of
// particular stats
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadDeadline(time.Now().Add(time.Duration(websocketConfig.PongWait) * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(time.Duration(websocketConfig.PongWait) * time.Second))
		return nil
	})
	for {
		_, _, err := c.conn.NextReader()
		if err != nil {
			c.conn.Close()
			break
		}
	}
}

// Only write one message at a time with writePump rather than aggregating as
// per the chat example, as our messages are JSON documents
func (c *Client) writePump() {
	ticker := time.NewTicker(time.Duration(websocketConfig.PingPeriod) * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(time.Duration(websocketConfig.WriteWait) * time.Second))
			if !ok {
				// Hub closed channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(time.Duration(websocketConfig.WriteWait) * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

// ServeWs handles websocket requests from the peer.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client
	go client.writePump()
	client.readPump()
}

type Message struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}
