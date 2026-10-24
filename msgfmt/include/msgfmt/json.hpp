#pragma once

#include <string>
#include <string_view>

#include "msgfmt/format.hpp"

namespace msgfmt {

// Дописывает строку UTF-8 как строковый литерал JSON: экранируются кавычка,
// обратная косая черта и управляющие символы U+0000–U+001F.
void append_json_string(std::string& out, std::string_view utf8);

// {"text":"...","ranges":[{"style":"...","offset":N,"length":N,"url":"..."}]}
std::string to_json(const Document& doc);

// {"error":"<code>"}
std::string error_json(std::string_view code);

}  // namespace msgfmt
