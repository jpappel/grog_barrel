package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type PlayerState byte

const (
	UNKNOWN PlayerState = iota
	PLAYING
	PAUSED
	LOADING
)

// PERF: unused space in struct
type inMsg struct {
	offset      uint16      // current timestamp in file
	playerState PlayerState // playerState
}

func (m inMsg) String() string {
	state := ""
	if m.playerState == UNKNOWN {
		state = "UNKNOWN"
	} else if m.playerState == PLAYING {
		state = "PLAYING"
	} else if m.playerState == PAUSED {
		state = "PAUSED"
	} else if m.playerState == LOADING {
		state = "LOADING"
	}
	return fmt.Sprintf("%s %d", state, m.offset)
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "<h1>Hello World!</h1>")
}

var upgrader = websocket.Upgrader{}
var shared *sync.Map

func barrel(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error(err.Error())
	}
	defer c.Close()

	remoteAddr := c.RemoteAddr().String()
	slog.Info("Opened web socket connection", slog.String("remote", remoteAddr))

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			slog.Error("Error while reading", slog.Any("error", err))
			break
		}
		// parse message
		msg := inMsg{
			offset:      binary.BigEndian.Uint16(message[:2]),
			playerState: PlayerState(message[2]),
		}

		slog.Info("recieved message", slog.String("msg", msg.String()))
		shared.Store(c.RemoteAddr(), msg)

		buf := make([]byte, 0, 128)
		shared.Range(func(k, v any) bool {
			msg := v.(inMsg)
			binary.BigEndian.AppendUint16(buf, msg.offset)
			buf = append(buf, byte(msg.playerState))
			buf = append(buf, []byte(remoteAddr)...)
			buf = append(buf, byte(0)) // NOTE: null byte delimits clients
			return true
		})

		if c.WriteMessage(2, buf) != nil {
			slog.Error("Error while writting", slog.Any("error", err))
			break
		}
	}

	shared.Delete(remoteAddr)
}

func main() {
	const HOSTNAME string = "localhost"
	const PORT int = 8080
	addr := fmt.Sprintf("%s:%d", HOSTNAME, PORT)

	shared = &sync.Map{}

	mux := http.NewServeMux()
	mux.HandleFunc("/", home)
	mux.HandleFunc("/barrel", barrel)

	slog.Info("Starting server", slog.String("bindAddress", addr))

	log.Fatal(http.ListenAndServe(addr, mux))
}
