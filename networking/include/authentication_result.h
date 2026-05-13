#pragma once

#include <array>
#include <chrono>
#include <cstdint>
#include <string>

namespace ausf {
namespace networking {

enum class AuthStatus : uint8_t {
    PENDING       = 0,  // authentication not yet completed
    AUTHENTICATED = 1,  // UE successfully authenticated (5G_AKA or EAP_AKA')
    REJECTED      = 2,  // credentials invalid or AUTS rejected
};

// Derived keys produced after a successful 5G authentication run.
// KAUSF and KSEAF per TS 33.501 Annex A.
struct DerivedKeys {
    std::array<uint8_t, 32> kausf{};  // KAUSF — AUSF anchor key
    std::array<uint8_t, 32> kseaf{};  // KSEAF — security anchor key delivered to SEAF/AMF
};

// Result of one Nausf_UEAuthentication procedure.
//
// The PFCP handler uses this to gate session establishment: per TS 29.244 the
// UPF must not forward traffic for a UE that has not completed authentication.
// After Nausf_UEAuthentication_Authenticate confirms the outcome, the auth
// service calls PFCPHandler::notifyAuthenticationResult() with this struct.
struct AuthenticationResult {
    std::string supi;         // e.g. "imsi-250010000000001"
    std::string auth_method;  // "5G_AKA" or "EAP_AKA_PRIME"
    AuthStatus  status  = AuthStatus::PENDING;
    DerivedKeys keys;         // meaningful only when status == AUTHENTICATED
    std::chrono::system_clock::time_point authenticated_at;

    bool        isAuthenticated() const noexcept { return status == AuthStatus::AUTHENTICATED; }
    std::string statusString()    const noexcept;
};

} // namespace networking
} // namespace ausf
