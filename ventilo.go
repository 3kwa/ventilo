// <fanout> was taken not <ventilo>!

// Turn key websocket broadcasting: POST a message on a channel it will be
// broadcasted to all the websockets listening on that channel.

// Channels are identified by the URL path e.g. connecting a websocket to
// /listen/to/a/channel registers the websocket on the channel to-a-channel.

// Symmetrically a POST on /broadcast/to/a/channel will send a message to all
// listeners on to-a-channel.

// Python by @3kwa https://gist.github.com/3kwa/5235b8289a2ac74f399d
// Go by @nf https://gist.github.com/nf/7c03729770315c05570f

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var httpAddr = flag.String("http", ":8080", "HTTP listen address")

func main() {
	flag.Parse()
	http.Handle("/", NewServer())
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}

type Server struct {
	mutex     sync.Mutex
	listeners map[string][]chan string
	messages  map[string]int
}

func NewServer() *Server {
	return &Server{
		listeners: make(map[string][]chan string),
		messages:  make(map[string]int)}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(request *http.Request) bool { return true },
}

const (
	channels  = "/channels/"
	broadcast = "/broadcast/"
	listen    = "/listen/"
)

type Channel struct {
	Name      string
	Listeners int
	Messages  int
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	switch {
	default:
		http.NotFound(writer, request)

	case path == channels:
		var list []Channel
		for name := range server.listeners {
			list = append(list, Channel{name, len(server.listeners[name]), server.messages[name]})
		}
		json_, _ := json.Marshal(list)
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Write(json_)

	case strings.HasPrefix(path, broadcast):
		name := strings.TrimPrefix(path, broadcast)
		server.broadcast(name, request.FormValue("message"))
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		fmt.Fprintf(writer, "OK\n")

	case strings.HasPrefix(path, listen):
		websocket_, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Print(err)
			return
		}
		name := strings.TrimPrefix(request.URL.Path, listen)
		channel := server.listen(name)
		defer server.hangup(name, channel)

		for message := range channel {
			err := websocket_.WriteMessage(websocket.TextMessage, []byte(message))
			if err != nil {
				log.Print(err)
				return
			}
		}
	}
}

func (server *Server) listen(name string) <-chan string {
	channel := make(chan string)
	server.mutex.Lock()
	server.listeners[name] = append(server.listeners[name], channel)
	server.mutex.Unlock()
	return channel
}

func (server *Server) hangup(name string, channel <-chan string) {
	// Remove channel from listener map.
	server.mutex.Lock()
	list := server.listeners[name]
	for i := range list {
		if list[i] == channel {
			list = append(list[:i], list[i+1:]...)
			break
		}
	}
	server.listeners[name] = list
	server.mutex.Unlock()

	// Drain channel for a minute, to unblock any in-flight senders.
	go func() {
		timeout := time.After(1 * time.Minute)
		for {
			select {
			case <-channel:
			case <-timeout:
				return
			}
		}
	}()
}

func (server *Server) broadcast(name, message string) {
	server.mutex.Lock()
	list := append([]chan string{}, server.listeners[name]...) // copy
	server.messages[name] += 1
	server.mutex.Unlock()
	for _, channel := range list {
		channel <- message
	}
}
