package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	logStateKey
)

// logState carries values the handler learns after the access log started.
type logState struct{ userID string }

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func setLogUser(ctx context.Context, user string) {
	if s, ok := ctx.Value(logStateKey).(*logState); ok {
		s.userID = user
	}
}

func (d Deps) logger() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

// requestID echoes a safe X-Request-ID or generates one, and sets the response
// header before anything else can write.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// accessLog writes one line per request: no body, no query, no headers.
func (d Deps) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		state := &logState{}
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r.WithContext(context.WithValue(r.Context(), logStateKey, state)))

		route := ""
		if rc := chi.RouteContext(r.Context()); rc != nil {
			route = rc.RoutePattern()
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		d.logger().Info("request",
			"request_id", requestIDFrom(r.Context()),
			"method", r.Method,
			"route", route,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"user_id", state.userID,
		)
	})
}

// recoverer turns a panic into the fixed 500 envelope. It logs the request ID
// only: the panic value and the stack can carry data.
func (d Deps) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			d.logger().Error("panic recovered", "request_id", requestIDFrom(r.Context()))
			writeError(w, r, http.StatusInternalServerError, codeInternal)
		}()
		next.ServeHTTP(w, r)
	})
}
