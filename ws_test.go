package ws

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConnect(t *testing.T) {
	var server MockServer
	server.Setup(t)
	defer server.Teardown()
	server.NewConnection()

	if server.NumberOfClients() != 1 {
		t.Errorf("expected 1 client but got %d clients", server.NumberOfClients())
	}
}

func TestErrorOnBadRequest(t *testing.T) {
	var actualError string
	server := MockServer{
		ErrorListener: func(_ *Client, err error) {
			actualError = err.Error()
		},
	}
	server.Setup(t)
	defer server.Teardown()
	expectedError := "websocket: the client is not using the websocket protocol: 'upgrade' token not found in 'Connection' header"
	resp, err := http.Get(server.Server.URL)

	if err != nil {
		t.Fatalf("should not have had an error getting from server but got: %s", err)
	}
	if actualError != expectedError {
		t.Errorf("expected error to be %#v but actual was %#v", expectedError, actualError)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("response should have had error code 400 but got %d", resp.StatusCode)
	}
}

type EventWaiter struct {
	done  chan struct{}
	once  sync.Once
	count int32
}

func (e *EventWaiter) Cancel() {
	e.once.Do(func() { close(e.done) })
}

func (e *EventWaiter) Wait(timeout time.Duration) {
	defer time.AfterFunc(timeout, e.Cancel).Stop()
	<-e.done
}

func (e *EventWaiter) Mark() {
	if atomic.AddInt32(&e.count, -1) <= 0 {
		e.Cancel()
	}
}

func NewEventWaiter(count int) *EventWaiter {
	return &EventWaiter{done: make(chan struct{}), count: int32(count)}
}

func TestEcho(t *testing.T) {
	server := MockServer{
		MessageListener: func(c *Client, msg []byte) { c.Write(msg) }, // echo
	}
	server.Setup(t)
	defer server.Teardown()
	expected := "hello world"
	connection := server.NewConnection()
	e := NewEventWaiter(1)
	connection.MessageListener = func(string) { e.Mark() }
	connection.Write(expected)
	e.Wait(time.Second)

	if len(connection.Messages) != 1 {
		t.Fatalf("connection should have received 1 message but received %d", len(connection.Messages))
	}
	actual := connection.Messages[0]
	if actual != expected {
		t.Errorf("expected %#v but got %#v", expected, actual)
	}
}

func TestError(t *testing.T) {
	e := NewEventWaiter(1)
	server := MockServer{
		ConnectListener: func(c *Client) {
			msg := websocket.FormatCloseMessage(websocket.CloseAbnormalClosure, t.Name())
			c.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
		}, // once connected, force abnormal closure.
		ErrorListener: func(_ *Client, err error) {
			e.Mark()
		},
	}
	server.Setup(t)
	defer server.Teardown()
	server.NewConnection()
	e.Wait(time.Second)
	if e.count > 0 {
		t.Errorf("expected to be informed of error but was not")
	}
}
