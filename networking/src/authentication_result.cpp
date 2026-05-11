#include "authentication_result.h"

namespace ausf {
namespace networking {

std::string AuthenticationResult::statusString() const noexcept {
    switch (status) {
        case AuthStatus::PENDING:       return "PENDING";
        case AuthStatus::AUTHENTICATED: return "AUTHENTICATED";
        case AuthStatus::REJECTED:      return "REJECTED";
        default:                        return "UNKNOWN";
    }
}

} // namespace networking
} // namespace ausf
