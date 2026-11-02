package main

// Truyền âm thanh máy tính sang điện thoại: PCM 16-bit mono 24 kHz qua WebSocket /audio.

import (
	"encoding/binary"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const audioRate = 24000

type audioHub struct {
	mu   sync.Mutex
	subs map[chan []byte]bool
	stop chan struct{}
}

var hub = &audioHub{subs: map[chan []byte]bool{}}

func (h *audioHub) subscribe() (chan []byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan []byte, 32)
	if len(h.subs) == 0 {
		h.stop = make(chan struct{})
		errc := make(chan error, 1)
		go captureLoop(h.stop, h.publish, errc)
		select {
		case err := <-errc:
			if err != nil {
				return nil, err
			}
		case <-time.After(3 * time.Second):
		}
	}
	h.subs[ch] = true
	return ch, nil
}

func (h *audioHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
	if len(h.subs) == 0 && h.stop != nil {
		close(h.stop)
		h.stop = nil
	}
}

func (h *audioHub) publish(b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default: // điện thoại chậm: bỏ bớt
		}
	}
}

// resampler: trộn về mono và đổi tần số mẫu về 24 kHz (nội suy tuyến tính).
type resampler struct {
	inRate float64
	pos    float64
	last   float32
	buf    []byte
}

func (r *resampler) push(mono []float32, out func([]byte)) {
	step := r.inRate / audioRate
	for len(mono) > 0 {
		for r.pos < float64(len(mono)) {
			i := int(r.pos)
			frac := float32(r.pos - float64(i))
			prev := r.last
			if i > 0 {
				prev = mono[i-1]
			}
			v := prev + (mono[i]-prev)*frac
			if v > 1 {
				v = 1
			} else if v < -1 {
				v = -1
			}
			r.buf = binary.LittleEndian.AppendUint16(r.buf, uint16(int16(math.Round(float64(v*32767)))))
			r.pos += step
		}
		r.pos -= float64(len(mono))
		r.last = mono[len(mono)-1]
		mono = nil
	}
	if len(r.buf) >= audioRate*2/25 { // gửi mỗi ~40 ms
		out(r.buf)
		r.buf = nil
	}
}

func audioHandler(auth *Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.ok(r.URL.Query().Get("token")) {
			http.Error(w, "token không hợp lệ", http.StatusUnauthorized)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		ch, err := hub.subscribe()
		if err != nil {
			c.WriteMessage(websocket.TextMessage, []byte(`{"t":"err","msg":"Không lấy được âm thanh: `+err.Error()+`"}`))
			return
		}
		defer hub.unsubscribe(ch)
		c.WriteMessage(websocket.TextMessage, []byte(`{"t":"audio","rate":24000,"ch":1}`))
		go func() { // đọc để phát hiện điện thoại ngắt
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					c.Close()
					return
				}
			}
		}()
		for b := range ch {
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.WriteMessage(websocket.BinaryMessage, b) != nil {
				return
			}
		}
	}
}
