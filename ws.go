package ws

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// A Handler upgrades to a websocket and starts the client that maintains the connection.
type Handler struct {
	Upgrader     websocket.Upgrader
	Logger       *log.Logger
	PingInterval time.Duration
	WriteTimeout time.Duration
	ReadTimeout  time.Duration

	ConnectListener func(*Client)
	MessageListener func(*Client, []byte)
	ErrorListener   func(*Client, error)
	CloseListener   func(*Client)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		if h.ErrorListener != nil {
			h.ErrorListener(nil, err)
		}
		return
	}

	var c Client
	writeTimeout := defaultDuration(h.PingInterval, time.Second*25)
	pingInterval := defaultDuration(h.PingInterval, time.Second*25)
	c = Client{
		conn:         conn,
		pingInterval: pingInterval,
		writeTimeout: writeTimeout,
		readTimeout:  defaultDuration(h.ReadTimeout, pingInterval+2*writeTimeout),
		done:         make(chan struct{}),

		messageListener: func(msg []byte) {
			if h.MessageListener != nil {
				h.MessageListener(&c, msg)
			}
		},
		errorListener: func(err error) {
			if h.ErrorListener != nil {
				h.ErrorListener(&c, err)
			} else if h.Logger != nil {
				h.Logger.Println(err)
			}
		},
		closeListener: func() {
			if h.CloseListener != nil {
				h.CloseListener(&c)
			}
		},
	}
	if h.ConnectListener != nil {
		h.ConnectListener(&c)
	}
	c.start()
}

func defaultDuration(x, standard time.Duration) time.Duration {
	if x == 0 {
		return standard
	}
	return x
}

// A Client maintains a connection to a client.
type Client struct {
	mu   sync.Mutex // guards conn.WriteMessage
	once sync.Once  // used in Close method.

	conn         *websocket.Conn
	pingInterval time.Duration
	writeTimeout time.Duration
	readTimeout  time.Duration
	done         chan struct{}

	messageListener func([]byte)
	errorListener   func(error)
	closeListener   func()
}

// Close attempts to inform the client before terminating the connection.
// Subsequent calls to Close result in a no-op.
func (c *Client) Close(closeCode ...int) {
	c.once.Do(func() {
		go c.closeListener()
		close(c.done)
		closeCode = append(closeCode, websocket.CloseNoStatusReceived)
		msg := websocket.FormatCloseMessage(closeCode[0], "")
		err := c.conn.WriteControl(websocket.CloseMessage, msg, c.writeDeadline())
		c.maybeError(err)
		c.conn.Close()
	})
}

// Write sends the message to the client.
func (c *Client) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn.SetWriteDeadline(c.writeDeadline())
	err := c.conn.WriteMessage(websocket.TextMessage, msg)
	c.maybeError(err)
	return err
}

func (c *Client) writeDeadline() time.Time {
	return time.Now().Add(c.writeTimeout)
}

func (c *Client) maybeError(err error) {
	if err == nil {
		return
	}
	closeCode := websocket.CloseNoStatusReceived
	if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
		go c.errorListener(fmt.Errorf("websocket was closed unexpectedly: %s", err))
	}
	go c.Close(closeCode)
}

func (c *Client) start() {
	go c.ping()
	go c.read()
}

func (c *Client) read() {
	defer c.Close()
	resetPongDeadline := func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
		return nil
	}
	c.conn.SetPongHandler(resetPongDeadline)
	resetPongDeadline("")

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			c.maybeError(err)
			return
		}
		go c.messageListener(msg)
	}
}

func (c *Client) ping() {
	defer c.Close()

	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := c.conn.WriteControl(websocket.PingMessage, nil, c.writeDeadline())
			c.maybeError(err)
		case <-c.done:
			return
		}
	}
}
