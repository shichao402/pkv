package unlock

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	defaultTimeout = 120 * time.Second
	csrfCookieName = "pkv_unlock_csrf"
)

// UnlockFunc validates the master password and returns a Bitwarden session key.
type UnlockFunc func(ctx context.Context, password string) (session string, err error)

type Options struct {
	Timeout      time.Duration
	OpenBrowser  BrowserOpener
	OnUnlockPage func(url string)
}

type Server struct {
	token     string
	csrfToken string
	unlockFn  UnlockFunc
	timeout   time.Duration
	openPage  BrowserOpener

	mu       sync.Mutex
	done     chan unlockResult
	shutdown func(context.Context) error
}

type unlockResult struct {
	session string
	err     error
}

// Run starts a localhost unlock web server, auto-opens the browser, and blocks
// until unlock succeeds, times out, or ctx is canceled.
func Run(ctx context.Context, unlockFn UnlockFunc, opts Options) (string, error) {
	if unlockFn == nil {
		return "", fmt.Errorf("unlock function is nil")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	openPage := opts.OpenBrowser
	if openPage == nil {
		openPage = openBrowser
	}

	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return "", err
	}

	srv := &Server{
		token:     token,
		csrfToken: csrfToken,
		unlockFn:  unlockFn,
		timeout:   timeout,
		openPage:  openPage,
		done:      make(chan unlockResult, 1),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/unlock", srv.handleUnlock)

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	srv.shutdown = httpServer.Shutdown

	go func() {
		_ = httpServer.Serve(listener)
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/unlock?token=%s", listener.Addr().(*net.TCPAddr).Port, token)
	if opts.OnUnlockPage != nil {
		opts.OnUnlockPage(url)
	}
	if err := openPage(url); err != nil {
		_ = srv.close()
		return "", fmt.Errorf("open browser: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-waitCtx.Done():
		_ = srv.close()
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("unlock timed out after %s", timeout)
		}
		return "", waitCtx.Err()
	case result := <-srv.done:
		_ = srv.close()
		if result.err != nil {
			return "", result.err
		}
		return result.session, nil
	}
}

func (s *Server) close() error {
	if s.shutdown == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.shutdown(ctx)
}

func (s *Server) finish(result unlockResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case s.done <- result:
	default:
	}
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") == "" || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(s.token)) != 1 {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.serveForm(w, r)
	case http.MethodPost:
		s.handlePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) serveForm(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    s.csrfToken,
		Path:     "/unlock",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(s.timeout.Seconds()),
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = unlockPage.Execute(w, map[string]string{
		"CSRFToken": s.csrfToken,
		"Token":     s.token,
	})
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.csrfToken)) != 1 {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.FormValue("csrf_token")), []byte(s.csrfToken)) != 1 {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}

	password := r.FormValue("password")
	if password == "" {
		s.renderResult(w, false, "Master password is required.")
		return
	}

	session, err := s.unlockFn(r.Context(), password)
	if err != nil {
		s.renderResult(w, false, "Unlock failed. Check your master password and try again.")
		return
	}

	s.renderResult(w, true, "Vault unlocked. You can close this tab and return to your editor.")
	s.finish(unlockResult{session: session})
}

func (s *Server) renderResult(w http.ResponseWriter, ok bool, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = unlockResultPage.Execute(w, map[string]any{
		"OK":      ok,
		"Message": message,
		"Token":   s.token,
	})
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var unlockPage = template.Must(template.New("unlock").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>PKV Unlock</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 28rem; margin: 4rem auto; padding: 0 1rem; color: #1a1a1a; }
  h1 { font-size: 1.25rem; margin-bottom: 0.5rem; }
  p { color: #555; font-size: 0.95rem; line-height: 1.5; }
  label { display: block; margin: 1.25rem 0 0.35rem; font-weight: 500; }
  input[type=password] { width: 100%; padding: 0.55rem 0.65rem; font-size: 1rem; border: 1px solid #ccc; border-radius: 6px; box-sizing: border-box; }
  button { margin-top: 1rem; padding: 0.55rem 1rem; font-size: 1rem; border: none; border-radius: 6px; background: #2563eb; color: #fff; cursor: pointer; }
  button:hover { background: #1d4ed8; }
</style>
</head>
<body>
  <h1>Unlock Bitwarden</h1>
  <p>Enter your Bitwarden master password to allow PKV to sync notes.</p>
  <form method="POST" action="/unlock?token={{.Token}}">
    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
    <label for="password">Master password</label>
    <input id="password" name="password" type="password" autocomplete="current-password" autofocus required>
    <button type="submit">Unlock</button>
  </form>
</body>
</html>`))

var unlockResultPage = template.Must(template.New("result").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>PKV Unlock</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 28rem; margin: 4rem auto; padding: 0 1rem; color: #1a1a1a; }
  h1 { font-size: 1.25rem; margin-bottom: 0.5rem; }
  p { color: {{if .OK}}#166534{{else}}#b91c1c{{end}}; font-size: 0.95rem; line-height: 1.5; }
  a { color: #2563eb; }
</style>
</head>
<body>
  <h1>{{if .OK}}Unlocked{{else}}Unlock failed{{end}}</h1>
  <p>{{.Message}}</p>
  {{if not .OK}}<p><a href="/unlock?token={{.Token}}">Try again</a></p>{{end}}
</body>
</html>`))

// SetOpenBrowserForTest overrides browser auto-open in tests.
func SetOpenBrowserForTest(fn BrowserOpener) func() {
	prev := openBrowser
	if fn == nil {
		openBrowser = defaultOpenBrowser
	} else {
		openBrowser = fn
	}
	return func() { openBrowser = prev }
}
