package game

import (
	"time"

	"github.com/gorilla/websocket"
)

type Player struct {
	ID             string
	Username       string
	Conn           *websocket.Conn
	Send           chan []byte
	DisconnectedAt *time.Time
	IsDisconnected bool
}
