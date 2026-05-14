package editor

import (
	"sync"

	"github.com/gorilla/websocket"
)

// wsClient wraps a single websocket connection in a per-client write pump so
// all writes are serialized on a dedicated goroutine. Send is non-blocking
// and drops on overflow.
type wsClient struct {
	conn   *websocket.Conn
	send   chan []byte
	closed chan struct{}
	once   sync.Once
}

const wsSendQueueSize = 64

func newWSClient(conn *websocket.Conn) *wsClient {
	c := &wsClient{
		conn:   conn,
		send:   make(chan []byte, wsSendQueueSize),
		closed: make(chan struct{}),
	}
	go c.writePump()
	return c
}

// Send queues a message for the client. Drops the message if the queue is
// full or the client is closed.
func (c *wsClient) Send(msg []byte) {
	select {
	case <-c.closed:
		return
	default:
	}
	select {
	case c.send <- msg:
	default:
	}
}

func (c *wsClient) Close() {
	c.once.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}

func (c *wsClient) writePump() {
	defer c.Close()
	for {
		select {
		case msg := <-c.send:
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-c.closed:
			return
		}
	}
}
