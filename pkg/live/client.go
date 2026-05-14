package live

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	queueCapacity     = 1024
	closeAckTimeout   = 500 * time.Millisecond
	contentTypeNDJSON = "application/x-ndjson"
)

// HTTPObserver streams events to a single editor's /api/live/events endpoint
// over a chunked POST. Publish is non-blocking; events are dropped on queue
// overflow. On any transport error the observer disables itself and the
// runtime continues.
type HTTPObserver struct {
	baseURL string
	token   string

	queue      chan EventEnvelope
	closeOnce  sync.Once
	closeCh    chan struct{}
	shutdownCh chan struct{}

	startOnce sync.Once
	started   atomic.Bool
	dropped   atomic.Uint64
}

// NewHTTPObserver returns an observer that lazily connects to baseURL on the
// first Publish. The connection uses HTTP/1.1 chunked transfer encoding.
func NewHTTPObserver(baseURL, token string) *HTTPObserver {
	return &HTTPObserver{
		baseURL:    baseURL,
		token:      token,
		queue:      make(chan EventEnvelope, queueCapacity),
		closeCh:    make(chan struct{}),
		shutdownCh: make(chan struct{}),
	}
}

func (o *HTTPObserver) Publish(env EventEnvelope) error {
	select {
	case <-o.closeCh:
		return nil
	default:
	}
	o.startOnce.Do(o.start)
	select {
	case <-o.closeCh:
	case o.queue <- env:
	default:
		o.dropped.Add(1)
	}
	return nil
}

// Close signals shutdown, drains queued events with a bounded deadline, and
// waits for the server's 204 ack. Returns nil even on transport failure —
// the live channel is best-effort.
func (o *HTTPObserver) Close() error {
	o.shutdown()
	if !o.started.Load() {
		return nil
	}
	select {
	case <-o.shutdownCh:
	case <-time.After(2 * closeAckTimeout):
	}
	return nil
}

// Dropped returns the number of events lost to queue overflow.
func (o *HTTPObserver) Dropped() uint64 { return o.dropped.Load() }

func (o *HTTPObserver) shutdown() {
	o.closeOnce.Do(func() { close(o.closeCh) })
}

func (o *HTTPObserver) start() {
	o.started.Store(true)

	pr, pw := io.Pipe()

	req, err := http.NewRequest(http.MethodPost, o.baseURL+"/api/live/events", pr)
	if err != nil {
		o.shutdown()
		close(o.shutdownCh)
		return
	}
	req.Header.Set("Authorization", "Bearer "+o.token)
	req.Header.Set("Content-Type", contentTypeNDJSON)

	httpDone := make(chan struct{})
	go func() {
		defer close(httpDone)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	go o.writerLoop(pw, httpDone)
}

func (o *HTTPObserver) writerLoop(pw *io.PipeWriter, httpDone <-chan struct{}) {
	defer close(o.shutdownCh)
	enc := json.NewEncoder(pw)
	encodeErr := false

	for {
		select {
		case env := <-o.queue:
			if !encodeErr {
				if err := enc.Encode(env); err != nil {
					encodeErr = true
					o.shutdown()
				}
			}
		case <-o.closeCh:
			// Drain any queued events non-blockingly, best-effort.
		drainLoop:
			for {
				select {
				case env := <-o.queue:
					if !encodeErr {
						if err := enc.Encode(env); err != nil {
							encodeErr = true
						}
					}
				default:
					break drainLoop
				}
			}
			_ = pw.Close()
			select {
			case <-httpDone:
			case <-time.After(closeAckTimeout):
			}
			return
		}
	}
}

// FanoutObserver fans Publish to multiple child observers. Each child has its
// own internal queue and goroutines, so a slow child cannot back-pressure the
// others. Close closes children in parallel.
type FanoutObserver struct {
	children []Observer
}

func NewFanoutObserver(children ...Observer) *FanoutObserver {
	return &FanoutObserver{children: children}
}

func (f *FanoutObserver) Publish(env EventEnvelope) error {
	for _, c := range f.children {
		_ = c.Publish(env)
	}
	return nil
}

func (f *FanoutObserver) Close() error {
	var wg sync.WaitGroup
	for _, c := range f.children {
		wg.Add(1)
		go func(c Observer) {
			defer wg.Done()
			_ = c.Close()
		}(c)
	}
	wg.Wait()
	return nil
}
