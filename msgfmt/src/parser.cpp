#include "msgfmt/format.hpp"
#include "msgfmt/utf.hpp"

namespace msgfmt {

std::string_view style_name(Style style) {
    switch (style) {
        case Style::Bold:
            return "bold";
        case Style::Italic:
            return "italic";
        case Style::Code:
            return "code";
        case Style::Link:
            return "link";
    }
    return "";
}

bool operator==(const Range& a, const Range& b) {
    return a.style == b.style && a.offset == b.offset && a.length == b.length && a.url == b.url;
}

// Разбор разметки появится отдельным шагом; пока вход возвращается как есть.
Document parse(std::u32string_view input) {
    return Document{encode_utf8(input), {}};
}

}  // namespace msgfmt
