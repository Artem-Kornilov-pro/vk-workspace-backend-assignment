#include "msgfmt/utf.hpp"

namespace msgfmt {

std::optional<std::u32string> decode_utf8(std::string_view bytes) {
    std::u32string out;
    out.reserve(bytes.size());

    std::size_t i = 0;
    while (i < bytes.size()) {
        const auto lead = static_cast<unsigned char>(bytes[i]);
        if (lead < 0x80) {
            out.push_back(lead);
            ++i;
            continue;
        }

        std::size_t length = 0;
        char32_t cp = 0;
        char32_t min_cp = 0;  // меньшие значения — избыточная запись
        if ((lead & 0xE0) == 0xC0) {
            length = 2;
            cp = lead & 0x1F;
            min_cp = 0x80;
        } else if ((lead & 0xF0) == 0xE0) {
            length = 3;
            cp = lead & 0x0F;
            min_cp = 0x800;
        } else if ((lead & 0xF8) == 0xF0) {
            length = 4;
            cp = lead & 0x07;
            min_cp = 0x10000;
        } else {
            return std::nullopt;  // байт продолжения или 0xF8–0xFF
        }

        if (bytes.size() - i < length) {
            return std::nullopt;
        }
        for (std::size_t k = 1; k < length; ++k) {
            const auto next = static_cast<unsigned char>(bytes[i + k]);
            if ((next & 0xC0) != 0x80) {
                return std::nullopt;
            }
            cp = (cp << 6) | (next & 0x3F);
        }
        if (cp < min_cp || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF)) {
            return std::nullopt;
        }

        out.push_back(cp);
        i += length;
    }
    return out;
}

void append_utf8(std::string& out, char32_t cp) {
    if (cp < 0x80) {
        out.push_back(static_cast<char>(cp));
    } else if (cp < 0x800) {
        out.push_back(static_cast<char>(0xC0 | (cp >> 6)));
        out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
    } else if (cp < 0x10000) {
        out.push_back(static_cast<char>(0xE0 | (cp >> 12)));
        out.push_back(static_cast<char>(0x80 | ((cp >> 6) & 0x3F)));
        out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
    } else {
        out.push_back(static_cast<char>(0xF0 | (cp >> 18)));
        out.push_back(static_cast<char>(0x80 | ((cp >> 12) & 0x3F)));
        out.push_back(static_cast<char>(0x80 | ((cp >> 6) & 0x3F)));
        out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
    }
}

std::string encode_utf8(std::u32string_view text) {
    std::string out;
    out.reserve(text.size());
    for (const char32_t cp : text) {
        append_utf8(out, cp);
    }
    return out;
}

}  // namespace msgfmt
