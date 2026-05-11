package namf

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	namfStream   = "ausf:namf:retry"
	namfGroup    = "ausf-workers"
	namfConsumer = "worker-0"

	// namfStreamMaxLen caps the stream length with MAXLEN ~ to avoid unbounded growth.
	namfStreamMaxLen = 1000
)

// RedisRetryingClient is a Redis Streams-backed drop-in replacement for
// RetryingClient.  Retry state is persisted in a Redis Stream so that
// in-flight notifications survive pod restarts.
//
// Activate by setting AUSF_NAMF_QUEUE_BACKEND=redis and AUSF_NAMF_REDIS_URL
// to a valid redis:// or rediss:// URL.
type RedisRetryingClient struct {
	inner       *Client
	maxAttempts int
	rdb         *redis.Client
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewRedisRetryingClient creates a RedisRetryingClient connected to redisURL.
// maxAttempts ≤ 0 uses the default (10).
// Returns an error when the Redis URL is invalid or Redis is unreachable at startup.
func NewRedisRetryingClient(inner *Client, maxAttempts int, redisURL string) (*RedisRetryingClient, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("namf redis: parse URL %q: %w", redisURL, err)
	}
	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("namf redis: ping failed: %w", err)
	}

	if maxAttempts < 1 {
		maxAttempts = 10
	}
	return &RedisRetryingClient{
		inner:       inner,
		maxAttempts: maxAttempts,
		rdb:         rdb,
		stopCh:      make(chan struct{}),
	}, nil
}

// Start creates the consumer group (idempotent) and launches the background retry goroutine.
func (rc *RedisRetryingClient) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// "0" means "deliver all existing messages from the beginning on first read".
	// BUSYGROUP means the group already exists — safe to ignore.
	if err := rc.rdb.XGroupCreateMkStream(ctx, namfStream, namfGroup, "0").Err(); err != nil {
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			logRetryJSON("WARN", "namf redis: consumer group create failed, continuing", map[string]any{
				"error": err.Error(),
			})
		}
	}
	rc.wg.Add(1)
	go rc.run()
}

// Stop signals the background goroutine and waits for it to exit, then closes
// the Redis connection.
func (rc *RedisRetryingClient) Stop() {
	close(rc.stopCh)
	rc.wg.Wait()
	_ = rc.rdb.Close()
}

// NotifyUEAuthenticationStatus attempts synchronous delivery via the inner
// *Client (which retries internally).  On failure the notification is
// persisted to the Redis Stream for asynchronous retry.
func (rc *RedisRetryingClient) NotifyUEAuthenticationStatus(
	notification UEAuthenticationStatusNotification,
	notifyURI string,
) error {
	err := rc.inner.NotifyUEAuthenticationStatus(notification, notifyURI)
	if err != nil {
		item := retryItem{
			notification: notification,
			notifyURI:    notifyURI,
			attempt:      1,
			retryAfter:   time.Now().Add(retryInitialDelay),
		}
		if xErr := rc.xadd(item); xErr != nil {
			logRetryJSON("ERROR", "namf redis: enqueue failed — notification may be lost on restart", map[string]any{
				"auth_ctx_id": notification.AuthCtxID,
				"error":       xErr.Error(),
			})
		}
	}
	return err
}

func (rc *RedisRetryingClient) run() {
	defer rc.wg.Done()

	ticker := time.NewTicker(retryTickInterval)
	defer ticker.Stop()

	// Recover any messages that were delivered to this consumer but not ACKed
	// before the previous process exited (PEL = pending entry list).
	pending := rc.reclaimPEL()

	for {
		select {
		case <-rc.stopCh:
			return

		case now := <-ticker.C:
			// Pull newly available messages from the stream.
			pending = append(pending, rc.readNew()...)

			var remaining []retryItem
			for _, item := range pending {
				if now.Before(item.retryAfter) {
					remaining = append(remaining, item)
					continue
				}

				if err := rc.inner.NotifyUEAuthenticationStatus(item.notification, item.notifyURI); err == nil {
					logRetryJSON("INFO", "namf redis async retry succeeded", map[string]any{
						"auth_ctx_id": item.notification.AuthCtxID,
						"attempt":     item.attempt,
					})
					rc.xack(item.streamID)

				} else if item.attempt < rc.maxAttempts {
					delay := retryBackoff(item.attempt)
					logRetryJSON("WARN", "namf redis async retry failed, rescheduling", map[string]any{
						"auth_ctx_id":        item.notification.AuthCtxID,
						"attempt":            item.attempt,
						"next_retry_seconds": int(delay.Seconds()),
						"error":              err.Error(),
					})
					updated := retryItem{
						notification: item.notification,
						notifyURI:    item.notifyURI,
						attempt:      item.attempt + 1,
						retryAfter:   now.Add(delay),
					}
					// Persist the rescheduled item *before* ACKing the old one so
					// that a crash between the two operations leaves the item in the
					// PEL (recoverable) rather than lost.
					if xErr := rc.xadd(updated); xErr != nil {
						logRetryJSON("WARN", "namf redis reschedule xadd failed, retrying from memory", map[string]any{
							"error": xErr.Error(),
						})
						remaining = append(remaining, updated) // fall back to in-memory
					} else {
						rc.xack(item.streamID)
						// The new stream entry will be claimed via readNew() on the next tick.
					}

				} else {
					logRetryJSON("ERROR", "namf redis notification permanently failed after max attempts", map[string]any{
						"auth_ctx_id": item.notification.AuthCtxID,
						"supi":        item.notification.SUPI,
						"attempts":    item.attempt,
					})
					rc.xack(item.streamID)
				}
			}
			pending = remaining
		}
	}
}

// reclaimPEL reads all messages in the consumer's pending-entry list (PEL).
// These are messages that were delivered to this consumer in a previous process
// lifetime but never ACKed because the process crashed.
func (rc *RedisRetryingClient) reclaimPEL() []retryItem {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := rc.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    namfGroup,
		Consumer: namfConsumer,
		Streams:  []string{namfStream, "0"}, // "0" = all PEL messages for this consumer
		Count:    500,
	}).Result()
	if err != nil && err != redis.Nil {
		logRetryJSON("WARN", "namf redis: PEL reclaim failed", map[string]any{"error": err.Error()})
		return nil
	}

	var items []retryItem
	for _, stream := range results {
		for _, msg := range stream.Messages {
			item, parseErr := retryItemFromXMessage(msg)
			if parseErr != nil {
				logRetryJSON("WARN", "namf redis: corrupt PEL message, discarding", map[string]any{
					"stream_id": msg.ID,
					"error":     parseErr.Error(),
				})
				rc.xack(msg.ID)
				continue
			}
			items = append(items, item)
		}
	}
	if len(items) > 0 {
		logRetryJSON("INFO", "namf redis: reclaimed PEL items after restart", map[string]any{
			"count": len(items),
		})
	}
	return items
}

// readNew reads newly available messages from the stream (messages not yet
// delivered to any consumer in the group).
func (rc *RedisRetryingClient) readNew() []retryItem {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	results, err := rc.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    namfGroup,
		Consumer: namfConsumer,
		Streams:  []string{namfStream, ">"}, // ">" = only new messages
		Count:    50,
	}).Result()
	if err != nil && err != redis.Nil {
		logRetryJSON("WARN", "namf redis: read new messages failed", map[string]any{"error": err.Error()})
		return nil
	}

	var items []retryItem
	for _, stream := range results {
		for _, msg := range stream.Messages {
			item, parseErr := retryItemFromXMessage(msg)
			if parseErr != nil {
				logRetryJSON("WARN", "namf redis: corrupt stream message, discarding", map[string]any{
					"stream_id": msg.ID,
					"error":     parseErr.Error(),
				})
				rc.xack(msg.ID)
				continue
			}
			items = append(items, item)
		}
	}
	return items
}

// xadd appends item to the Redis stream.
func (rc *RedisRetryingClient) xadd(item retryItem) error {
	notifJSON, err := json.Marshal(item.notification)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return rc.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: namfStream,
		MaxLen: namfStreamMaxLen,
		Approx: true,
		Values: map[string]any{
			"notification": string(notifJSON),
			"notify_uri":   item.notifyURI,
			"attempt":      strconv.Itoa(item.attempt),
			"retry_after":  strconv.FormatInt(item.retryAfter.Unix(), 10),
		},
	}).Err()
}

// xack acknowledges a stream message so it is removed from the PEL.
func (rc *RedisRetryingClient) xack(streamID string) {
	if streamID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rc.rdb.XAck(ctx, namfStream, namfGroup, streamID).Err(); err != nil {
		logRetryJSON("WARN", "namf redis: XACK failed", map[string]any{
			"stream_id": streamID,
			"error":     err.Error(),
		})
	}
}

// retryItemFromXMessage parses a Redis XMessage into a retryItem.
func retryItemFromXMessage(msg redis.XMessage) (retryItem, error) {
	vals := msg.Values
	notifJSON, _ := vals["notification"].(string)
	notifyURI, _ := vals["notify_uri"].(string)
	attemptStr, _ := vals["attempt"].(string)
	retryAfterStr, _ := vals["retry_after"].(string)

	var notification UEAuthenticationStatusNotification
	if err := json.Unmarshal([]byte(notifJSON), &notification); err != nil {
		return retryItem{}, fmt.Errorf("unmarshal notification: %w", err)
	}

	attempt, _ := strconv.Atoi(attemptStr)
	retryAfterUnix, _ := strconv.ParseInt(retryAfterStr, 10, 64)

	return retryItem{
		streamID:     msg.ID,
		notification: notification,
		notifyURI:    notifyURI,
		attempt:      attempt,
		retryAfter:   time.Unix(retryAfterUnix, 0),
	}, nil
}

// Compile-time assertion that *RedisRetryingClient implements Notifier.
var _ Notifier = (*RedisRetryingClient)(nil)
