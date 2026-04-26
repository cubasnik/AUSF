#include "pfcp_handler.h"
#include <iostream>

using namespace ausf::networking;

int main() {
    std::cout << "=== AUSF Networking Layer (C++) ===" << std::endl;
    
    // Create PFCP Handler
    PFCPHandler handler(8805);
    
    if (!handler.initialize()) {
        std::cerr << "Failed to initialize PFCP Handler" << std::endl;
        return 1;
    }
    
    if (!handler.start()) {
        std::cerr << "Failed to start PFCP Handler" << std::endl;
        return 1;
    }
    
    PFCPHeader associationHeader;
    associationHeader.version = 1;
    associationHeader.message_type = static_cast<uint8_t>(PFCPMessageType::ASSOCIATION_SETUP_REQUEST);
    associationHeader.message_length = 8;
    associationHeader.seid = 0;
    associationHeader.sequence_number = 1;
    std::vector<uint8_t> associationPayload(8, 0x10);

    PFCPHeader establishHeader;
    establishHeader.version = 1;
    establishHeader.message_type = static_cast<uint8_t>(PFCPMessageType::SESSION_ESTABLISHMENT_REQUEST);
    establishHeader.message_length = 16;
    establishHeader.seid = 12345;
    establishHeader.sequence_number = 2;
    std::vector<uint8_t> establishPayload(16, 0x20);

    PFCPHeader modifyHeader;
    modifyHeader.version = 1;
    modifyHeader.message_type = static_cast<uint8_t>(PFCPMessageType::SESSION_MODIFICATION_REQUEST);
    modifyHeader.message_length = 12;
    modifyHeader.seid = 12345;
    modifyHeader.sequence_number = 3;
    std::vector<uint8_t> modifyPayload(12, 0x30);

    PFCPHeader deleteHeader;
    deleteHeader.version = 1;
    deleteHeader.message_type = static_cast<uint8_t>(PFCPMessageType::SESSION_DELETION_REQUEST);
    deleteHeader.message_length = 10;
    deleteHeader.seid = 12345;
    deleteHeader.sequence_number = 4;
    std::vector<uint8_t> deletePayload(10, 0x40);

    if (!handler.handleMessage(associationHeader, associationPayload)) {
        std::cerr << "Association setup failed" << std::endl;
        return 1;
    }

    if (!handler.handleMessage(establishHeader, establishPayload)) {
        std::cerr << "Session establishment failed" << std::endl;
        return 1;
    }

    if (!handler.handleMessage(modifyHeader, modifyPayload)) {
        std::cerr << "Session modification failed" << std::endl;
        return 1;
    }

    if (!handler.handleMessage(deleteHeader, deletePayload)) {
        std::cerr << "Session deletion failed" << std::endl;
        return 1;
    }

    if (!handler.isAssociationEstablished() || handler.getSessionCount() != 0) {
        std::cerr << "PFCP protocol flow validation failed" << std::endl;
        return 1;
    }
    
    handler.stop();
    
    std::cout << "AUSF Networking layer test completed successfully!" << std::endl;
    return 0;
}
