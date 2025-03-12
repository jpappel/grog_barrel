package grog

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpappel/grog_barrel/pkg/util"
)

const MAX_CONNECTIONS = 256

var ErrRoomFull error = errors.New("Room is at capacity")

// Name limited to a length of 255
type Client struct {
	Name    string
	Addr    string
	Version util.SemVer
}

type Room struct {
	Name        string
	Connections atomic.Int32
	// TODO: convert into list of drivers
	Driver      WebSocketDriver
	Open        bool
	statuses    sync.Map
	wg          sync.WaitGroup
	usersChange chan bool
	ids         struct {
		vals [MAX_CONNECTIONS]Client
		sync.RWMutex
	}
	lastAnnounce int
	logger       *slog.Logger
}

func NewRoom(name string, logger *slog.Logger) *Room {
	r := new(Room)

	r.Name = name
	r.logger = logger

	// PERF: profile channel size
	r.usersChange = make(chan bool, 5)

	return r
}

func (r *Room) Join(client Client) (byte, error) {
	r.ids.Lock()
	defer r.ids.Unlock()

	for i, v := range r.ids.vals {
		if v.Addr != "" {
			continue
		}

		r.wg.Add(1)
		conns := r.Connections.Add(1)

		r.ids.vals[i] = client

		if conns == 1 {
			go r.run()
		}
		r.usersChange <- true
		return byte(i), nil
	}
	return 0, ErrRoomFull
}

func (r *Room) Leave(id byte) {
	r.ids.Lock()
	defer r.ids.Unlock()

	user := r.ids.vals[id]
	if user.Addr == "" {
		return
	}

	r.statuses.Delete(user.Addr)
	r.ids.vals[id].Addr = ""

	r.wg.Done()
	if conns := r.Connections.Add(-1); conns < 0 {
		r.logger.Error("Invalid numer of connections",
			slog.String("roomName", r.Name), slog.Int("connections", int(conns)))
		panic("Negative Number of connections")
	}

	r.usersChange <- true
}

func (r *Room) Update(addr string, msg StatusMessage) {
	r.statuses.Store(addr, msg)
}

// Build the status message continuously until room.done
func (r *Room) buildStatus(buf *[]byte) error {
	r.statuses.Range(func(k, v any) bool {
		msg := v.(StatusMessage)
		*buf = binary.BigEndian.AppendUint16(*buf, msg.Offset)
		*buf = append(*buf, byte(msg.PlayerState))
		*buf = append(*buf, msg.Id)
		return true
	})

	if err := r.Driver.WriteStatus(*buf); err != nil {
		r.logger.Error("Failed to prepare status message",
			slog.String("roomName", r.Name),
			slog.String("err", err.Error()),
			slog.Int("msgLen", len(*buf)),
		)
		return err
	}

	return nil
}

// Check for new Announcements
func (r *Room) Check(lastAnnounce int) (int, bool) {
	return max(r.lastAnnounce, lastAnnounce), r.lastAnnounce > lastAnnounce
}

func (r *Room) buildAnnounce(buf *[]byte) error {
	r.ids.RLock()
	defer r.ids.RUnlock()

	// NOTE: this only works when MAX_CONNECTIONS <= 255
	*buf = append(*buf, byte(r.Connections.Load()))

	for i, client := range r.ids.vals {
		if client.Addr == "" {
			continue
		}

		*buf = append(*buf, byte(i))
		*buf = append(*buf, byte(len(client.Name)))
		*buf = append(*buf, client.Name...)
	}

	if err := r.Driver.WriteAnnounce(*buf); err != nil {
		r.logger.Error("Failed to prepare announce message",
			slog.String("roomName", r.Name))
		return err
	}

	return nil
}

func (r *Room) runStatus(done <-chan struct{}, d time.Duration) {
	buf := make([]byte, 0, 1024)
	buf = append(buf, byte(STATUS_MSG))

	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		buf = buf[:1]
		select {
		case <-done:
			break
		case <-ticker.C:
			if err := r.buildStatus(&buf); err != nil {
				// TODO: propagate error
				panic(err)
			}
		}
	}
}

func (r *Room) runAnnounce(done <-chan struct{}, usersChange <-chan bool) {
	buf := make([]byte, 0, 1024)
	buf = append(buf, byte(ANNOUNCE_MSG))
	for {
		buf = buf[:1]
		select {
		case <-done:
			break
		case <-usersChange:
			if err := r.buildAnnounce(&buf); err != nil {
				// TODO: propagate error
				panic(err)
			}
			r.lastAnnounce += 1
		}
	}
}

// Start Generating the prepared messages after recieiving on start channel
func (r *Room) run() {
	statusDone := make(chan struct{})
	announceDone := make(chan struct{})
	defer close(statusDone)
	defer close(announceDone)

	go r.runStatus(statusDone, 1*time.Second)
	go r.runAnnounce(announceDone, r.usersChange)
	r.wg.Wait()
}
