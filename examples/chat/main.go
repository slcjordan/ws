package main

import (
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/slcjordan/ws"
)

// A Room is a chatroom where people can talk.
type Room struct {
	sync.Mutex
	clients map[*ws.Client]struct{}
}

// Add adds a client to the room.
func (r *Room) Add(c *ws.Client) {
	r.Lock()
	defer r.Unlock()

	r.clients[c] = struct{}{}
}

// Remove removes a client from the room.
func (r *Room) Remove(c *ws.Client) {
	r.Lock()
	defer r.Unlock()

	delete(r.clients, c)
}

// Broadcast broadcasts a message to all clients.
func (r *Room) Broadcast(_ *ws.Client, msg []byte) {
	r.Lock()
	defer r.Unlock()

	s := "<div>" + html.EscapeString(string(msg)) + "</div>"
	b := []byte(s)
	for c := range r.clients {
		go c.Write(b)
	}
}

func main() {
	r := Room{
		clients: make(map[*ws.Client]struct{}),
	}
	logger := log.New(os.Stderr, " chat ", log.LstdFlags|log.Lshortfile)
	h := ws.Handler{
		ConnectListener: r.Add,
		CloseListener:   r.Remove,
		MessageListener: r.Broadcast,
		Logger:          logger,
	}

	http.Handle("/ws", &h)
	var port = ":8000"
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		index.Execute(w, map[string]string{
			"hostname": "localhost",
			"port":     port,
		})
	})
	logger.Printf("serving at %s\n", port)
	http.ListenAndServe(port, nil)
}

var index = template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html lang="en">
<head>
	<style>
	.line{
		margin: 1em;
	}
	.chatroom{
		padding: 1em;
		height: 20em;
		width: 15em;
		background-color: #f1f1f1;
	}
	</style>
	<meta charset="UTF-8">
	<title></title>
</head>
<body>
	<div class="chatroom"></div>
	<input class="line" type="text" ></input>
	<script>
	(function(){
		var ws = new WebSocket("ws://{{.hostname}}{{.port}}/ws");
		var line = document.getElementsByClassName("line")[0];
		var chatroom = document.getElementsByClassName("chatroom")[0];

		function keypress(event){
			if (event.keyCode != 13){
				return;
			}
			var text = line.value;
			ws.send(text);
			line.value = "";
		};
		line.addEventListener("keypress", keypress);

		ws.onmessage = function(event){
			chatroom.innerHTML += event.data;
		};
	})()
	</script>
</body>
</html>`))
