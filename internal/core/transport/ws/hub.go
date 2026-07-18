package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"messenger/internal/core/domain"
	"messenger/internal/core/redis"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	userID uuid.UUID
	send   chan []byte
}

type Hub struct {
	mu          sync.RWMutex
	clients     map[uuid.UUID]map[*Client]bool
	jwtSecret   string
	redisClient *redis.Client
}

func NewHub(jwtSecret string, rdb *redis.Client) *Hub {
	return &Hub{
		clients:     make(map[uuid.UUID]map[*Client]bool),
		jwtSecret:   jwtSecret,
		redisClient: rdb,
	}
}

func (h *Hub) BroadcastToUsers(userIDs []uuid.UUID, event domain.WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, userID := range userIDs {
		for client := range h.clients[userID] {
			select {
			case client.send <- data:
			default:
				close(client.send)
				delete(h.clients[userID], client)
			}
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}

	userID, err := parseToken(token, h.jwtSecret)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{
		hub:    h,
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, 64),
	}

	h.register(client)

	go client.writePump()
	go client.readPump()
}

func (h *Hub) register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.userID] == nil {
		h.clients[client.userID] = make(map[*Client]bool)
	}
	h.clients[client.userID][client] = true

	if h.redisClient != nil {
		h.redisClient.SetUserOnline(context.Background(), client.userID.String())
	}
}

func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conns, ok := h.clients[client.userID]; ok {
		if _, exists := conns[client]; exists {
			delete(conns, client)
			close(client.send)
		}
		if len(conns) == 0 {
			delete(h.clients, client.userID)
			if h.redisClient != nil {
				h.redisClient.SetUserOffline(context.Background(), client.userID.String())
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

func parseToken(tokenString, secret string) (uuid.UUID, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, jwt.ErrTokenInvalidClaims
	}

	switch v := claims["user_id"].(type) {
	case string:
		return uuid.Parse(v)
	default:
		return uuid.Nil, jwt.ErrTokenInvalidClaims
	}
}
