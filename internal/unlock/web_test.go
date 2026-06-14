package unlock

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestWebUnlockSuccess(t *testing.T) {
	started := make(chan string, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		session, err := Run(context.Background(), func(_ context.Context, password string) (string, error) {
			if password != "secret-pass" {
				t.Errorf("password = %q, want secret-pass", password)
			}
			return "test-session-token", nil
		}, Options{
			Timeout: 5 * time.Second,
			OpenBrowser: func(pageURL string) error {
				started <- pageURL
				return nil
			},
		})
		if err != nil {
			t.Errorf("Run() error = %v", err)
			return
		}
		if session != "test-session-token" {
			t.Errorf("session = %q, want test-session-token", session)
		}
	}()

	pageURL := <-started
	if !strings.HasPrefix(pageURL, "http://127.0.0.1:") {
		t.Fatalf("pageURL = %q, want localhost URL", pageURL)
	}
	if !strings.Contains(pageURL, "/unlock?token=") {
		t.Fatalf("pageURL = %q, want unlock token path", pageURL)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 2 * time.Second}

	getResp, err := client.Get(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	DrainBody(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getResp.StatusCode)
	}

	form := url.Values{}
	form.Set("password", "secret-pass")
	for _, cookie := range jar.Cookies(mustParseURL(t, pageURL)) {
		if cookie.Name == csrfCookieName {
			form.Set("csrf_token", cookie.Value)
		}
	}

	postResp, err := client.PostForm(pageURL, form)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(postResp.Body)
	_ = postResp.Body.Close()
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d body = %s", postResp.StatusCode, body)
	}
	if !strings.Contains(string(body), "Vault unlocked") {
		t.Fatalf("POST body = %q, want success message", string(body))
	}

	<-done
}

func TestWebUnlockRejectsMissingToken(t *testing.T) {
	started := make(chan string, 1)

	go func() {
		_, _ = Run(context.Background(), func(_ context.Context, _ string) (string, error) {
			return "unused", nil
		}, Options{
			Timeout: time.Second,
			OpenBrowser: func(pageURL string) error {
				started <- pageURL
				return nil
			},
		})
	}()

	pageURL := <-started
	base := strings.Split(pageURL, "?")[0]
	resp, err := http.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAutoOpenBrowserInvoked(t *testing.T) {
	opened := make(chan string, 1)

	go func() {
		_, _ = Run(context.Background(), func(_ context.Context, _ string) (string, error) {
			return "session", nil
		}, Options{
			Timeout: time.Second,
			OpenBrowser: func(pageURL string) error {
				opened <- pageURL
				return nil
			},
		})
	}()

	pageURL := <-opened
	if !strings.Contains(pageURL, "token=") {
		t.Fatalf("opened URL = %q, want token query", pageURL)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// DrainBody closes the body without logging content.
func DrainBody(body io.ReadCloser) {
	if body != nil {
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}
}
