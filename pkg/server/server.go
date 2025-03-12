package server

import (
	"encoding/binary"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jpappel/grog_barrel/pkg/grog"
	"github.com/jpappel/grog_barrel/pkg/util"
)

var ServerVersion = util.ServerVersion

var tmpl *template.Template
var upgrader = websocket.Upgrader{}

var rooms map[string]*grog.Room

func home(w http.ResponseWriter, r *http.Request) {
	tmpl.Execute(w, nil)
}

func script(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "client.js")
}

func writeError(c *websocket.Conn, msg string) error {
	buf := make([]byte, 0, 1+len(msg))
	buf = append(buf, byte(3))
	buf = append(buf, msg...)

	deadline := time.Now().Add(1 * time.Second)
	return c.WriteControl(websocket.CloseMessage, buf, deadline)
}

func barrel(logger *slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		defer c.Close()

		client, err := parseClient(c, logger)
		if err == ErrIncompatibleVersion || err == ErrInvalidName {
			writeError(c, err.Error())
			return
		} else if err != nil {
			writeError(c, "Internal Server Error")
			return
		}

		clientInfo := slog.Group("clientInfo",
			slog.String("version", client.Version.String()),
			slog.String("name", client.Name),
			slog.String("addr", client.Addr),
		)
		logger.Debug("Version Negotiated", clientInfo)

		roomName := r.PathValue("roomName")

		room, ok := rooms[roomName]
		if !ok {
			rooms[roomName] = grog.NewRoom(roomName, logger)
			room = rooms[roomName]
		}

		id, err := room.Join(client)
		if err == grog.ErrRoomFull {
			logger.Debug("Room is full")
			writeError(c, "Room is full")
			return
		} else if err != nil {
			logger.Error("Unexpected error occured while joining",
				slog.String("roomName", roomName),
				clientInfo,
				slog.String("err", err.Error()),
			)
			writeError(c, "Internal Server Error")
			return
		}
		defer room.Leave(id)

		roomInfo := slog.Group("roomInfo",
			slog.String("roomName", roomName),
			slog.Int("clientRoomId", int(id)),
		)
		logger.Info("User Joined Room", roomInfo, clientInfo)

		lastAnnouncement := 0
		updates := false

		for {
			lastAnnouncement, updates = room.Check(lastAnnouncement)
			if updates {
				if err := c.WritePreparedMessage(room.Driver.Announcements); err != nil {
					logger.Error("Error while writting announcement",
						slog.String("error", err.Error()),
						roomInfo,
						clientInfo,
					)
					break
				}
			}

			_, message, err := c.ReadMessage()
			if websocket.IsCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived) {
				break
			} else if err != nil {
				logger.Error("Error while reading message",
					slog.Any("error", err),
					roomInfo,
					clientInfo,
				)
				break
			}

			msg := grog.StatusMessage{
				Offset:      binary.BigEndian.Uint16(message[:2]),
				PlayerState: grog.PlayerState(message[2]),
				Id:          id,
			}

			logger.Debug("recieved message",
				roomInfo,
				clientInfo,
				slog.String("content", msg.String()),
			)
			room.Update(client.Addr, msg)

			if err := c.WritePreparedMessage(room.Driver.Status); err != nil {
				logger.Error("Error while writting",
					slog.String("error", err.Error()),
					roomInfo,
					clientInfo,
				)
				writeError(c, "Internal Server Error")
				break
			}
		}
		logger.Info("Closing web socket connection", roomInfo, clientInfo)
	}
}

func New(l *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/barrel/{roomName}", http.HandlerFunc(barrel(l)))
	mux.HandleFunc("/client.js", script)
	mux.HandleFunc("/", home)

	return mux
}

func init() {
	rooms = make(map[string]*grog.Room)

	// parse templates
	var err error
	tmpl, err = template.ParseFiles("templates/index.html")
	if err != nil {
		fmt.Println("Unable to parse templates")
		panic(err)
	}
}
