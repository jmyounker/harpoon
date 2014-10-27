package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Lovingly adapted from https://github.com/streadway/handy/tree/master/report

// Log wraps any http.Handler, and emits a JSON log of each request to the
// passed writer.
func Log(w io.Writer, next http.Handler) http.Handler {
	out := json.NewEncoder(w)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{
			ResponseWriter: w,
			event: event{
				Status:         200, // can be overwritten
				Time:           time.Now().UTC(),
				Method:         r.Method,
				URL:            r.RequestURI,
				Path:           r.URL.Path,
				Proto:          r.Proto,
				Host:           r.Host,
				RemoteAddr:     r.RemoteAddr,
				ForwardedFor:   r.Header.Get("X-Forwarded-For"),
				ForwardedProto: r.Header.Get("X-Forwarded-Proto"),
				Authorization:  r.Header.Get("Authorization"),
				Referrer:       r.Header.Get("Referer"),
				UserAgent:      r.Header.Get("User-Agent"),
				Range:          r.Header.Get("Range"),
			},
		}

		start := time.Now()
		next.ServeHTTP(rec, r)
		rec.event.Milliseconds = int(time.Since(start) / time.Millisecond)
		out.Encode(rec.event)
	})
}

type recorder struct {
	http.ResponseWriter
	event
}

type event struct {
	Status         int       `json:"status"`
	Size           int64     `json:"size"`
	Milliseconds   int       `json:"ms"`
	Time           time.Time `json:"time,omitempty"`
	Method         string    `json:"method,omitempty"`
	URL            string    `json:"url,omitempty"`
	Path           string    `json:"path,omitempty"`
	Proto          string    `json:"proto,omitempty"`
	Host           string    `json:"host,omitempty"`
	RemoteAddr     string    `json:"remote_addr,omitempty"`
	ForwardedFor   string    `json:"forwarded_for,omitempty"`
	ForwardedProto string    `json:"forwarded_proto,omitempty"`
	Authorization  string    `json:"authorization,omitempty"`
	Referrer       string    `json:"referrer,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	Range          string    `json:"range,omitempty"`
}

// Write sums the writes to produce the actual number of bytes written.
func (r *recorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.event.Size += int64(n)
	return n, err
}

// WriteHeader captures the status code. On success, this method may not be
// called, so initialize your event struct with the status value you wish to
// report on success, like 200.
func (r *recorder) WriteHeader(code int) {
	r.event.Status = code
	r.ResponseWriter.WriteHeader(code)
}
