package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"messenger/internal/core/domain"
	"messenger/internal/core/redis"
	httpmw "messenger/internal/core/transport/http/middleware"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	userID uuid.UUID
	send   chan []byte
}

type Hub struct {
	mu             sync.RWMutex
	clients        map[uuid.UUID]map[*Client]bool
	jwtSecret      string
	redisClient    *redis.Client
	allowedOrigins []string
}

func NewHub(jwtSecret string, rdb *redis.Client, allowedOrigins []string) *Hub {
	return &Hub{
		clients:        make(map[uuid.UUID]map[*Client]bool),
		jwtSecret:      jwtSecret,
		redisClient:    rdb,
		allowedOrigins: allowedOrigins,
	}
}

func (h *Hub) upgrader(r *http.Request) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			return httpmw.OriginAllowed(req.Header.Get("Origin"), r, h.allowedOrigins)
		},
	}
}

func (h *Hub) BroadcastToUsers(userIDs []uuid.UUID, event domain.WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	targets := make([]*Client, 0)
	for _, userID := range userIDs {
		for client := range h.clients[userID] {
			targets = append(targets, client)
		}
	}
	h.mu.RUnlock()

	for _, client := range targets {
		select {
		case client.send <- data:
		default:
			// Медленный клиент не должен блокировать рассылку; polling fallback
			// подхватит сообщение, если событие пропущено.
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
	if !httpmw.OriginAllowed(r.Header.Get("Origin"), r, h.allowedOrigins) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	upgrader := h.upgrader(r)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{hub: h, conn: conn, userID: userID, send: make(chan []byte, 64)}
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
		_ = h.redisClient.SetUserOnline(context.Background(), client.userID.String())
	}
}

func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns, ok := h.clients[client.userID]
	if !ok {
		return
	}
	if _, exists := conns[client]; !exists {
		return
	}
	delete(conns, client)
	close(client.send)
	if len(conns) == 0 {
		delete(h.clients, client.userID)
		if h.redisClient != nil {
			_ = h.redisClient.SetUserOffline(context.Background(), client.userID.String())
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		_ = c.conn.Close()
	}()
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func parseToken(tokenString, secret string) (uuid.UUID, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil || !token.Valid {
		return uuid.Nil, jwt.ErrTokenInvalidClaims
	}
	switch v := claims["user_id"].(type) {
	case string:
		return uuid.Parse(v)
	default:
		return uuid.Nil, jwt.ErrTokenInvalidClaims
	}
}
