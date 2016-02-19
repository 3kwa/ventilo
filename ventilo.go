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
	mu sync.Mutex
	m  map[string][]chan string
	t  map[string]int
}

func NewServer() *Server {
	return &Server{
		m: make(map[string][]chan string),
		t: make(map[string]int)}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	status    = "/status/"
	broadcast = "/broadcast/"
	listen    = "/listen/"
)

type Status struct {
	Channel   string
	Listeners int
	Messages  int
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	default:
		http.NotFound(w, r)

	case p == status:
		var ls []Status
		for p := range s.m {
			ls = append(ls, Status{p, len(s.m[p]), s.t[p]})
		}
		js, _ := json.Marshal(ls)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

	case strings.HasPrefix(p, broadcast):
		p = strings.TrimPrefix(p, broadcast)
		s.broadcast(p, r.FormValue("message"))
		w.Header().Set("Access-Control-Allow-Origin", "*")
		fmt.Fprintf(w, "OK")

	case strings.HasPrefix(p, listen):
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print(err)
			return
		}

		p = strings.TrimPrefix(r.URL.Path, listen)
		c := s.listen(p)
		defer s.hangup(p, c)

		for m := range c {
			err := conn.WriteMessage(websocket.TextMessage, []byte(m))
			if err != nil {
				log.Print(err)
				return
			}
		}
	}
}

func (s *Server) listen(p string) <-chan string {
	c := make(chan string)
	s.mu.Lock()
	s.m[p] = append(s.m[p], c)
	s.mu.Unlock()
	return c
}

func (s *Server) hangup(p string, c <-chan string) {
	// Remove channel from listener map.
	s.mu.Lock()
	ls := s.m[p]
	for i := range ls {
		if ls[i] == c {
			ls = append(ls[:i], ls[i+1:]...)
			break
		}
	}
	s.m[p] = ls
	s.mu.Unlock()

	// Drain channel for a minute, to unblock any in-flight senders.
	go func() {
		timeout := time.After(1 * time.Minute)
		for {
			select {
			case <-c:
			case <-timeout:
				return
			}
		}
	}()
}

func (s *Server) broadcast(p, m string) {
	s.mu.Lock()
	ls := append([]chan string{}, s.m[p]...) // copy
	s.t[p] += 1
	s.mu.Unlock()
	for _, c := range ls {
		c <- m
	}
}
