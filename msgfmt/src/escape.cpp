#include "msgfmt/chars.hpp"
#include "msgfmt/format.hpp"
#include "msgfmt/utf.hpp"

namespace msgfmt {

std::string escape(std::u32string_view input) {
    std::string out;
    out.reserve(input.size());
    for (const char32_t cp : input) {
        if (is_service_char(cp)) {
            out.push_back('\\');
        }
        append_utf8(out, cp);
    }
    return out;
}

}  // namespace msgfmt
