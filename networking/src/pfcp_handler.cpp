#include "pfcp_handler.h"
#include <iostream>
#include <cstring>

namespace ausf {
namespace networking {

namespace {
bool isValidHeader(const PFCPHeader& header, std::size_t payload_size) {
    return header.version == 1 && header.message_length == payload_size;
}
}

// PFCPSession Implementation
PFCPSession::PFCPSession(uint64_t seid) 
    : seid_(seid), status_("CREATED") {
    std::cout << "PFCP Session created with SEID: " << seid << std::endl;
}

PFCPSession::~PFCPSession() {
    std::cout << "PFCP Session destroyed with SEID: " << seid_ << std::endl;
}

void PFCPSession::handleAssociationSetup() {
    status_ = "ASSOCIATION_SETUP";
    std::cout << "Association setup for SEID: " << seid_ << std::endl;
}

void PFCPSession::handleSessionEstablishment() {
    status_ = "SESSION_ESTABLISHED";
    std::cout << "Session established for SEID: " << seid_ << std::endl;
}

void PFCPSession::handleSessionModification() {
    status_ = "SESSION_MODIFIED";
    std::cout << "Session modified for SEID: " << seid_ << std::endl;
}

void PFCPSession::handleSessionDeletion() {
    status_ = "SESSION_DELETED";
    std::cout << "Session deleted for SEID: " << seid_ << std::endl;
}

uint64_t PFCPSession::getSEID() const {
    return seid_;
}

std::string PFCPSession::getStatus() const {
    return status_;
}

// PFCPHandler Implementation
PFCPHandler::PFCPHandler(int port) 
    : port_(port), is_running_(false), association_established_(false) {
    std::cout << "PFCP Handler initialized on port " << port << std::endl;
}

PFCPHandler::~PFCPHandler() {
    if (is_running_) {
        stop();
    }
}

bool PFCPHandler::initialize() {
    // Initialize PFCP socket and other resources
    std::cout << "Initializing PFCP Handler on port " << port_ << std::endl;
    return true;
}

bool PFCPHandler::start() {
    is_running_ = true;
    std::cout << "PFCP Handler started" << std::endl;
    return true;
}

void PFCPHandler::stop() {
    is_running_ = false;
    std::cout << "PFCP Handler stopped" << std::endl;
}

bool PFCPHandler::handleMessage(const PFCPHeader& header, const std::vector<uint8_t>& payload) {
    if (!is_running_) {
        std::cerr << "PFCP Handler is not running" << std::endl;
        return false;
    }

    if (!isValidHeader(header, payload.size())) {
        std::cerr << "Invalid PFCP header (version/length mismatch)" << std::endl;
        return false;
    }

    std::cout << "Handling PFCP message [seq=" << header.sequence_number << "] type="
              << messageTypeToString(header.message_type) << " seid=" << header.seid << std::endl;

    switch (static_cast<PFCPMessageType>(header.message_type)) {
        case PFCPMessageType::HEARTBEAT_REQUEST:
            if (!association_established_) {
                std::cerr << "Reject heartbeat: association is not established" << std::endl;
                return false;
            }
            std::cout << "Heartbeat received from peer, responding with HEARTBEAT_RESPONSE" << std::endl;
            processPDU(payload);
            return true;

        case PFCPMessageType::ASSOCIATION_SETUP_REQUEST:
            association_established_ = true;
            std::cout << "Association established with peer node" << std::endl;
            processPDU(payload);
            return true;

        case PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST: {
            if (!association_established_) {
                std::cerr << "Reject session establishment: association is not established" << std::endl;
                return false;
            }
            auto session = std::make_shared<PFCPSession>(header.seid);
            session->handleSessionEstablishment();
            sessions_[header.seid] = session;
            processPDU(payload);
            return true;
        }

        case PFCPMessageType::SESSION_MODIFICATION_REQUEST: {
            auto it = sessions_.find(header.seid);
            if (it == sessions_.end()) {
                std::cerr << "Reject session modification: unknown SEID " << header.seid << std::endl;
                return false;
            }
            it->second->handleSessionModification();
            processPDU(payload);
            return true;
        }

        case PFCPMessageType::SESSION_DELETION_REQUEST: {
            auto it = sessions_.find(header.seid);
            if (it == sessions_.end()) {
                std::cerr << "Reject session deletion: unknown SEID " << header.seid << std::endl;
                return false;
            }
            it->second->handleSessionDeletion();
            sessions_.erase(it);
            processPDU(payload);
            return true;
        }

        default:
            std::cerr << "Unsupported PFCP message type: " << static_cast<int>(header.message_type) << std::endl;
            return false;
    }
}

bool PFCPHandler::isAssociationEstablished() const {
    return association_established_;
}

std::size_t PFCPHandler::getSessionCount() const {
    return sessions_.size();
}

void PFCPHandler::processPDU(const std::vector<uint8_t>& pdu) {
    std::cout << "Processing PDU of size: " << pdu.size() << " bytes" << std::endl;
    if (!pdu.empty()) {
        std::cout << "First payload byte (simulated IE marker): 0x" << std::hex
                  << static_cast<int>(pdu[0]) << std::dec << std::endl;
    }
}

std::string PFCPHandler::messageTypeToString(uint8_t message_type) {
    switch (static_cast<PFCPMessageType>(message_type)) {
        case PFCPMessageType::HEARTBEAT_REQUEST:
            return "HEARTBEAT_REQUEST";
        case PFCPMessageType::HEARTBEAT_RESPONSE:
            return "HEARTBEAT_RESPONSE";
        case PFCPMessageType::ASSOCIATION_SETUP_REQUEST:
            return "ASSOCIATION_SETUP_REQUEST";
        case PFCPMessageType::ASSOCIATION_SETUP_RESPONSE:
            return "ASSOCIATION_SETUP_RESPONSE";
        case PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST:
            return "SESSION_ESTABLISHMENT_REQUEST";
        case PFCPMessageType::SESSION_ESTABLISHMENT_RESPONSE:
            return "SESSION_ESTABLISHMENT_RESPONSE";
        case PFCPMessageType::SESSION_MODIFICATION_REQUEST:
            return "SESSION_MODIFICATION_REQUEST";
        case PFCPMessageType::SESSION_MODIFICATION_RESPONSE:
            return "SESSION_MODIFICATION_RESPONSE";
        case PFCPMessageType::SESSION_DELETION_REQUEST:
            return "SESSION_DELETION_REQUEST";
        case PFCPMessageType::SESSION_DELETION_RESPONSE:
            return "SESSION_DELETION_RESPONSE";
        default:
            return "UNKNOWN";
    }
}

} // namespace networking
} // namespace ausf
