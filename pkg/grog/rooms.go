package grog

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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

type Messages struct {
	/* NOTE: consider using a double buffer to avoid lock contention
	   could possible be implemented as a bool to swap between
	   the two buffers.
	*/
	status           []byte
	announcements    []byte
	PreparedStatus   *websocket.PreparedMessage
	PreparedAnnounce *websocket.PreparedMessage
	statusLock       sync.RWMutex
	announcementLock sync.RWMutex
}

type Room struct {
	Name        string
	Connections atomic.Int32
	Messages    Messages
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

func (c Client) String() string {
	return fmt.Sprintf("%s @ %s : %s", c.Name, c.Addr, c.Version.String())
}

func (m *Messages) Status() []byte {
	// FIXME: idk if this actually protects the slice for reading
	m.statusLock.RLock()
	defer m.statusLock.RUnlock()
	return m.status
}

func (m *Messages) Announcements() []byte {
	m.announcementLock.RLock()
	defer m.announcementLock.RUnlock()
	return m.announcements
}

func NewRoom(name string, logger *slog.Logger) *Room {
	r := new(Room)

	r.Name = name
	r.logger = logger

	r.Messages.status = make([]byte, 2, 1024)
	r.Messages.announcements = make([]byte, 1, 1024)
	r.Messages.status[0] = byte(STATUS_MSG)
	r.Messages.announcements[0] = byte(ANNOUNCE_MSG)

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

		if conns == 1 && !r.Open {
			r.Open = true
			go r.run()
		}
		r.usersChange <- true
		r.logger.Debug("User Joined")
		return byte(i), nil
	}
	r.logger.Warn("Room Full")
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
	} else if conns == 0 {
		r.Open = false
	}

	r.usersChange <- true
}

func (r *Room) Update(client Client, msg ClientStatusMessage) {
	r.statuses.Store(client.Addr, msg)
}

// Build the status message continuously until room.done
func (r *Room) buildStatus() error {
	r.Messages.statusLock.Lock()
	r.Messages.status = r.Messages.status[:2]

	count := 0
	r.statuses.Range(func(k, v any) bool {
		msg := v.(ClientStatusMessage)
		r.Messages.status = msg.WriteBytes(r.Messages.status)
		count++
		return true
	})
	r.Messages.statusLock.Unlock()
	r.Messages.status[1] = byte(count)

	var err error
	r.Messages.statusLock.RLock()
	defer r.Messages.statusLock.RUnlock()
	r.Messages.PreparedStatus, err = websocket.NewPreparedMessage(2, r.Messages.status)
	if err != nil {
		r.logger.Error("Failed to prepare status message",
			slog.String("roomName", r.Name),
			slog.String("err", err.Error()),
			slog.Int("msgLen", len(r.Messages.status)),
		)
		return err
	}

	return nil
}

// Check for new Announcements
func (r *Room) Check(lastAnnounce int) (int, bool) {
	r.Messages.announcementLock.RLock()
	defer r.Messages.announcementLock.RUnlock()
	return max(r.lastAnnounce, lastAnnounce), r.lastAnnounce > lastAnnounce
}

func (r *Room) buildAnnounce() error {
	r.ids.RLock()
	r.Messages.announcementLock.Lock()
	r.Messages.announcements = r.Messages.announcements[:1]

	// NOTE: this only works when MAX_CONNECTIONS <= 255
	r.Messages.announcements = append(r.Messages.announcements, byte(r.Connections.Load()))
	// r.Messages.announcements = append(r.Messages.announcements, 0)

	for id, client := range r.ids.vals {
		if client.Addr == "" {
			continue
		}

		r.Messages.announcements = append(r.Messages.announcements, byte(id))
		r.Messages.announcements = append(r.Messages.announcements, byte(len(client.Name)))
		r.Messages.announcements = append(r.Messages.announcements, client.Name...)
	}
	r.ids.RUnlock()
	// r.Messages.announcements[0] = byte(connections)
	r.Messages.announcementLock.Unlock()

	var err error
	r.Messages.PreparedAnnounce, err = websocket.NewPreparedMessage(2, r.Messages.announcements)
	if err != nil {
		r.logger.Error("Failed to prepare announce message",
			slog.String("roomName", r.Name))
		return err
	}

	return nil
}

func (r *Room) runStatus(done <-chan struct{}, d time.Duration) {

	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			break
		case <-ticker.C:
			if err := r.buildStatus(); err != nil {
				panic(err)
			}
		}
	}
}

func (r *Room) runAnnounce(done <-chan struct{}, usersChange <-chan bool) {
	for {
		select {
		case <-done:
			break
		case <-usersChange:
			if err := r.buildAnnounce(); err != nil {
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
