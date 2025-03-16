package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"

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

func (d UnixDriver) WriteError(msg string) error {
	buf := make([]byte, 0, 1+len(msg))
	buf = append(buf, byte(3))
	buf = append(buf, msg...)

	_, err := d.conn.Write(buf)
	return err
}

func (d UnixDriver) ParseClient() (grog.Client, error) {
	// PERF: reuse buffer to avoid pressure on GC
	buf := make([]byte, 0, 259)
	n, err := d.conn.Read(buf)
	if err != nil {
		return grog.Client{}, err
	} else if n < 3 || n > 259 {
		errMsg := fmt.Sprint("Invalid clientAnnounce length:", n)
		return grog.Client{}, errors.New(errMsg)
	}

	clientAddr := d.conn.RemoteAddr().String()
	client, err := parseClient(buf, clientAddr, d.logger)
	return client, err
}

// Read a room name from a connection and attempt to return the corresponding room
func (d UnixDriver) ParseRoom() (*grog.Room, error) {
	buf := make([]byte, 0, 256)
	n, err := d.conn.Read(buf)
	if err != nil {
		return nil, err
	} else if n > 256 {
		return nil, ErrInvalidRoomName
	}

	name := string(buf[:n])
	room, ok := rooms[name]
	if !ok {
		rooms[name] = grog.NewRoom(name, d.logger)
		room = rooms[name]
	}

	// NOTE: file mode could be a potential security concern
	if err := os.Mkdir(d.baseDir+"/"+name, 0775); err != nil {
		return nil, err
	}

	return room, nil
}

func handleNewConn(d UnixDriver, clientRooms chan<- ClientRoom, more chan<- bool) {
	client, err := d.ParseClient()
	if err != nil {
		d.WriteError(err.Error())
	}

	room, err := d.ParseRoom()
	client.Addr = d.baseDir + "/" + room.Name + "/" + client.Name
	clientRooms <- ClientRoom{client, room}

	if _, err := d.conn.Write([]byte(client.Addr)); err != nil {
		d.logger.Error("Error sending client addr")
		panic(err)
	}

	d.conn.Close()
	more <- true
}

// TODO: clean up this ugly af function
func listenNewConns(ctx context.Context,
	ln *net.UnixListener,
	clientRooms chan<- ClientRoom,
	baseDir string,
	logger *slog.Logger,
) {
	more := make(chan bool)
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

			driver := UnixDriver{conn, logger, baseDir}
			go handleNewConn(driver, clientRooms, more)
		}
	}
}

func handleClient(client grog.Client, room *grog.Room) {
	addr := net.UnixAddr{Name: client.Addr, Net: "Unix"}
	ln, err := net.ListenUnix("unix", &addr)
	if err != nil {
		panic(err)
	}

	conn, err := ln.AcceptUnix()
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	id, err := room.Join(client)
	if err != nil {
		panic(err)
	}
	defer room.Leave(id)

	lastAnnouncement := 0
	updates := false

	buf := make([]byte, 0, 1024)

	for {
		lastAnnouncement, updates = room.Check(lastAnnouncement)
		if updates {
			if _, err := conn.Write(room.Messages.announcements); err != nil {
				// TODO: log error
				break
			}
		}

		_, err := conn.Read(buf)
		if err != nil {
			// TODO: log error
			break
		}

		msg := parseStatusMessage(buf, id)
		room.Update(client.Addr, msg)

		if _, err := conn.Write(room.Messages.status); err != nil {
			break
		}

		buf = buf[:1]
	}
}

// TODO: add cancel to function
func listenClients(clientRooms <-chan ClientRoom) {
	for {
		clientRoom := <-clientRooms
		go handleClient(clientRoom.Client, clientRoom.Room)
	}
}

func SockServer() {
	logger := slog.New(slog.Default().Handler())

	baseDir, err := os.MkdirTemp("", "grogbarrel_*")
	if err != nil {
		logger.Error("Error occured while creating base dir")
		panic(err)
	}
	defer os.RemoveAll(baseDir)

	newConns := net.UnixAddr{Name: baseDir + "/join.sock", Net: "Unix"}
	newLn, err := net.ListenUnix("unix", &newConns)
	if err != nil {
		logger.Error("error opening new connection socket")
		panic(err)
	}
	defer newLn.Close()

	ctx, newConnsCancel := context.WithCancel(context.Background())
	clientRooms := make(chan ClientRoom)
	go listenNewConns(ctx, newLn, clientRooms, baseDir, logger)
	defer newConnsCancel()

	listenClients(clientRooms)
}
