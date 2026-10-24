#include "msgfmt/json.hpp"

namespace msgfmt {

void append_json_string(std::string& out, std::string_view utf8) {
    static constexpr char kHex[] = "0123456789abcdef";

    out.push_back('"');
    for (const char c : utf8) {
        switch (c) {
            case '"':
                out += "\\\"";
                break;
            case '\\':
                out += "\\\\";
                break;
            case '\b':
                out += "\\b";
                break;
            case '\f':
                out += "\\f";
                break;
            case '\n':
                out += "\\n";
                break;
            case '\r':
                out += "\\r";
                break;
            case '\t':
                out += "\\t";
                break;
            default: {
                const auto byte = static_cast<unsigned char>(c);
                if (byte < 0x20) {
                    out += "\\u00";
                    out.push_back(kHex[byte >> 4]);
                    out.push_back(kHex[byte & 0x0F]);
                } else {
                    out.push_back(c);  // в том числе байты многобайтовых символов UTF-8
                }
            }
        }
    }
    out.push_back('"');
}

std::string to_json(const Document& doc) {
    std::string out = "{\"text\":";
    append_json_string(out, doc.text);
    out += ",\"ranges\":[";
    for (std::size_t i = 0; i < doc.ranges.size(); ++i) {
        const Range& range = doc.ranges[i];
        if (i > 0) {
            out.push_back(',');
        }
        out += "{\"style\":";
        append_json_string(out, style_name(range.style));
        out += ",\"offset\":";
        out += std::to_string(range.offset);
        out += ",\"length\":";
        out += std::to_string(range.length);
        out += ",\"url\":";
        append_json_string(out, range.url);
        out.push_back('}');
    }
    out += "]}";
    return out;
}

std::string error_json(std::string_view code) {
    std::string out = "{\"error\":";
    append_json_string(out, code);
    out.push_back('}');
    return out;
}

}  // namespace msgfmt
