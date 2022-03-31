package tunnel

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/andrewstucki/light/tunnel/proto"
)

var heartbeatTimeout = 5 * time.Second

type requestChannel struct {
	ctx       context.Context
	heartbeat time.Time
	requests  chan (*proto.APIRequest)
	responses chan (*proto.APIResponse)

	mutex  sync.RWMutex
	cancel func()
}

func newRequestChannel() *requestChannel {
	ctx, cancel := context.WithCancel(context.Background())
	return &requestChannel{
		ctx:       ctx,
		heartbeat: time.Now(),
		requests:  make(chan *proto.APIRequest),
		responses: make(chan *proto.APIResponse),
		cancel:    cancel,
	}
}

func (r *requestChannel) close() {
	r.cancel()
}

func (r *requestChannel) send(ctx context.Context, request *proto.APIRequest) (*proto.APIResponse, error) {
	select {
	case <-ctx.Done():
		return nil, io.EOF
	case <-r.ctx.Done():
		return nil, io.EOF
	case r.requests <- request:
		select {
		case <-ctx.Done():
			return nil, io.EOF
		case <-r.ctx.Done():
			return nil, io.EOF
		case response := <-r.responses:
			return response, nil
		}
	}
}

func (r *requestChannel) handle(fn func(*proto.APIRequest) (*proto.APIResponse, error)) error {
	for {
		select {
		case <-r.ctx.Done():
			return io.EOF
		case request := <-r.requests:
			response, err := fn(request)
			if err != nil {
				return err
			}
			select {
			case <-r.ctx.Done():
				return io.EOF
			case r.responses <- response:
			}
		}
	}
}

type tunnelID struct {
	id    string
	nonce string
}

type tunnelRegistry struct {
	ids      map[string]tunnelID
	sessions map[tunnelID]*requestChannel

	mutex sync.RWMutex
}

func newTunnelRegistry() *tunnelRegistry {
	return &tunnelRegistry{
		ids:      make(map[string]tunnelID),
		sessions: make(map[tunnelID]*requestChannel),
	}
}

func (r *tunnelRegistry) createSession(id string) (string, bool, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if tunnelID, ok := r.ids[id]; ok {
		if _, ok := r.sessions[tunnelID]; ok {
			return "", false, nil
		}
	}
	serial, err := serialNumber()
	if err != nil {
		return "", false, err
	}
	nonce := serial.Text(32)
	tunnelID := tunnelID{
		id:    id,
		nonce: nonce,
	}
	r.ids[id] = tunnelID
	r.sessions[tunnelID] = newRequestChannel()
	return nonce, true, nil
}

func (r *tunnelRegistry) sessionByID(id string) (*requestChannel, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if tunnelID, ok := r.ids[id]; ok {
		if session, ok := r.sessions[tunnelID]; ok {
			return session, true
		}
	}
	return nil, false
}

func (r *tunnelRegistry) get(id tunnelID) (*requestChannel, bool) {
	r.mutex.RLock()
	session, ok := r.sessions[id]
	r.mutex.RUnlock()
	return session, ok
}

func (r *tunnelRegistry) clear(id tunnelID) {
	r.mutex.Lock()
	session, ok := r.sessions[id]
	if ok {
		session.close()
	}
	delete(r.sessions, id)
	delete(r.ids, id.id)
	r.mutex.Unlock()
}

func (r *tunnelRegistry) reap(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(heartbeatTimeout / 2):
			r.mutex.Lock()
			for id, session := range r.sessions {
				session.mutex.RLock()
				lastHeartbeat := session.heartbeat
				session.mutex.RUnlock()
				if time.Since(lastHeartbeat) > heartbeatTimeout*2 {
					session.close()
					delete(r.sessions, id)
				}
			}
			r.mutex.Unlock()
		}
	}
}
