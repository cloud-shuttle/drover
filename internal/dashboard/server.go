package dashboard

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFS embed.FS

// Server is the dashboard HTTP server
type Server struct {
	db     *sql.DB
	store  *db.Store
	hub    *Hub
	addr   string
	server *http.Server
}

// Config holds server configuration
type Config struct {
	Addr        string
	DatabaseURL string
	DB          *sql.DB // Pass existing connection
	Store       *db.Store
}

// New creates a new dashboard server
func New(cfg Config) (*Server, error) {
	db := cfg.DB
	if db == nil {
		var err error
		db, err = sql.Open("sqlite3", cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("open db: %w", err)
		}
	}

	s := &Server{
		db:    db,
		store: cfg.Store,
		hub:   newHub(),
		addr:  cfg.Addr,
	}
	return s, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/epics", s.handleEpics)
	mux.HandleFunc("GET /api/tasks", s.handleTasks)
	mux.HandleFunc("GET /api/tasks/", s.handleTask)
	mux.HandleFunc("POST /api/tasks/", s.handleTaskAction)
	mux.HandleFunc("GET /api/workers", s.handleWorkers)
	mux.HandleFunc("GET /api/graph", s.handleGraph)
	mux.HandleFunc("GET /api/worktrees/", s.handleWorktreeAPI)
	mux.HandleFunc("GET /ws", s.handleWebSocket)

	// Static files
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /", http.FileServer(http.FS(static)))

	s.server = &http.Server{Addr: s.addr, Handler: mux}

	// Start hub for WebSocket broadcasts
	go s.hub.run()

	// Start stats broadcaster
	go s.broadcastStats()

	log.Printf("Dashboard running at http://localhost%s", s.addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// Broadcast broadcasts an event to all connected clients
func (s *Server) Broadcast(eventType string, data any) {
	s.hub.broadcast <- Event{Type: eventType, Data: data}
}

func (s *Server) broadcastStats() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats, err := s.getStatus()
		if err != nil {
			continue
		}
		s.Broadcast("stats_update", stats)
	}
}

// ============================================================================
// WebSocket Hub
// ============================================================================

// Hub manages WebSocket connections
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Event
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// Event is a WebSocket event
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Client represents a WebSocket client
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Event, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			msg, _ := json.Marshal(event)
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
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

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{hub: s.hub, conn: conn, send: make(chan []byte, 256)}
	s.hub.register <- client

	go client.writePump()
	go client.readPump()
}
