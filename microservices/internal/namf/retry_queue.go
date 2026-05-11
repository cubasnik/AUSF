package namf

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	// retryQueueDepth is the maximum number of pending notifications held in
	// memory.  If the queue is full, the oldest-waiting notification is dropped
	// with an ERROR log so that newer events are not starved.
	retryQueueDepth = 512

	// retryInitialDelay is the backoff applied before the first async retry.
	retryInitialDelay = 5 * time.Second

	// retryMaxDelay caps the exponential backoff so that a persistently
	// unreachable AMF does not stall other retries indefinitely.
	retryMaxDelay = 5 * time.Minute

	// retryTickInterval is how often the background goroutine wakes to check
	// whether any queued item is ready for its next attempt.
	retryTickInterval = 500 * time.Millisecond
)

// retryItem holds a notification that could not be delivered synchronously.
type retryItem struct {
	notification UEAuthenticationStatusNotification
	notifyURI    string
	attempt      int
	retryAfter   time.Time
}

// RetryingClient wraps a *Client and adds an in-memory async retry queue so
// that Namf_Communication notifications that exhaust all synchronous delivery
// attempts (inside *Client) are retried in the background with exponential
// backoff up to maxAttempts total async attempts.
//
// Call Start before passing the client to the auth service, and defer Stop (or
// call it during graceful shutdown) to allow in-flight retries to be logged
// before the process exits.
type RetryingClient struct {
	inner       *Client
	maxAttempts int
	queue       chan retryItem
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewRetryingClient wraps inner with an async retry queue.
// maxAttempts is the number of additional async delivery attempts after the
// underlying *Client exhausts its own synchronous retries; 0 or negative uses
// the default of 10.
func NewRetryingClient(inner *Client, maxAttempts int) *RetryingClient {
	if maxAttempts < 1 {
		maxAttempts = 10
	}
	return &RetryingClient{
		inner:       inner,
		maxAttempts: maxAttempts,
		queue:       make(chan retryItem, retryQueueDepth),
		stopCh:      make(chan struct{}),
	}
}

// Start launches the background retry goroutine.
func (rc *RetryingClient) Start() {
	rc.wg.Add(1)
	go rc.run()
}

// Stop signals the background goroutine to exit and waits for it to finish.
// Should be called during graceful shutdown (e.g. via defer).
func (rc *RetryingClient) Stop() {
	close(rc.stopCh)
	rc.wg.Wait()
}

// NotifyUEAuthenticationStatus attempts synchronous delivery via the inner
// *Client (which already retries up to maxAttempts internally).  If delivery
// still fails, the notification is enqueued for asynchronous retry and the
// error is returned so the caller can log it.
func (rc *RetryingClient) NotifyUEAuthenticationStatus(
	notification UEAuthenticationStatusNotification,
	notifyURI string,
) error {
	err := rc.inner.NotifyUEAuthenticationStatus(notification, notifyURI)
	if err != nil {
		rc.enqueue(retryItem{
			notification: notification,
			notifyURI:    notifyURI,
			attempt:      1,
			retryAfter:   time.Now().Add(retryInitialDelay),
		})
	}
	return err
}

// enqueue adds item to the retry queue non-blockingly.  If the queue is full
// the item is dropped with an ERROR log (queue full is a rare condition that
// indicates a persistent AMF outage; we prefer losing the oldest queued
// notification over blocking the calling goroutine).
func (rc *RetryingClient) enqueue(item retryItem) {
	select {
	case rc.queue <- item:
	default:
		logRetryJSON("ERROR", "namf retry queue full, notification dropped", map[string]any{
			"auth_ctx_id": item.notification.AuthCtxID,
			"supi":        item.notification.SUPI,
		})
	}
}

// run is the background retry goroutine.  It wakes every retryTickInterval and
// attempts redelivery for any item whose retryAfter time has passed.
func (rc *RetryingClient) run() {
	defer rc.wg.Done()

	ticker := time.NewTicker(retryTickInterval)
	defer ticker.Stop()

	var pending []retryItem

	for {
		select {
		case <-rc.stopCh:
			return

		case item := <-rc.queue:
			pending = append(pending, item)

		case now := <-ticker.C:
			var remaining []retryItem
			for _, item := range pending {
				if now.Before(item.retryAfter) {
					remaining = append(remaining, item)
					continue
				}

				if err := rc.inner.NotifyUEAuthenticationStatus(item.notification, item.notifyURI); err == nil {
					logRetryJSON("INFO", "namf async retry succeeded", map[string]any{
						"auth_ctx_id": item.notification.AuthCtxID,
						"attempt":     item.attempt,
					})
					// successfully delivered — do not re-enqueue
				} else if item.attempt < rc.maxAttempts {
					delay := retryBackoff(item.attempt)
					logRetryJSON("WARN", "namf async retry failed, requeuing", map[string]any{
						"auth_ctx_id":        item.notification.AuthCtxID,
						"attempt":            item.attempt,
						"next_retry_seconds": int(delay.Seconds()),
						"error":              err.Error(),
					})
					remaining = append(remaining, retryItem{
						notification: item.notification,
						notifyURI:    item.notifyURI,
						attempt:      item.attempt + 1,
						retryAfter:   now.Add(delay),
					})
				} else {
					logRetryJSON("ERROR", "namf notification permanently failed after max attempts", map[string]any{
						"auth_ctx_id": item.notification.AuthCtxID,
						"supi":        item.notification.SUPI,
						"attempts":    item.attempt,
					})
					// permanent failure — drop
				}
			}
			pending = remaining
		}
	}
}

// retryBackoff returns the delay before attempt+1, doubling from
// retryInitialDelay up to retryMaxDelay.
func retryBackoff(attempt int) time.Duration {
	delay := retryInitialDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > retryMaxDelay {
			return retryMaxDelay
		}
	}
	return delay
}

func logRetryJSON(level, msg string, fields map[string]any) {
	fields["level"] = level
	fields["msg"] = msg
	data, _ := json.Marshal(fields)
	_, _ = fmt.Fprintln(os.Stderr, string(data))
}
