package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
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

var tmpl *template.Template
var logger *slog.Logger
var rooms map[string]*Room

type Room struct {
	name        string
	connections sync.Map
	ids         [256]string // max number of connections is 256
	idsLock     sync.RWMutex
}

type inMsg struct {
	offset      uint16      // current timestamp in file
	playerState PlayerState // playerState
	id          byte        // consistent id from client
}

func (r *Room) Register(addr string) byte {
	r.idsLock.Lock()
	defer r.idsLock.Unlock()

	for i, v := range r.ids {
		if v == "" {
			r.ids[i] = addr
			return byte(i)
		}
	}
	return 0
}

func (r *Room) Free(id byte) {
	r.idsLock.Lock()
	defer r.idsLock.Unlock()

	remoteAddr := r.ids[id]
	r.connections.Delete(remoteAddr)
	r.ids[id] = ""
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
	tmpl.Execute(w, nil)
}

func script(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "client.js")
}

var upgrader = websocket.Upgrader{}

func barrel(w http.ResponseWriter, r *http.Request) {
	roomName := r.PathValue("roomName")
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(err.Error())
	}
	defer c.Close()

	room, ok := rooms[roomName]
	if !ok {
		rooms[roomName] = &Room{
			name:        roomName,
			connections: sync.Map{},
			idsLock:     sync.RWMutex{},
		}
		room = rooms[roomName]
	}

	remoteAddr := c.RemoteAddr().String()
	id := room.Register(remoteAddr)
	defer room.Free(id)

	logger.Info("Opened websocket connection",
		slog.String("roomName", roomName),
		slog.String("remote", remoteAddr),
		slog.Int("id", int(id)),
	)

	for {
		// TODO: check message type for errors
		_, message, err := c.ReadMessage()
		if err != nil {
			logger.Error("Error while reading", slog.Any("error", err))
			break
		}
		// parse message
		msg := inMsg{
			offset:      binary.BigEndian.Uint16(message[:2]),
			playerState: PlayerState(message[2]),
			id:          id,
		}

		logger.Debug("recieved message",
			slog.String("roomName", roomName),
			slog.String("remote", remoteAddr),
			slog.Int("id", int(id)),
			slog.String("content", msg.String()),
		)
		room.connections.Store(remoteAddr, msg)

		buf := make([]byte, 0)
		room.connections.Range(func(k, v any) bool {
			msg := v.(inMsg)
			buf = binary.BigEndian.AppendUint16(buf, msg.offset)
			buf = append(buf, byte(msg.playerState))
			buf = append(buf, msg.id)
			return true
		})

		if err := c.WriteMessage(2, buf); err != nil {
			logger.Error("Error while writting", slog.Any("error", err))
			break
		}
	}
	logger.Info("Closing web socket connection",
		slog.String("roomName", roomName),
		slog.String("remote", remoteAddr),
		slog.Int("id", int(id)),
	)
}

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	hostname := flag.String("hostname", "localhost", "hostname to listen on")
	loglvl := flag.String("l", "warn", "log level (debug, info, warn, error)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}
	flag.Parse()

	loggerOpts := new(slog.HandlerOptions)
	switch *loglvl {
	case "debug":
		loggerOpts.Level = slog.LevelDebug
		loggerOpts.AddSource = true
	case "info":
		loggerOpts.Level = slog.LevelInfo
	case "warn":
		loggerOpts.Level = slog.LevelWarn
	case "error":
		loggerOpts.Level = slog.LevelError
	default:
		panic(fmt.Sprintf("Unkown log level %s", *loglvl))
	}
	logger = slog.New(slog.NewTextHandler(os.Stdout, loggerOpts))

	addr := fmt.Sprintf("%s:%d", *hostname, *port)

	rooms = make(map[string]*Room)

	// parse templtes
	var err error
	tmpl, err = template.ParseFiles("templates/index.html")
	if err != nil {
		logger.Error("Unable to parse templates", slog.String("Error", err.Error()))
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/barrel/{roomName}", barrel)
	mux.HandleFunc("/client.js", script)
	mux.HandleFunc("/", home)

	logger.Info("Starting server",
		slog.String("bindAddress", addr))

	logger.Info(http.ListenAndServe(addr, mux).Error())
}
