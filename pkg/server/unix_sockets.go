package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/jpappel/grog_barrel/pkg/grog"
)

type ClientRoom struct {
	Client grog.Client
	Room   *grog.Room
}

type UnixDriver struct {
	conn    *net.UnixConn
	logger  *slog.Logger
	baseDir string
}

type SockServer struct {
	shutdown chan struct{}
	baseDir  string
	logger   *slog.Logger
}

func (d UnixDriver) WriteError(msg string) error {
	buf := make([]byte, 0, 1+len(msg))
	buf = append(buf, byte(3))
	buf = append(buf, msg...)

	_, err := d.conn.Write(buf)
	return err
}

func (d UnixDriver) ParseClient() (grog.Client, error) {
	// PERF: reuse buffer to avoid pressure on GC
	buf := make([]byte, 259)
	n, err := d.conn.Read(buf)
	if err != nil {
		return grog.Client{}, err
	} else if n < 3 || n > 259 {
		errMsg := fmt.Sprint("Invalid clientAnnounce length:", n)
		return grog.Client{}, errors.New(errMsg)
	}
	buf = buf[:n]

	clientAddr := d.conn.LocalAddr().String()
	client, err := parseClient(buf, clientAddr, d.logger)
	return client, err
}

// Read a room name from a connection and attempt to return the corresponding room
func (d UnixDriver) ParseRoom() (*grog.Room, error) {
	buf := make([]byte, 256)
	n, err := d.conn.Read(buf)
	if err != nil {
		return nil, err
	} else if n > 256 {
		return nil, ErrInvalidRoomName
	}
	buf = buf[:n]

	name := string(buf)
	room, ok := rooms[name]
	if !ok {
		rooms[name] = grog.NewRoom(name, d.logger)
		room = rooms[name]
	}

	// NOTE: file mode could be a potential security concern
	if err := os.Mkdir(d.baseDir+"/"+name, 0775); err != nil && !os.IsExist(err) {
		return nil, err
	}

	return room, nil
}

func handleNewConn(d UnixDriver, clientRooms chan<- ClientRoom, more chan<- bool) {
	defer func() {
		d.conn.Close()
		more <- true
	}()

	//FIXME: set reasonable deadline
	d.conn.SetDeadline(time.Now().Add(50 * time.Second))
	client, err := d.ParseClient()
	if err == io.EOF {
		d.WriteError("Unexpected end of message")
		return
	} else if os.IsTimeout(err) {
		d.WriteError("Took too long to send clientAnnounce")
		d.logger.Info("Timed out while waiting for clientAnnounce")
		return
	} else if err != nil {
		errStr := err.Error()
		d.WriteError(errStr)
		d.logger.Warn("Error occured while parsing client", slog.String("err", errStr))
		return
	}
	d.logger.Debug("Parsed Client", slog.String("client", client.String()))

	d.conn.SetDeadline(time.Now().Add(50 * time.Second))
	room, err := d.ParseRoom()
	if err == io.EOF {
		d.WriteError("Unexpected end of message")
		return
	} else if os.IsTimeout(err) {
		d.WriteError("Took too long to send room name")
		d.logger.Info("Timed out while waiting for room name")
		return
	} else if err != nil {
		errStr := err.Error()
		d.WriteError(errStr)
		d.logger.Warn("Error occured while parsing room", slog.String("err", errStr))
		return
	}
	client.Addr = d.baseDir + "/" + room.Name + "/" + client.Name
	clientRooms <- ClientRoom{client, room}

	d.conn.SetDeadline(time.Now().Add(150 * time.Second))
	if _, err := d.conn.Write([]byte(client.Addr)); err != nil {
		d.logger.Error("Error sending client addr")
		panic(err)
	}
}

func (s *SockServer) listenNewConns(ctx context.Context,
	ln *net.UnixListener,
	clientRooms chan<- ClientRoom,
) {
	more := make(chan bool, 1)
	go func() {
		more <- true
	}()
	defer close(more)

	for {
		select {
		case <-ctx.Done():
			break
		case <-more:
			conn, err := ln.AcceptUnix()
			if err != nil {
				// TODO: handle error
				break
			}
			s.logger.Info("New connection", slog.String("addr", conn.RemoteAddr().String()))

			driver := UnixDriver{conn, s.logger, s.baseDir}
			go handleNewConn(driver, clientRooms, more)
		}
	}
}

func handleClientConn(ctx context.Context, client grog.Client, room *grog.Room, logger *slog.Logger) {
	addr := net.UnixAddr{Name: client.Addr, Net: "Unix"}
	socketCtx, cancelSocket := context.WithCancel(ctx)
	logger = logger.With(slog.String("addr", client.Addr))
	ln, err := net.ListenUnix("unix", &addr)
	if err != nil {
		logger.Error("Failed to listen for client connection",
			slog.String("err", err.Error()),
		)
		panic(err)
	}
	ln.SetUnlinkOnClose(true)
	go func() {
		<-socketCtx.Done()
		ln.Close()
	}()
	defer cancelSocket()

	// NOTE: consider changing deadline
	ln.SetDeadline(time.Now().Add(1 * time.Minute))

	conn, err := ln.AcceptUnix()
	if os.IsTimeout(err) {
		logger.Warn("Timed out while waiting for client connection")
		return
	} else if err != nil {
		logger.Error("Error occured while accepting client connection",
			slog.String("err", err.Error()),
		)
		panic(err)
	}
	defer conn.Close()
	defer logger.Info("Closing connection")

	id, err := room.Join(client)
	if err != nil {
		logger.Error("Failed to join room", slog.String("err", err.Error()))
		panic(err)
	}
	defer room.Leave(id)

	logger = logger.With(slog.Group("client",
		slog.Int("id", int(id)),
		slog.String("name", client.Name),
	))
	lastAnnouncement := 0
	updates := false

	buf := make([]byte, 8)

	// poll until first room has built a new announcement
	for range 5 {
		lastAnnouncement, updates = room.Check(lastAnnouncement)
		if updates {
            // FIXME: bufer has length of 1 instead of correct amount
			announcement := room.Messages.Announcements()
			logger.Debug("Sending first serverAnnounce", slog.Int("len", len(announcement)))
			if _, err := conn.Write(announcement); err != nil {
				logger.Error("Failed to send first serverAnnounce",
					slog.String("err", err.Error()),
				)
				return
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !updates {
		logger.Error("Room never built a new announcement for new client")
		return
	}
	updates = false

	for {
		lastAnnouncement, updates = room.Check(lastAnnouncement)
		logger.Debug("announcement Check",
			slog.Int("lastAnnounce", lastAnnouncement),
			slog.Bool("updates", updates),
		)
		if updates {
			announcement := room.Messages.Announcements()
			logger.Debug("Sending serverAnnounce", slog.Int("len", len(announcement)))
			if _, err := conn.Write(announcement); err != nil {
				// TODO: log error
				break
			}
			updates = false
		}

		conn.SetDeadline(time.Now().Add(15 * time.Minute))
		logger.Debug("Waiting on clientStatus")
		n, err := conn.Read(buf)
		if n != 3 {
			logger.Warn("Incorrect read size for clientStatus", slog.Int("size", n))
			// TODO: write error to client
			break
		} else if err == io.EOF {
			break
		} else if err != nil {
			logger.Error("Error while reading client status",
				slog.String("err", err.Error()),
			)
			break
		}
		buf = buf[:n]

		msg := parseStatusMessage(buf, id)
		room.Update(client, msg)

		status := room.Messages.Status()
		logger.Debug("status", slog.Int("len", len(status)))
		conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
		if _, err := conn.Write(status); err != nil {
			break
		}

		// buf = buf[:1]
	}
}

func listenClients(ctx context.Context, clientRooms <-chan ClientRoom, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			break
		case clientRoom := <-clientRooms:
			go handleClientConn(ctx, clientRoom.Client, clientRoom.Room, logger)
		}
	}
}

func NewSockServer(baseDir string, logger *slog.Logger) *SockServer {
	srv := &SockServer{
		baseDir: baseDir,
		logger:  logger,
	}

	return srv
}

func (s *SockServer) Run(ctx context.Context) {
	ln, err := net.ListenUnix("unix",
		&net.UnixAddr{Name: s.baseDir + "/join.sock", Net: "Unix"},
	)
	if err != nil {
		s.logger.Error("error opening new connection socket",
			slog.String("newConnAddr", ln.Addr().String()),
		)
		panic(err)
	}
	defer ln.Close()
	s.logger.Info("Opened socket for new connections",
		slog.String("sockAddr", ln.Addr().String()),
	)

	// PERF: profile channel buffer size
	clientRooms := make(chan ClientRoom, 5)
	go s.listenNewConns(ctx, ln, clientRooms)
	s.logger.Info("Listening for new connections on socket",
		slog.String("sockAddr", ln.Addr().String()),
	)
	defer close(clientRooms)

	s.logger.Info("Listening for new socket clients")
	listenClients(ctx, clientRooms, s.logger)
}
