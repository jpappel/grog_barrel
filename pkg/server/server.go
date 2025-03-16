package server

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

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

func barrel(logger *slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error(err.Error())
			return
		}
		defer c.Close()

		driver := WsDriver{c, logger}

		client, err := driver.ParseClient()
		if err == ErrIncompatibleVersion || err == ErrInvalidClientName {
			driver.WriteError(err.Error())
			return
		} else if err != nil {
			driver.WriteError("Internal Server Error")
			return
		}

		clientInfo := slog.Group("clientInfo",
			slog.String("version", client.Version.String()),
			slog.String("name", client.Name),
			slog.String("addr", client.Addr),
		)
		logger = logger.With(clientInfo)

		roomName := r.PathValue("roomName")

		room, ok := rooms[roomName]
		if !ok {
			rooms[roomName] = grog.NewRoom(roomName, logger)
			room = rooms[roomName]
		}

		id, err := room.Join(client)
		if err == grog.ErrRoomFull {
			logger.Debug("Room is full")
			driver.WriteError("Room is full")
			return
		} else if err != nil {
			logger.Error("Unexpected error occured while joining",
				slog.String("roomName", roomName),
				slog.String("err", err.Error()),
			)
			driver.WriteError("Internal Server Error")
			return
		}
		defer room.Leave(id)

		roomInfo := slog.Group("roomInfo",
			slog.String("roomName", roomName),
			slog.Int("clientRoomId", int(id)),
		)
		logger = logger.With(roomInfo)
		logger.Info("User Joined Room")

		lastAnnouncement := 0
		updates := false

		for {
			lastAnnouncement, updates = room.Check(lastAnnouncement)
			if updates {
				if err := c.WritePreparedMessage(room.Messages.PreparedAnnounce); err != nil {
					logger.Error("Error while writting announcement",
						slog.String("error", err.Error()),
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
				)
				break
			}

			msg := parseStatusMessage(message, id)

			logger.Debug("recieved message",
				slog.String("content", msg.String()),
			)
			room.Update(client.Addr, msg)

			if err := c.WritePreparedMessage(room.Messages.PreparedStatus); err != nil {
				logger.Error("Error while writting",
					slog.String("error", err.Error()),
				)
				driver.WriteError("Internal Server Error")
				break
			}
		}
		logger.Info("Closing web socket connection")
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
