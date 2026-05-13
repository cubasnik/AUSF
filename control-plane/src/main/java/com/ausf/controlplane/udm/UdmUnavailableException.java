package com.ausf.controlplane.udm;

/** Thrown by {@link HttpUdmClient} when the UDM circuit breaker is in the OPEN state. */
class UdmUnavailableException extends RuntimeException {
    UdmUnavailableException(String message) {
        super(message);
    }
}
