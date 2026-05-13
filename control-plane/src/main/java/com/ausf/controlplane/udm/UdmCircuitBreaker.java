package com.ausf.controlplane.udm;

import java.time.Duration;
import java.time.Instant;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

/**
 * Thread-safe, self-contained circuit breaker for the UDM HTTP client.
 *
 * <p>State machine:
 * <ul>
 *   <li><b>CLOSED</b> – all requests are allowed; consecutive failures are counted.</li>
 *   <li><b>OPEN</b>   – all requests are rejected immediately after the failure threshold is reached.</li>
 *   <li><b>HALF_OPEN</b> – one probe request is allowed after {@code openDuration} elapses;
 *       success closes the breaker, failure re-opens it.</li>
 * </ul>
 *
 * <p>4xx responses are <em>not</em> counted as breaker failures (they indicate a client-side
 * problem rather than UDM unavailability).
 */
class UdmCircuitBreaker {

    enum State { CLOSED, OPEN, HALF_OPEN }

    private final int failureThreshold;
    private final Duration openDuration;

    private final AtomicInteger consecutiveFailures = new AtomicInteger(0);
    private final AtomicReference<Instant> openedAt = new AtomicReference<>(null);
    private final AtomicReference<State> state = new AtomicReference<>(State.CLOSED);

    UdmCircuitBreaker(int failureThreshold, Duration openDuration) {
        this.failureThreshold = failureThreshold;
        this.openDuration = openDuration;
    }

    /**
     * Returns {@code true} when the breaker will allow the next call through.
     * Transitions OPEN → HALF_OPEN when {@code openDuration} has elapsed.
     */
    boolean allowRequest() {
        State current = state.get();
        if (current == State.CLOSED) {
            return true;
        }
        if (current == State.OPEN) {
            Instant opened = openedAt.get();
            if (opened != null && Duration.between(opened, Instant.now()).compareTo(openDuration) >= 0) {
                // Transition to HALF_OPEN to allow one probe
                if (state.compareAndSet(State.OPEN, State.HALF_OPEN)) {
                    return true;
                }
            }
            return false;
        }
        // HALF_OPEN: only one probe is allowed; block any concurrent call
        return false;
    }

    /**
     * Records a successful response. Resets the breaker to CLOSED.
     */
    void onSuccess() {
        consecutiveFailures.set(0);
        openedAt.set(null);
        state.set(State.CLOSED);
    }

    /**
     * Records a failure that counts toward the breaker threshold
     * (i.e. transport errors and 5xx responses).
     */
    void onFailure() {
        int failures = consecutiveFailures.incrementAndGet();
        if (failures >= failureThreshold || state.get() == State.HALF_OPEN) {
            openedAt.set(Instant.now());
            state.set(State.OPEN);
        }
    }

    /** Returns the current state (intended for testing and health exposure). */
    State getState() {
        return state.get();
    }
}
