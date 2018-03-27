package ws

import (
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type MockServer struct {
	T            *testing.T
	Mu           sync.Mutex     // guards clients.
	WG           sync.WaitGroup // waits for open connections.
	Handler      *Handler
	Server       *httptest.Server
	Clients      map[*Client]struct{}
	PingInterval time.Duration

	ConnectListener func(*Client)
	CloseListener   func(*Client)
	MessageListener func(*Client, []byte)
	ErrorListener   func(*Client, error)
}

func (m *MockServer) Setup(t *testing.T) {
	m.T = t
	m.Clients = make(map[*Client]struct{})
	m.Handler = &Handler{
		Logger:       log.New(os.Stdout, "["+t.Name()+"] ", log.LstdFlags|log.Lshortfile),
		PingInterval: defaultDuration(m.PingInterval, time.Second),
		WriteTimeout: time.Second,

		ErrorListener:   m.ErrorListener,
		MessageListener: m.MessageListener,
		ConnectListener: m.Add,
		CloseListener:   m.Remove,
	}
	m.Server = httptest.NewServer(m.Handler)
}

func (m *MockServer) Teardown() {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	for c := range m.Clients {
		c.Close()
	}
	m.WG.Wait()
	m.Server.Close()
}

func (m *MockServer) URL() string {
	addr, _ := url.Parse(m.Server.URL)
	addr.Scheme = "ws"
	return addr.String()
}

func (m *MockServer) Add(c *Client) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	m.Clients[c] = struct{}{}
	if m.ConnectListener != nil {
		m.ConnectListener(c)
	}
}

func (m *MockServer) NumberOfClients() int {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	return len(m.Clients)
}

func (m *MockServer) Remove(c *Client) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	delete(m.Clients, c)
	if m.CloseListener != nil {
		m.CloseListener(c)
	}
}

func (m *MockServer) NewConnection() *MockConnection {
	conn, _, err := websocket.DefaultDialer.Dial(m.URL(), nil)
	if err != nil {
		m.T.Fatalf("could not open connection to %s: %s", m.URL(), err)
	}
	mockConn := MockConnection{
		T:    m.T,
		Conn: conn,
		WG:   &m.WG,
	}
	m.WG.Add(1)
	go mockConn.run()
	return &mockConn
}

type MockConnection struct {
	T               *testing.T
	Conn            *websocket.Conn
	WG              *sync.WaitGroup
	MessageListener func(string)
	Messages        []string
}

func (m *MockConnection) Write(msg string) {
	err := m.Conn.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		m.T.Fatalf("could not write message %#v: %s", msg, err)
	}
}

func (m *MockConnection) run() {
	defer m.WG.Done()
	defer m.Conn.Close()

	for {
		msgType, msg, err := m.Conn.ReadMessage()
		if msgType == websocket.TextMessage {
			m.Messages = append(m.Messages, string(msg))
			if m.MessageListener != nil {
				m.MessageListener(string(msg))
			}
		}
		if err != nil {
			return
		}
	}
}
