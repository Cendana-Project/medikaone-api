package infrastructure

import (
	"context"
	"net/url"
	"strings"
	"sync"
)

type healthCheck func(context.Context) error

var healthChecks = struct {
	sync.RWMutex
	items map[string]healthCheck
}{items: make(map[string]healthCheck)}

func registerHealthCheck(name string, check healthCheck) {
	healthChecks.Lock()
	defer healthChecks.Unlock()
	healthChecks.items[name] = check
}

func snapshotHealthChecks() map[string]healthCheck {
	healthChecks.RLock()
	defer healthChecks.RUnlock()

	checks := make(map[string]healthCheck, len(healthChecks.items))
	for name, check := range healthChecks.items {
		checks[name] = check
	}
	return checks
}

func redactConnectionError(err error, rawURL string) string {
	if err == nil {
		return ""
	}
	if rawURL == "" {
		return "connection failed"
	}
	message := strings.ReplaceAll(err.Error(), rawURL, "[redacted connection URL]")
	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil || parsed.User == nil {
		return message
	}
	if password, ok := parsed.User.Password(); ok && password != "" {
		message = strings.ReplaceAll(message, password, "[redacted]")
		message = strings.ReplaceAll(message, url.QueryEscape(password), "[redacted]")
	}
	return message
}
