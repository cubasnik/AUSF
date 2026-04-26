#pragma once

#include <string>
#include <vector>
#include <memory>
#include <cstdint>
#include <unordered_map>

namespace ausf {
namespace networking {

enum class PFCPMessageType : uint8_t {
    HEARTBEAT_REQUEST = 1,
    HEARTBEAT_RESPONSE = 2,
    ASSOCIATION_SETUP_REQUEST = 5,
    ASSOCIATION_SETUP_RESPONSE = 6,
    SESSION_ESTABLISHMENT_REQUEST = 50,
    SESSION_ESTABLISHMENT_RESPONSE = 51,
    SESSION_MODIFICATION_REQUEST = 52,
    SESSION_MODIFICATION_RESPONSE = 53,
    SESSION_DELETION_REQUEST = 54,
    SESSION_DELETION_RESPONSE = 55
};

// PFCP Protocol Data Unit Header
struct PFCPHeader {
    uint8_t version;
    uint8_t message_type;
    uint16_t message_length;
    uint64_t seid;
    uint32_t sequence_number;
};

// PFCP Session
class PFCPSession {
public:
    PFCPSession(uint64_t seid);
    ~PFCPSession();
    
    void handleAssociationSetup();
    void handleSessionEstablishment();
    void handleSessionModification();
    void handleSessionDeletion();
    
    uint64_t getSEID() const;
    std::string getStatus() const;
    
private:
    uint64_t seid_;
    std::string status_;
    std::vector<uint8_t> session_data_;
};

// PFCP Handler Main Class
class PFCPHandler {
public:
    PFCPHandler(int port);
    ~PFCPHandler();
    
    bool initialize();
    bool start();
    void stop();
    
    bool handleMessage(const PFCPHeader& header, const std::vector<uint8_t>& payload);
    bool isAssociationEstablished() const;
    std::size_t getSessionCount() const;
    
private:
    int port_;
    bool is_running_;
    bool association_established_;
    std::unordered_map<uint64_t, std::shared_ptr<PFCPSession>> sessions_;
    
    void processPDU(const std::vector<uint8_t>& pdu);
    static std::string messageTypeToString(uint8_t message_type);
};

} // namespace networking
} // namespace ausf
