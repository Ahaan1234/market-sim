package ws

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"quantsim-server/engine"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Client is a single connected WebSocket peer.
type Client struct {
	conn *websocket.Conn
	send chan []byte // buffered outbound messages
	hub  *Hub
}

// Hub maintains the set of connected clients and fans out broadcast messages.
type Hub struct {
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	runner     *engine.Runner
}

// NewHub creates a Hub wired to the given engine runner.
func NewHub(runner *engine.Runner) *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		runner:     runner,
	}
}

// Run is the Hub's event loop. Must be called in its own goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}
			log.Printf("ws: client connected (%d total)", len(h.clients))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				log.Printf("ws: client disconnected (%d total)", len(h.clients))
			}

		case msg := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow client: drop it rather than blocking the broadcaster.
					close(c.send)
					delete(h.clients, c)
				}
			}
		}
	}
}

// Broadcast enqueues msg for delivery to every connected client.
// Non-blocking: drops if the internal broadcast channel is full.
func (h *Hub) Broadcast(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
		log.Println("ws: broadcast channel full, dropping message")
	}
}

// ServeWS upgrades the HTTP connection to WebSocket and registers the client.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade error: %v", err)
		return
	}

	c := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		hub:  h,
	}
	h.register <- c

	go c.writePump()
	c.readPump() // blocks until the client disconnects
}

// writePump drains the client's send channel and writes to the WebSocket.
// When send is closed (e.g. slow-client eviction), the goroutine exits.
func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// inboundMsg is the shape of messages sent by the browser over WebSocket.
type inboundMsg struct {
	Type      string  `json:"type"`       // "order" or "ping"
	TraderID  string  `json:"trader_id"`
	OrderID   string  `json:"order_id"`
	Side      string  `json:"side"`
	OrderType string  `json:"order_type"` // "LIMIT" or "MARKET"
	Price     float64 `json:"price"`
	Qty       float64 `json:"qty"`
}

// engineOrder is the shape the C++ engine's Protocol.h expects on stdin.
type engineOrder struct {
	TraderID string  `json:"trader_id"`
	OrderID  string  `json:"order_id"`
	Side     string  `json:"side"`
	Type     string  `json:"type"`  // "LIMIT" or "MARKET"
	Price    float64 `json:"price"`
	Qty      float64 `json:"qty"`
}

// readPump reads inbound messages from the WebSocket connection.
// Runs in the goroutine that called ServeWS; exits on disconnect.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var msg inboundMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "order":
			if !validateOrder(&msg) {
				continue
			}
			orderType := msg.OrderType
			if orderType == "" {
				orderType = "LIMIT"
			}
			order := engineOrder{
				TraderID: msg.TraderID,
				OrderID:  msg.OrderID,
				Side:     msg.Side,
				Type:     orderType,
				Price:    msg.Price,
				Qty:      msg.Qty,
			}
			orderJSON, err := json.Marshal(order)
			if err != nil {
				continue
			}
			if err := c.hub.runner.SendOrder(orderJSON); err != nil {
				log.Printf("ws: SendOrder error: %v", err)
			}

		case "ping":
			select {
			case c.send <- []byte(`{"type":"pong"}`):
			default:
			}
		}
	}
}

// validateOrder performs minimal defence-in-depth checks before forwarding
// to the engine. The engine validates fully; this just blocks obvious garbage.
func validateOrder(m *inboundMsg) bool {
	if m.TraderID == "" || m.OrderID == "" {
		return false
	}
	if m.Side != "BUY" && m.Side != "SELL" {
		return false
	}
	if m.Qty <= 0 {
		return false
	}
	return true
}
